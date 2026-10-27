package ospc

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	spake2 "github.com/backkem/spake2-go"
)

// ErrAuthenticationFailed is the cause of a connection closed because
// authentication failed, locally or as reported by the remote agent.
var ErrAuthenticationFailed = errors.New("authentication failed")

type exchangeInfoState struct {
	requestIds map[uint64]bool // Of the agent-info-requests sent.
	done       chan struct{}   // Closed when the remote info is complete.
}

// exchangeInfo sends the local auth-capabilities and requests the remote
// agent-info. The returned channel is closed once both the remote agent-info
// and auth-capabilities are received.
func (c *baseConnection) exchangeInfo() (<-chan struct{}, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.exchangeInfoState != nil {
		return c.exchangeInfoState.done, nil
	}

	// Auth Info
	localAuthInfo := c.localAgent.AuthenticationInfo()
	authMsg := &msgAuthCapabilities{
		PskEaseOfInput:      uint64(localAuthInfo.PSKConfig.EaseOfInput),
		PskInputMethods:     []msgPskInputMethod{PskInputMethodNumeric},
		PskMinBitsOfEntropy: uint64(localAuthInfo.PSKConfig.Entropy),
	}

	err := c.send(authMsg)
	if err != nil {
		return nil, err
	}

	// Remote AgentInfo
	state := &exchangeInfoState{
		requestIds: make(map[uint64]bool),
		done:       make(chan struct{}),
	}
	c.exchangeInfoState = state
	if err := c.sendAgentInfoRequest(); err != nil {
		return nil, err
	}

	return state.done, nil
}

// sendAgentInfoRequest sends an agent-info-request with a new request ID.
//
// Caller should hold connection lock.
func (c *baseConnection) sendAgentInfoRequest() error {
	id := c.nextRequestID()
	c.exchangeInfoState.requestIds[id] = true
	return c.send(&msgAgentInfoRequest{
		msgRequest: msgRequest{
			RequestId: msgRequestId(id),
		},
	})
}

// retryAgentInfoRequest resends the agent-info-request if no response
// arrived yet.
func (c *baseConnection) retryAgentInfoRequest() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.remoteAgent.HasInfo() {
		return nil
	}
	c.log.Debug("no agent-info-response yet, resending agent-info-request")
	return c.sendAgentInfoRequest()
}

// ErrNoAgentInfo is returned by Dial and the listener when the peer doesn't
// complete metadata discovery in time: it didn't answer agent-info-request
// or didn't send auth-capabilities. Peers that don't implement network.bs
// "Metadata Discovery" and "Authentication" fail this way, see
// InteropQuirks.TolerateMissingAgentInfo.
var ErrNoAgentInfo = errors.New("peer did not complete metadata discovery")

// infoExchangeTimeout bounds the info exchange of new connections.
const infoExchangeTimeout = 10 * time.Second

// infoRetryInterval is how often the agent-info-request is resent while
// waiting for the response.
const infoRetryInterval = 1500 * time.Millisecond

// tolerateInfoGrace is how long the info exchange waits with the
// TolerateMissingAgentInfo quirk before proceeding without it.
const tolerateInfoGrace = 2 * time.Second

// awaitInfo runs the info exchange and waits for it to complete, at most
// infoExchangeTimeout. It does not close the connection on failure.
func (c *baseConnection) awaitInfo(ctx context.Context) error {
	done, err := c.exchangeInfo()
	if err != nil {
		return fmt.Errorf("failed to exchange info: %w", err)
	}

	timeout := infoExchangeTimeout
	tolerate := c.localAgent.quirks.TolerateMissingAgentInfo
	if tolerate {
		timeout = tolerateInfoGrace
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	// A request can get lost if it reaches the peer before the peer is ready
	// for it, as seen with osp_demo. Resending it is harmless: responses to
	// any of the requests are accepted.
	retry := time.NewTicker(infoRetryInterval)
	defer retry.Stop()

	for {
		select {
		case <-retry.C:
			if err := c.retryAgentInfoRequest(); err != nil {
				return fmt.Errorf("failed to exchange info: %w", err)
			}
			continue
		case <-ctx.Done():
			return fmt.Errorf("%w (missing %s): %w", ErrNoAgentInfo, c.missingInfo(), ctx.Err())
		case <-c.ctx.Done():
			return c.err()
		case <-done:
			return nil
		case <-timer.C:
			if tolerate {
				c.proceedWithoutInfo()
				return nil
			}
			return fmt.Errorf("%w within %v (missing %s)", ErrNoAgentInfo, timeout, c.missingInfo())
		}
	}
}

// missingInfo describes which part of the info exchange is missing.
func (c *baseConnection) missingInfo() string {
	var missing []string
	if !c.remoteAgent.HasInfo() {
		missing = append(missing, "agent-info-response")
	}
	if !c.remoteAgent.HasAuthenticationInfo() {
		missing = append(missing, "auth-capabilities")
	}
	if len(missing) == 0 {
		return "nothing"
	}
	return strings.Join(missing, " and ")
}

// proceedWithoutInfo continues after an incomplete info exchange, see
// InteropQuirks.TolerateMissingAgentInfo.
func (c *baseConnection) proceedWithoutInfo() {
	c.log.Warn("interop quirk: peer did not complete metadata discovery, proceeding without it", "missing", c.missingInfo())

	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.remoteAgent.HasAuthenticationInfo() {
		// Assume equal capabilities: the advertising agent presents the PSK.
		c.remoteAgent.setAuthenticationInfo(*c.localAgent.AuthenticationInfo())
	}
	c.determineAuthenticationRole()
	c.doAuthNotify()
}

// caller should hold connection lock.
func (c *baseConnection) checkAgentInfoComplete() {
	state := c.exchangeInfoState
	if state == nil || !c.remoteAgent.HasInfo() || !c.remoteAgent.HasAuthenticationInfo() {
		return
	}
	select {
	case <-state.done:
		return // Already complete.
	default:
	}
	c.determineAuthenticationRole()
	c.doAuthNotify()
	close(state.done)
}

type AuthenticationRole int

const (
	AuthenticationRolePresenter AuthenticationRole = 0
	AuthenticationRoleConsumer  AuthenticationRole = 1
)

func (t AuthenticationRole) String() string {
	switch t {
	case AuthenticationRolePresenter:
		return "Presenter"
	case AuthenticationRoleConsumer:
		return "Consumer"
	default:
		return fmt.Sprintf("Unknown AuthenticationRole: %d", t)
	}
}

// GetAuthenticationRole determines if the agent should act as presenter or consumer of the PSK.
// Only correct after auth-capabilities exchange.
func (c *baseConnection) GetAuthenticationRole() AuthenticationRole {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.authenticationRole
}

// Caller should hold connection lock.
func (c *baseConnection) determineAuthenticationRole() {
	c.authenticationRole = c.getAuthenticationRole()
}

// Caller should hold connection lock.
func (c *baseConnection) getAuthenticationRole() AuthenticationRole {
	localAuthInfo := c.localAgent.AuthenticationInfo()
	remoteAuthInfo := c.remoteAgent.AuthenticationInfo()
	if remoteAuthInfo == nil {
		remoteAuthInfo = localAuthInfo
	}

	if localAuthInfo.PSKConfig.EaseOfInput == remoteAuthInfo.PSKConfig.EaseOfInput {
		if c.agentRole == AgentRoleServer {
			return AuthenticationRolePresenter
		}
		return AuthenticationRoleConsumer
	}
	if localAuthInfo.PSKConfig.EaseOfInput < remoteAuthInfo.PSKConfig.EaseOfInput {
		return AuthenticationRolePresenter
	}
	return AuthenticationRoleConsumer
}

// GeneratePSK creates a random PSK for the PSK presenter to show to the user.
// network.bs: "The PSK presenter must generate a PSK that has at least as
// many bits of entropy as it receives in this field, and at least as many
// bits of entropy as it sends in this field." It returns the PSK as canonical
// decimal text (see ParsePSK), to be shown with FormatPSK and passed to
// AuthenticatePSK.
func (c *baseConnection) GeneratePSK() ([]byte, error) {
	c.mu.Lock()
	localAuthInfo := c.localAgent.AuthenticationInfo()
	remoteAuthInfo := c.remoteAgent.AuthenticationInfo()
	c.mu.Unlock()
	if remoteAuthInfo == nil {
		remoteAuthInfo = localAuthInfo
	}
	bits := max(localAuthInfo.PSKConfig.Entropy, remoteAuthInfo.PSKConfig.Entropy, minPSKEntropy)
	// A peer asking for more than the spec's maximum of 60 bits is out of
	// range; don't let it force an unusable PSK.
	bits = min(bits, maxPSKEntropy)
	return generatePSK(bits)
}

// authenticationState is the state of the SPAKE2 authentication
// (network.bs "Authentication with SPAKE2") of one connection.
type authenticationState struct {
	// psk is the canonical PSK (SPAKE2 pw), nil until AuthenticatePSK.
	psk []byte
	// remoteHandshake is the peer's initiating auth-spake2-handshake (pA),
	// kept until the PSK is known.
	remoteHandshake *msgAuthSpake2Handshake

	spakeState *spake2.SPAKE2 // Nil until the local PSK is known.
	alice      bool           // The local agent initiated.
	// remotePublic is pB, for Alice.
	remotePublic       []byte
	sentConfirmation   bool
	remoteConfirmation []byte
	verified           bool // The peer's confirmation is valid.
	failed             bool

	localResult  *msgAuthStatusResult
	remoteResult *msgAuthStatusResult

	finished bool
	done     chan struct{} // Closed once verified and the peer's auth-status received.
}

// authState returns the authentication state, creating it if needed.
//
// Caller should hold connection lock.
func (c *baseConnection) authState() *authenticationState {
	if c.authenticationState == nil {
		c.authenticationState = &authenticationState{
			done: make(chan struct{}),
		}
	}
	return c.authenticationState
}

// AcceptAuthenticate waits until the authentication role is known, after the
// auth-capabilities exchange. The presenter then shows a PSK to the user
// (GeneratePSK, FormatPSK), the consumer asks the user for it; both pass it
// to AuthenticatePSK.
func (c *baseConnection) AcceptAuthenticate(ctx context.Context) (role AuthenticationRole, err error) {
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	case <-c.ctx.Done():
		return 0, c.err()
	case <-c.authNotify:
		return c.GetAuthenticationRole(), nil
	}
}

// failOnError closes the connection if err is not nil. Caller must not hold
// the connection lock.
func (c *baseConnection) failOnError(err error) error {
	if err != nil {
		c.shutdown(err)
	}
	return err
}

// Caller should hold connection lock
func (c *baseConnection) doAuthNotify() {
	if c.authNotified {
		return
	}
	c.authNotified = true
	close(c.authNotify)
}

// Caller should hold connection lock.
//
// Per network.bs: "For an advertising agent, the at field in its mDNS TXT
// record must be used as the auth-initiation-token in the first
// authentication message sent to or from that agent." The token is sent in
// the first authentication message this agent sends, and left unset after.
func (c *baseConnection) nextAuthInitiationToken() string {
	if c.authTokenSent {
		return ""
	}
	c.authTokenSent = true
	return c.authToken
}

// Caller should hold connection lock.
//
// Per network.bs: "Agents should discard any authentication message whose
// auth-initiation-token is set and does not match the at provided by the
// advertising agent." If the at is unknown, e.g. the remote agent was not
// discovered through mDNS, no token can be checked.
func (c *baseConnection) validAuthInitiationToken(token string) bool {
	return token == "" || c.authToken == "" || token == c.authToken
}

// AuthenticatePSK authenticates with SPAKE2 using psk, a base-10 numeric PSK
// in any form ParsePSK accepts. It blocks until authentication is complete
// or ctx is done.
//
// If the peer already initiated, the local agent answers as Bob. Otherwise it
// initiates as Alice: as PSK presenter with psk-status psk-shown, as PSK
// consumer with psk-needs-presentation (network.bs "Authentication with
// SPAKE2"). If both initiate, the presenter's initiation wins, as in
// Chromium's openscreen: the consumer abandons its own and answers.
func (c *baseConnection) AuthenticatePSK(ctx context.Context, psk []byte) (*Connection, error) {
	done, err := c.authenticatePSK(psk)
	if err != nil {
		return nil, err
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.ctx.Done():
		return nil, c.err()
	case <-done:
		conn, err := c.finishAuthentication()
		return conn, c.failOnError(err)
	}
}

func (c *baseConnection) finishAuthentication() (*Connection, error) {
	c.mu.Lock()
	remoteResult := c.authenticationState.remoteResult
	c.mu.Unlock()

	if remoteResult == nil || *remoteResult != AuthStatusResultAuthenticated {
		return nil, fmt.Errorf("%w: remote result %v", ErrAuthenticationFailed, remoteResult)
	}

	return c.handOff(true)
}

// skipAuthentication hands the connection to the application protocol
// without authenticating the peer, see InteropQuirks.AllowSkipAuthentication.
func (c *baseConnection) skipAuthentication() (*Connection, error) {
	if !c.localAgent.quirks.AllowSkipAuthentication {
		return nil, fmt.Errorf("%w: AllowSkipAuthentication", ErrQuirkDisabled)
	}
	// An authentication the peer started is abandoned: messages for it are
	// dropped after the hand-off.
	c.log.Warn("INSECURE interop quirk: skipping authentication, the peer is NOT authenticated")
	return c.handOff(false)
}

// handOff hands the connection to the application protocol.
func (c *baseConnection) handOff(authenticated bool) (*Connection, error) {
	appConn, err := c.netConn.IntoApplicationConnection()
	if err != nil {
		return nil, err
	}
	if authenticated {
		c.remoteAgent.setAuthenticated()
	}

	c.app.Store(&connectedState{
		appConn:               appConn,
		acceptDataChannel:     make(chan *DataChannel),
		acceptTransport:       make(chan *PooledWebTransport),
		acceptTransportStream: make(chan ApplicationStream),
		acceptStream:          make(chan ApplicationStream),
	})
	if !c.runApplication() {
		// Closed concurrently, make sure the application connection is too.
		_ = appConn.Close()
		return nil, c.err()
	}

	return &Connection{
		base: c,
	}, nil
}

// authenticatePSK sets the PSK and progresses authentication. The returned
// channel is closed once authentication completes. Usage errors leave the
// connection open, other errors close it.
func (c *baseConnection) authenticatePSK(psk []byte) (<-chan struct{}, error) {
	pw, err := ParsePSK(string(psk))
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	authState := c.authState()
	if authState.psk != nil {
		c.mu.Unlock()
		return nil, errors.New("already authenticating")
	}
	authState.psk = []byte(pw)
	if !c.localAgent.quirks.Spake2SuiteP256 {
		err = c.authFailed(AuthStatusResultUnknownError, ErrSpake2SuiteUnsupported)
	} else {
		err = c.authenticatePSKProgress()
	}
	c.mu.Unlock()

	if err := c.failOnError(err); err != nil {
		return nil, err
	}
	return authState.done, nil
}

// authFailed records a failed local result, informs the remote agent and
// returns the error that closes the connection.
//
// Caller should hold connection lock.
func (c *baseConnection) authFailed(result msgAuthStatusResult, err error) error {
	authState := c.authState()
	authState.failed = true
	authState.localResult = &result
	if sendErr := c.sendAuthStatus(); sendErr != nil {
		c.log.Debug("failed to send auth-status", "err", sendErr)
	}
	return fmt.Errorf("%w: %w", ErrAuthenticationFailed, err)
}

// spakeFailureResult maps a SPAKE2 error to an auth-status result.
func spakeFailureResult(err error) msgAuthStatusResult {
	if errors.Is(err, spake2.ErrInvalidConfirmation) {
		return AuthStatusResultProofInvalid
	}
	return AuthStatusResultUnknownError
}

// spake2Identities returns the SPAKE2 identities. network.bs: "The values A
// and B used in SPAKE2 are the agent fingerprints of the client and server,
// respectively." The client is the agent that opened the QUIC connection,
// regardless of who initiates SPAKE2. The fingerprints are used as their
// base64 text (network.bs "Computing the Agent Fingerprint"), as Chromium's
// openscreen does.
func (c *baseConnection) spake2Identities() (a, b []byte) {
	local, remote := []byte(c.localAgent.PeerID), []byte(c.remoteAgent.PeerID)
	if c.agentRole == AgentRoleClient {
		return local, remote
	}
	return remote, local
}

// authInitiate starts SPAKE2 as Alice.
//
// Caller should hold connection lock.
func (c *baseConnection) authInitiate() error {
	authState := c.authenticationState
	a, b := c.spake2Identities()
	authState.spakeState = newSpake2(true, authState.psk, a, b)
	authState.alice = true
	pA, err := authState.spakeState.Start()
	if err != nil {
		return c.authFailed(AuthStatusResultUnknownError, err)
	}
	// network.bs: the presenter initiates with psk-status psk-shown, the
	// consumer with psk-needs-presentation, both with pA.
	status := AuthSpake2PskStatusPskNeedsPresentation
	if c.authenticationRole == AuthenticationRolePresenter {
		status = AuthSpake2PskStatusPskShown
	}
	c.log.Debug("SPAKE2: initiating as Alice", "role", c.authenticationRole, "psk-status", status)
	return c.sendAuthSpake2Handshake(status, pA)
}

// authRespond answers the peer's initiating handshake as Bob.
//
// Caller should hold connection lock.
func (c *baseConnection) authRespond(msg *msgAuthSpake2Handshake) error {
	authState := c.authenticationState
	a, b := c.spake2Identities()
	authState.spakeState = newSpake2(false, authState.psk, a, b)
	authState.alice = false
	pB, err := authState.spakeState.Exchange(msg.PublicValue)
	if err != nil {
		return c.authFailed(AuthStatusResultProofInvalid, err)
	}
	// network.bs: the consumer answers psk-shown with psk-input and pB; "The
	// PSK presenter, on receiving this message [psk-needs-presentation],
	// creates a PSK and presents it to the the user. Once that is done, it
	// sends an auth-spake2-handshake message to the PSK consumer with
	// psk-status set to psk-input and the public-value field set to pB."
	c.log.Debug("SPAKE2: responding as Bob", "role", c.authenticationRole, "peer-psk-status", msg.PskStatus)
	return c.sendAuthSpake2Handshake(AuthSpake2PskStatusPskInput, pB)
}

// authenticatePSKProgress advances the authentication state machine as far as
// the available inputs allow. A returned error is fatal for the connection.
//
// Caller should hold connection lock
func (c *baseConnection) authenticatePSKProgress() error {
	authState := c.authState()
	if authState.failed || authState.finished {
		return nil
	}

	// Start SPAKE2 once the PSK is known.
	if authState.spakeState == nil {
		if authState.psk == nil {
			return nil // Waiting for AuthenticatePSK.
		}
		var err error
		if hs := authState.remoteHandshake; hs != nil {
			err = c.authRespond(hs)
		} else {
			err = c.authInitiate()
		}
		if err != nil || authState.failed {
			return err
		}
	}

	// network.bs: "Once an agent knows both pA and pB from
	// auth-spake2-handshake messages, it computes and sends a
	// auth-spake2-confirmation". Alice sends cA once she has pB. Bob sends cB
	// once he has verified cA, as spake2-go only derives cB then; Alice
	// doesn't wait for cB, so the order doesn't matter.
	if authState.alice && authState.remotePublic != nil && !authState.sentConfirmation {
		cA, err := authState.spakeState.Finish(authState.remotePublic)
		if err != nil {
			return c.authFailed(spakeFailureResult(err), err)
		}
		if err := c.sendAuthSpake2Confirmation(cA); err != nil {
			return err
		}
		authState.sentConfirmation = true
	}

	// network.bs: "Once an agent receives an auth-spake2-confirmation
	// message, it validates that message using the procedure in [SPAKE2] and
	// then replies with an auth-status authenticated message".
	if authState.remoteConfirmation != nil && !authState.verified {
		if authState.alice {
			if !authState.sentConfirmation {
				return nil // Waiting for pB.
			}
			if err := authState.spakeState.Verify(authState.remoteConfirmation); err != nil {
				return c.authFailed(spakeFailureResult(err), err)
			}
		} else {
			cB, err := authState.spakeState.Confirm(authState.remoteConfirmation)
			if err != nil {
				return c.authFailed(spakeFailureResult(err), err)
			}
			if err := c.sendAuthSpake2Confirmation(cB); err != nil {
				return err
			}
			authState.sentConfirmation = true
		}
		authState.verified = true

		status := AuthStatusResultAuthenticated
		authState.localResult = &status
		if err := c.sendAuthStatus(); err != nil {
			return err
		}
	}

	// Hand off once both agents report success, so the peer is ready for
	// application messages.
	if authState.verified && authState.remoteResult != nil {
		authState.finished = true
		close(authState.done)
	}
	return nil
}

// Caller should hold connection lock
func (c *baseConnection) sendAuthSpake2Handshake(status msgAuthSpake2PskStatus, publicValue []byte) error {
	msg := &msgAuthSpake2Handshake{
		PublicValue: publicValue,
		PskStatus:   status,
	}
	if at := c.nextAuthInitiationToken(); at != "" {
		msg.AuthInitiationToken.Token = &at
	}

	return c.send(msg)
}

// Caller should hold connection lock
func (c *baseConnection) sendAuthSpake2Confirmation(payload []byte) error {
	return c.send(&msgAuthSpake2Confirmation{
		ConfirmationValue: payload,
	})
}

// Caller should hold connection lock
func (c *baseConnection) sendAuthStatus() error {
	return c.send(&msgAuthStatus{
		Result: *c.authenticationState.localResult,
	})
}

func (c *baseConnection) handleAgentInfoRequest(msg *msgAgentInfoRequest) error {
	info := c.localAgent.AgentInfoMessage()
	return c.send(&msgAgentInfoResponse{
		msgResponse: msgResponse{
			RequestId: msg.RequestId,
		},
		AgentInfo: msgAgentInfo(info),
	})
}

func (c *baseConnection) handleAgentInfoResponse(msg *msgAgentInfoResponse) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.exchangeInfoState == nil {
		c.log.Debug("ignoring unsolicited agent-info-response")
		return nil
	}

	if !c.exchangeInfoState.requestIds[uint64(msg.RequestId)] {
		c.log.Debug("ignoring agent-info-response with wrong request ID", "id", msg.RequestId)
		return nil
	}

	c.remoteAgent.setInfo(AgentInfo{
		DisplayName:  msg.AgentInfo.DisplayName,
		ModelName:    msg.AgentInfo.ModelName,
		Capabilities: msg.AgentInfo.Capabilities,
		StateToken:   msg.AgentInfo.StateToken,
		Locales:      msg.AgentInfo.Locales,
	})

	c.checkAgentInfoComplete()

	return nil
}

func (c *baseConnection) handleAuthCapabilities(msg *msgAuthCapabilities) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.remoteAgent.setAuthenticationInfo(AgentAuthenticationInfo{
		PSKConfig: PSKConfig{
			EaseOfInput: int(msg.PskEaseOfInput),
			Entropy:     int(msg.PskMinBitsOfEntropy),
		},
	})

	c.checkAgentInfoComplete()

	return nil
}

func (c *baseConnection) handleAuthSpake2Handshake(msg *msgAuthSpake2Handshake) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if token := msg.AuthInitiationToken.Token; token != nil && !c.validAuthInitiationToken(*token) {
		c.log.Warn("discarding auth-spake2-handshake with invalid auth-initiation-token")
		return nil
	}

	authState := c.authState()
	if authState.failed || authState.finished {
		return nil
	}

	switch {
	case authState.spakeState == nil:
		// The peer initiated. Answer once the PSK is known.
		if authState.remoteHandshake != nil {
			c.log.Debug("ignoring repeated initiating auth-spake2-handshake")
			return nil
		}
		authState.remoteHandshake = msg

	case authState.alice && authState.remotePublic == nil:
		if msg.PskStatus == AuthSpake2PskStatusPskNeedsPresentation {
			// Both agents initiated, network.bs doesn't say how to resolve
			// this. The presenter's initiation wins: as presenter, ignore the
			// consumer's. If both claim to be the consumer the roles disagree
			// and the peer's psk-input reply will never come; ignore it too.
			c.log.Debug("both agents initiated SPAKE2, ignoring the peer's psk-needs-presentation", "role", c.authenticationRole)
			return nil
		}
		if msg.PskStatus == AuthSpake2PskStatusPskShown && c.authenticationRole == AuthenticationRoleConsumer {
			// Both agents initiated and the peer is the presenter: abandon
			// the local initiation and answer as Bob.
			c.log.Debug("both agents initiated SPAKE2, answering the presenter's")
			authState.spakeState = nil
			authState.alice = false
			authState.remoteHandshake = msg
			break
		}
		authState.remotePublic = msg.PublicValue

	default:
		c.log.Debug("ignoring unexpected auth-spake2-handshake", "psk-status", msg.PskStatus)
		return nil
	}

	return c.authenticatePSKProgress()
}

func (c *baseConnection) handleAuthSpake2Confirmation(msg *msgAuthSpake2Confirmation) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	authState := c.authenticationState
	if authState == nil {
		return errors.New("unsolicited auth-spake2-confirmation")
	}
	if authState.remoteConfirmation != nil {
		c.log.Debug("ignoring repeated auth-spake2-confirmation")
		return nil
	}

	// The CDDL declares confirmation-value as "bytes .size 64", but the
	// network.bs suite's MAC is HMAC-SHA-256, whose output (cA, cB in
	// RFC 9382) is 32 bytes. That is a spec inconsistency: like Chromium's
	// openscreen, send and accept the 32-byte MAC, and don't check the
	// declared size; SPAKE2 verification rejects wrong values.
	authState.remoteConfirmation = msg.ConfirmationValue

	return c.authenticatePSKProgress()
}

func (c *baseConnection) handleAuthStatus(msg *msgAuthStatus) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// The auth-status message is merely informative as each agent
	// independently computes the outcome of SPAKE2 through key confirmation
	// verification. Any value of result other than authenticated means that
	// authentication failed, and the agent must immediately disconnect.
	// Returning an error makes the read loop close the connection.
	res := msg.Result
	if res != AuthStatusResultAuthenticated {
		return fmt.Errorf("%w: remote result %v", ErrAuthenticationFailed, res)
	}

	authState := c.authenticationState
	if authState == nil {
		return errors.New("unsolicited auth-status")
	}
	authState.remoteResult = &res

	return c.authenticatePSKProgress()
}

func (c *baseConnection) handleAgentStatusRequest(msg *msgAgentStatusRequest) error {
	return c.send(&msgAgentStatusResponse{
		msgResponse: msgResponse{
			RequestId: msg.RequestId,
		},
	})
}

// runNetwork starts the network protocol read loop. It runs until the
// connection is handed off to the application protocol or closed. Read and
// handler errors close the connection.
func (c *baseConnection) runNetwork() bool {
	return c.goroutine(func() {
		for {
			msg, err := c.netConn.ReadMessage()
			if errors.Is(err, ErrTransportHandedOff) {
				return
			} else if err != nil {
				c.shutdown(fmt.Errorf("network protocol: read: %w", err))
				return
			}
			c.log.Debug("receive", "msg", msgName(msg))

			err = c.handleNetworkMessage(msg)
			if err != nil {
				c.shutdown(fmt.Errorf("network protocol: %w", err))
				return
			}
		}
	})
}

func (c *baseConnection) handleNetworkMessage(msg interface{}) error {
	switch typedMsg := msg.(type) {
	case *msgAgentInfoRequest:
		return c.handleAgentInfoRequest(typedMsg)

	case *msgAgentInfoResponse:
		return c.handleAgentInfoResponse(typedMsg)

	case *msgAuthCapabilities:
		return c.handleAuthCapabilities(typedMsg)

	case *msgAuthSpake2Handshake:
		return c.handleAuthSpake2Handshake(typedMsg)

	case *msgAuthSpake2Confirmation:
		return c.handleAuthSpake2Confirmation(typedMsg)

	case *msgAuthStatus:
		return c.handleAuthStatus(typedMsg)

	case *msgAgentStatusRequest:
		// Only reached on transports that don't answer keep-alives themselves.
		return c.handleAgentStatusRequest(typedMsg)

	case *msgAgentStatusResponse:
		// Keep-alive response, nothing to do.
		return nil

	default:
		c.log.Debug("unhandled network message", "msg", msgName(typedMsg))
		return nil
	}
}
