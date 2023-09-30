package lp2p

import (
	"context"
	"fmt"

	"github.com/backkem/go-lp2p/ospc"
)

// Data channel supports simple message passing over WebTransport.
type DataChannel struct {
	ch *ospc.DataChannel
}

// CreateDataChannel creates a new data channel
func (c *LP2PConnection) CreateDataChannel() (*DataChannel, error) {
	dc, err := c.conn.CreateDataChannel(context.Background())
	if err != nil {
		return nil, err
	}

	return &DataChannel{
		ch: ch,
	}, nil
}

type OnDataChannelEvent struct {
	Channel *DataChannel
}

// OnDataChannel fires when a data channel is opened.
func (c *LP2PConnection) OnDataChannel(callback func(e OnDataChannelEvent)) {
	// TODO: event wiring
}

type OnMessageEvent struct {
	Payload Payload // TODO: Just use interface instead?
}

func (c *DataChannel) OnMessage(callback func(e OnMessageEvent)) {
	// TODO: Event wiring
}

// Send a message to the other peer
func (c *DataChannel) Send(data []byte) error {
	return c.dc.SendMessage(data)
}

// Send a message to the other peer
func (c *DataChannel) SendText(data string) error {
	return c.dc.SendMessageWithEncoding([]byte(data), ospc.DataEncodingString)
}

type OnDataChannelOpenEvent struct {
}

func (c *DataChannel) OnOpen(callback func(e OnDataChannelOpenEvent)) {
	// TODO: Event wiring
}

// TODO: teardown
// type OnCloseEvent struct {
// }
//
// func (c *DataChannel) OnClose(callback func (e OnCloseEvent)) {
//
// }

// TODO: teardown
// func (c *DataChannel) Close() error {
//
// }

// PayloadType are the different types of data that can be
// represented in a DataChannel message
type PayloadType int

// PayloadType enums
const (
	PayloadTypeString = iota + 1
	PayloadTypeBinary
	PayloadTypeArrayBuffer
)

func (p PayloadType) String() string {
	switch p {
	case PayloadTypeString:
		return "Payload Type String"
	case PayloadTypeBinary:
		return "Payload Type Binary"
	case PayloadTypeArrayBuffer:
		return "Payload Type ArrayBuffer"
	default:
		return fmt.Sprintf("Invalid PayloadType (%d)", p)
	}
}

// Payload is the body of a DataChannel message
type Payload interface {
	PayloadType() PayloadType
}

// PayloadString is a string DataChannel message
type PayloadString struct {
	Data []byte
}

// PayloadType returns the type of payload
func (p PayloadString) PayloadType() PayloadType {
	return PayloadTypeString
}

// PayloadBinary is a binary DataChannel message
type PayloadBinary struct {
	Data []byte
}

// PayloadType returns the type of payload
func (p PayloadBinary) PayloadType() PayloadType {
	return PayloadTypeBinary
}
