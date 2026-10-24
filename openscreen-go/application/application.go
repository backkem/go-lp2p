// Package application provides an OpenScreen Application Protocol implementation.
//
// The application protocol builds on top of the network protocol (ospc) and provides
// higher-level messaging for agent discovery and information exchange.
//
// Messages are sent on unidirectional QUIC streams, as application.bs
// requires: one stream per message, or per group of messages that must be
// processed in order. A request and its response travel on separate streams
// and are correlated by their request-id.
package application

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/fxamacker/cbor/v2"

	ospc "github.com/backkem/go-lp2p/openscreen-go/network"
)

// ErrClosed is returned once the ApplicationConnection is closed.
var ErrClosed = errors.New("application connection closed")

// incomingQueueLen bounds the received messages not yet consumed by
// ReceiveMessage. When it is full, reading further messages stalls.
const incomingQueueLen = 64

// Message is a received application message.
type Message struct {
	TypeKey ospc.TypeKey
	// Body is the CBOR-encoded message.
	Body cbor.RawMessage
}

// Decode decodes the message body into v.
func (m *Message) Decode(v interface{}) error {
	return ospc.DecodeCBOR(bytes.NewReader(m.Body), v)
}

// requestID returns the request-id of a request or response message: the
// unsigned integer at key 0 of the message map.
func (m *Message) requestID() (uint64, bool) {
	var v struct {
		RequestID *uint64 `cbor:"0,keyasint"`
	}
	if err := cbor.Unmarshal(m.Body, &v); err != nil || v.RequestID == nil {
		return 0, false
	}
	return *v.RequestID, true
}

type responseKey struct {
	typeKey   ospc.TypeKey
	requestID uint64
}

// ApplicationConnection wraps an authenticated ospc.Connection and provides
// application-level message exchange capabilities.
//
// It owns accepting incoming streams of the connection: don't call
// Connection.AcceptStream next to it. It reads every message from every
// incoming stream. Responses to requests sent with Request are handed to the
// waiting caller, all other messages are returned by ReceiveMessage in the
// order they were read.
//
// Request IDs are not tracked per connection: they are allocated by the local
// agent per remote agent (keyed by its certificate fingerprint), so they keep
// increasing when reconnecting to the same peer.
type ApplicationConnection struct {
	conn *ospc.Connection

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	incoming chan *Message

	mu      sync.Mutex
	pending map[responseKey]chan *Message
}

// NewApplicationConnection creates a new ApplicationConnection from an authenticated
// ospc.Connection and starts reading incoming messages.
func NewApplicationConnection(conn *ospc.Connection) *ApplicationConnection {
	ctx, cancel := context.WithCancel(context.Background())
	c := &ApplicationConnection{
		conn:     conn,
		ctx:      ctx,
		cancel:   cancel,
		incoming: make(chan *Message, incomingQueueLen),
		pending:  make(map[responseKey]chan *Message),
	}
	c.wg.Add(1)
	go c.acceptLoop()
	return c
}

// Connection returns the underlying ospc.Connection.
func (c *ApplicationConnection) Connection() *ospc.Connection {
	return c.conn
}

// LocalAgent returns the local agent.
func (c *ApplicationConnection) LocalAgent() *ospc.Agent {
	return c.conn.LocalAgent()
}

// RemoteAgent returns the remote agent.
func (c *ApplicationConnection) RemoteAgent() *ospc.Agent {
	return c.conn.RemoteAgent()
}

// Close closes the underlying connection and waits for the readers to exit.
func (c *ApplicationConnection) Close() error {
	c.cancel()
	err := c.conn.Close()
	c.wg.Wait()
	return err
}

func (c *ApplicationConnection) acceptLoop() {
	defer c.wg.Done()
	for {
		s, err := c.conn.AcceptStream(c.ctx)
		if err != nil {
			return
		}
		c.wg.Add(1)
		go c.readStream(s)
	}
}

// readStream reads the messages of a stream in order until it ends.
func (c *ApplicationConnection) readStream(s ospc.ApplicationStream) {
	defer c.wg.Done()
	defer s.Close()
	for {
		typeKey, err := ospc.ReadTypeKey(s)
		if err != nil {
			return // io.EOF at the end of the stream.
		}
		msg := &Message{TypeKey: typeKey}
		if err := ospc.DecodeCBOR(s, &msg.Body); err != nil {
			return
		}
		if !c.route(msg) {
			return
		}
	}
}

// route hands msg to a waiting request or queues it for ReceiveMessage. It
// reports false once the connection is closed.
func (c *ApplicationConnection) route(msg *Message) bool {
	if id, ok := msg.requestID(); ok {
		key := responseKey{typeKey: msg.TypeKey, requestID: id}
		c.mu.Lock()
		ch, ok := c.pending[key]
		delete(c.pending, key)
		c.mu.Unlock()
		if ok {
			ch <- msg // Buffered, see Request.
			return true
		}
	}

	select {
	case c.incoming <- msg:
		return true
	case <-c.ctx.Done():
		return false
	case <-c.conn.Done():
		return false
	}
}

// nextRequestID returns the next request ID for message correlation.
// The counter is shared with the network layer and all other connections
// to the same remote agent.
func (c *ApplicationConnection) nextRequestID() uint64 {
	return c.LocalAgent().NextRequestID(c.RemoteAgent().PeerID)
}

// NextRequestID allocates a request-id for a request to the remote agent.
func (c *ApplicationConnection) NextRequestID() ospc.RequestID {
	return ospc.RequestID(c.nextRequestID())
}

// SendMessage sends a typed message on a new unidirectional stream.
// The stream is closed after sending.
func (c *ApplicationConnection) SendMessage(ctx context.Context, typeKey ospc.TypeKey, msg interface{}) error {
	stream, err := c.conn.OpenUniStream(ctx)
	if err != nil {
		return fmt.Errorf("failed to open stream: %w", err)
	}
	if err := ospc.WriteTypedMessage(stream, typeKey, msg); err != nil {
		stream.Close()
		return fmt.Errorf("failed to send message: %w", err)
	}
	return stream.Close()
}

// Request sends a request message and waits for the response with type key
// respType and the same request-id, which is decoded into resp. The request
// must carry requestID at key 0 (see NextRequestID).
func (c *ApplicationConnection) Request(ctx context.Context, reqType ospc.TypeKey, requestID ospc.RequestID, req interface{}, respType ospc.TypeKey, resp interface{}) error {
	key := responseKey{typeKey: respType, requestID: uint64(requestID)}
	ch := make(chan *Message, 1)
	c.mu.Lock()
	if _, ok := c.pending[key]; ok {
		c.mu.Unlock()
		return fmt.Errorf("request-id %d already pending", requestID)
	}
	c.pending[key] = ch
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		delete(c.pending, key)
		c.mu.Unlock()
	}()

	if err := c.SendMessage(ctx, reqType, req); err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}

	select {
	case msg := <-ch:
		if err := msg.Decode(resp); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-c.ctx.Done():
		return ErrClosed
	case <-c.conn.Done():
		return fmt.Errorf("%w: %v", ErrClosed, c.conn.Err())
	}
}

// ReceiveMessage returns the next received message that is not a response
// to a pending Request.
func (c *ApplicationConnection) ReceiveMessage(ctx context.Context) (*Message, error) {
	select {
	case msg := <-c.incoming:
		return msg, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.ctx.Done():
		return nil, ErrClosed
	case <-c.conn.Done():
		return nil, fmt.Errorf("%w: %v", ErrClosed, c.conn.Err())
	}
}

// SendAgentInfoRequest sends an agent-info-request message and waits for the response.
// Returns the AgentInfo from the response.
func (c *ApplicationConnection) SendAgentInfoRequest(ctx context.Context) (*ospc.MsgAgentInfo, error) {
	reqID := c.NextRequestID()
	var resp ospc.MsgAgentInfoResponse
	err := c.Request(ctx,
		ospc.TypeKeyAgentInfoRequest, reqID, &ospc.MsgAgentInfoRequest{RequestID: reqID},
		ospc.TypeKeyAgentInfoResponse, &resp)
	if err != nil {
		return nil, err
	}
	return &resp.AgentInfo, nil
}

// ReceiveAgentInfoRequest waits for the next message, which must be an
// agent-info-request. Returns the request and a function to send the
// response. If the next message is of another type, it is discarded and an
// error returned.
func (c *ApplicationConnection) ReceiveAgentInfoRequest(ctx context.Context) (*ospc.MsgAgentInfoRequest, func(*ospc.MsgAgentInfo) error, error) {
	msg, err := c.ReceiveMessage(ctx)
	if err != nil {
		return nil, nil, err
	}
	if msg.TypeKey != ospc.TypeKeyAgentInfoRequest {
		return nil, nil, fmt.Errorf("unexpected message type %d, expected %d", msg.TypeKey, ospc.TypeKeyAgentInfoRequest)
	}
	var req ospc.MsgAgentInfoRequest
	if err := msg.Decode(&req); err != nil {
		return nil, nil, fmt.Errorf("failed to read request: %w", err)
	}

	respond := func(info *ospc.MsgAgentInfo) error {
		return c.SendMessage(c.ctx, ospc.TypeKeyAgentInfoResponse, &ospc.MsgAgentInfoResponse{
			RequestID: req.RequestID,
			AgentInfo: *info,
		})
	}

	return &req, respond, nil
}

// AnswerAgentInfoRequest answers msg, a received agent-info-request, with
// the local agent's agent-info (see ospc.Agent.AgentInfoMessage). After the
// network phase a peer may send agent-info-request at any time, and it
// arrives through ReceiveMessage like any other message.
func (c *ApplicationConnection) AnswerAgentInfoRequest(ctx context.Context, msg *Message) error {
	if msg.TypeKey != ospc.TypeKeyAgentInfoRequest {
		return fmt.Errorf("unexpected message type %d, expected %d", msg.TypeKey, ospc.TypeKeyAgentInfoRequest)
	}
	var req ospc.MsgAgentInfoRequest
	if err := msg.Decode(&req); err != nil {
		return fmt.Errorf("failed to read request: %w", err)
	}
	return c.SendMessage(ctx, ospc.TypeKeyAgentInfoResponse, &ospc.MsgAgentInfoResponse{
		RequestID: req.RequestID,
		AgentInfo: c.LocalAgent().AgentInfoMessage(),
	})
}

var _ io.Closer = (*ApplicationConnection)(nil)
