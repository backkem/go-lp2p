package lp2p

import (
	"context"
	"fmt"
	"sync"

	"github.com/backkem/go-lp2p/ospc"
)

// Data channel supports simple message passing over WebTransport.
type DataChannel struct {
	dc *ospc.DataChannel

	mu          sync.Mutex
	cbOnOpen    func(e OnOpenEvent)
	cbOnMessage func(e OnMessageEvent)
}

// DataChannelInit can be used to configure properties of the underlying
// channel.
type DataChannelInit struct {
	Protocol string
	ID       uint64
}

// CreateDataChannel creates a new data channel
func (c *LP2PConnection) CreateDataChannel(label string, opts *DataChannelInit) (*DataChannel, error) {
	props := ospc.DataChannelParameters{
		Label: label,
	}
	if opts != nil {
		props.Protocol = opts.Protocol
		props.ID = opts.ID
	}
	oDc, err := c.conn.OpenDataChannel(context.Background(), props)
	if err != nil {
		return nil, err
	}

	dc := &DataChannel{
		mu: sync.Mutex{},
		dc: oDc,
	}

	dc.run()

	return dc, nil
}

func (c *LP2PConnection) run(transportListener *LP2PQuicTransportListener) {
	// Listen for data channels.
	go func() {
		for {
			oDc, err := c.conn.AcceptDataChannel(context.Background())
			if err != nil {
				return
			}

			dc := &DataChannel{
				mu: sync.Mutex{},
				dc: oDc,
			}

			dc.run()

			c.onDataChannelHandler.OnCallback(OnDataChannelEvent{
				Channel: dc,
			})
		}
	}()

	// Listen for pooled Transports
	go func() {
		for {
			t, err := c.conn.AcceptTransport(context.Background())
			if err != nil {
				return
			}

			transportListener.handleTransport(incomingTransport{
				Transport:   t,
				IsDedicated: false,
			})
		}
	}()

}

// OnDataChannel fires when a data channel is opened.
type OnDataChannelEvent struct {
	Channel *DataChannel
}

func (c *DataChannel) run() {
	go func() {
		for {
			data, enc, err := c.dc.ReceiveMessageWithEncoding()
			if err != nil {
				return
			}

			c.onMessage(data, enc)
		}
	}()
}

type OnMessageEvent struct {
	Payload Payload
}

func (c *DataChannel) OnMessage(callback func(e OnMessageEvent)) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cbOnMessage = callback
}

func (c *DataChannel) onMessage(data []byte, encoding ospc.DataEncoding) {
	var payload Payload
	switch encoding {
	case ospc.DataEncodingBinary:
		payload = PayloadBinary{
			Data: data,
		}
	case ospc.DataEncodingString:
		payload = PayloadString{
			Data: data,
		}
	}
	e := OnMessageEvent{
		Payload: payload,
	}
	c.mu.Lock()
	cb := c.cbOnMessage
	c.mu.Unlock()

	cb(e)
}

// Send a message to the other peer
func (c *DataChannel) Send(data []byte) error {
	return c.dc.SendMessage(data)
}

// Send a message to the other peer
func (c *DataChannel) SendText(data string) error {
	return c.dc.SendMessageWithEncoding([]byte(data), ospc.DataEncodingString)
}

type OnOpenEvent struct {
	Channel *DataChannel
}

func (c *DataChannel) OnOpen(callback func(e OnOpenEvent)) {
	c.mu.Lock()
	c.cbOnOpen = callback
	c.mu.Unlock()

	// Just call right away for now.
	c.onOpen()
}

func (c *DataChannel) onOpen() {
	e := OnOpenEvent{
		Channel: c,
	}
	c.mu.Lock()
	cb := c.cbOnOpen
	c.mu.Unlock()

	cb(e)
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
