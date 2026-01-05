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
	"sync/atomic"

	ospc "github.com/backkem/go-lp2p/openscreen-go/network"
)

// ApplicationConnection wraps an authenticated ospc.Connection and provides
// application-level message exchange capabilities.
type ApplicationConnection struct {
	conn      *ospc.Connection
	requestID uint64
}

// NewApplicationConnection creates a new ApplicationConnection from an authenticated
// ospc.Connection.
func NewApplicationConnection(conn *ospc.Connection) *ApplicationConnection {
	return &ApplicationConnection{
		conn:      conn,
		requestID: 0,
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
func (c *ApplicationConnection) nextRequestID() uint64 {
	return atomic.AddUint64(&c.requestID, 1)
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

	// Create and send the request
	reqID := c.nextRequestID()
	req := &ospc.MsgAgentInfoRequest{
		RequestID: ospc.RequestID(reqID),
	}

	if err := ospc.WriteTypeKey(ospc.TypeKeyAgentInfoRequest, stream); err != nil {
		return nil, fmt.Errorf("failed to write type key: %w", err)
	}
	if err := ospc.EncodeCBOR(req, stream); err != nil {
		return nil, fmt.Errorf("failed to encode request: %w", err)
	}

	// Read the response
	typeKey, err := ospc.ReadTypeKey(stream)
	if err != nil {
		return nil, fmt.Errorf("failed to read response type key: %w", err)
	}
	if typeKey != ospc.TypeKeyAgentInfoResponse {
		return nil, fmt.Errorf("unexpected response type: %d, expected %d", typeKey, ospc.TypeKeyAgentInfoResponse)
	}

	var resp ospc.MsgAgentInfoResponse
	if err := ospc.DecodeCBOR(stream, &resp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Verify request ID matches
	if resp.RequestID != ospc.RequestID(reqID) {
		return nil, fmt.Errorf("request ID mismatch: got %d, expected %d", resp.RequestID, reqID)
	}

	return &resp.AgentInfo, nil
}

// ReceiveAgentInfoRequest waits for and receives an agent-info-request message.
// Returns the request and a function to send the response.
func (c *ApplicationConnection) ReceiveAgentInfoRequest(ctx context.Context) (*ospc.MsgAgentInfoRequest, func(*ospc.MsgAgentInfo) error, error) {
	// Accept an incoming stream
	stream, err := c.conn.AcceptStream(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to accept stream: %w", err)
	}

	// Read the request type key
	typeKey, err := ospc.ReadTypeKey(stream)
	if err != nil {
		stream.Close()
		return nil, nil, fmt.Errorf("failed to read type key: %w", err)
	}
	if typeKey != ospc.TypeKeyAgentInfoRequest {
		stream.Close()
		return nil, nil, fmt.Errorf("unexpected message type: %d, expected %d", typeKey, ospc.TypeKeyAgentInfoRequest)
	}

	// Decode the request
	var req ospc.MsgAgentInfoRequest
	if err := ospc.DecodeCBOR(stream, &req); err != nil {
		stream.Close()
		return nil, nil, fmt.Errorf("failed to decode request: %w", err)
	}

	// Return a respond function that sends the response and closes the stream
	respond := func(info *ospc.MsgAgentInfo) error {
		defer stream.Close()

		resp := &ospc.MsgAgentInfoResponse{
			RequestID: req.RequestID,
			AgentInfo: *info,
		}

		if err := ospc.WriteTypeKey(ospc.TypeKeyAgentInfoResponse, stream); err != nil {
			return fmt.Errorf("failed to write response type key: %w", err)
		}
		if err := ospc.EncodeCBOR(resp, stream); err != nil {
			return fmt.Errorf("failed to encode response: %w", err)
		}

		return nil
	}

	return &req, respond, nil
}

// SendMessage sends a raw typed message on a new stream.
// The stream is closed after sending.
func (c *ApplicationConnection) SendMessage(ctx context.Context, typeKey ospc.TypeKey, msg interface{}) error {
	stream, err := c.conn.OpenStream(ctx)
	if err != nil {
		return fmt.Errorf("failed to open stream: %w", err)
	}
	defer stream.Close()

	if err := ospc.WriteTypeKey(typeKey, stream); err != nil {
		return fmt.Errorf("failed to write type key: %w", err)
	}
	if err := ospc.EncodeCBOR(msg, stream); err != nil {
		return fmt.Errorf("failed to encode message: %w", err)
	}

	return nil
}

// ReceiveMessage waits for and receives a raw typed message from an incoming stream.
// Returns the type key, the raw reader for decoding, and the stream for cleanup.
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
