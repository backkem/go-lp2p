package ospc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync/atomic"
	"time"
)

// LP2P session extension (local-peer-to-peer index.bs "Session extension").
//
// Both agents exchange agent-info and auth-capabilities as in OSP; only the
// client ever starts SPAKE2 (as the PSK consumer, by asking the presenter to
// show the PSK). The requesting origin is only sent to a verified server:
// once the client has verified the server's certificate (by SPAKE2, or
// because it trusts its fingerprint) it sends an lp2p-session-request on a
// new bidirectional stream. A server that hasn't verified the client answers
// a provisional authentication-required without looking at the origin, and
// the client authenticates; once the client is verified the server reports
// the session (RequestingOrigin) and answers success (CompleteSession) or
// permanent-error (RefuseSession). The client treats the session as accepted
// once it has received success. Agents that trust each other run no
// ceremony.
//
// The extension is enabled per agent with AgentConfig.SessionExtension (for
// listeners) and per connection with DiscoveredAgent.DialSession.

var (
	// ErrSessionRefused is the cause of a connection closed because the
	// server refused the session with lp2p-session-response permanent-error,
	// or because RefuseSession was called.
	ErrSessionRefused = errors.New("session refused")
	// ErrSessionUnsupported is returned by RequestSession when the peer
	// lists neither data-channels nor quic-transport in its agent-info, or
	// the transport can't carry the session stream.
	ErrSessionUnsupported = errors.New("peer does not support LP2P sessions")
	// ErrSessionProtocol is the cause of a connection closed for a violation
	// of the session extension, e.g. a second lp2p-session-request.
	ErrSessionProtocol = errors.New("session extension protocol violation")
	// ErrSessionNotVerified is returned by RequestSession, CompleteSession
	// and RequestingOrigin when the peer's certificate wasn't verified first.
	ErrSessionNotVerified = errors.New("peer certificate not verified")
)

// SessionCue is what AwaitSession reports.
type SessionCue int

const (
	// SessionCueAuthenticate: a trust ceremony is due. On the client, the
	// server answered authentication-required; on the server, the client
	// started SPAKE2. Run AcceptAuthenticate and AuthenticatePSK (the
	// client first StartAuthentication), then AwaitSession again.
	SessionCueAuthenticate SessionCue = iota + 1
	// SessionCueAccepted (client): the server accepted the session; call
	// CompleteSession.
	SessionCueAccepted
	// SessionCueRequested (server): the lp2p-session-request arrived. If
	// the client isn't verified yet, call RequireAuthentication and wait for
	// SessionCueAuthenticate; otherwise RequestingOrigin is available and
	// the session is accepted with CompleteSession or refused with
	// RefuseSession.
	SessionCueRequested
)

func (c SessionCue) String() string {
	switch c {
	case SessionCueAuthenticate:
		return "authenticate"
	case SessionCueAccepted:
		return "accepted"
	case SessionCueRequested:
		return "requested"
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
// stream, read past the request, carries the responses.
type incomingSessionRequest struct {
	msg    *msgLp2pSessionRequest
	stream ApplicationStream // Nil if the request came on a unidirectional stream.
}

// sessionState is the state of the session extension on one connection.
//
// Fields are protected by baseConnection.mu unless noted.
type sessionState struct {
	// origin is the requesting origin: given to DialSession by the client,
	// received by the server. Immutable once requested is closed.
	origin string
	// requested is closed once the lp2p-session-request was sent (client)
	// or received (server).
	requested chan struct{}
	// stream is the session stream. Set once requested is closed.
	stream ApplicationStream
	// requestID is the request-id of the lp2p-session-request, on the server.
	requestID msgRequestId
	// authStarted is closed on the server once the client started SPAKE2;
	// authRequired on the client once authentication-required arrived.
	authStarted  chan struct{}
	authRequired chan struct{}
	// authCues counts the authentication cues AwaitSession reported and
	// requestedCues the request cues, so each is reported once.
	authCues      atomic.Int32
	requestedCues atomic.Int32
	// authRequiredSent is set on the server once the provisional response
	// was sent.
	authRequiredSent bool
	// accepted is closed once success was sent (server) or received
	// (client).
	accepted chan struct{}
	// completed is set once CompleteSession ran.
	completed bool
	// responded is set on the server once a final response was sent.
	responded bool
}

func newSessionState() *sessionState {
	return &sessionState{
		requested:    make(chan struct{}),
		authStarted:  make(chan struct{}),
		authRequired: make(chan struct{}),
		accepted:     make(chan struct{}),
	}
}

// isRequested reports whether the lp2p-session-request was sent or received.
func (s *sessionState) isRequested() bool {
	select {
	case <-s.requested:
		return true
	default:
		return false
	}
}

// notifySessionAuthStarted reports, on a server with the session extension,
// that the client started SPAKE2.
//
// Caller should hold connection lock.
func (c *baseConnection) notifySessionAuthStarted() {
	if c.session == nil || c.agentRole != AgentRoleServer {
		return
	}
	select {
	case <-c.session.authStarted:
	default:
		close(c.session.authStarted)
	}
}

// requestSession is run by the client once it has verified the server: it
// opens the session stream, sends the lp2p-session-request and reads the
// responses in the background.
func (c *baseConnection) requestSession(ctx context.Context) error {
	if !c.remoteAgent.Authenticated() {
		return ErrSessionNotVerified
	}
	if !c.remoteAgent.HasCapability(AgentCapabilityDataChannels) && !c.remoteAgent.HasCapability(AgentCapabilityQuickTransport) {
		return fmt.Errorf("%w: agent-info lists neither data-channels nor quic-transport", ErrSessionUnsupported)
	}
	opener, ok := c.netConn.(networkStreamOpener)
	if !ok {
		return fmt.Errorf("%w: transport has no bidirectional streams before the hand-off", ErrSessionUnsupported)
	}

	c.mu.Lock()
	if c.session.isRequested() {
		c.mu.Unlock()
		return nil
	}
	origin := c.session.origin
	c.mu.Unlock()

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
	c.session.stream = stream
	close(c.session.requested)
	c.mu.Unlock()

	if !c.goroutine(func() { c.readSessionResponses(stream) }) {
		return c.err()
	}
	return nil
}

// readSessionResponses reads the server's lp2p-session-responses: an
// optional provisional authentication-required, then the final result, after
// which the stream ends.
func (c *baseConnection) readSessionResponses(stream ApplicationStream) {
	for {
		msg, err := readMessage(stream)
		if err != nil {
			c.shutdown(fmt.Errorf("%w: session stream ended before a final lp2p-session-response: %w", ErrSessionProtocol, err))
			return
		}
		resp, ok := msg.(*msgLp2pSessionResponse)
		if !ok {
			c.shutdown(fmt.Errorf("%w: got %s on the session stream, want lp2p-session-response", ErrSessionProtocol, msgName(msg)))
			return
		}
		c.log.Debug("receive", "msg", msgName(resp), "result", resp.Result)
		switch resp.Result {
		case LP2PSessionResultAuthenticationRequired:
			c.mu.Lock()
			select {
			case <-c.session.authRequired:
				c.mu.Unlock()
				c.shutdown(fmt.Errorf("%w: second authentication-required", ErrSessionProtocol))
				return
			default:
				close(c.session.authRequired)
			}
			c.mu.Unlock()
			continue
		case LP2PSessionResultSuccess:
			c.mu.Lock()
			close(c.session.accepted)
			c.mu.Unlock()
			// Both agents end their side of the stream.
			_ = stream.Close()
			c.expectSessionStreamEnd(stream)
			return
		default:
			// Ending the stream tells the server the refusal arrived.
			_ = stream.Close()
			c.shutdown(fmt.Errorf("%w: result %d", ErrSessionRefused, resp.Result))
			return
		}
	}
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
			Result:      LP2PSessionResultPermanentError,
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
	if c.session.isRequested() {
		return fmt.Errorf("%w: second lp2p-session-request", ErrSessionProtocol)
	}
	c.log.Debug("session requested")
	c.session.origin = req.msg.RequestingOrigin
	c.session.stream = req.stream
	c.session.requestID = req.msg.RequestId
	close(c.session.requested)
	return nil
}

// sendSessionResponse answers the lp2p-session-request on the server. Only
// one final response is sent.
//
// Caller should hold connection lock.
func (c *baseConnection) sendSessionResponse(result msgLp2pSessionResult) error {
	s := c.session
	if s.responded {
		return nil
	}
	final := result != LP2PSessionResultAuthenticationRequired
	if final {
		s.responded = true
	}
	resp := &msgLp2pSessionResponse{
		msgResponse: msgResponse{RequestId: s.requestID},
		Result:      result,
	}
	c.log.Debug("send", "msg", msgName(resp), "result", result)
	if err := writeMessage(resp, s.stream); err != nil {
		return err
	}
	if !final {
		return nil
	}
	// Both agents end their side of the stream.
	if err := s.stream.Close(); err != nil {
		return err
	}
	if result == LP2PSessionResultSuccess {
		close(s.accepted)
		stream := s.stream
		c.goroutine(func() { c.expectSessionStreamEnd(stream) })
	}
	return nil
}

// RequestSession sends the lp2p-session-request with the origin given to
// DialSession, on the client. The server's certificate must be verified
// first, by AuthenticatePSK or TrustPeer (ErrSessionNotVerified otherwise).
// It fails with ErrSessionUnsupported if the agent lists neither
// data-channels nor quic-transport. Then follow the server's cues with
// AwaitSession.
func (c *UnauthenticatedConnection) RequestSession(ctx context.Context) error {
	b := c.base
	if b.session == nil || b.agentRole != AgentRoleClient {
		return errors.New("not a client with the session extension")
	}
	return b.requestSession(ctx)
}

// RequestingOrigin returns the requesting origin the client sent in its
// lp2p-session-request, on the server, once the client's certificate is
// verified: before, it isn't looked at. It is the ASCII serialization of an
// origin as asserted by the client's user agent: parse it, and treat a value
// that doesn't parse as an opaque origin, which matches nothing.
func (c *UnauthenticatedConnection) RequestingOrigin() (origin string, ok bool) {
	b := c.base
	if b.session == nil || b.agentRole != AgentRoleServer || !b.remoteAgent.Authenticated() {
		return "", false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.session.isRequested() {
		return "", false
	}
	return b.session.origin, true
}

// RequireAuthentication answers the lp2p-session-request with the
// provisional authentication-required, on a server that hasn't verified the
// client's certificate. The client then starts SPAKE2: wait for
// SessionCueAuthenticate with AwaitSession and run AcceptAuthenticate and
// AuthenticatePSK.
func (c *UnauthenticatedConnection) RequireAuthentication() error {
	b := c.base
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.session == nil || b.agentRole != AgentRoleServer {
		return errors.New("not a server with the session extension")
	}
	if !b.session.isRequested() {
		return fmt.Errorf("%w: no lp2p-session-request received", ErrSessionProtocol)
	}
	if b.session.authRequiredSent {
		return nil
	}
	b.session.authRequiredSent = true
	return b.sendSessionResponse(LP2PSessionResultAuthenticationRequired)
}

// RefuseSession refuses the session on the server: it answers the
// lp2p-session-request with permanent-error and closes the connection once
// the client ended its side of the stream, or after a short wait. Call it
// when no receiver allows the requesting origin, or when the user declines.
func (c *UnauthenticatedConnection) RefuseSession() error {
	b := c.base
	b.mu.Lock()
	if b.session == nil || b.agentRole != AgentRoleServer {
		b.mu.Unlock()
		return errors.New("not a server with the session extension")
	}
	if !b.session.isRequested() {
		b.mu.Unlock()
		b.shutdown(ErrSessionRefused)
		b.wg.Wait()
		return nil
	}
	err := b.sendSessionResponse(LP2PSessionResultPermanentError)
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
	c.pendingCause.Store(&cause)
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

// AwaitSession waits for the next cue of the session exchange.
//
// On the client, after RequestSession: SessionCueAuthenticate when the
// server answers authentication-required, SessionCueAccepted when it
// accepts. On the server, after Accept: SessionCueAuthenticate when the
// client starts SPAKE2, SessionCueRequested when its lp2p-session-request
// arrives. Each cue is reported once; it fails with ErrSessionRefused if the
// server refuses, or with the close reason.
func (c *UnauthenticatedConnection) AwaitSession(ctx context.Context) (SessionCue, error) {
	b := c.base
	if b.session == nil {
		return 0, errors.New("connection without the session extension")
	}
	s := b.session
	authCue := s.authRequired
	if b.agentRole == AgentRoleServer {
		authCue = s.authStarted
	}
	if s.authCues.Load() > 0 {
		authCue = nil // Already reported.
	}
	var requested, accepted <-chan struct{}
	if b.agentRole == AgentRoleServer {
		if s.requestedCues.Load() == 0 {
			requested = s.requested
		}
	} else {
		accepted = s.accepted
	}
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	case <-b.ctx.Done():
		return 0, b.err()
	case <-authCue:
		s.authCues.Add(1)
		return SessionCueAuthenticate, nil
	case <-requested:
		s.requestedCues.Add(1)
		return SessionCueRequested, nil
	case <-accepted:
		return SessionCueAccepted, nil
	}
}

// CompleteSession finishes the session exchange and returns the connection.
//
// On the server it accepts the session: it sends the success
// lp2p-session-response, once the client's certificate is verified (by
// AuthenticatePSK, or because the caller trusts its fingerprint, see
// TrustPeer) and the user granted the session.
//
// On the client it waits for the success response, if not received yet.
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
	if b.agentRole == AgentRoleServer && !b.session.isRequested() {
		b.mu.Unlock()
		return nil, fmt.Errorf("%w: no lp2p-session-request received", ErrSessionProtocol)
	}
	b.session.completed = true
	var err error
	if b.agentRole == AgentRoleServer {
		err = b.sendSessionResponse(LP2PSessionResultSuccess)
	}
	b.mu.Unlock()
	if err := b.failOnError(err); err != nil {
		return nil, err
	}
	return c.connection()
}

// TrustPeer marks the peer's certificate as verified without SPAKE2, because
// its fingerprint (RemoteAgent().PeerID) is a trust anchor of the caller.
func (c *UnauthenticatedConnection) TrustPeer() error {
	c.base.remoteAgent.setAuthenticated()
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
