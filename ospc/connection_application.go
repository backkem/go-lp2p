package ospc

import (
	"context"
	"fmt"

	"github.com/quic-go/quic-go"
)

func (c *baseConnection) openStream(ctx context.Context) (ApplicationStream, error) {
	_ = ctx
	stream, err := c.connectedState.appConn.OpenStreamSync(ctx)
	if err != nil {
		return nil, err
	}

	bStream := newBaseStream(stream, c.handleApplicationMessage)
	c.handleApplicationStream(bStream)

	return stream, nil
}

func (c *baseConnection) runApplication() {
	c.mu.Lock()
	defer c.mu.Unlock()

	acceptCtx, acceptCancel := context.WithCancel(context.Background())
	c.acceptCancel = acceptCancel

	// Steam accept loop
	go func() {
		for {
			s, err := c.connectedState.appConn.AcceptStream(acceptCtx)
			if err != nil {
				fmt.Printf("AcceptStream error: %s\n", err)
				c.closeWithError(fmt.Errorf("acceptStream error: %v", err))
				return
			}

			bStream := newBaseStream(s, c.handleApplicationMessage)
			c.handleApplicationStream(bStream)
		}
	}()
}

type connectedState struct {
	appConn ApplicationConnection

	acceptDataChannel     chan *DataChannel
	acceptTransport       chan *PooledWebTransport
	acceptTransportStream chan *baseStream
}

func (c *baseConnection) handleApplicationStream(stream *baseStream) {
	go func() {
		for {
			handler := stream.Handler()
			if handler == nil {
				return
			}

			msg, err := readMessage(stream.stream)
			if err == quic.ErrServerClosed {
				return
			} else if err != nil {
				fmt.Printf("application protocol: failed to read message: %v\n", err)
				// c.closeWithError(fmt.Errorf("failed to read message: %v", err))
				return
			}

			err = handler(msg, stream)
			if err != nil {
				fmt.Printf("failed to handle message: %v\n", err)
				c.closeWithError(fmt.Errorf("failed to handle message: %v", err))
				return
			}
		}
	}()
}

func (c *baseConnection) handleDataChannelOpenRequest(msg *msgDataChannelOpenRequest, stream *baseStream) error {
	dc := &DataChannel{
		DataChannelParameters: DataChannelParameters{
			Label:    msg.Label,
			ID:       uint64(msg.ChannelId),
			Protocol: msg.Protocol,
		},
		stream: stream.stream,
	}
	stream.SetHandler(nil)

	// TODO: send data-exchange-start-response

	c.mu.Lock()
	close := c.close
	accept := c.connectedState.acceptDataChannel
	c.mu.Unlock()

	select {
	case <-close:
		return c.err()
	case accept <- dc:
		return nil
	}
}

func (c *baseConnection) handleDataTransportStartRequest(msg *msgDataTransportStartRequest, info struct{}) error {
	// TODO: msg validation
	_, _ = msg, info
	c.mu.Lock()
	t, err := c.createDataTransport()
	if err != nil {
		c.mu.Unlock()
		return err
	}
	c.mu.Unlock()

	// TODO: send data-transport-start-response

	c.mu.Lock()
	close := c.close
	accept := c.connectedState.acceptTransport
	c.mu.Unlock()

	select {
	case <-close:
		return c.err()
	case accept <- t:
		return nil
	}
}

// Caller should hold the connection lock
func (c *baseConnection) createDataTransport() (*PooledWebTransport, error) {
	t := &PooledWebTransport{
		conn: c,
	}
	return t, nil
}

func (c *baseConnection) handleDataTransportStreamRequest(msg *msgDataTransportStreamRequest, stream *baseStream) error {
	// TODO: msg validation
	_ = msg
	// Stop message handling for this stream
	stream.SetHandler(nil)

	// TODO: send data-transport-stream-response

	c.mu.Lock()
	close := c.close
	accept := c.connectedState.acceptTransportStream
	c.mu.Unlock()

	if accept == nil {
		fmt.Printf("handleDataTransportStreamRequest: no transport, ignoring stream request\n")
		return nil // No-one is listening
	}

	select {
	case <-close:
		return c.err()
	case accept <- stream:
		return nil
	}
}

// Handoff the underlying quick connection for use by another protocol.
// func (c *baseConnection) Handoff() (quic.Connection, error) {
// 	c.mu.Lock()
// 	c.acceptCancel() // Stop stream handling loop
// 	done := c.done
// 	c.closeErr = ErrHandedOff
// 	c.mu.Unlock()
//
// 	<-done
//
// 	return c.conn, nil
// }

func (c *baseConnection) handleApplicationMessage(msg interface{}, stream *baseStream) (err error) {
	switch typedMsg := msg.(type) {
	case *msgDataChannelOpenRequest:
		err = c.handleDataChannelOpenRequest(typedMsg, stream)

	case *msgDataTransportStartRequest:
		err = c.handleDataTransportStartRequest(typedMsg, struct{}{})

	case *msgDataTransportStreamRequest:
		err = c.handleDataTransportStreamRequest(typedMsg, stream)

	default:
		fmt.Printf("baseConnection: unhandled message type: %T\n", typedMsg)
	}

	if err != nil {
		return err
	}
	return nil
}
