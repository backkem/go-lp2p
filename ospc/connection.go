package ospc

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"sync"

	spake2 "github.com/backkem/go-lp2p/spake2"
	quic "github.com/quic-go/quic-go"
)

var ErrConnectionClosed = errors.New("connection closed")
var ErrHandedOff = errors.New("connection handed off")

// Connection
type Connection struct {
	base *baseConnection
}

func (c *Connection) LocalAgent() *Agent {
	return c.base.localAgent
}

func (c *Connection) RemoteAgent() *Agent {
	return c.base.RemoteAgent()
}

// Handoff the underlying quick connection for use by another protocol.
// func (c *Connection) Handoff() (quic.Connection, error) {
// 	return c.base.Handoff()
// }

// Close the connection and all associated steams.
func (c *Connection) Close() error {
	return c.base.Close()
}

type AgentState struct {
	StateToken string // 8 characters in the range [0-9A-Za-z]
	RequestId  uint64
}

func newAgentState() AgentState {
	s := AgentState{
		RequestId: 1,
	}
	s.newStateToken()

	return s
}

func (s *AgentState) newStateToken() {
	s.StateToken = randomAlphaNum(8)
}

func (s *AgentState) nextRequestID() uint64 {
	id := s.RequestId
	s.RequestId++
	return id
}

func randomAlphaNum(length int) string {
	const chars = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	return randomCharset(chars, length)
}

func randomAT(length int) string {
	const chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	return randomCharset(chars, length)
}

func randomCharset(chars string, length int) string {
	var result string
	max := big.NewInt(int64(len(chars)))

	for i := 0; i < length; i++ {
		randomIndex, _ := rand.Int(rand.Reader, max)
		result += string(chars[randomIndex.Int64()])
	}

	return result
}

// baseConnection is the actual connection handler implementation.
type baseConnection struct {
	conn quic.Connection

	mu sync.Mutex

	agentRole  AgentRole
	agentState AgentState

	localAgent  *Agent
	remoteAgent *Agent

	exchangeInfoState  *exchangeInfoState
	authenticationRole AuthenticationRole

	authNotify          chan struct{}
	authenticationState *authenticationState

	connectedState *connectedState

	acceptCancel context.CancelFunc
	close        chan struct{}
	closeErr     error
	done         chan struct{}
}

func newBaseConnection(conn quic.Connection, localAgent *Agent, remoteAgent *Agent, role AgentRole) *baseConnection {
	bConn := &baseConnection{
		mu:          sync.Mutex{},
		agentRole:   role,
		agentState:  newAgentState(), // TODO: reconnect
		localAgent:  localAgent,
		remoteAgent: remoteAgent,
		conn:        conn,
		authNotify:  make(chan struct{}),
		close:       make(chan struct{}),
		done:        make(chan struct{}),
	}

	return bConn
}

func (c *baseConnection) RemoteAgent() *Agent {
	return c.remoteAgent
}

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

	stream, err := c.openStream(ctx)
	if err != nil {
		return err
	}

	// Auth Info
	localAuthInfo := c.localAgent.AuthenticationInfo()
	authMsg := &msgAuthCapabilities{
		PskEaseOfInput:      uint64(localAuthInfo.PSKConfig.EaseOfInput),
		PskInputMethods:     []msgPskInputMethod{PskInputMethodNumeric},
		PskMinBitsOfEntropy: uint64(localAuthInfo.PSKConfig.Entropy),
	}

	err = writeMessage(authMsg, stream)
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

	err = writeMessage(infoMsg, stream)
	if err != nil {
		return err
	}

	c.exchangeInfoState = state

	return nil
}

func (c *baseConnection) openStream(ctx context.Context) (quic.Stream, error) {
	stream, err := c.conn.OpenStreamSync(ctx)
	if err != nil {
		return nil, err
	}

	bStream := newBaseStream(stream, c.handleMessage)

	c.handleStream(bStream)

	return stream, nil
}

func (c *baseConnection) handleAgentInfoRequest(msg *msgAgentInfoRequest, stream quic.Stream) error {
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
	err := writeMessage(infoMsg, stream)
	if err != nil {
		return err
	}

	return nil
}

func (c *baseConnection) handleAgentInfoResponse(msg *msgAgentInfoResponse, stream quic.Stream) error {
	_ = stream
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

func (c *baseConnection) handleAuthCapabilities(msg *msgAuthCapabilities, stream quic.Stream) error {
	_ = stream
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
	stream quic.Stream

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
func (c *baseConnection) newAuthenticationState(stream quic.Stream) (*authenticationState, error) {
	if stream == nil {
		var err error
		stream, err = c.openStream(context.Background())
		if err != nil {
			return nil, err
		}
	}

	c.authenticationState = &authenticationState{
		stream: stream,
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

	// c.authenticationState = nil

	c.connectedState = &connectedState{
		acceptDataChannel:     make(chan *DataChannel),
		acceptTransport:       make(chan *Transport),
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
		authState, err = c.newAuthenticationState(nil)
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
		authState, err = c.newAuthenticationState(nil)
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
	authState := c.authenticationState

	at, err := c.getAuthInitiationToken()
	if err != nil {
		return err
	}

	msg := &msgAuthSpake2NeedPskDeprecated{
		AuthInitiationToken: at,
	}

	err = writeMessage(msg, authState.stream)
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

	err = writeMessage(msg, authState.stream)
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

	err := writeMessage(msg, authState.stream)
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

	err := writeMessage(msg, authState.stream)
	if err != nil {
		return err
	}

	return nil
}

func (c *baseConnection) handleAuthSpake2NeedPsk(msg *msgAuthSpake2NeedPskDeprecated, stream quic.Stream) error {
	_ = msg
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.authenticationRole == AuthenticationRoleConsumer ||
		c.authenticationState != nil {
		fmt.Println("ignoring spake2-need-psk")
		return nil
	}

	_, err := c.newAuthenticationState(stream)
	if err != nil {
		return err
	}

	return c.authenticatePSKProgress()
}

func (c *baseConnection) handleAuthSpake2Handshake(msg *msgAuthSpake2HandshakeDeprecated, stream quic.Stream) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	err := c.validateAuthInitiationToken(msg.AuthInitiationToken)
	if err != nil {
		return err
	}

	authState := c.authenticationState
	if authState == nil {
		authState, err = c.newAuthenticationState(stream)
		if err != nil {
			return err
		}
	}

	authState.remotePublic = msg.Payload

	return c.authenticatePSKProgress()
}

func (c *baseConnection) handleAuthSpake2Confirmation(msg *msgAuthSpake2ConfirmationDeprecated, stream quic.Stream) error {
	_ = stream
	c.mu.Lock()
	defer c.mu.Unlock()

	authState := c.authenticationState
	if authState == nil {
		return errors.New("unsolicited auth-spake2-confirmation")
	}

	authState.remoteConfirmation = msg.Payload

	return c.authenticatePSKProgress()
}

func (c *baseConnection) handleAuthStatus(msg *msgAuthStatus, stream quic.Stream) error {
	_ = stream
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

func (c *baseConnection) run() {
	c.mu.Lock()
	defer c.mu.Unlock()

	acceptCtx, acceptCancel := context.WithCancel(context.Background())
	c.acceptCancel = acceptCancel

	// Steam accept loop
	go func() {
		for {
			s, err := c.conn.AcceptStream(acceptCtx)
			if err != nil {
				fmt.Printf("AcceptStream error: %s\n", err)
				c.closeWithError(fmt.Errorf("acceptStream error: %v", err))
				return
			}

			bStream := newBaseStream(s, c.handleMessage)
			c.handleStream(bStream)
		}
	}()
}

func (c *baseConnection) handleStream(stream *baseStream) {
	go func() {
		for {
			handler := stream.Handler()
			if handler == nil {
				return
			}

			msg, err := readMessage(stream.stream)
			if err == quic.ErrServerClosed {
				return
			} else if err != nil {
				fmt.Printf("failed to read message: %v\n", err)
				// c.closeWithError(fmt.Errorf("failed to read message: %v", err))
				return
			}

			err = handler(msg, stream)
			if err != nil {
				fmt.Printf("failed to handle message: %v\n", err)
				c.closeWithError(fmt.Errorf("failed to handle message: %v", err))
				return
			}
		}
	}()
}

func (c *baseConnection) handleMessage(msg interface{}, stream *baseStream) (err error) {
	switch typedMsg := msg.(type) {
	case *msgAgentInfoRequest:
		err = c.handleAgentInfoRequest(typedMsg, stream.stream)

	case *msgAgentInfoResponse:
		err = c.handleAgentInfoResponse(typedMsg, stream.stream)

	case *msgAuthCapabilities:
		err = c.handleAuthCapabilities(typedMsg, stream.stream)

	case *msgAuthSpake2NeedPskDeprecated:
		err = c.handleAuthSpake2NeedPsk(typedMsg, stream.stream)

	case *msgAuthSpake2HandshakeDeprecated:
		err = c.handleAuthSpake2Handshake(typedMsg, stream.stream)

	case *msgAuthSpake2ConfirmationDeprecated:
		err = c.handleAuthSpake2Confirmation(typedMsg, stream.stream)

	case *msgAuthStatus:
		err = c.handleAuthStatus(typedMsg, stream.stream)

	case *msgDataChannelOpenRequest:
		err = c.handleDataChannelOpenRequest(typedMsg, stream)

	case *msgDataTransportStartRequest:
		err = c.handleDataTransportStartRequest(typedMsg, struct{}{})

	case *msgDataTransportStreamRequest:
		err = c.handleDataTransportStreamRequest(typedMsg, stream)

	default:
		fmt.Printf("baseConnection: unhandled message type: %T\n", typedMsg)
	}

	if err != nil {
		return err
	}
	return nil
}

type connectedState struct {
	acceptDataChannel     chan *DataChannel
	acceptTransport       chan *Transport
	acceptTransportStream chan *baseStream
}

func (c *baseConnection) handleDataChannelOpenRequest(msg *msgDataChannelOpenRequest, stream *baseStream) error {
	dc := &DataChannel{
		DataChannelParameters: DataChannelParameters{
			Label:    msg.Label,
			ID:       uint64(msg.ChannelId),
			Protocol: msg.Protocol,
		},
		stream: stream.stream,
	}
	stream.SetHandler(nil)

	// TODO: send data-exchange-start-response

	c.mu.Lock()
	close := c.close
	accept := c.connectedState.acceptDataChannel
	c.mu.Unlock()

	select {
	case <-close:
		return c.err()
	case accept <- dc:
		return nil
	}
}

func (c *baseConnection) handleDataTransportStartRequest(msg *msgDataTransportStartRequest, info struct{}) error {
	// TODO: msg validation
	_, _ = msg, info
	c.mu.Lock()
	t, err := c.createDataTransport()
	if err != nil {
		c.mu.Unlock()
		return err
	}
	c.mu.Unlock()

	// TODO: send data-transport-start-response

	c.mu.Lock()
	close := c.close
	accept := c.connectedState.acceptTransport
	c.mu.Unlock()

	select {
	case <-close:
		return c.err()
	case accept <- t:
		return nil
	}
}

// Caller should hold the connection lock
func (c *baseConnection) createDataTransport() (*Transport, error) {
	t := &Transport{
		conn: c,
	}
	return t, nil
}

func (c *baseConnection) handleDataTransportStreamRequest(msg *msgDataTransportStreamRequest, stream *baseStream) error {
	// TODO: msg validation
	_ = msg
	// Stop message handling for this stream
	stream.SetHandler(nil)

	// TODO: send data-transport-stream-response

	c.mu.Lock()
	close := c.close
	accept := c.connectedState.acceptTransportStream
	c.mu.Unlock()

	if accept == nil {
		fmt.Printf("handleDataTransportStreamRequest: no transport, ignoring stream request\n")
		return nil // No-one is listening
	}

	select {
	case <-close:
		return c.err()
	case accept <- stream:
		return nil
	}
}

// Handoff the underlying quick connection for use by another protocol.
// func (c *baseConnection) Handoff() (quic.Connection, error) {
// 	c.mu.Lock()
// 	c.acceptCancel() // Stop stream handling loop
// 	done := c.done
// 	c.closeErr = ErrHandedOff
// 	c.mu.Unlock()
//
// 	<-done
//
// 	return c.conn, nil
// }

// Close the connection and all associated steams.
func (c *baseConnection) Close() error {
	return c.closeWithError(ErrConnectionClosed)
}

func (c *baseConnection) closeWithError(err error) error {
	c.mu.Lock()
	if c.closeErr != nil {
		c.mu.Unlock()
		return c.closeErr
	}

	c.closeErr = err
	c.conn.CloseWithError(1, "Closed")

	if c.exchangeInfoState != nil {
		result := exchangeInfoResult{
			conn: c,
			err:  err,
		}
		c.exchangeInfoState.done <- result
	}

	done := c.done
	close(done)
	c.mu.Unlock()

	// Block till runLoop is gone
	<-done
	return nil
}

func (c *baseConnection) err() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.closeErr
}
