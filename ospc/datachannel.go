package ospc

import (
	"context"
	"fmt"
)

// DataChannelParameters
type DataChannelParameters struct {
	Label    string
	Protocol string
	ID       uint64
}

// OpenDataChannel opens a data channel
func (c *Connection) OpenDataChannel(ctx context.Context, params DataChannelParameters) (*DataChannel, error) {
	return c.base.OpenDataChannel(ctx, params)
}

// AcceptStream accepts a data channel
func (c *Connection) AcceptDataChannel(ctx context.Context) (*DataChannel, error) {
	return c.base.AcceptDataChannel(ctx)
}

func (c *baseConnection) OpenDataChannel(ctx context.Context, params DataChannelParameters) (*DataChannel, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	msg := &msgDataChannelOpenRequest{
		msgRequest: msgRequest{
			RequestId: msgRequestId(c.agentState.nextRequestID()),
		},
		ChannelId: params.ID,
		Label:     params.Label,
		Protocol:  params.Protocol,
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

	return &DataChannel{
		DataChannelParameters: params,
		stream:                stream,
	}, nil
}

func (c *baseConnection) AcceptDataChannel(ctx context.Context) (*DataChannel, error) {
	c.mu.Lock()
	close := c.close
	accept := c.connectedState.acceptDataChannel
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

type DataChannel struct {
	DataChannelParameters
	stream ApplicationStream
}

// SendMessage
func (c *DataChannel) SendMessage(payload []byte) error {
	return c.SendMessageWithEncoding(payload, DataEncodingBinary)
}

// SendMessageWithEncoding
func (c *DataChannel) SendMessageWithEncoding(payload []byte, enc DataEncoding) error {
	msg := &msgDataFrame{
		EncodingId: uint64(enc),
		Payload:    payload,
	}

	return writeMessage(msg, c.stream)
}

// ReceiveMessage
func (c *DataChannel) ReceiveMessage() ([]byte, error) {
	b, _, err := c.ReceiveMessageWithEncoding()
	return b, err
}

// ReceiveMessageWithEncoding
func (c *DataChannel) ReceiveMessageWithEncoding() ([]byte, DataEncoding, error) {
	msg, err := readMessage(c.stream)
	if err != nil {
		return nil, 0, err
	}

	dataFrame, ok := msg.(*msgDataFrame)
	if !ok {
		return nil, 0, fmt.Errorf("unexpected message type: %T", msg)
	}

	return dataFrame.Payload, DataEncoding(dataFrame.EncodingId), nil
}

// Read reads a packet of len(p) bytes as binary data
func (c *DataChannel) Read(p []byte) (int, error) {
	n, _, err := c.ReadDataChannel(p)
	return n, err
}

// ReadDataChannel reads a packet of len(p) bytes
func (c *DataChannel) ReadDataChannel(p []byte) (int, DataEncoding, error) {
	msg, err := readMessage(c.stream)
	if err != nil {
		return 0, 0, err
	}

	dataFrame, ok := msg.(msgDataFrame)
	if !ok {
		return 0, 0, fmt.Errorf("unexpected message type")
	}

	payload := dataFrame.Payload

	n := copy(p, payload)
	return int(n), DataEncoding(dataFrame.EncodingId), nil
}

// Write writes len(p) bytes from p as binary data
func (c *DataChannel) Write(p []byte) (n int, err error) {
	return c.WriteDataChannel(p, DataEncodingBinary)
}

// WriteDataChannel writes len(p) bytes from p
func (c *DataChannel) WriteDataChannel(p []byte, enc DataEncoding) (n int, err error) {
	msg := &msgDataFrame{
		EncodingId: uint64(enc),
		Payload:    p,
	}

	err = writeMessage(msg, c.stream)
	if err != nil {
		return 0, err
	}

	return len(p), nil
}

// Close closes the DataChannel and the underlying Quic stream.
func (c *DataChannel) Close() error {
	return c.stream.Close()
}
