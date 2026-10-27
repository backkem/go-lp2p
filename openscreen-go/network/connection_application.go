package ospc

import (
	"bytes"
	"context"
	"fmt"
	"io"
)

type connectedState struct {
	appConn ApplicationConnection

	acceptDataChannel     chan *DataChannel
	acceptTransport       chan *PooledWebTransport
	acceptTransportStream chan ApplicationStream
	acceptStream          chan ApplicationStream

	dataChannelIDs dataChannelIDs
}

// runApplication starts the application stream dispatcher. It is the only
// consumer of incoming application streams. It reports false if the
// connection is already closed.
func (c *baseConnection) runApplication() bool {
	appConn := c.app.Load().appConn
	return c.goroutine(func() {
		for {
			s, err := appConn.AcceptStream(c.ctx)
			if err != nil {
				c.shutdown(fmt.Errorf("application protocol: accept stream: %w", err))
				return
			}
			if !c.goroutine(func() { c.routeStream(s) }) {
				_ = s.Close()
				return
			}
		}
	})
}

// routeStream reads the type key of the first message of an incoming stream
// and hands the stream to its consumer: an internal protocol handler or, for
// any other type key, AcceptStream.
func (c *baseConnection) routeStream(s ApplicationStream) {
	rec := &recordingReader{r: s}
	typeKey, err := readTypeKey(rec)
	if err != nil {
		c.log.Debug("application protocol: failed to read type key", "err", err)
		_ = s.Close()
		return
	}

	switch typeKey {
	case typeKeyDataChannelOpenRequest, typeKeyDataTransportStartRequest, typeKeyDataTransportStream:
	default:
		// Not ours: hand the stream over as received.
		deliver(c, c.app.Load().acceptStream, ApplicationStream(&prefixedStream{
			ApplicationStream: s,
			r:                 io.MultiReader(bytes.NewReader(rec.buf), s),
		}), s.Close)
		return
	}

	msg, err := newMessageByType(typeKey)
	if err != nil {
		_ = s.Close()
		return
	}
	if err := decodeCBOR(s, msg); err != nil {
		c.log.Debug("application protocol: failed to decode message", "type", typeKey, "err", err)
		_ = s.Close()
		return
	}
	c.log.Debug("receive", "msg", msgName(msg))

	app := c.app.Load()
	switch typedMsg := msg.(type) {
	case *msgDataChannelOpenRequest:
		c.acceptDataChannelRequest(typedMsg, s)

	case *msgDataTransportStartRequest:
		// TODO: send data-transport-start-response. The stream carries no
		// further messages.
		deliver(c, app.acceptTransport, c.createDataTransport(), s.Close)

	case *msgDataTransportStream:
		deliver(c, app.acceptTransportStream, s, s.Close)
	}
}

// deliver hands v to the consumer of ch. If the connection closes first,
// drop is called instead.
func deliver[T any](c *baseConnection, ch chan T, v T, drop func() error) {
	select {
	case ch <- v:
	case <-c.ctx.Done():
		_ = drop()
	}
}

// accept waits for a value on ch.
func accept[T any](ctx context.Context, c *baseConnection, ch chan T) (T, error) {
	var zero T
	select {
	case <-ctx.Done():
		return zero, ctx.Err()
	case <-c.ctx.Done():
		return zero, c.err()
	case v := <-ch:
		return v, nil
	}
}

func (c *baseConnection) acceptStream(ctx context.Context) (ApplicationStream, error) {
	return accept(ctx, c, c.app.Load().acceptStream)
}

// openStream opens an application stream and writes its first message.
func (c *baseConnection) openStream(ctx context.Context, msg interface{}) (ApplicationStream, error) {
	stream, err := c.app.Load().appConn.OpenStreamSync(ctx)
	if err != nil {
		return nil, err
	}
	c.log.Debug("send", "msg", msgName(msg))
	err = writeMessage(msg, stream)
	if err != nil {
		_ = stream.Close()
		return nil, err
	}
	return stream, nil
}

func (c *baseConnection) createDataTransport() *PooledWebTransport {
	return &PooledWebTransport{
		conn: c,
	}
}

// recordingReader records the bytes read through it.
type recordingReader struct {
	r   io.Reader
	buf []byte
}

func (r *recordingReader) Read(p []byte) (int, error) {
	n, err := r.r.Read(p)
	r.buf = append(r.buf, p[:n]...)
	return n, err
}

// prefixedStream is a stream of which some bytes were already read. They are
// replayed before reading the rest of the stream.
type prefixedStream struct {
	ApplicationStream
	r io.Reader
}

func (s *prefixedStream) Read(p []byte) (int, error) {
	return s.r.Read(p)
}
