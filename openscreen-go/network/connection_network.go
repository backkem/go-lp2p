package ospc

import (
	"context"
	"crypto/rand"
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

// GeneratePSK creates a PSK based on the negotiated config.
func (c *baseConnection) GeneratePSK() ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	localAuthInfo := c.localAgent.AuthenticationInfo()
	remoteAuthInfo := c.remoteAgent.AuthenticationInfo()
	if remoteAuthInfo == nil {
		remoteAuthInfo = localAuthInfo
	}
	minBits := maxInt(
		localAuthInfo.PSKConfig.Entropy,
		remoteAuthInfo.PSKConfig.Entropy,
	)

	// We round up to full byte
	buf := make([]byte, (minBits+7)/8)

	_, err := rand.Read(buf)
	if err != nil {
		return nil, err
	}

	return buf, nil
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

type authenticationStatus int

const (
	authStatusNew = iota + 1
	authStatusRequested
	authStatusAwaitPSK
	authStatusAwaitHandshake
	authStatusAwaitConfirmation
	authStatusAwaitResult
	authStatusDone
)

func (s authenticationStatus) String() string {
	switch s {
	case authStatusNew:
		return "New"
	case authStatusRequested:
		return "Requested"
	case authStatusAwaitPSK:
		return "AwaitPSK"
	case authStatusAwaitHandshake:
		return "AwaitHandshake"
	case authStatusAwaitConfirmation:
		return "AwaitConfirmation"
	case authStatusAwaitResult:
		return "AwaitResult"
	case authStatusDone:
		return "Done"
	default:
		return fmt.Sprintf("Invalid authStatus (%d)", s)
	}
}

type authenticationState struct {
	status authenticationStatus

	localPSK           []byte
	spakeState         *spake2.SPAKE2
	remotePublic       []byte
	sharedSecret       []byte
	remoteConfirmation []byte
	localResult        *msgAuthStatusResult
	remoteResult       *msgAuthStatusResult

	done chan struct{}
}

// Caller should hold connection lock
func (c *baseConnection) newAuthenticationState() *authenticationState {
	c.authenticationState = &authenticationState{
		status: authStatusNew,
		done:   make(chan struct{}),
	}

	return c.authenticationState
}

// AcceptAuthenticate is used to handle an incoming authentication request.
// It has to be called for every UnauthenticatedConnection.
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

// RequestAuthenticatePSK is used to request authentication.
// As collecting agent it sends a auth-spake2-need-psk message.
// As presenting agent it's a no-op.
func (c *baseConnection) RequestAuthenticatePSK() error {
	c.mu.Lock()
	if c.authenticationRole == AuthenticationRolePresenter {
		// No-op as presenter
		c.mu.Unlock()
		return nil
	}
	if c.authenticationState != nil {
		c.mu.Unlock()
		return errors.New("already authenticating")
	}
	err := c.authenticatePSKProgress()
	c.mu.Unlock()

	return c.failOnError(err)
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

// Authenticate is used to authenticate. It will block until authentication is complete
// or the context is closed.
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
	c.mu.Lock()
	authenticating := c.authenticationState != nil
	c.mu.Unlock()
	if authenticating {
		return nil, errors.New("authentication already in progress")
	}
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
	if psk == nil {
		return nil, errors.New("no psk provided")
	}

	c.mu.Lock()
	authState := c.authenticationState
	if authState == nil {
		authState = c.newAuthenticationState()
	}
	if authState.localPSK != nil {
		c.mu.Unlock()
		return nil, errors.New("already authenticating")
	}
	authState.localPSK = psk
	err := c.authenticatePSKProgress()
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
	c.authenticationState.localResult = &result
	if sendErr := c.sendAuthStatus(); sendErr != nil {
		c.log.Debug("failed to send auth-status", "err", sendErr)
	}
	return fmt.Errorf("%w: %v", ErrAuthenticationFailed, err)
}

// spakeFailureResult maps a SPAKE2 error to an auth-status result.
func spakeFailureResult(err error) msgAuthStatusResult {
	if errors.Is(err, spake2.ErrInvalidConfirmation) {
		return AuthStatusResultProofInvalid
	}
	return AuthStatusResultUnknownError
}

// authenticatePSKProgress advances the authentication state machine as far as
// the available inputs allow. A returned error is fatal for the connection.
//
// Caller should hold connection lock
func (c *baseConnection) authenticatePSKProgress() error {
	authState := c.authenticationState
	if authState == nil {
		authState = c.newAuthenticationState()
	}

	role := c.authenticationRole
	c.log.Debug("auth progress", "role", role, "status", authState.status)

	if authState.status == authStatusNew {
		if role == AuthenticationRolePresenter {
			if authState.localPSK == nil {
				c.doAuthNotify()
			}
		} else {
			if authState.remotePublic == nil {
				err := c.sendAuthSpake2NeedPsk()
				if err != nil {
					return err
				}
			} else if authState.localPSK == nil {
				c.doAuthNotify()
			}
		}

		authState.status = authStatusAwaitPSK
	}

	if authState.status == authStatusAwaitPSK {
		if authState.localPSK == nil {
			return nil // continue waiting
		}
		if role == AuthenticationRolePresenter {
			client := spake2.NewClient(authState.localPSK, &spake2.Options{
				Ciphersuite: spake2.DefaultCiphersuite(),
				IdentityA:   []byte(c.localAgent.PeerID),
				IdentityB:   []byte(c.remoteAgent.PeerID),
			})
			authState.spakeState = client
			localPublic, err := client.Start()
			if err != nil {
				return c.authFailed(AuthStatusResultUnknownError, err)
			}

			err = c.sendAuthSpake2Handshake(localPublic)
			if err != nil {
				return err
			}
		}

		authState.status = authStatusAwaitHandshake
	}

	if authState.status == authStatusAwaitHandshake {
		if authState.remotePublic == nil {
			return nil // continue waiting
		}
		if role == AuthenticationRolePresenter {
			localConfirmation, err := authState.spakeState.Finish(authState.remotePublic)
			if err != nil {
				return c.authFailed(spakeFailureResult(err), err)
			}

			err = c.sendAuthSpake2Confirmation(localConfirmation)
			if err != nil {
				return err
			}
		} else {
			server := spake2.NewServer(authState.localPSK, &spake2.Options{
				Ciphersuite: spake2.DefaultCiphersuite(),
				IdentityA:   []byte(c.remoteAgent.PeerID),
				IdentityB:   []byte(c.localAgent.PeerID),
			})
			authState.spakeState = server

			localPublic, err := server.Exchange(authState.remotePublic)
			if err != nil {
				return c.authFailed(spakeFailureResult(err), err)
			}

			err = c.sendAuthSpake2Handshake(localPublic)
			if err != nil {
				return err
			}
		}

		authState.status = authStatusAwaitConfirmation
	}

	if authState.status == authStatusAwaitConfirmation {
		if authState.remoteConfirmation == nil {
			return nil // continue waiting
		}

		if role == AuthenticationRolePresenter {
			err := authState.spakeState.Verify(authState.remoteConfirmation)
			if err != nil {
				return c.authFailed(spakeFailureResult(err), err)
			}
		} else {
			localConfirmation, err := authState.spakeState.Confirm(authState.remoteConfirmation)
			if err != nil {
				return c.authFailed(spakeFailureResult(err), err)
			}

			err = c.sendAuthSpake2Confirmation(localConfirmation)
			if err != nil {
				return err
			}
		}

		var err error
		authState.sharedSecret, err = authState.spakeState.SharedKey()
		if err != nil {
			return c.authFailed(AuthStatusResultUnknownError, err)
		}

		status := AuthStatusResultAuthenticated
		authState.localResult = &status
		err = c.sendAuthStatus()
		if err != nil {
			return err
		}

		authState.status = authStatusAwaitResult
	}

	if authState.status == authStatusAwaitResult {
		if authState.remoteResult == nil {
			return nil // continue waiting
		}

		close(authState.done)

		authState.status = authStatusDone
	}

	if authState.status == authStatusDone {
		return nil
	}

	return errors.New("invalid authentication status")
}

// Caller should hold connection lock
func (c *baseConnection) sendAuthSpake2NeedPsk() error {
	return c.send(&msgAuthSpake2NeedPskDeprecated{
		AuthInitiationToken: c.nextAuthInitiationToken(),
	})
}

// Caller should hold connection lock
func (c *baseConnection) sendAuthSpake2Handshake(publicValue []byte) error {
	pskStatus := AuthSpake2PskStatusPskInput
	if c.authenticationRole == AuthenticationRolePresenter {
		pskStatus = AuthSpake2PskStatusPskShown
	}

	msg := &msgAuthSpake2Handshake{
		PublicValue: publicValue,
		PskStatus:   pskStatus,
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
	localInfo := c.localAgent.Info()
	return c.send(&msgAgentInfoResponse{
		msgResponse: msgResponse{
			RequestId: msg.RequestId,
		},
		AgentInfo: msgAgentInfo{
			DisplayName: localInfo.DisplayName,
			ModelName:   localInfo.ModelName,
			Capabilities: []msgAgentCapability{
				AgentCapabilityDataChannels,
				AgentCapabilityQuickTransport,
			},
			StateToken: c.localAgent.StateToken(),
			Locales:    localInfo.Locales,
		},
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
		DisplayName: msg.AgentInfo.DisplayName,
		ModelName:   msg.AgentInfo.ModelName,
		StateToken:  msg.AgentInfo.StateToken,
		Locales:     msg.AgentInfo.Locales,
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

func (c *baseConnection) handleAuthSpake2NeedPsk(msg *msgAuthSpake2NeedPskDeprecated) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.validAuthInitiationToken(msg.AuthInitiationToken) {
		c.log.Warn("discarding auth-spake2-need-psk with invalid auth-initiation-token")
		return nil
	}

	if c.authenticationRole == AuthenticationRoleConsumer ||
		c.authenticationState != nil {
		c.log.Debug("ignoring auth-spake2-need-psk")
		return nil
	}

	c.newAuthenticationState()

	return c.authenticatePSKProgress()
}

func (c *baseConnection) handleAuthSpake2Handshake(msg *msgAuthSpake2Handshake) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if token := msg.AuthInitiationToken.Token; token != nil && !c.validAuthInitiationToken(*token) {
		c.log.Warn("discarding auth-spake2-handshake with invalid auth-initiation-token")
		return nil
	}

	authState := c.authenticationState
	if authState == nil {
		authState = c.newAuthenticationState()
	}

	authState.remotePublic = msg.PublicValue

	return c.authenticatePSKProgress()
}

func (c *baseConnection) handleAuthSpake2Confirmation(msg *msgAuthSpake2Confirmation) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	authState := c.authenticationState
	if authState == nil {
		return errors.New("unsolicited auth-spake2-confirmation")
	}

	authState.remoteConfirmation = msg.ConfirmationValue

	return c.authenticatePSKProgress()
}

func (c *baseConnection) handleAuthStatus(msg *msgAuthStatus) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	authState := c.authenticationState
	if authState == nil {
		return errors.New("unsolicited auth-status")
	}

	// The auth-status message is merely informative as each agent
	// independently computes the outcome of SPAKE2 through key confirmation
	// verification. Any value of result other than authenticated means that
	// authentication failed, and the agent must immediately disconnect.
	// Returning an error makes the read loop close the connection.
	res := msg.Result
	if res != AuthStatusResultAuthenticated {
		return fmt.Errorf("%w: remote result %v", ErrAuthenticationFailed, res)
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

	case *msgAuthSpake2NeedPskDeprecated:
		return c.handleAuthSpake2NeedPsk(typedMsg)

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
