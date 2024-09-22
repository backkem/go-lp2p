package ospc

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"

	spake2 "github.com/backkem/go-lp2p/spake2"
)

type exchangeInfoState struct {
	requestId uint64
	done      chan exchangeInfoResult
}

type exchangeInfoResult struct {
	conn *baseConnection
	err  error
}

func (c *baseConnection) exchangeInfo(ctx context.Context, done chan exchangeInfoResult) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.exchangeInfoState != nil {
		fmt.Println("Already requesting remote AgentInfo")
		return nil
	}

	// Auth Info
	localAuthInfo := c.localAgent.AuthenticationInfo()
	authMsg := &msgAuthCapabilities{
		PskEaseOfInput:      uint64(localAuthInfo.PSKConfig.EaseOfInput),
		PskInputMethods:     []msgPskInputMethod{PskInputMethodNumeric},
		PskMinBitsOfEntropy: uint64(localAuthInfo.PSKConfig.Entropy),
	}

	err := writeMessage(authMsg, c.netConn)
	if err != nil {
		return err
	}

	// Remote AgentInfo
	state := &exchangeInfoState{
		requestId: c.agentState.nextRequestID(),
		done:      done,
	}
	infoMsg := &msgAgentInfoRequest{
		msgRequest: msgRequest{
			RequestId: msgRequestId(state.requestId),
		},
	}

	err = writeMessage(infoMsg, c.netConn)
	if err != nil {
		return err
	}

	c.exchangeInfoState = state

	return nil
}

// caller should hold connection lock.
func (c *baseConnection) checkAgentInfoComplete() {
	if c.exchangeInfoState != nil &&
		c.remoteAgent.HasInfo() &&
		c.remoteAgent.HasAuthenticationInfo() {

		state := c.exchangeInfoState
		c.determineAuthenticationRole()

		result := exchangeInfoResult{
			conn: c,
		}
		select {
		case state.done <- result:
			c.exchangeInfoState = nil
		case <-c.close:
			return
		}
	}
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
		return "authStatus: New"
	case authStatusRequested:
		return "authStatus: Requested"
	case authStatusAwaitPSK:
		return "authStatus: AwaitPSK"
	case authStatusAwaitHandshake:
		return "authStatus: AwaitHandshake"
	case authStatusAwaitConfirmation:
		return "authStatus: AwaitConfirmation"
	case authStatusAwaitResult:
		return "authStatus: AwaitResult"
	case authStatusDone:
		return "authStatus: Done"
	default:
		return fmt.Sprintf("Invalid authStatus (%d)", s)
	}
}

type authenticationState struct {
	status authenticationStatus

	localPSK           []byte
	spakeState         *spake2.Context
	remotePublic       []byte
	sharedSecret       []byte
	remoteConfirmation []byte
	localResult        *msgAuthStatusResult
	remoteResult       *msgAuthStatusResult

	done chan struct{}
}

// Caller should hold connection lock
func (c *baseConnection) newAuthenticationState() (*authenticationState, error) {
	c.authenticationState = &authenticationState{
		status: authStatusNew,
		done:   make(chan struct{}),
	}

	return c.authenticationState, nil
}

// AcceptAuthenticate is used to handle an incoming authentication request.
// It has to be called for every UnauthenticatedConnection.
func (c *baseConnection) AcceptAuthenticate(ctx context.Context) (role AuthenticationRole, err error) {
	c.mu.Lock()
	close := c.close
	authNotify := c.authNotify
	c.mu.Unlock()

	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	case <-close:
		return 0, c.err()
	case <-authNotify:
		return c.GetAuthenticationRole(), nil
	}
}

// RequestAuthenticatePSK is used to request authentication.
// As collecting agent it sends a auth-spake2-need-psk message.
// As presenting agent it's a no-op.
func (c *baseConnection) RequestAuthenticatePSK() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.authenticationRole == AuthenticationRolePresenter {
		// No-op as presenter
		return nil
	}

	authState := c.authenticationState
	if authState != nil {
		return errors.New("already authenticating")
	}

	return c.authenticatePSKProgress()
}

// Caller should hold connection lock
func (c *baseConnection) doAuthNotify() {
	// close := c.close
	authNotify := c.authNotify

	close(authNotify)
}

// Caller should hold connection lock.
func (c *baseConnection) getAuthInitiationToken() (string, error) {
	// TODO: wire up auth-initiation-token
	// For an advertising agent, the at field in its mDNS TXT record must be used as the
	// auth-initiation-token in the the first authentication message sent to or from that agent.
	at := "todo"

	return at, nil
}

// Caller should hold connection lock.
func (c *baseConnection) validateAuthInitiationToken(token string) error {
	// TODO: wire up auth-initiation-token
	// Agents should discard any authentication message whose auth-initiation-token is set and
	// does not match the at provided by the advertising agent.
	if token == "todo" {
		return nil
	}

	return errors.New("invalid auth-initiation-token")
}

// Authenticate is used to authenticate. It will block until authentication is complete
// or the context is closed.
func (c *baseConnection) AuthenticatePSK(ctx context.Context, psk []byte) (*Connection, error) {
	err := c.authenticatePSK(psk)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	close := c.close
	done := c.authenticationState.done
	c.mu.Unlock()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-close:
		return nil, c.err()
	case <-done:
		return c.finishAuthentication()
	}
}

func (c *baseConnection) finishAuthentication() (*Connection, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.authenticationState.remoteResult == nil ||
		*c.authenticationState.remoteResult != AuthStatusResultAuthenticated {
		return nil, fmt.Errorf("authentication failed: %d", c.authenticationState.remoteResult)
	}

	appConn, err := c.netConn.IntoApplicationConnection()
	if err != nil {
		return nil, err
	}
	// TODO: avoid needless goroutine by solving double locking differently.
	go c.runApplication()

	c.connectedState = &connectedState{
		appConn:               appConn,
		acceptDataChannel:     make(chan *DataChannel),
		acceptTransport:       make(chan *PooledWebTransport),
		acceptTransportStream: make(chan *baseStream),
	}

	return &Connection{
		base: c,
	}, nil
}

func (c *baseConnection) authenticatePSK(psk []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if psk == nil {
		return errors.New("no psk provided")
	}

	authState := c.authenticationState
	if authState == nil {
		var err error
		authState, err = c.newAuthenticationState()
		if err != nil {
			return err
		}
	}

	if authState.localPSK != nil {
		return fmt.Errorf("already authenticating")
	}

	authState.localPSK = psk

	return c.authenticatePSKProgress()
}

// Caller should hold connection lock
func (c *baseConnection) authenticatePSKProgress() error {
	authState := c.authenticationState
	if authState == nil {
		var err error
		authState, err = c.newAuthenticationState()
		if err != nil {
			return err
		}
	}

	// fmt.Printf("%s %s\n", c.localInfo.Nickname, c.authenticationState.status)

	role := c.authenticationRole

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
			client, err := spake2.NewClient([]byte{}, []byte{})
			if err != nil {
				return err
			}
			authState.spakeState = client
		} else {
			server, err := spake2.NewServer([]byte{}, []byte{})
			if err != nil {
				return err
			}
			authState.spakeState = server
		}

		err := c.sendAuthSpake2Handshake()
		if err != nil {
			return err
		}

		authState.status = authStatusAwaitHandshake
	}

	if authState.status == authStatusAwaitHandshake {
		if authState.remotePublic == nil {
			return nil // continue waiting
		}
		secret := spake2.NewMsgBuffer()
		n, err := authState.spakeState.ProcessMsg(secret, authState.remotePublic)
		if err != nil {
			if err == spake2.ErrValidationFailed {
				status := AuthStatusResultProofInvalid
				authState.localResult = &status
			} else {
				status := AuthStatusResultUnknownError
				authState.localResult = &status
			}
			return err
		}

		status := AuthStatusResultAuthenticated
		authState.localResult = &status
		authState.sharedSecret = secret[:n]

		err = c.sendAuthSpake2Confirmation()
		if err != nil {
			return err
		}

		authState.status = authStatusAwaitConfirmation
	}

	if authState.status == authStatusAwaitConfirmation {
		if authState.remoteConfirmation == nil {
			return nil // continue waiting
		}

		// TODO: update to new flow.

		err := c.sendAuthStatus()
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
	at, err := c.getAuthInitiationToken()
	if err != nil {
		return err
	}

	msg := &msgAuthSpake2NeedPskDeprecated{
		AuthInitiationToken: at,
	}

	err = writeMessage(msg, c.netConn)
	if err != nil {
		return err
	}

	return nil
}

// Caller should hold connection lock
func (c *baseConnection) sendAuthSpake2Handshake() error {
	authState := c.authenticationState
	payload := spake2.NewMsgBuffer()
	n, err := authState.spakeState.GenerateMsg(payload, authState.localPSK)
	if err != nil {
		return err
	}

	at, err := c.getAuthInitiationToken()
	if err != nil {
		return err
	}

	msg := &msgAuthSpake2HandshakeDeprecated{
		AuthInitiationToken: at,
		Payload:             payload[:n],
	}

	err = writeMessage(msg, c.netConn)
	if err != nil {
		return err
	}

	return nil
}

// Caller should hold connection lock
func (c *baseConnection) sendAuthSpake2Confirmation() error {
	authState := c.authenticationState
	confirmation := authState.sharedSecret

	msg := &msgAuthSpake2ConfirmationDeprecated{
		Payload: confirmation,
	}

	err := writeMessage(msg, c.netConn)
	if err != nil {
		return err
	}

	return nil
}

// Caller should hold connection lock
func (c *baseConnection) sendAuthStatus() error {
	authState := c.authenticationState

	msg := &msgAuthStatus{
		Result: *authState.localResult,
	}

	err := writeMessage(msg, c.netConn)
	if err != nil {
		return err
	}

	return nil
}

func (c *baseConnection) handleAgentInfoRequest(msg *msgAgentInfoRequest) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	localInfo := c.localAgent.Info()
	infoMsg := &msgAgentInfoResponse{
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
			// StateToken: , // TODO: State token
			Locales: localInfo.Locales,
		},
	}
	err := writeMessage(infoMsg, c.netConn)
	if err != nil {
		return err
	}

	return nil
}

func (c *baseConnection) handleAgentInfoResponse(msg *msgAgentInfoResponse) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.exchangeInfoState == nil {
		fmt.Println("ignoring unsolicited AgentInfoResponse")
		return nil
	}

	if c.exchangeInfoState.requestId != uint64(msg.RequestId) {
		fmt.Println("ignoring AgentInfoResponse with wrong request ID")
		return nil
	}

	c.remoteAgent.setInfo(AgentInfo{
		DisplayName: msg.AgentInfo.DisplayName,
		ModelName:   msg.AgentInfo.ModelName,
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
	_ = msg
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.authenticationRole == AuthenticationRoleConsumer ||
		c.authenticationState != nil {
		fmt.Println("ignoring spake2-need-psk")
		return nil
	}

	_, err := c.newAuthenticationState()
	if err != nil {
		return err
	}

	return c.authenticatePSKProgress()
}

func (c *baseConnection) handleAuthSpake2Handshake(msg *msgAuthSpake2HandshakeDeprecated) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	err := c.validateAuthInitiationToken(msg.AuthInitiationToken)
	if err != nil {
		return err
	}

	authState := c.authenticationState
	if authState == nil {
		authState, err = c.newAuthenticationState()
		if err != nil {
			return err
		}
	}

	authState.remotePublic = msg.Payload

	return c.authenticatePSKProgress()
}

func (c *baseConnection) handleAuthSpake2Confirmation(msg *msgAuthSpake2ConfirmationDeprecated) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	authState := c.authenticationState
	if authState == nil {
		return errors.New("unsolicited auth-spake2-confirmation")
	}

	authState.remoteConfirmation = msg.Payload

	return c.authenticatePSKProgress()
}

func (c *baseConnection) handleAuthStatus(msg *msgAuthStatus) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	authState := c.authenticationState
	if authState == nil {
		return errors.New("unsolicited auth-status")
	}

	res := msg.Result
	authState.remoteResult = &res

	return c.authenticatePSKProgress()
}

func (c *baseConnection) runNetwork() {
	go func() {
		for {
			msg, err := readMessage(c.netConn)
			if err != nil {
				fmt.Printf("failed to read message: %v\n", err)
				// c.closeWithError(fmt.Errorf("failed to read message: %v", err))
				return
			}

			c.handleNetworkMessage(msg)
		}
	}()
}

func (c *baseConnection) handleNetworkMessage(msg interface{}) (err error) {
	switch typedMsg := msg.(type) {
	case *msgAgentInfoRequest:
		err = c.handleAgentInfoRequest(typedMsg)

	case *msgAgentInfoResponse:
		err = c.handleAgentInfoResponse(typedMsg)

	case *msgAuthCapabilities:
		err = c.handleAuthCapabilities(typedMsg)

	case *msgAuthSpake2NeedPskDeprecated:
		err = c.handleAuthSpake2NeedPsk(typedMsg)

	case *msgAuthSpake2HandshakeDeprecated:
		err = c.handleAuthSpake2Handshake(typedMsg)

	case *msgAuthSpake2ConfirmationDeprecated:
		err = c.handleAuthSpake2Confirmation(typedMsg)

	case *msgAuthStatus:
		err = c.handleAuthStatus(typedMsg)

	default:
		fmt.Printf("baseConnection: unhandled message type: %T\n", typedMsg)
	}

	if err != nil {
		return err
	}
	return nil
}
