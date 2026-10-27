package ospc

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
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
		// The LP2P WebTransport extension carries datagrams in QUIC DATAGRAM
		// frames (RFC 9221).
		EnableDatagrams: true,
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

// maxEarlyStreams bounds the application streams held during the network
// phase, see quicNetworkConnection.
const maxEarlyStreams = 64

// quicNetworkConnection reads every incoming stream concurrently, so a stalled
// stream doesn't block messages on other streams. Messages within a stream
// are delivered in order. Each outgoing network message is sent on its own
// unidirectional stream.
//
// Incoming streams are routed by the type key of their first message:
//   - agent-status messages are always handled by the keep-alive;
//   - during the network phase, network protocol messages (agent-info and
//     auth-*) are delivered to ReadMessage;
//   - any other stream is an application stream. During the network phase
//     it is held, unread past its type key, and handed to the application
//     connection once the connection is handed off: a peer that completed
//     authentication first may already send application messages. After the
//     hand-off, streams are handed over right away.
//
// Application streams can be unidirectional, as application.bs requires, or
// bidirectional, as used by the data channel and pooled WebTransport
// protocols.
type quicNetworkConnection struct {
	conn      quic.Connection
	keepAlive *keepAlive
	log       *slog.Logger

	msgCh      chan interface{}
	appStreams chan ApplicationStream

	// Lifecycle
	ctx     context.Context // Lifetime of the connection
	cancel  context.CancelFunc
	netDone chan struct{} // Closed on hand-off or close
	wg      sync.WaitGroup

	mu           sync.Mutex // Protects state below
	closeErr     error
	earlyStreams []*quicApplicationStream // Held during the network phase.
	closeOnce    sync.Once
}

func newQuicNetworkConnection(conn quic.Connection, conf KeepAliveConfig, log *slog.Logger) *quicNetworkConnection {
	ctx, cancel := context.WithCancel(context.Background())
	q := &quicNetworkConnection{
		conn:       conn,
		msgCh:      make(chan interface{}),
		appStreams: make(chan ApplicationStream),
		ctx:        ctx,
		cancel:     cancel,
		netDone:    make(chan struct{}),
		log:        log,
	}
	q.keepAlive = newKeepAlive(conf, q.sendMessage, log)

	// Unidirectional streams, used for all OSP messages.
	q.wg.Add(1)
	go q.acceptStreams(func() (quic.ReceiveStream, quic.SendStream, error) {
		s, err := conn.AcceptUniStream(ctx)
		return s, nil, err
	})

	// Bidirectional streams: network messages from older ospc versions and
	// application protocols that need them.
	q.wg.Add(1)
	go q.acceptStreams(func() (quic.ReceiveStream, quic.SendStream, error) {
		s, err := conn.AcceptStream(ctx)
		return s, s, err
	})

	q.keepAlive.start()
	return q
}

func (q *quicNetworkConnection) acceptStreams(accept func() (quic.ReceiveStream, quic.SendStream, error)) {
	defer q.wg.Done()
	for {
		r, w, err := accept()
		if err != nil {
			return
		}
		q.wg.Add(1)
		go q.readStream(r, w)
	}
}

// isNetworkTypeKey reports whether key is a network protocol message
// handled during the network phase: metadata discovery and authentication.
func isNetworkTypeKey(key TypeKey) bool {
	switch key {
	case typeKeyAgentInfoRequest, typeKeyAgentInfoResponse,
		typeKeyAgentStatusRequest, typeKeyAgentStatusResponse:
		return true
	}
	return key >= 1000 && key < 1100 // auth-*
}

// isKeepAliveTypeKey reports whether key is an agent-status message, handled
// by the transport for the lifetime of the connection.
func isKeepAliveTypeKey(key TypeKey) bool {
	return key == typeKeyAgentStatusRequest || key == typeKeyAgentStatusResponse
}

// readStream routes an incoming stream by the type key of its first message.
// w is nil for unidirectional streams.
func (q *quicNetworkConnection) readStream(r quic.ReceiveStream, w quic.SendStream) {
	defer q.wg.Done()

	rec := &recordingReader{r: r}
	typeKey, err := readTypeKey(rec)
	if err != nil {
		if err != io.EOF {
			q.logReadError(r, err)
		}
		r.CancelRead(0)
		return
	}

	q.mu.Lock()
	handedOff := errors.Is(q.closeErr, ErrTransportHandedOff)
	q.mu.Unlock()
	if typeKey == typeKeyLp2pSessionRequest && !handedOff {
		// Session extension: the request is delivered with its stream, on
		// which the response goes.
		msg, err := readMessageBody(r, typeKey)
		if err != nil {
			q.logReadError(r, err)
			r.CancelRead(0)
			return
		}
		req := &incomingSessionRequest{msg: msg.(*msgLp2pSessionRequest)}
		if w != nil {
			req.stream = &quicApplicationStream{recv: r, send: w, r: r, keepAlive: q.keepAlive}
		}
		q.dispatch(req)
		return
	}
	if !isKeepAliveTypeKey(typeKey) && (handedOff || !isNetworkTypeKey(typeKey)) {
		q.routeApplicationStream(&quicApplicationStream{
			recv:      r,
			send:      w,
			r:         io.MultiReader(bytes.NewReader(rec.buf), r),
			keepAlive: q.keepAlive,
		})
		return
	}

	msg, err := readMessageBody(r, typeKey)
	for {
		if err == io.EOF {
			return
		} else if err != nil {
			q.logReadError(r, err)
			r.CancelRead(0)
			return
		}
		if !q.dispatch(msg) {
			return
		}
		msg, err = readMessage(r)
	}
}

func (q *quicNetworkConnection) logReadError(s quic.ReceiveStream, err error) {
	select {
	case <-q.ctx.Done():
	default:
		q.log.Debug("quic: failed to read message", "stream", s.StreamID(), "err", err)
	}
}

// routeApplicationStream holds s during the network phase and hands it to
// the application connection after the hand-off.
func (q *quicNetworkConnection) routeApplicationStream(s *quicApplicationStream) {
	q.mu.Lock()
	switch {
	case q.closeErr == nil:
		if len(q.earlyStreams) >= maxEarlyStreams {
			q.mu.Unlock()
			q.log.Debug("quic: too many application streams before hand-off, dropping stream")
			s.cancel()
			return
		}
		q.earlyStreams = append(q.earlyStreams, s)
		q.mu.Unlock()
		return
	case !errors.Is(q.closeErr, ErrTransportHandedOff):
		q.mu.Unlock()
		s.cancel()
		return
	}
	q.mu.Unlock()

	select {
	case q.appStreams <- s:
	case <-q.ctx.Done():
		s.cancel()
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
		// Handed off: only the keep-alive is handled by the transport.
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

// OpenNetworkStream opens a bidirectional stream during the network phase,
// for the session extension.
func (q *quicNetworkConnection) OpenNetworkStream(ctx context.Context) (ApplicationStream, error) {
	if err := q.closeError(); err != nil {
		return nil, err
	}
	s, err := q.conn.OpenStreamSync(ctx)
	if err != nil {
		return nil, err
	}
	return &quicApplicationStream{recv: s, send: s, r: s, keepAlive: q.keepAlive}, nil
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
	close(q.netDone)
	if !errors.Is(err, ErrTransportHandedOff) {
		for _, s := range q.earlyStreams {
			s.cancel()
		}
		q.earlyStreams = nil
	}
	return nil
}

// popEarlyStream returns the next application stream received before the
// hand-off, or nil.
func (q *quicNetworkConnection) popEarlyStream() *quicApplicationStream {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.earlyStreams) == 0 {
		return nil
	}
	s := q.earlyStreams[0]
	q.earlyStreams = q.earlyStreams[1:]
	return s
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

		q.mu.Lock()
		for _, s := range q.earlyStreams {
			s.cancel()
		}
		q.earlyStreams = nil
		q.mu.Unlock()
	})
	return err
}

var _ ApplicationConnection = &quicApplicationConnection{}
var _ UniStreamOpener = &quicApplicationConnection{}

type quicApplicationConnection struct {
	owner *quicNetworkConnection
}

// AcceptStream returns the next incoming application stream, unidirectional
// or bidirectional. Unidirectional streams fail on Write.
func (q *quicApplicationConnection) AcceptStream(ctx context.Context) (ApplicationStream, error) {
	if s := q.owner.popEarlyStream(); s != nil {
		return s, nil
	}
	select {
	case s := <-q.owner.appStreams:
		return s, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-q.owner.conn.Context().Done():
		return nil, context.Cause(q.owner.conn.Context())
	case <-q.owner.ctx.Done():
		return nil, ErrTransportClosed
	}
}

func (q *quicApplicationConnection) OpenStreamSync(ctx context.Context) (ApplicationStream, error) {
	s, err := q.owner.conn.OpenStreamSync(ctx)
	if err != nil {
		return nil, err
	}
	return &quicApplicationStream{recv: s, send: s, r: s, keepAlive: q.owner.keepAlive}, nil
}

// OpenUniStreamSync opens a unidirectional application stream. Read fails on
// the returned stream.
func (q *quicApplicationConnection) OpenUniStreamSync(ctx context.Context) (ApplicationStream, error) {
	s, err := q.owner.conn.OpenUniStreamSync(ctx)
	if err != nil {
		return nil, err
	}
	return &quicApplicationStream{send: s, keepAlive: q.owner.keepAlive}, nil
}

func (q *quicApplicationConnection) Close() error {
	return q.owner.closeConn(QuicErrorCodeNoLongerNeeded, "Closed")
}

// SupportsDatagrams reports whether QUIC datagrams were negotiated.
func (q *quicApplicationConnection) SupportsDatagrams() bool {
	return q.owner.conn.ConnectionState().SupportsDatagrams
}

// SendDatagram sends a QUIC DATAGRAM frame.
func (q *quicApplicationConnection) SendDatagram(p []byte) error {
	err := q.owner.conn.SendMessage(p)
	if err == nil {
		q.owner.keepAlive.reset()
	}
	return err
}

// ReceiveDatagram returns the next received QUIC DATAGRAM frame.
func (q *quicApplicationConnection) ReceiveDatagram(ctx context.Context) ([]byte, error) {
	return q.owner.conn.ReceiveMessage(ctx)
}

var _ ApplicationStream = &quicApplicationStream{}

// ErrStreamDirection is returned when reading from a stream that can only be
// written or writing to a stream that can only be read: one half of a
// unidirectional stream.
var ErrStreamDirection = errors.New("unidirectional stream: operation not supported in this direction")

// quicApplicationStream is a bidirectional stream or one half of a
// unidirectional stream: recv or send is nil.
type quicApplicationStream struct {
	recv      quic.ReceiveStream
	send      quic.SendStream
	r         io.Reader // Reads recv, possibly replaying already read bytes.
	keepAlive *keepAlive
}

func (s *quicApplicationStream) Read(p []byte) (int, error) {
	if s.r == nil {
		return 0, ErrStreamDirection
	}
	return s.r.Read(p)
}

// Write resets the keep-alive timer, as data sent on any stream counts as
// connection activity.
func (s *quicApplicationStream) Write(p []byte) (int, error) {
	if s.send == nil {
		return 0, ErrStreamDirection
	}
	n, err := s.send.Write(p)
	if n > 0 {
		s.keepAlive.reset()
	}
	return n, err
}

// Close closes the sending side. For a receive-only stream it stops reading,
// discarding anything not read yet.
func (s *quicApplicationStream) Close() error {
	if s.send != nil {
		return s.send.Close()
	}
	s.recv.CancelRead(0)
	return nil
}

func (s *quicApplicationStream) streamID() quic.StreamID {
	if s.recv != nil {
		return s.recv.StreamID()
	}
	return s.send.StreamID()
}

// cancel aborts both directions of the stream.
func (s *quicApplicationStream) cancel() {
	s.cancelRead(0)
	s.cancelWrite(0)
}

// cancelRead stops receiving with the given application error code.
func (s *quicApplicationStream) cancelRead(code uint64) {
	if s.recv != nil {
		s.recv.CancelRead(quic.StreamErrorCode(code))
	}
}

// cancelWrite aborts sending with the given application error code.
func (s *quicApplicationStream) cancelWrite(code uint64) {
	if s.send != nil {
		s.send.CancelWrite(quic.StreamErrorCode(code))
	}
}

// receiveOnly reports whether the stream is the receiving half of a
// unidirectional stream.
func (s *quicApplicationStream) receiveOnly() bool {
	return s.send == nil
}
