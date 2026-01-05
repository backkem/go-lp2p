package ospc

import (
	"context"
	"fmt"
)

// PooledWebTransport implements WebTransport pooled over an
// existing OpenScreenProtocol Application Transport.
type PooledWebTransport struct {
	conn *baseConnection
}

// NewTransport
func (c *Connection) NewTransport(ctx context.Context) (*PooledWebTransport, error) {
	return c.base.NewTransport(ctx)
}

func (c *baseConnection) NewTransport(ctx context.Context) (*PooledWebTransport, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	msg := &msgDataTransportStartRequest{
		RequestID: c.agentState.nextRequestID(),
	}

	stream, err := c.connectedState.appConn.OpenStreamSync(ctx)
	if err != nil {
		return nil, err
	}

	err = writeMessage(msg, stream)
	if err != nil {
		return nil, err
	}

	// TODO: await data-exchange-start-response

	t, err := c.createDataTransport()
	if err != nil {
		return nil, err
	}

	return t, nil
}

type TransportListener struct {
	conn *baseConnection
}

func (c *Connection) NewTransportListener() (*PooledWebTransport, error) {
	return c.base.createDataTransport()
}

func (c *baseConnection) NewTransportListener() (*PooledWebTransport, error) {
	t, err := c.createDataTransport()
	return t, err
}

func (l *TransportListener) Accept(ctx context.Context) (*PooledWebTransport, error) {
	return l.conn.AcceptTransport(ctx)
}

func (l *TransportListener) Close() error {
	// TODO: NOP for now
	return nil
}

func (c *Connection) AcceptTransport(ctx context.Context) (*PooledWebTransport, error) {
	return c.base.AcceptTransport(ctx)
}

func (c *baseConnection) AcceptTransport(ctx context.Context) (*PooledWebTransport, error) {
	c.mu.Lock()
	close := c.close
	accept := c.connectedState.acceptTransport
	c.mu.Unlock()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-close:
		return nil, c.err()
	case dc := <-accept:
		return dc, nil
	}
}

func (t *PooledWebTransport) AcceptStream(ctx context.Context) (*QuicStream, error) {
	s, err := t.conn.AcceptTransportStream(ctx)
	if err != nil {
		return nil, err
	}
	return s, err
}

func (c *baseConnection) AcceptTransportStream(ctx context.Context) (*QuicStream, error) {
	c.mu.Lock()
	close := c.close
	accept := c.connectedState.acceptTransportStream
	c.mu.Unlock()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-close:
		return nil, c.err()
	case s := <-accept:
		return &QuicStream{
			stream: s,
		}, nil
	}
}

func (t *PooledWebTransport) OpenStreamSync(ctx context.Context) (*QuicStream, error) {
	s, err := t.conn.OpenTransportStream(ctx)
	if err != nil {
		return nil, err
	}

	return &QuicStream{
		stream: s,
	}, nil
}

func (c *baseConnection) OpenTransportStream(ctx context.Context) (*baseStream, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	msg := &msgDataTransportStreamRequest{
		RequestID: c.agentState.nextRequestID(),
	}

	stream, err := c.connectedState.appConn.OpenStreamSync(ctx)
	if err != nil {
		return nil, err
	}

	err = writeMessage(msg, stream)
	if err != nil {
		return nil, err
	}

	// TODO: await data-exchange-start-response

	return newBaseStream(stream, nil), nil
}

func (t *PooledWebTransport) CloseWithError(uint64, string) error {
	// TODO: NOP for now
	return nil
}

// Stream
type QuicStream struct {
	stream *baseStream
}

func (s *QuicStream) StreamID() int64 {
	switch stream := s.stream.stream.(type) {
	case *QuicApplicationStream:
		return int64(stream.stream.StreamID())

	default:
		// TODO: move to transport interface?
		panic(fmt.Sprintf("unknown stream type: %T", s.stream.stream))
	}
}

func (s *QuicStream) Read(p []byte) (int, error) {
	n, err := s.stream.stream.Read(p)
	// fmt.Printf("QuicStream.Read: %d %s %v", n, string(p[:n]), err)
	return n, err
}

func (s *QuicStream) Write(p []byte) (n int, err error) {
	return s.stream.stream.Write(p)
}
func (s *QuicStream) Close() error {
	return s.stream.stream.Close()
}
