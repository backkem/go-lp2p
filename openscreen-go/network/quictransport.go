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

// NewTransport opens a pooled WebTransport on the connection.
func (c *Connection) NewTransport(ctx context.Context) (*PooledWebTransport, error) {
	return c.base.NewTransport(ctx)
}

func (c *baseConnection) NewTransport(ctx context.Context) (*PooledWebTransport, error) {
	_, err := c.openStream(ctx, &msgDataTransportStartRequest{
		RequestID: c.nextRequestID(),
	})
	if err != nil {
		return nil, err
	}

	// TODO: await data-transport-start-response

	return c.createDataTransport(), nil
}

// NewTransportListener returns a pooled WebTransport for accepting transport
// streams, without requesting one from the remote agent.
func (c *Connection) NewTransportListener() (*PooledWebTransport, error) {
	return c.base.createDataTransport(), nil
}

// AcceptTransport accepts a pooled WebTransport opened by the remote agent.
func (c *Connection) AcceptTransport(ctx context.Context) (*PooledWebTransport, error) {
	return accept(ctx, c.base, c.base.app.Load().acceptTransport)
}

func (t *PooledWebTransport) AcceptStream(ctx context.Context) (*QuicStream, error) {
	s, err := accept(ctx, t.conn, t.conn.app.Load().acceptTransportStream)
	if err != nil {
		return nil, err
	}
	return &QuicStream{stream: s}, nil
}

func (t *PooledWebTransport) OpenStreamSync(ctx context.Context) (*QuicStream, error) {
	s, err := t.conn.openStream(ctx, &msgDataTransportStreamRequest{
		RequestID: t.conn.nextRequestID(),
	})
	if err != nil {
		return nil, err
	}

	// TODO: await data-transport-stream-response

	return &QuicStream{stream: s}, nil
}

func (t *PooledWebTransport) CloseWithError(uint64, string) error {
	// TODO: NOP for now
	return nil
}

// QuicStream is a stream of a PooledWebTransport.
type QuicStream struct {
	stream ApplicationStream
}

func (s *QuicStream) StreamID() int64 {
	switch stream := s.stream.(type) {
	case *quicApplicationStream:
		return int64(stream.stream.StreamID())

	default:
		// TODO: move to transport interface?
		panic(fmt.Sprintf("unknown stream type: %T", s.stream))
	}
}

func (s *QuicStream) Read(p []byte) (int, error) {
	return s.stream.Read(p)
}

func (s *QuicStream) Write(p []byte) (n int, err error) {
	return s.stream.Write(p)
}

func (s *QuicStream) Close() error {
	return s.stream.Close()
}
