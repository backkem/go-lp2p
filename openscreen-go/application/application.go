// Package application provides an OpenScreen Application Protocol implementation.
//
// The application protocol builds on top of the network protocol (ospc) and provides
// higher-level messaging for agent discovery and information exchange.
//
// This implementation focuses on AgentInfo message exchange to match the Rust POC.
package application

import (
	"context"
	"fmt"
	"io"

	ospc "github.com/backkem/go-lp2p/openscreen-go/network"
)

// ApplicationConnection wraps an authenticated ospc.Connection and provides
// application-level message exchange capabilities.
//
// Request IDs are not tracked per connection: they are allocated by the local
// agent per remote agent (keyed by its certificate fingerprint), so they keep
// increasing when reconnecting to the same peer.
type ApplicationConnection struct {
	conn *ospc.Connection
}

// NewApplicationConnection creates a new ApplicationConnection from an authenticated
// ospc.Connection.
func NewApplicationConnection(conn *ospc.Connection) *ApplicationConnection {
	return &ApplicationConnection{
		conn: conn,
	}
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

// Close closes the underlying connection.
func (c *ApplicationConnection) Close() error {
	return c.conn.Close()
}

// nextRequestID returns the next request ID for message correlation.
// The counter is shared with the network layer and all other connections
// to the same remote agent.
func (c *ApplicationConnection) nextRequestID() uint64 {
	return c.LocalAgent().NextRequestID(c.RemoteAgent().PeerID)
}

// SendAgentInfoRequest sends an agent-info-request message and waits for the response.
// Returns the AgentInfo from the response.
func (c *ApplicationConnection) SendAgentInfoRequest(ctx context.Context) (*ospc.MsgAgentInfo, error) {
	// Open a new stream for this request/response exchange
	stream, err := c.conn.OpenStream(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to open stream: %w", err)
	}
	defer stream.Close()

	reqID := ospc.RequestID(c.nextRequestID())
	req := &ospc.MsgAgentInfoRequest{RequestID: reqID}
	if err := ospc.WriteTypedMessage(stream, ospc.TypeKeyAgentInfoRequest, req); err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	var resp ospc.MsgAgentInfoResponse
	if err := ospc.ReadTypedMessage(stream, ospc.TypeKeyAgentInfoResponse, &resp); err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}
	if resp.RequestID != reqID {
		return nil, fmt.Errorf("request ID mismatch: got %d, expected %d", resp.RequestID, reqID)
	}

	return &resp.AgentInfo, nil
}

// ReceiveAgentInfoRequest waits for and receives an agent-info-request message.
// Returns the request and a function to send the response.
//
// Incoming streams are taken from Connection.AcceptStream; a stream carrying
// another message type is closed and an error returned.
func (c *ApplicationConnection) ReceiveAgentInfoRequest(ctx context.Context) (*ospc.MsgAgentInfoRequest, func(*ospc.MsgAgentInfo) error, error) {
	stream, err := c.conn.AcceptStream(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to accept stream: %w", err)
	}

	var req ospc.MsgAgentInfoRequest
	if err := ospc.ReadTypedMessage(stream, ospc.TypeKeyAgentInfoRequest, &req); err != nil {
		stream.Close()
		return nil, nil, fmt.Errorf("failed to read request: %w", err)
	}

	// Return a respond function that sends the response and closes the stream
	respond := func(info *ospc.MsgAgentInfo) error {
		defer stream.Close()

		resp := &ospc.MsgAgentInfoResponse{
			RequestID: req.RequestID,
			AgentInfo: *info,
		}
		if err := ospc.WriteTypedMessage(stream, ospc.TypeKeyAgentInfoResponse, resp); err != nil {
			return fmt.Errorf("failed to send response: %w", err)
		}
		return nil
	}

	return &req, respond, nil
}

// SendMessage sends a typed message on a new stream.
// The stream is closed after sending.
func (c *ApplicationConnection) SendMessage(ctx context.Context, typeKey ospc.TypeKey, msg interface{}) error {
	stream, err := c.conn.OpenStream(ctx)
	if err != nil {
		return fmt.Errorf("failed to open stream: %w", err)
	}
	defer stream.Close()

	if err := ospc.WriteTypedMessage(stream, typeKey, msg); err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}

	return nil
}

// ReceiveMessage waits for and receives a typed message from an incoming stream.
// Returns the type key, a reader positioned at the CBOR body (see
// ospc.DecodeCBOR), and the stream, which the caller must close.
func (c *ApplicationConnection) ReceiveMessage(ctx context.Context) (ospc.TypeKey, io.Reader, ospc.ApplicationStream, error) {
	stream, err := c.conn.AcceptStream(ctx)
	if err != nil {
		return 0, nil, nil, fmt.Errorf("failed to accept stream: %w", err)
	}

	typeKey, err := ospc.ReadTypeKey(stream)
	if err != nil {
		stream.Close()
		return 0, nil, nil, fmt.Errorf("failed to read type key: %w", err)
	}

	return typeKey, stream, stream, nil
}
