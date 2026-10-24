package ospc

import (
	"context"
	"crypto/tls"
	"io"
	"log/slog"
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
	// Logger receives transport logs. Nil discards them.
	Logger *slog.Logger
}

func (t *QuicTransport) logger() *slog.Logger {
	if t.Logger == nil {
		return discardLogger
	}
	return t.Logger
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
	return newQuicNetworkConnection(qConn, t.KeepAlive, t.logger()), nil
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
	tr := &quic.Transport{
		Conn:                  conn,
		ConnectionIDGenerator: &ospConnectionIDGenerator{},
	}
	qConn, err := tr.Listen(tlsConf, t.quicConfig())
	if err != nil {
		conn.Close()
		return nil, err
	}

	return &quicNetworkListener{
		listener:  qConn,
		transport: tr,
		keepAlive: t.KeepAlive,
		log:       t.logger(),
	}, nil
}

var _ NetworkListener = &quicNetworkListener{}

type quicNetworkListener struct {
	listener  *quic.Listener
	transport *quic.Transport
	keepAlive KeepAliveConfig
	log       *slog.Logger

	mu     sync.Mutex // Protects the state below.
	active int        // Accepted connections not yet closed.
	closed bool
}

func (l *quicNetworkListener) Accept(ctx context.Context) (NetworkConnection, error) {
	conn, err := l.acceptRaw(ctx)
	if err != nil {
		return nil, err
	}
	return l.wrap(conn), nil
}

// acceptRaw accepts a QUIC connection without starting the network protocol
// on it, so connections for other ALPNs can be handed off untouched.
func (l *quicNetworkListener) acceptRaw(ctx context.Context) (quic.Connection, error) {
	conn, err := l.listener.Accept(ctx)
	if err != nil {
		return nil, err
	}
	l.track(conn)
	return conn, nil
}

// track keeps the UDP socket open while conn is alive.
func (l *quicNetworkListener) track(conn quic.Connection) {
	l.mu.Lock()
	l.active++
	l.mu.Unlock()

	go func() {
		<-conn.Context().Done()

		l.mu.Lock()
		l.active--
		closeTransport := l.closed && l.active == 0
		l.mu.Unlock()
		if closeTransport {
			_ = l.transport.Close()
		}
	}()
}

func (l *quicNetworkListener) wrap(conn quic.Connection) NetworkConnection {
	return newQuicNetworkConnection(conn, l.keepAlive, l.log)
}

// Close stops listening. Accepted connections are not closed, they share the
// UDP socket, which is closed once the last of them is closed.
func (l *quicNetworkListener) Close() error {
	err := l.listener.Close()

	l.mu.Lock()
	l.closed = true
	closeTransport := l.active == 0
	l.mu.Unlock()
	if closeTransport {
		if tErr := l.transport.Close(); err == nil {
			err = tErr
		}
	}
	return err
}

func (l *quicNetworkListener) Addr() net.Addr {
	return l.listener.Addr()
}

var _ NetworkConnection = &quicNetworkConnection{}

// QuicNetworkConnection reads every incoming stream concurrently, so a stalled
// stream doesn't block messages on other streams. Messages within a stream
// are delivered in order. Each outgoing message is sent on its own
// unidirectional stream.
//
// Unidirectional streams keep being read after the hand-off to the
// application protocol, which only uses bidirectional streams. This keeps
// agent-status messages, and with that the keep-alive, working.
type quicNetworkConnection struct {
	conn      quic.Connection
	keepAlive *keepAlive
	log       *slog.Logger

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

func newQuicNetworkConnection(conn quic.Connection, conf KeepAliveConfig, log *slog.Logger) *quicNetworkConnection {
	ctx, cancel := context.WithCancel(context.Background())
	bidiCtx, bidiCancel := context.WithCancel(ctx)
	q := &quicNetworkConnection{
		conn:       conn,
		msgCh:      make(chan interface{}),
		ctx:        ctx,
		cancel:     cancel,
		bidiCancel: bidiCancel,
		netDone:    make(chan struct{}),
		log:        log,
	}
	q.keepAlive = newKeepAlive(conf, q.sendMessage, log)

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

func (q *quicNetworkConnection) acceptStreams(accept func() (quic.ReceiveStream, error)) {
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
func (q *quicNetworkConnection) readStream(s quic.ReceiveStream) {
	defer q.wg.Done()
	for {
		msg, err := readMessage(s)
		if err == io.EOF {
			return
		} else if err != nil {
			select {
			case <-q.ctx.Done():
			default:
				q.log.Debug("quic: failed to read message", "stream", s.StreamID(), "err", err)
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
func (q *quicNetworkConnection) dispatch(msg interface{}) bool {
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
		q.log.Debug("quic: dropping message received after hand-off", "msg", msgName(msg))
		return true
	case <-q.ctx.Done():
		return false
	}
}

// ReadMessage returns the next network protocol message received on any of
// the streams.
func (q *quicNetworkConnection) ReadMessage() (interface{}, error) {
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
func (q *quicNetworkConnection) Write(p []byte) (int, error) {
	if err := q.closeError(); err != nil {
		return 0, err
	}
	return q.write(p)
}

func (q *quicNetworkConnection) write(p []byte) (int, error) {
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
func (q *quicNetworkConnection) sendMessage(msg interface{}) error {
	return writeMessage(msg, writerFunc(q.write))
}

type writerFunc func(p []byte) (int, error)

func (f writerFunc) Write(p []byte) (int, error) {
	return f(p)
}

func (q *quicNetworkConnection) setRequestIDSource(next func() uint64) {
	q.keepAlive.setRequestIDSource(next)
}

func (q *quicNetworkConnection) IsReliable() bool {
	return true
}

func (q *quicNetworkConnection) ConnectionState() tls.ConnectionState {
	return q.conn.ConnectionState().TLS
}

func (q *quicNetworkConnection) closeError() error {
	q.mu.Lock()
	defer q.mu.Unlock()

	return q.closeErr
}

// endNetworkPhase stops delivering messages to the network protocol.
func (q *quicNetworkConnection) endNetworkPhase(err error) error {
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

func (q *quicNetworkConnection) IntoApplicationConnection() (ApplicationConnection, error) {
	if err := q.endNetworkPhase(ErrTransportHandedOff); err != nil {
		return nil, err
	}
	return &quicApplicationConnection{owner: q}, nil
}

func (q *quicNetworkConnection) Close() error {
	q.endNetworkPhase(ErrTransportClosed)
	return q.closeConn(QuicErrorCodeNoLongerNeeded, "Closed")
}

// closeConn closes the QUIC connection and waits for the stream readers.
func (q *quicNetworkConnection) closeConn(code quic.ApplicationErrorCode, reason string) error {
	var err error
	q.closeOnce.Do(func() {
		q.keepAlive.stop()
		q.cancel()
		err = q.conn.CloseWithError(code, reason)
		q.wg.Wait()
	})
	return err
}

var _ ApplicationConnection = &quicApplicationConnection{}

type quicApplicationConnection struct {
	owner *quicNetworkConnection
}

func (q *quicApplicationConnection) AcceptStream(ctx context.Context) (ApplicationStream, error) {
	s, err := q.owner.conn.AcceptStream(ctx)
	if err != nil {
		return nil, err
	}
	return &quicApplicationStream{stream: s, keepAlive: q.owner.keepAlive}, nil
}

func (q *quicApplicationConnection) OpenStreamSync(ctx context.Context) (ApplicationStream, error) {
	s, err := q.owner.conn.OpenStreamSync(ctx)
	if err != nil {
		return nil, err
	}
	return &quicApplicationStream{stream: s, keepAlive: q.owner.keepAlive}, nil
}

func (q *quicApplicationConnection) Close() error {
	return q.owner.closeConn(QuicErrorCodeNoLongerNeeded, "Closed")
}

var _ ApplicationStream = &quicApplicationStream{}

type quicApplicationStream struct {
	stream    quic.Stream
	keepAlive *keepAlive
}

func (s *quicApplicationStream) Read(p []byte) (int, error) {
	return s.stream.Read(p)
}

// Write resets the keep-alive timer, as data sent on any stream counts as
// connection activity.
func (s *quicApplicationStream) Write(p []byte) (int, error) {
	n, err := s.stream.Write(p)
	if n > 0 {
		s.keepAlive.reset()
	}
	return n, err
}

func (s *quicApplicationStream) Close() error {
	return s.stream.Close()
}
