package ospc

import (
	"context"
	"crypto/rand"
	"errors"
	"log/slog"
	"math/big"
	"sync"
	"sync/atomic"
)

var ErrConnectionClosed = errors.New("connection closed")

// Connection is an authenticated connection to a remote agent.
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
// The stream is a raw bidirectional byte stream. The first bytes written
// should be the type key of an OSP message, as the receiving agent routes
// incoming streams by it, see AcceptStream.
func (c *Connection) OpenStream(ctx context.Context) (ApplicationStream, error) {
	return c.base.app.Load().appConn.OpenStreamSync(ctx)
}

// OpenUniStream opens a new unidirectional application stream, which can only
// be written to. Per application.bs, OSP messages are sent on unidirectional
// streams: write the type key and CBOR body of each message (see
// WriteTypedMessage), messages that must be processed in order on the same
// stream, and close it when done. Transports without unidirectional streams
// fall back to a bidirectional stream.
func (c *Connection) OpenUniStream(ctx context.Context) (ApplicationStream, error) {
	appConn := c.base.app.Load().appConn
	if u, ok := appConn.(UniStreamOpener); ok {
		return u.OpenUniStreamSync(ctx)
	}
	return appConn.OpenStreamSync(ctx)
}

// Authenticated reports whether the remote agent was authenticated. It is
// only false for connections handed over by
// UnauthenticatedConnection.SkipAuthentication.
func (c *Connection) Authenticated() bool {
	return c.base.remoteAgent.Authenticated()
}

// AcceptStream accepts an incoming application stream for use by custom
// protocols. Streams opened by the peer as unidirectional streams can only
// be read: Write fails.
//
// The connection owns accepting streams: it reads the type key of the first
// message on every incoming stream. Streams for protocols handled by the
// connection itself (data channels, pooled WebTransport) are consumed
// internally, all others are returned here unchanged, i.e. starting with
// their type key.
func (c *Connection) AcceptStream(ctx context.Context) (ApplicationStream, error) {
	return c.base.acceptStream(ctx)
}

// Done is closed when the connection is closed, by either side.
func (c *Connection) Done() <-chan struct{} {
	return c.base.ctx.Done()
}

// Err returns why the connection was closed, or nil if it is still open.
func (c *Connection) Err() error {
	return c.base.err()
}

// Close the connection and all associated streams. It waits for all
// goroutines of the connection to exit. Close is idempotent.
func (c *Connection) Close() error {
	return c.base.Close()
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
//
// Lifecycle: the connection owns a context that every goroutine it starts
// selects on. shutdown cancels it, with the reason as its cause, and closes
// the transport. Close additionally waits for all goroutines to exit.
type baseConnection struct {
	netConn NetworkConnection
	log     *slog.Logger

	// Lifecycle
	ctx       context.Context
	cancel    context.CancelCauseFunc
	wg        sync.WaitGroup
	goMu      sync.Mutex // Protects stopped, orders wg.Add with Close.
	stopped   bool
	closeOnce sync.Once

	// Immutable after construction.
	agentRole   AgentRole
	localAgent  *Agent
	remoteAgent *Agent
	// authToken is the at TXT value of the advertising agent: the local one
	// for a listening agent, the remote one for a dialing agent. Empty if
	// unknown.
	authToken string

	mu sync.Mutex // Protects the state below.

	exchangeInfoState  *exchangeInfoState
	authenticationRole AuthenticationRole

	authNotify          chan struct{}
	authNotified        bool
	authenticationState *authenticationState
	authTokenSent       bool

	// session is the state of the session extension, nil without it.
	session *sessionState

	// app is set once authenticated, before the Connection is handed out
	// and before the application goroutines start.
	app atomic.Pointer[connectedState]
}

// newBaseConnection creates a connection. session enables the session
// extension on it.
func newBaseConnection(nc NetworkConnection, localAgent *Agent, remoteAgent *Agent, role AgentRole, authToken string, session bool) *baseConnection {
	// TODO: Retransmission in case NetworkConnection is not reliable.

	ctx, cancel := context.WithCancelCause(context.Background())
	bConn := &baseConnection{
		netConn:     nc,
		log:         localAgent.logger().With("peer", remoteAgent.PeerID.short()),
		ctx:         ctx,
		cancel:      cancel,
		agentRole:   role,
		localAgent:  localAgent,
		remoteAgent: remoteAgent,
		authToken:   authToken,
		authNotify:  make(chan struct{}),
	}
	if session {
		bConn.session = newSessionState()
	}

	// Transport-level keep-alive requests share the per-peer request IDs.
	if src, ok := nc.(interface{ setRequestIDSource(func() uint64) }); ok {
		src.setRequestIDSource(bConn.nextRequestID)
	}

	return bConn
}

// goroutine runs f in a goroutine owned by the connection. It reports false,
// without running f, if the connection is already shut down.
func (c *baseConnection) goroutine(f func()) bool {
	c.goMu.Lock()
	defer c.goMu.Unlock()

	if c.stopped {
		return false
	}
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		f()
	}()
	return true
}

// nextRequestID returns the next request ID for a message sent to the remote
// agent. IDs are tracked by the local agent per remote PeerID so they survive
// reconnects.
func (c *baseConnection) nextRequestID() uint64 {
	return c.localAgent.NextRequestID(c.remoteAgent.PeerID)
}

func (c *baseConnection) RemoteAgent() *Agent {
	return c.remoteAgent
}

// send writes a network protocol message. It keeps working after the
// hand-off on transports that allow it, for authentication messages a
// client sends after the server accepted its session on trust.
func (c *baseConnection) send(msg interface{}) error {
	c.log.Debug("send", "msg", msgName(msg))
	if s, ok := c.netConn.(interface{ sendMessage(interface{}) error }); ok {
		return s.sendMessage(msg)
	}
	return writeMessage(msg, c.netConn)
}

// Close the connection and all associated streams, and wait for its
// goroutines to exit. Must not be called from a connection goroutine.
func (c *baseConnection) Close() error {
	c.shutdown(ErrConnectionClosed)
	c.wg.Wait()
	return nil
}

// shutdown closes the connection with the given reason without waiting for
// its goroutines. It is idempotent, only the first reason is kept. It may be
// called from connection goroutines, but not while holding c.mu.
func (c *baseConnection) shutdown(cause error) {
	c.closeOnce.Do(func() {
		c.goMu.Lock()
		c.stopped = true
		c.goMu.Unlock()

		if cause != ErrConnectionClosed {
			c.log.Info("connection closed", "reason", cause)
		}
		c.cancel(cause)
		if app := c.app.Load(); app != nil {
			if err := app.appConn.Close(); err != nil {
				c.log.Debug("close application connection", "err", err)
			}
		}
		if err := c.netConn.Close(); err != nil {
			c.log.Debug("close transport", "err", err)
		}
	})
}

// err returns the reason the connection was closed, or nil.
func (c *baseConnection) err() error {
	if c.ctx.Err() == nil {
		return nil
	}
	return context.Cause(c.ctx)
}
