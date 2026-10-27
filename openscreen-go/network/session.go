package ospc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"
)

// LP2P session extension (local-peer-to-peer index.bs "Session extension").
//
// The QUIC client sends an lp2p-session-request carrying the requesting
// origin on a new bidirectional stream right after the handshake, before
// authentication. The QUIC server reports it as the requesting origin hint
// (RequestingOrigin) and either refuses the session (RefuseSession), or, if
// it hasn't verified the client's certificate, asks it to authenticate by
// sending auth-capabilities (RequestAuthentication): only the client starts
// SPAKE2, the server only responds. Once the server has verified the
// client's certificate and accepted the session it answers success
// (CompleteSession). The client treats the session as accepted once it has
// received success and verified the server's certificate.
//
// The extension is enabled per agent with AgentConfig.SessionExtension (for
// listeners) and per connection with DiscoveredAgent.DialSession.

var (
	// ErrSessionRefused is the cause of a connection closed because the
	// server refused the session with lp2p-session-response permanent-error,
	// or because RefuseSession was called.
	ErrSessionRefused = errors.New("session refused")
	// ErrSessionUnsupported is returned by DialSession when the peer lists
	// neither data-channels nor quic-transport in its agent-info, or the
	// transport can't carry the session stream.
	ErrSessionUnsupported = errors.New("peer does not support LP2P sessions")
	// ErrNoSessionRequest is the cause of a connection closed by a listener
	// with the session extension because the client sent no
	// lp2p-session-request in time.
	ErrNoSessionRequest = errors.New("client sent no lp2p-session-request")
	// ErrSessionProtocol is the cause of a connection closed for a violation
	// of the session extension, e.g. a second lp2p-session-request.
	ErrSessionProtocol = errors.New("session extension protocol violation")
	// ErrSessionNotVerified is returned by CompleteSession on the server when
	// the client's certificate wasn't verified first.
	ErrSessionNotVerified = errors.New("peer certificate not verified")
)

// SessionCue is what AwaitSession reports on the client.
type SessionCue int

const (
	// SessionCueAuthenticate: the server sent auth-capabilities, so it
	// hasn't verified the client's certificate. Authenticate with
	// AcceptAuthenticate and AuthenticatePSK, then call CompleteSession.
	SessionCueAuthenticate SessionCue = iota + 1
	// SessionCueAccepted: the server accepted the session (success) without
	// asking for authentication, because it trusts the client's certificate.
	// If the server's certificate isn't trusted either, authenticate first
	// (the client initiates; the server answers); then call CompleteSession.
	SessionCueAccepted
)

func (c SessionCue) String() string {
	switch c {
	case SessionCueAuthenticate:
		return "authenticate"
	case SessionCueAccepted:
		return "accepted"
	default:
		return fmt.Sprintf("SessionCue(%d)", int(c))
	}
}

// networkStreamOpener is implemented by network connections that can open a
// bidirectional stream during the network phase, which the session stream
// needs.
type networkStreamOpener interface {
	OpenNetworkStream(ctx context.Context) (ApplicationStream, error)
}

// incomingSessionRequest is delivered by the transport in place of a message
// when a bidirectional stream starts with an lp2p-session-request. The
// stream, read past the request, carries the response.
type incomingSessionRequest struct {
	msg    *msgLp2pSessionRequest
	stream ApplicationStream // Nil if the request came on a unidirectional stream.
}

// sessionState is the state of the session extension on one connection.
//
// Fields are protected by baseConnection.mu unless noted.
type sessionState struct {
	// origin is the requesting origin: sent by the client, received by the
	// server. Immutable once requested is closed.
	origin string
	// requested is closed once the lp2p-session-request was sent (client)
	// or received (server).
	requested chan struct{}
	// stream is the session stream. Set once requested is closed.
	stream ApplicationStream
	// authRequested is set on the server once auth-capabilities was sent.
	authRequested bool
	// accepted is closed once success was sent (server) or received
	// (client).
	accepted chan struct{}
	// completed is set once CompleteSession ran.
	completed bool
	// responded is set on the server once a response was sent.
	responded bool
	// requestID is the request-id of the lp2p-session-request, on the server.
	requestID msgRequestId
}

func newSessionState() *sessionState {
	return &sessionState{
		requested: make(chan struct{}),
		accepted:  make(chan struct{}),
	}
}

// requestedOrigin returns the requesting origin once known.
func (s *sessionState) requestedOrigin() (string, bool) {
	select {
	case <-s.requested:
		return s.origin, true
	default:
		return "", false
	}
}

// isAccepted reports whether success was sent or received.
func (s *sessionState) isAccepted() bool {
	select {
	case <-s.accepted:
		return true
	default:
		return false
	}
}

// startSession is run by the client once the remote agent-info is known: it
// opens the session stream, sends the lp2p-session-request and reads the
// response in the background.
func (c *baseConnection) startSession(ctx context.Context, origin string) error {
	if !c.remoteAgent.HasCapability(AgentCapabilityDataChannels) && !c.remoteAgent.HasCapability(AgentCapabilityQuickTransport) {
		return fmt.Errorf("%w: agent-info lists neither data-channels nor quic-transport", ErrSessionUnsupported)
	}
	opener, ok := c.netConn.(networkStreamOpener)
	if !ok {
		return fmt.Errorf("%w: transport has no bidirectional streams before authentication", ErrSessionUnsupported)
	}
	stream, err := opener.OpenNetworkStream(ctx)
	if err != nil {
		return fmt.Errorf("open session stream: %w", err)
	}
	req := &msgLp2pSessionRequest{
		msgRequest:       msgRequest{RequestId: msgRequestId(c.nextRequestID())},
		RequestingOrigin: origin,
	}
	c.log.Debug("send", "msg", msgName(req), "origin", origin)
	if err := writeMessage(req, stream); err != nil {
		abortStream(stream)
		return fmt.Errorf("send lp2p-session-request: %w", err)
	}

	c.mu.Lock()
	c.session.origin = origin
	c.session.stream = stream
	close(c.session.requested)
	c.mu.Unlock()

	if !c.goroutine(func() { c.readSessionResponse(stream) }) {
		return c.err()
	}
	return nil
}

// readSessionResponse reads the server's lp2p-session-response and then
// expects the stream to end.
func (c *baseConnection) readSessionResponse(stream ApplicationStream) {
	msg, err := readMessage(stream)
	if err != nil {
		c.shutdown(fmt.Errorf("%w: session stream ended before lp2p-session-response: %w", ErrSessionProtocol, err))
		return
	}
	resp, ok := msg.(*msgLp2pSessionResponse)
	if !ok {
		c.shutdown(fmt.Errorf("%w: got %s on the session stream, want lp2p-session-response", ErrSessionProtocol, msgName(msg)))
		return
	}
	c.log.Debug("receive", "msg", msgName(resp), "result", resp.Result)
	if resp.Result != ResultSuccess {
		// Ending the stream tells the server the refusal arrived.
		_ = stream.Close()
		c.shutdown(fmt.Errorf("%w: result %d", ErrSessionRefused, resp.Result))
		return
	}

	c.mu.Lock()
	close(c.session.accepted)
	c.mu.Unlock()
	// Both agents end their side of the stream.
	_ = stream.Close()
	c.expectSessionStreamEnd(stream)
}

// expectSessionStreamEnd reads the session stream to its end; any further
// message is a protocol violation.
func (c *baseConnection) expectSessionStreamEnd(stream ApplicationStream) {
	var b [1]byte
	n, err := stream.Read(b[:])
	if n > 0 || (err != nil && err != io.EOF) {
		if c.ctx.Err() != nil {
			return
		}
		c.shutdown(fmt.Errorf("%w: unexpected data after the session exchange: %v", ErrSessionProtocol, err))
	}
}

// handleSessionRequest handles an lp2p-session-request on the server.
func (c *baseConnection) handleSessionRequest(req *incomingSessionRequest) error {
	if c.session == nil || c.agentRole != AgentRoleServer {
		// A client never receives one; an agent without the extension can't
		// provide a session. Refuse so the peer doesn't wait for a response,
		// then close, as the extension requires after a refusal.
		cause := fmt.Errorf("%w: unexpected lp2p-session-request", ErrSessionProtocol)
		if req.stream == nil {
			return cause
		}
		err := writeMessage(&msgLp2pSessionResponse{
			msgResponse: msgResponse{RequestId: req.msg.RequestId},
			Result:      ResultPermanentError,
		}, req.stream)
		if err != nil || req.stream.Close() != nil {
			return cause
		}
		c.goroutine(func() { c.closeAfterRefusal(req.stream, cause) })
		return nil
	}
	if req.stream == nil {
		return fmt.Errorf("%w: lp2p-session-request on a unidirectional stream", ErrSessionProtocol)
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.session.requestedOrigin(); ok {
		return fmt.Errorf("%w: second lp2p-session-request", ErrSessionProtocol)
	}
	c.log.Debug("session requested", "origin", req.msg.RequestingOrigin)
	c.session.origin = req.msg.RequestingOrigin
	c.session.stream = req.stream
	c.session.requestID = req.msg.RequestId
	close(c.session.requested)
	return nil
}

// sessionRequested reports whether the server received the
// lp2p-session-request. Without the extension it is true.
//
// Caller should hold connection lock.
func (c *baseConnection) sessionRequested() bool {
	if c.session == nil || c.agentRole != AgentRoleServer {
		return true
	}
	_, ok := c.session.requestedOrigin()
	return ok
}

// sendSessionResponse answers the lp2p-session-request on the server. Only
// the first response is sent.
//
// Caller should hold connection lock.
func (c *baseConnection) sendSessionResponse(result msgResult) error {
	s := c.session
	if s.responded {
		return nil
	}
	s.responded = true
	resp := &msgLp2pSessionResponse{
		msgResponse: msgResponse{RequestId: s.requestID},
		Result:      result,
	}
	c.log.Debug("send", "msg", msgName(resp), "result", result)
	if err := writeMessage(resp, s.stream); err != nil {
		return err
	}
	// Both agents end their side of the stream.
	if err := s.stream.Close(); err != nil {
		return err
	}
	if result == ResultSuccess {
		close(s.accepted)
		stream := s.stream
		c.goroutine(func() { c.expectSessionStreamEnd(stream) })
	}
	return nil
}

// RequestingOrigin returns the requesting origin hint the client sent in its
// lp2p-session-request, on a connection accepted by a listener with the
// session extension. It is the ASCII serialization of an origin as asserted
// by the client's user agent: parse it, and treat a value that doesn't
// parse as an opaque origin, which matches nothing.
func (c *UnauthenticatedConnection) RequestingOrigin() (origin string, ok bool) {
	if c.base.session == nil {
		return "", false
	}
	return c.base.session.requestedOrigin()
}

// RefuseSession refuses the session on the server: it answers the
// lp2p-session-request with permanent-error and closes the connection. Call
// it before any trust ceremony or prompt when no receiver allows the
// requesting origin, or when the user declines.
func (c *UnauthenticatedConnection) RefuseSession() error {
	b := c.base
	b.mu.Lock()
	if b.session == nil || b.agentRole != AgentRoleServer {
		b.mu.Unlock()
		return errors.New("not a server with the session extension")
	}
	err := b.sendSessionResponse(ResultPermanentError)
	stream := b.session.stream
	b.mu.Unlock()
	if err != nil {
		b.log.Debug("failed to send lp2p-session-response", "err", err)
		b.shutdown(ErrSessionRefused)
	} else {
		b.closeAfterRefusal(stream, ErrSessionRefused)
	}
	b.wg.Wait()
	return nil
}

// sessionRefusalGrace is how long a refusing server waits for the client
// to acknowledge the refusal before closing the connection.
const sessionRefusalGrace = 2 * time.Second

// closeAfterRefusal closes the connection with cause once the client ended
// its side of the session stream, which it does on receiving the refusal,
// or after sessionRefusalGrace. Closing the QUIC connection right away could
// discard the response before the client reads it.
func (c *baseConnection) closeAfterRefusal(stream ApplicationStream, cause error) {
	ended := make(chan struct{})
	go func() {
		defer close(ended)
		var buf [1]byte
		for {
			if _, err := stream.Read(buf[:]); err != nil {
				return
			}
		}
	}()
	select {
	case <-ended:
	case <-time.After(sessionRefusalGrace):
	case <-c.ctx.Done():
	}
	c.shutdown(cause)
}

// RequestAuthentication asks the client to authenticate, on the server with
// the session extension: it sends auth-capabilities, the cue for the client
// to start SPAKE2. Call it after checking the requesting origin, when the
// client's certificate isn't trusted yet; then run AcceptAuthenticate and
// AuthenticatePSK as usual. The server never initiates SPAKE2.
func (c *UnauthenticatedConnection) RequestAuthentication() error {
	b := c.base
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.session == nil || b.agentRole != AgentRoleServer {
		return errors.New("not a server with the session extension")
	}
	if !b.sessionRequested() {
		return fmt.Errorf("%w: no lp2p-session-request received", ErrSessionProtocol)
	}
	if b.session.authRequested {
		return nil
	}
	b.session.authRequested = true
	return b.sendAuthCapabilities()
}

// AwaitSession waits, on the client of a connection made with DialSession,
// for the server's cue: SessionCueAuthenticate when it sends
// auth-capabilities, SessionCueAccepted when it accepts the session right
// away. It fails with ErrSessionRefused if the server refuses.
func (c *UnauthenticatedConnection) AwaitSession(ctx context.Context) (SessionCue, error) {
	b := c.base
	if b.session == nil || b.agentRole != AgentRoleClient {
		return 0, errors.New("not a client with the session extension")
	}
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	case <-b.ctx.Done():
		return 0, b.err()
	case <-b.session.accepted:
		return SessionCueAccepted, nil
	case <-b.authNotify:
		return SessionCueAuthenticate, nil
	}
}

// CompleteSession finishes the session exchange and returns the connection.
//
// On the server it accepts the session: it sends the success
// lp2p-session-response, once the client's certificate is verified (by
// AuthenticatePSK, or because the caller trusts its fingerprint, see
// TrustPeer) and the user granted the session.
//
// On the client it waits for the success response, if not received yet. The
// caller must have verified the server's certificate first, by
// AuthenticatePSK or because it trusts its fingerprint (TrustPeer).
//
// Until it returns, neither agent sends messages of the data channel or
// WebTransport extensions.
func (c *UnauthenticatedConnection) CompleteSession(ctx context.Context) (*Connection, error) {
	b := c.base
	if b.session == nil {
		return nil, errors.New("connection without the session extension")
	}
	if !b.remoteAgent.Authenticated() {
		return nil, ErrSessionNotVerified
	}

	if b.agentRole == AgentRoleClient {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-b.ctx.Done():
			return nil, b.err()
		case <-b.session.accepted:
		}
	}

	b.mu.Lock()
	if b.session.completed {
		b.mu.Unlock()
		return c.connection()
	}
	b.session.completed = true
	var err error
	if b.agentRole == AgentRoleServer {
		err = b.sendSessionResponse(ResultSuccess)
	}
	b.mu.Unlock()
	if err := b.failOnError(err); err != nil {
		return nil, err
	}
	return c.connection()
}

// TrustPeer marks the peer's certificate as verified without SPAKE2, because
// its fingerprint (RemoteAgent().PeerID) is a trust anchor of the caller. It
// hands the connection to the application protocol. With the session
// extension, CompleteSession still has to run before the connection is used.
func (c *UnauthenticatedConnection) TrustPeer() error {
	if c.authenticated.Load() {
		return nil
	}
	if _, err := c.base.handOff(true); err != nil {
		return err
	}
	c.authenticated.Store(true)
	return nil
}

// connection returns the application connection, handing off if needed.
func (c *UnauthenticatedConnection) connection() (*Connection, error) {
	b := c.base
	if app := b.app.Load(); app != nil {
		c.authenticated.Store(true)
		return &Connection{base: b}, nil
	}
	conn, err := b.handOff(true)
	if err != nil {
		return nil, err
	}
	c.authenticated.Store(true)
	return conn, nil
}
