package ospc

import (
	"context"
)

// Transport implements WebTransport pooled over an
// existing OpenScreenProtocol connection.
type Transport struct {
	conn *baseConnection
}

// NewTransport
func (c *Connection) NewTransport(ctx context.Context) (*Transport, error) {
	return c.base.NewTransport(ctx)
}

func (c *baseConnection) NewTransport(ctx context.Context) (*Transport, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	msg := &msgDataTransportStartRequest{
		RequestID: c.agentState.nextRequestID(),
	}

	stream, err := c.conn.OpenStreamSync(ctx)
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

func (c *Connection) NewTransportListener() (*Transport, error) {
	return c.base.createDataTransport()
}

func (c *baseConnection) NewTransportListener() (*Transport, error) {
	t, err := c.createDataTransport()
	return t, err
}

func (l *TransportListener) Accept(ctx context.Context) (*Transport, error) {
	return l.conn.AcceptTransport(ctx)
}

func (l *TransportListener) Close() error {
	// TODO: NOP for now
	return nil
}

func (c *Connection) AcceptTransport(ctx context.Context) (*Transport, error) {
	return c.base.AcceptTransport(ctx)
}

func (c *baseConnection) AcceptTransport(ctx context.Context) (*Transport, error) {
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

func (t *Transport) AcceptStream(ctx context.Context) (*QuicStream, error) {
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

func (t *Transport) OpenStreamSync(ctx context.Context) (*QuicStream, error) {
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

	stream, err := c.conn.OpenStreamSync(ctx)
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

func (t *Transport) CloseWithError(uint64, string) error {
	// TODO: NOP for now
	return nil
}

// Stream
type QuicStream struct {
	stream *baseStream
}

func (s *QuicStream) StreamID() int64 {
	return int64(s.stream.stream.StreamID())
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
