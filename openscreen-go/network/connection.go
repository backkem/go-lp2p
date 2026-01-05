package ospc

import (
	"context"
	"crypto/rand"
	"errors"
	"math/big"
	"sync"
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

// OpenStream opens a new application stream for use by custom protocols.
// The stream is a raw bidirectional byte stream over QUIC.
func (c *Connection) OpenStream(ctx context.Context) (ApplicationStream, error) {
	return c.base.connectedState.appConn.OpenStreamSync(ctx)
}

// AcceptStream accepts an incoming application stream.
// Returns streams opened by the remote peer for custom protocol use.
func (c *Connection) AcceptStream(ctx context.Context) (ApplicationStream, error) {
	return c.base.connectedState.appConn.AcceptStream(ctx)
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
	netConn NetworkConnection

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

func newBaseConnection(nc NetworkConnection, localAgent *Agent, remoteAgent *Agent, role AgentRole) *baseConnection {

	// TODO: Retransmission in case NetworkConnection is not reliable.

	bConn := &baseConnection{
		mu:          sync.Mutex{},
		agentRole:   role,
		agentState:  newAgentState(), // TODO: reconnect
		localAgent:  localAgent,
		remoteAgent: remoteAgent,
		netConn:     nc,
		authNotify:  make(chan struct{}),
		close:       make(chan struct{}),
		done:        make(chan struct{}),
	}

	return bConn
}

func (c *baseConnection) RemoteAgent() *Agent {
	return c.remoteAgent
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
	var closingErr error
	if c.connectedState != nil {
		closingErr = c.connectedState.appConn.Close()
	} else {
		closingErr = c.netConn.Close()
	}

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
	return closingErr
}

func (c *baseConnection) err() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.closeErr
}
