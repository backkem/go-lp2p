package ospc

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/quic-go/quic-go"
)

// QUIC application error codes used when closing a connection.
const (
	// QuicErrorCodeNoLongerNeeded is used when neither side needs the
	// connection anymore.
	QuicErrorCodeNoLongerNeeded quic.ApplicationErrorCode = 5139
)

var _ NetworkTransport = &QuicTransport{}

type QuicTransport struct {
	// IdleTimeout is the QUIC idle_timeout transport parameter. Zero means
	// DefaultIdleTimeout.
	IdleTimeout time.Duration
	// KeepAlive configures sending agent-status-request messages.
	KeepAlive KeepAliveConfig
}

func NewQuicTransport() *QuicTransport {
	return &QuicTransport{}
}

func (t *QuicTransport) quicConfig() *quic.Config {
	idleTimeout := t.IdleTimeout
	if idleTimeout == 0 {
		idleTimeout = DefaultIdleTimeout
	}
	return &quic.Config{
		MaxIdleTimeout: idleTimeout,
		// QUIC PING frames should not be used to keep the connection alive,
		// agent-status-request messages are used instead.
		KeepAlivePeriod: 0,
	}
}

func (t *QuicTransport) DialAddr(ctx context.Context, addr string, tlsConf *tls.Config) (NetworkConnection, error) {
	qConn, err := quic.DialAddr(ctx, addr, tlsConf, t.quicConfig())
	if err != nil {
		return nil, err
	}
	return NewQuicNetworkConnection(qConn, t.KeepAlive), nil
}

type ospConnectionIDGenerator struct {
}

func (g *ospConnectionIDGenerator) GenerateConnectionID() (quic.ConnectionID, error) {
	return quic.ConnectionID{}, nil
}

func (g *ospConnectionIDGenerator) ConnectionIDLen() int {
	return 0
}

func (t *QuicTransport) ListenAddr(addr string, tlsConf *tls.Config) (NetworkListener, error) {
	// ListenAddr is a version of quic.ListenAddr that overwrites the
	// ConnectionID behavior to match the OSP zero-length requirement.
	conn, err := listenUDP(addr)
	if err != nil {
		return nil, err
	}
	qConn, err := (&quic.Transport{
		Conn:                  conn,
		ConnectionIDGenerator: &ospConnectionIDGenerator{},
	}).Listen(tlsConf, t.quicConfig())
	if err != nil {
		return nil, err
	}

	return &QuicNetworkListener{
		listener:  qConn,
		keepAlive: t.KeepAlive,
	}, nil
}

var _ NetworkListener = &QuicNetworkListener{}

type QuicNetworkListener struct {
	listener  *quic.Listener
	keepAlive KeepAliveConfig
}

func (l *QuicNetworkListener) Accept(ctx context.Context) (NetworkConnection, error) {
	conn, err := l.listener.Accept(ctx)
	if err != nil {
		return nil, err
	}
	return NewQuicNetworkConnection(conn, l.keepAlive), nil
}

func (l *QuicNetworkListener) Addr() net.Addr {
	return l.listener.Addr()
}

var _ NetworkConnection = &QuicNetworkConnection{}

// QuicNetworkConnection reads every incoming stream concurrently, so a stalled
// stream doesn't block messages on other streams. Messages within a stream
// are delivered in order. Each outgoing message is sent on its own
// unidirectional stream.
//
// Unidirectional streams keep being read after the hand-off to the
// application protocol, which only uses bidirectional streams. This keeps
// agent-status messages, and with that the keep-alive, working.
type QuicNetworkConnection struct {
	conn      quic.Connection
	keepAlive *keepAlive

	msgCh chan interface{}

	// Lifecycle
	ctx        context.Context // Lifetime of the connection
	cancel     context.CancelFunc
	bidiCancel context.CancelFunc // Stops accepting bidirectional streams
	netDone    chan struct{}      // Closed on hand-off or close
	wg         sync.WaitGroup

	mu        sync.Mutex // Protects state below
	closeErr  error
	closeOnce sync.Once
}

func NewQuicNetworkConnection(conn quic.Connection, conf KeepAliveConfig) *QuicNetworkConnection {
	ctx, cancel := context.WithCancel(context.Background())
	bidiCtx, bidiCancel := context.WithCancel(ctx)
	q := &QuicNetworkConnection{
		conn:       conn,
		msgCh:      make(chan interface{}),
		ctx:        ctx,
		cancel:     cancel,
		bidiCancel: bidiCancel,
		netDone:    make(chan struct{}),
	}
	q.keepAlive = newKeepAlive(conf, q.sendMessage)

	// Accept unidirectional streams (spec-compliant peers)
	q.wg.Add(1)
	go q.acceptStreams(func() (quic.ReceiveStream, error) {
		return conn.AcceptUniStream(ctx)
	})

	// Accept bidirectional streams (backward compat), network phase only.
	q.wg.Add(1)
	go q.acceptStreams(func() (quic.ReceiveStream, error) {
		return conn.AcceptStream(bidiCtx)
	})

	q.keepAlive.start()
	return q
}

func (q *QuicNetworkConnection) acceptStreams(accept func() (quic.ReceiveStream, error)) {
	defer q.wg.Done()
	for {
		s, err := accept()
		if err != nil {
			return
		}
		q.wg.Add(1)
		go q.readStream(s)
	}
}

// readStream reads the messages of a single stream in order.
func (q *QuicNetworkConnection) readStream(s quic.ReceiveStream) {
	defer q.wg.Done()
	for {
		msg, err := readMessage(s)
		if err == io.EOF {
			return
		} else if err != nil {
			select {
			case <-q.ctx.Done():
			default:
				fmt.Printf("quic: failed to read message from stream %d: %v\n", s.StreamID(), err)
			}
			s.CancelRead(0)
			return
		}

		if !q.dispatch(msg) {
			return
		}
	}
}

// dispatch delivers a message read from a stream. It reports false if the
// connection is closed.
func (q *QuicNetworkConnection) dispatch(msg interface{}) bool {
	if q.keepAlive.handleMessage(msg) {
		return true
	}

	select {
	case q.msgCh <- msg:
		return true
	case <-q.netDone:
		// Handed off: the remaining unidirectional stream traffic is only
		// for the keep-alive.
		select {
		case <-q.ctx.Done():
			return false
		default:
		}
		fmt.Printf("quic: dropping %T received after hand-off\n", msg)
		return true
	case <-q.ctx.Done():
		return false
	}
}

// ReadMessage returns the next network protocol message received on any of
// the streams.
func (q *QuicNetworkConnection) ReadMessage() (interface{}, error) {
	select {
	case msg := <-q.msgCh:
		return msg, nil
	case <-q.netDone:
		return nil, q.closeError()
	case <-q.conn.Context().Done():
		return nil, context.Cause(q.conn.Context())
	}
}

// Write opens a new unidirectional stream per call, writes the data,
// and closes the stream (sending FIN). This matches the spec requirement
// of one unidirectional stream per message.
func (q *QuicNetworkConnection) Write(p []byte) (int, error) {
	if err := q.closeError(); err != nil {
		return 0, err
	}
	return q.write(p)
}

func (q *QuicNetworkConnection) write(p []byte) (int, error) {
	s, err := q.conn.OpenUniStreamSync(q.ctx)
	if err != nil {
		return 0, err
	}
	n, err := s.Write(p)
	if err != nil {
		s.CancelWrite(0)
		return n, err
	}
	if closeErr := s.Close(); closeErr != nil {
		return n, closeErr
	}
	q.keepAlive.reset()
	return n, nil
}

// sendMessage writes a message on a new unidirectional stream. Unlike Write
// it keeps working after the hand-off.
func (q *QuicNetworkConnection) sendMessage(msg interface{}) error {
	return writeMessage(msg, writerFunc(q.write))
}

type writerFunc func(p []byte) (int, error)

func (f writerFunc) Write(p []byte) (int, error) {
	return f(p)
}

func (q *QuicNetworkConnection) setRequestIDSource(next func() uint64) {
	q.keepAlive.setRequestIDSource(next)
}

func (q *QuicNetworkConnection) IsReliable() bool {
	return true
}

func (q *QuicNetworkConnection) ConnectionState() tls.ConnectionState {
	return q.conn.ConnectionState().TLS
}

func (q *QuicNetworkConnection) closeError() error {
	q.mu.Lock()
	defer q.mu.Unlock()

	return q.closeErr
}

// endNetworkPhase stops delivering messages to the network protocol.
func (q *QuicNetworkConnection) endNetworkPhase(err error) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.closeErr != nil {
		return q.closeErr
	}
	q.closeErr = err
	q.bidiCancel()
	close(q.netDone)
	return nil
}

func (q *QuicNetworkConnection) IntoApplicationConnection() (ApplicationConnection, error) {
	if err := q.endNetworkPhase(ErrTransportHandedOff); err != nil {
		return nil, err
	}
	return &QuicApplicationConnection{owner: q}, nil
}

func (q *QuicNetworkConnection) Close() error {
	q.endNetworkPhase(ErrTransportClosed)
	return q.closeConn(QuicErrorCodeNoLongerNeeded, "Closed")
}

// closeConn closes the QUIC connection and waits for the stream readers.
func (q *QuicNetworkConnection) closeConn(code quic.ApplicationErrorCode, reason string) error {
	var err error
	q.closeOnce.Do(func() {
		q.keepAlive.stop()
		q.cancel()
		err = q.conn.CloseWithError(code, reason)
		q.wg.Wait()
	})
	return err
}

var _ ApplicationConnection = &QuicApplicationConnection{}

type QuicApplicationConnection struct {
	owner *QuicNetworkConnection
}

func (q *QuicApplicationConnection) AcceptStream(ctx context.Context) (ApplicationStream, error) {
	s, err := q.owner.conn.AcceptStream(ctx)
	if err != nil {
		return nil, err
	}
	return &QuicApplicationStream{stream: s, keepAlive: q.owner.keepAlive}, nil
}

func (q *QuicApplicationConnection) OpenStreamSync(ctx context.Context) (ApplicationStream, error) {
	s, err := q.owner.conn.OpenStreamSync(ctx)
	if err != nil {
		return nil, err
	}
	return &QuicApplicationStream{stream: s, keepAlive: q.owner.keepAlive}, nil
}

func (q *QuicApplicationConnection) Close() error {
	return q.owner.closeConn(QuicErrorCodeNoLongerNeeded, "Closed")
}

var _ ApplicationStream = &QuicApplicationStream{}

type QuicApplicationStream struct {
	stream    quic.Stream
	keepAlive *keepAlive
}

func (s *QuicApplicationStream) Read(p []byte) (int, error) {
	return s.stream.Read(p)
}

// Write resets the keep-alive timer, as data sent on any stream counts as
// connection activity.
func (s *QuicApplicationStream) Write(p []byte) (int, error) {
	n, err := s.stream.Write(p)
	if n > 0 {
		s.keepAlive.reset()
	}
	return n, err
}

func (s *QuicApplicationStream) Close() error {
	return s.stream.Close()
}
