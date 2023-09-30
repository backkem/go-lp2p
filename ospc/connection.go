package ospc

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"sync"

	quic "github.com/quic-go/quic-go"
)

var ErrConnectionClosed = errors.New("connection closed")
var ErrHandedOff = errors.New("connection handed off")

// Connection
type Connection struct {
	base *baseConnection
}

func (c *Connection) LocalConfig() AgentConfig {
	return c.base.localInfo
}

func (c *Connection) RemoteConfig() AgentConfig {
	return c.base.RemoteConfig()
}

// Handoff the underlying quick connection for use by another protocol.
func (c *Connection) Handoff() (quic.Connection, error) {
	return c.base.Handoff()
}

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

type AgentInfo struct {
	DisplayName string
	ModelName   string
	// Capabilities []agentCapability
	StateToken string
	Locales    []string
}

type AgentAuthenticationInfo struct {
	PSKConfig PSKConfig
}

// baseConnection is the actual connection handler implementation.
type baseConnection struct {
	conn quic.Connection

	mu sync.Mutex

	agentRole  AgentRole
	agentState AgentState

	localInfo                AgentConfig
	remoteInfo               *AgentInfo
	remoteAuthenticationInfo *AgentAuthenticationInfo

	exchangeInfoState *exchangeInfoState

	authNotify          chan struct{}
	authenticationState *authenticationState
	isAuthenticated     bool

	connectedState

	acceptCancel context.CancelFunc
	close        chan struct{}
	closeErr     error
	done         chan struct{}
}

func (c *baseConnection) RemoteConfig() AgentConfig {
	// Since the Connection is only emitted after
	// agent info exchange, this should not be nil.
	if c.remoteInfo == nil {
		panic("Connection without remote info")
	}
	if c.remoteAuthenticationInfo == nil {
		panic("Connection without remote auth info")
	}
	return AgentConfig{
		Nickname: c.remoteInfo.DisplayName,
		// Certificate: c.conn.ConnectionState().TLS.PeerCertificates,
		PSKConfig: c.remoteAuthenticationInfo.PSKConfig,
	}
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

	stream, err := c.conn.OpenStreamSync(ctx)
	if err != nil {
		return err
	}
	c.handleStream(stream)

	// Auth Info
	authMsg := &msgAuthCapabilities{
		PskEaseOfInput:      uint64(c.localInfo.PSKConfig.EaseOfInput),
		PskInputMethods:     []msgPskInputMethod{msgPskInputMethodNumeric},
		PskMinBitsOfEntropy: uint64(c.localInfo.PSKConfig.Entropy),
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
		RequestID: state.requestId,
	}

	err = writeMessage(infoMsg, stream)
	if err != nil {
		return err
	}

	c.exchangeInfoState = state

	return nil
}

func (c *baseConnection) handleAgentInfoRequest(msg *msgAgentInfoRequest, stream quic.Stream) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	infoMsg := &msgAgentInfoResponse{
		RequestID: msg.RequestID,
		AgentInfo: msgPartAgentInfo{
			DisplayName:  c.localInfo.Nickname,
			ModelName:    c.localInfo.ModelName,
			Capabilities: []agentCapability{agentCapabilityExchangeData},
			// StateToken: , // TODO: State token
			// Locales:  // TODO: Locales
		},
	}
	err := writeMessage(infoMsg, stream)
	if err != nil {
		return err
	}

	return nil
}

func (c *baseConnection) handleAgentInfoResponse(msg *msgAgentInfoResponse, stream quic.Stream) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	fmt.Println("handleAgentInfoResponse")

	if c.exchangeInfoState == nil {
		fmt.Println("ignoring unsolicited AgentInfoResponse")
		return nil
	}

	if c.exchangeInfoState.requestId != msg.RequestID {
		fmt.Println("ignoring AgentInfoResponse with wrong request ID")
		return nil
	}

	c.remoteInfo = &AgentInfo{
		DisplayName: msg.AgentInfo.DisplayName,
		ModelName:   msg.AgentInfo.ModelName,
	}

	c.checkAgentInfoComplete()

	return nil
}

func (c *baseConnection) handleAuthCapabilities(msg *msgAuthCapabilities, stream quic.Stream) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.remoteAuthenticationInfo = &AgentAuthenticationInfo{
		PSKConfig: PSKConfig{
			EaseOfInput: int(msg.PskEaseOfInput),
			// TOOD: PskInputMethods
			Entropy: int(msg.PskMinBitsOfEntropy),
		},
	}

	c.checkAgentInfoComplete()

	return nil
}

// caller should hold connection lock.
func (c *baseConnection) checkAgentInfoComplete() {
	fmt.Println("checkAgentInfoComplete",
		c.exchangeInfoState != nil,
		c.remoteInfo != nil,
		c.remoteAuthenticationInfo != nil,
	)

	if c.exchangeInfoState != nil &&
		c.remoteInfo != nil &&
		c.remoteAuthenticationInfo != nil {

		state := c.exchangeInfoState
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
func (c *baseConnection) GetAuthenticationRole() AuthenticationRole {
	// Since the Connection is only emitted after
	// agent info exchange, this should not be nil.
	if c.remoteAuthenticationInfo == nil {
		panic("missing remote peer authentication info")
	}

	if c.localInfo.PSKConfig.EaseOfInput == c.remoteAuthenticationInfo.PSKConfig.EaseOfInput {
		if c.agentRole == AgentRoleServer {
			return AuthenticationRolePresenter
		}
		return AuthenticationRoleConsumer
	}
	if c.localInfo.PSKConfig.EaseOfInput < c.remoteAuthenticationInfo.PSKConfig.EaseOfInput {
		return AuthenticationRolePresenter
	}
	return AuthenticationRoleConsumer
}

// GeneratePSK creates a PSK based on the negotiated config.
func (c *baseConnection) GeneratePSK() ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	minBits := maxInt(
		c.localInfo.PSKConfig.Entropy,
		c.remoteAuthenticationInfo.PSKConfig.Entropy,
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

type authenticationState struct {
	done chan struct{}
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

// RequestAuthenticatePSK is used to request authentication as an initiating
// collector agent.
func (c *baseConnection) RequestAuthenticatePSK() error {
	// TODO: send auth-spake2-need-psk

	return nil
}

// Authenticate is used to authenticate. It will block until authentication is complete
// or the context is closed.
func (c *baseConnection) AuthenticatePSK(ctx context.Context, psk []byte) (*Connection, error) {
	// TODO
	// https://github.com/niomon/spake2-go/blob/master/spake2_test.go

	return &Connection{
		base: c,
	}, nil
}

type authenticatedState struct {
	acceptCh chan *Connection
}

func (c *baseConnection) run() {
	c.mu.Lock()
	defer c.mu.Unlock()

	acceptCtx, acceptCancel := context.WithCancel(context.Background())
	c.acceptCancel = acceptCancel

	// Steam accept loop
	go func() {
		s, err := c.conn.AcceptStream(acceptCtx)
		if err != nil {
			fmt.Printf("AcceptStream error: %s\n", err)
			c.closeWithError(fmt.Errorf("acceptStream error: %v", err))
			return
		}

		c.handleStream(s)
	}()

	// Logic loop
	// Handle all streams
	// Message handlers
	// - Metadata FSM

	// go func() {
	// 	for {
	// 		select {
	// 		case <-closeCh: // Shutdown initiated
	// 			c.conn.CloseWithError(1, "Closed")
	//
	// 			close(doneCh)
	//
	// 		case s <- streams:
	//
	// 		}
	// 	}
	// }()
}

func (c *baseConnection) handleStream(stream quic.Stream) {
	go func() {
		for {
			msg, err := readMessage(stream)
			if err != nil {
				fmt.Printf("failed to read message: %v\n", err)
				// c.closeWithError(fmt.Errorf("failed to read message: %v", err))
				return
			}

			err = c.handleMessage(msg, stream)
			if err != nil {
				fmt.Printf("failed to handle message: %v\n", err)
				c.closeWithError(fmt.Errorf("failed to handle message: %v", err))
				return
			}
		}
	}()
}

func (c *baseConnection) handleMessage(msg interface{}, stream quic.Stream) (err error) {
	switch typedMsg := msg.(type) {
	case *msgAgentInfoRequest:
		err = c.handleAgentInfoRequest(typedMsg, stream)

	case *msgAgentInfoResponse:
		err = c.handleAgentInfoResponse(typedMsg, stream)

	case *msgAuthCapabilities:
		err = c.handleAuthCapabilities(typedMsg, stream)

	default:
		fmt.Printf("baseConnection: unhandled message type: %T\n", typedMsg)
	}

	if err != nil {
		return err
	}
	return nil
}

type connectedState struct {
	acceptCh chan *DataChannel
}

// Handoff the underlying quick connection for use by another protocol.
func (c *baseConnection) Handoff() (quic.Connection, error) {
	c.mu.Lock()
	c.acceptCancel() // Stop stream handling loop
	done := c.done
	c.closeErr = ErrHandedOff
	c.mu.Unlock()

	<-done

	return c.conn, nil
}

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
