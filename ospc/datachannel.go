package ospc

import (
	"context"
	"fmt"

	quic "github.com/quic-go/quic-go"
)

// OpenDataChannel opens a data channel
func (c *Connection) OpenDataChannel(ctx context.Context) (*DataChannel, error) {
	return c.base.OpenDataChannel(ctx)
}

// AcceptStream accepts a data channel
func (c *Connection) AcceptDataChannel(ctx context.Context) (*DataChannel, error) {
	return c.base.AcceptDataChannel(ctx)
}

func (c *baseConnection) OpenDataChannel(ctx context.Context) (*DataChannel, error) {
	// TODO
	return nil, nil
}

func (c *baseConnection) AcceptDataChannel(ctx context.Context) (*DataChannel, error) {
	// TODO
	return nil, nil
}

type DataChannel struct {
	stream quic.Stream
}

// SendMessage
func (c *DataChannel) SendMessage(payload []byte) error {
	return c.SendMessageWithEncoding(payload, DataEncodingBinary)
}

// SendMessageWithEncoding
func (c *DataChannel) SendMessageWithEncoding(payload []byte, enc DataEncoding) error {
	msg := &msgDataFrame{
		EncodingId: enc,
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

	dataFrame, ok := msg.(msgDataFrame)
	if !ok {
		return nil, 0, fmt.Errorf("unexpected message type")
	}

	return dataFrame.Payload, dataFrame.EncodingId, nil
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
	if err != nil {
		return int(n), 0, err
	}

	return int(n), DataEncoding(dataFrame.EncodingId), nil
}

// Write writes len(p) bytes from p as binary data
func (c *DataChannel) Write(p []byte) (n int, err error) {
	return c.WriteDataChannel(p, DataEncodingBinary)
}

// WriteDataChannel writes len(p) bytes from p
func (c *DataChannel) WriteDataChannel(p []byte, enc DataEncoding) (n int, err error) {
	msg := &msgDataFrame{
		EncodingId: enc,
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
