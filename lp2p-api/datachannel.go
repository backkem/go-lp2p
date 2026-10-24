package lp2p

import (
	"errors"
	"fmt"
	"io"
	"sync"

	ospc "github.com/backkem/go-lp2p/openscreen-go/network"
)

// LP2PDataChannelState is the readyState of an LP2PDataChannel, as for
// RTCDataChannel.
type LP2PDataChannelState string

const (
	LP2PDataChannelStateConnecting LP2PDataChannelState = "connecting"
	LP2PDataChannelStateOpen       LP2PDataChannelState = "open"
	LP2PDataChannelStateClosing    LP2PDataChannelState = "closing"
	LP2PDataChannelStateClosed     LP2PDataChannelState = "closed"
)

// LP2PDataChannel supports simple message passing with a peer.
//
// Closing is a handshake, as for RTCDataChannel: Close stops sending and
// fires onclosing; once the peer has closed too, onclose fires. Closing the
// LP2PConnection closes all its channels.
type LP2PDataChannel struct {
	dc       *ospc.DataChannel
	label    string
	protocol string
	id       uint64

	mu    sync.Mutex
	state LP2PDataChannelState

	onOpen    eventSlot[OnOpenEvent]
	onMessage eventSlot[OnMessageEvent]
	onError   eventSlot[OnErrorEvent]
	onClosing eventSlot[OnClosingEvent]
	onClose   eventSlot[OnCloseEvent]
	done      chan struct{}
}

// LP2PDataChannelInit configures a data channel.
type LP2PDataChannelInit struct {
	Protocol string
	// ID is the channel-id (0 to 65,534). If nil, the lowest ID not used
	// by an open channel of the connection is taken.
	ID *uint64
}

// LP2PDataChannelEvent is fired at an LP2PConnection (ondatachannel) when
// the peer opens a data channel.
type LP2PDataChannelEvent struct {
	Channel *LP2PDataChannel
}

// OnOpenEvent is fired when the channel opens.
type OnOpenEvent struct {
	Channel *LP2PDataChannel
}

// OnMessageEvent is fired for every message received.
type OnMessageEvent struct {
	Payload Payload
}

// OnErrorEvent is fired when the channel fails, before onclose.
type OnErrorEvent struct {
	Error error
}

// OnClosingEvent is fired when the channel starts closing.
type OnClosingEvent struct{}

// OnCloseEvent is fired when the channel is closed.
type OnCloseEvent struct{}

// CreateDataChannel creates a data channel to the peer.
func (c *LP2PConnection) CreateDataChannel(label string, init *LP2PDataChannelInit) (*LP2PDataChannel, error) {
	if c.isClosed() {
		return nil, errClosed("LP2PConnection")
	}
	props := ospc.DataChannelInit{Label: label}
	if init != nil {
		props.Protocol = init.Protocol
		props.ID = init.ID
	}
	// Blocks until the peer accepted the channel with a
	// data-channel-open-response (or DataChannelOpenTimeout passed).
	oDc, err := c.conn.OpenDataChannel(c.ctx, props)
	if err != nil {
		return nil, toDOMException(err)
	}
	return c.adoptDataChannel(oDc, oDc.DataChannelParameters), nil
}

func (c *LP2PConnection) adoptDataChannel(oDc *ospc.DataChannel, p ospc.DataChannelParameters) *LP2PDataChannel {
	dc := &LP2PDataChannel{
		dc:       oDc,
		label:    p.Label,
		protocol: p.Protocol,
		id:       p.ID,
		state:    LP2PDataChannelStateOpen,
		done:     make(chan struct{}),
	}
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		dc.run()
	}()
	// Like the browser, fire open asynchronously: the handler is likely set
	// after this returns, and the event is held until it is.
	dc.onOpen.fire(OnOpenEvent{Channel: dc})
	return dc
}

// Label of the channel.
func (c *LP2PDataChannel) Label() string { return c.label }

// Protocol of the channel.
func (c *LP2PDataChannel) Protocol() string { return c.protocol }

// ID of the channel.
func (c *LP2PDataChannel) ID() uint64 { return c.id }

// ReadyState of the channel.
func (c *LP2PDataChannel) ReadyState() LP2PDataChannelState {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.state
}

// OnOpen sets the onopen event handler.
func (c *LP2PDataChannel) OnOpen(cb func(OnOpenEvent)) { c.onOpen.set(cb) }

// OnMessage sets the onmessage event handler.
func (c *LP2PDataChannel) OnMessage(cb func(OnMessageEvent)) { c.onMessage.set(cb) }

// OnError sets the onerror event handler.
func (c *LP2PDataChannel) OnError(cb func(OnErrorEvent)) { c.onError.set(cb) }

// OnClosing sets the onclosing event handler.
func (c *LP2PDataChannel) OnClosing(cb func(OnClosingEvent)) { c.onClosing.set(cb) }

// OnClose sets the onclose event handler.
func (c *LP2PDataChannel) OnClose(cb func(OnCloseEvent)) { c.onClose.set(cb) }

// Send a binary message to the peer.
func (c *LP2PDataChannel) Send(data []byte) error {
	return c.send(data, ospc.DataEncodingBinary)
}

// SendText sends a string message to the peer.
func (c *LP2PDataChannel) SendText(data string) error {
	return c.send([]byte(data), ospc.DataEncodingString)
}

func (c *LP2PDataChannel) send(data []byte, enc ospc.DataEncoding) error {
	if s := c.ReadyState(); s != LP2PDataChannelStateOpen {
		return newDOMException(InvalidStateError, fmt.Sprintf("LP2PDataChannel is %s", s), nil)
	}
	if err := c.dc.SendMessageWithEncoding(data, enc); err != nil {
		return toDOMException(err)
	}
	return nil
}

// Close starts closing the channel. It doesn't block; onclose fires once
// the peer has closed its side.
func (c *LP2PDataChannel) Close() error {
	if !c.transition(LP2PDataChannelStateClosing) {
		return nil
	}
	c.onClosing.fire(OnClosingEvent{})
	return c.dc.Close()
}

// transition moves the channel forward to state, reporting whether it moved.
func (c *LP2PDataChannel) transition(state LP2PDataChannelState) bool {
	order := map[LP2PDataChannelState]int{
		LP2PDataChannelStateConnecting: 0,
		LP2PDataChannelStateOpen:       1,
		LP2PDataChannelStateClosing:    2,
		LP2PDataChannelStateClosed:     3,
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if order[state] <= order[c.state] {
		return false
	}
	c.state = state
	return true
}

func (c *LP2PDataChannel) run() {
	defer close(c.done)
	for {
		data, enc, err := c.dc.ReceiveMessageWithEncoding()
		if err != nil {
			c.closed(err)
			return
		}
		c.onMessage.fire(OnMessageEvent{Payload: newPayload(data, enc)})
	}
}

// closed finishes the closing handshake once the receive side has ended.
func (c *LP2PDataChannel) closed(err error) {
	weClosed := c.ReadyState() == LP2PDataChannelStateClosing
	if !weClosed && !errors.Is(err, io.EOF) {
		c.onError.fire(OnErrorEvent{Error: toDOMException(err)})
	}
	if c.transition(LP2PDataChannelStateClosing) {
		// The peer closed first: answer the handshake.
		c.onClosing.fire(OnClosingEvent{})
		_ = c.dc.Close()
	}
	if c.transition(LP2PDataChannelStateClosed) {
		c.onClose.fire(OnCloseEvent{})
	}
}

func newPayload(data []byte, enc ospc.DataEncoding) Payload {
	if enc == ospc.DataEncodingString {
		return PayloadString{Data: data}
	}
	return PayloadBinary{Data: data}
}

// PayloadType are the different types of data that can be
// represented in a DataChannel message
type PayloadType int

// PayloadType enums
const (
	PayloadTypeString PayloadType = iota + 1
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

// String returns the message text.
func (p PayloadString) String() string { return string(p.Data) }

// PayloadBinary is a binary DataChannel message
type PayloadBinary struct {
	Data []byte
}

// PayloadType returns the type of payload
func (p PayloadBinary) PayloadType() PayloadType {
	return PayloadTypeBinary
}
