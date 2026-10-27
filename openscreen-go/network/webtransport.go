package ospc

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/quic-go/quic-go"
)

// LP2P WebTransport extension (local-peer-to-peer index.bs "WebTransport
// extension"): WebTransport sessions pooled on the QUIC connection of an
// authenticated OSP connection.
//
// A session is opened with data-transport-start-request on a new
// bidirectional stream, its control stream, and answered there with
// data-transport-start-response. Exchange-ids are even for the QUIC client
// and odd for the QUIC server, never reused. Each WebTransport stream is a
// QUIC stream of the same directionality whose first message is
// data-transport-stream with the exchange-id; datagrams are QUIC DATAGRAM
// frames prefixed with the exchange-id as a varint. A session is closed
// with data-transport-close followed by FIN on the control stream, drained
// with data-transport-drain. Dedicated (q2q) connections are not
// implemented: every session is pooled.

const (
	// WebTransportSessionGone is the RESET_STREAM and STOP_SENDING code for
	// a stream whose session is closed or unknown (2^32). Codes below it are
	// WebTransport stream error codes; unknown codes from 2^32 are treated
	// as session-gone.
	WebTransportSessionGone uint64 = 1 << 32
	// MaxWebTransportStreamErrorCode is the largest WebTransport stream
	// error code.
	MaxWebTransportStreamErrorCode = 1<<32 - 1
)

// WebTransportOpenTimeout bounds how long OpenWebTransport waits for the
// data-transport-start-response when the context has no earlier deadline.
var WebTransportOpenTimeout = 10 * time.Second

// webTransportHoldTimeout is how long a stream for a session that may not be
// open yet is held before it is refused with session-gone.
var webTransportHoldTimeout = 2 * time.Second

// webTransportDatagramQueue is the number of received datagrams buffered per
// session; further ones are dropped until ReceiveDatagram catches up.
const webTransportDatagramQueue = 64

var (
	// ErrWebTransportUnsupported is returned by OpenWebTransport when the
	// peer doesn't list quic-transport in its agent-info: the extension's
	// messages must not be sent to it.
	ErrWebTransportUnsupported = errors.New("peer does not support the WebTransport extension")
	// ErrWebTransportRefused is returned by OpenWebTransport when the peer
	// answers with a result other than success.
	ErrWebTransportRefused = errors.New("WebTransport session refused by the peer")
	// ErrWebTransportClosed is returned when using a closed session, and by
	// Closed for a session that was closed abruptly without a close code.
	ErrWebTransportClosed = errors.New("WebTransport session closed")
	// ErrWebTransportSessionGone is returned by stream operations when the
	// peer reset or stopped the stream with session-gone. It carries no
	// stream error code.
	ErrWebTransportSessionGone = errors.New("WebTransport stream reset: session gone")
	// ErrWebTransportNoDatagrams is returned by SendDatagram when QUIC
	// datagrams weren't negotiated on the connection.
	ErrWebTransportNoDatagrams = errors.New("QUIC datagrams not supported on this connection")
	// ErrWebTransportDatagramTooLarge is returned by SendDatagram for a
	// datagram that doesn't fit in a QUIC DATAGRAM frame.
	ErrWebTransportDatagramTooLarge = errors.New("datagram too large")
)

// WebTransportCloseInfo is the close code and reason of a cleanly closed
// session.
type WebTransportCloseInfo struct {
	CloseCode uint32
	Reason    string
}

// WebTransportStreamError is returned by stream operations when the stream
// was reset or stopped with a WebTransport stream error code.
type WebTransportStreamError struct {
	Code   uint32
	Remote bool // The peer reset or stopped the stream.
}

func (e *WebTransportStreamError) Error() string {
	side := "locally"
	if e.Remote {
		side = "by the peer"
	}
	return fmt.Sprintf("WebTransport stream aborted %s with code %d", side, e.Code)
}

// streamCanceler is implemented by application streams that can be reset
// and stopped with an error code.
type streamCanceler interface {
	cancelRead(code uint64)
	cancelWrite(code uint64)
}

// datagramConnection is implemented by application connections that carry
// QUIC datagrams.
type datagramConnection interface {
	SupportsDatagrams() bool
	SendDatagram(p []byte) error
	ReceiveDatagram(ctx context.Context) ([]byte, error)
}

// webTransportState is the WebTransport state of one connection.
type webTransportState struct {
	conn *baseConnection

	accept chan *IncomingWebTransport

	mu       sync.Mutex
	sessions map[uint64]*WebTransportSession // Open sessions.
	used     map[uint64]bool                 // Exchange-ids ever used, by either side.
	opening  map[uint64]bool                 // Local exchange-ids awaiting the response.
	nextID   uint64                          // Next local exchange-id.
	held     map[uint64][]*WebTransportStream
}

func newWebTransportState(c *baseConnection) *webTransportState {
	parity := uint64(0)
	if c.agentRole == AgentRoleServer {
		parity = 1
	}
	return &webTransportState{
		conn:     c,
		accept:   make(chan *IncomingWebTransport),
		sessions: make(map[uint64]*WebTransportSession),
		used:     make(map[uint64]bool),
		opening:  make(map[uint64]bool),
		nextID:   parity,
		held:     make(map[uint64][]*WebTransportStream),
	}
}

// peerParity is the parity of the exchange-ids the peer allocates.
func (w *webTransportState) peerParity() uint64 {
	return (w.nextID + 1) % 2
}

func (w *webTransportState) allocate() (uint64, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	id := w.nextID
	if id > MaxExchangeID {
		return 0, errors.New("exchange-ids exhausted")
	}
	w.nextID += 2
	w.used[id] = true
	w.opening[id] = true
	return id, nil
}

// failed records that a local session didn't open.
func (w *webTransportState) failed(id uint64) {
	w.mu.Lock()
	delete(w.opening, id)
	w.mu.Unlock()
}

// open registers an open session and hands it the streams held for it.
func (w *webTransportState) open(s *WebTransportSession) {
	w.mu.Lock()
	w.sessions[s.id] = s
	delete(w.opening, s.id)
	held := w.held[s.id]
	delete(w.held, s.id)
	w.mu.Unlock()
	for _, st := range held {
		st := st
		if !w.conn.goroutine(func() { s.deliverStream(st) }) {
			st.sessionGone()
		}
	}
}

func (w *webTransportState) remove(s *WebTransportSession) {
	w.mu.Lock()
	if w.sessions[s.id] == s {
		delete(w.sessions, s.id)
	}
	w.mu.Unlock()
}

func (w *webTransportState) get(id uint64) *WebTransportSession {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.sessions[id]
}

// route hands an incoming stream to its session. A stream for an exchange-id
// never seen is held for a short time, as its session may be about to open;
// one for a session that is closed is refused right away.
func (w *webTransportState) route(id uint64, st *WebTransportStream) {
	w.mu.Lock()
	if s, ok := w.sessions[id]; ok {
		w.mu.Unlock()
		s.deliverStream(st)
		return
	}
	if w.used[id] && !w.opening[id] {
		w.mu.Unlock()
		st.sessionGone()
		return
	}
	w.held[id] = append(w.held[id], st)
	w.mu.Unlock()

	time.AfterFunc(webTransportHoldTimeout, func() {
		w.mu.Lock()
		held := w.held[id]
		for i, h := range held {
			if h == st {
				w.held[id] = append(held[:i:i], held[i+1:]...)
				w.mu.Unlock()
				st.sessionGone()
				return
			}
		}
		w.mu.Unlock()
	})
}

// abortAll fails every session, when the connection closes.
func (w *webTransportState) abortAll(err error) {
	w.mu.Lock()
	sessions := make([]*WebTransportSession, 0, len(w.sessions))
	for _, s := range w.sessions {
		sessions = append(sessions, s)
	}
	held := w.held
	w.held = make(map[uint64][]*WebTransportStream)
	w.mu.Unlock()
	for _, s := range sessions {
		s.abort(err)
	}
	for _, streams := range held {
		for _, st := range streams {
			st.sessionGone()
		}
	}
}

// runDatagrams routes received datagrams to their sessions. Datagrams for a
// session that is not open are dropped.
func (w *webTransportState) runDatagrams(dc datagramConnection) {
	for {
		p, err := dc.ReceiveDatagram(w.conn.ctx)
		if err != nil {
			return
		}
		id, err := readVaruint(bytes.NewReader(p))
		if err != nil {
			continue
		}
		s := w.get(id)
		if s == nil {
			continue
		}
		n := varuintLen(id)
		select {
		case s.datagrams <- p[n:]:
		default:
			s.conn.log.Debug("WebTransport: dropping datagram, queue full", "exchange-id", id)
		}
	}
}

// varuintLen is the encoded size of v as a QUIC varint.
func varuintLen(v uint64) int {
	switch {
	case v <= maxInt6:
		return 1
	case v <= maxInt14:
		return 2
	case v <= maxInt30:
		return 4
	default:
		return 8
	}
}

// SupportsDatagrams reports whether QUIC datagrams were negotiated on the
// connection, so WebTransport sessions on it are "supports-unreliable".
func (c *Connection) SupportsDatagrams() bool {
	dc, ok := c.base.app.Load().appConn.(datagramConnection)
	return ok && dc.SupportsDatagrams()
}

// OpenWebTransport opens a WebTransport session on the connection. It
// returns once the peer accepted it, or fails with ErrWebTransportRefused.
// It fails with ErrWebTransportUnsupported if the peer doesn't list
// quic-transport in its agent-info.
func (c *Connection) OpenWebTransport(ctx context.Context) (*WebTransportSession, error) {
	return c.base.openWebTransport(ctx)
}

// AcceptWebTransport returns the next WebTransport session the peer opens,
// to be accepted or refused.
func (c *Connection) AcceptWebTransport(ctx context.Context) (*IncomingWebTransport, error) {
	return accept(ctx, c.base, c.base.app.Load().wt.accept)
}

func (c *baseConnection) openWebTransport(ctx context.Context) (*WebTransportSession, error) {
	if !c.remoteAgent.HasCapability(AgentCapabilityQuickTransport) {
		return nil, ErrWebTransportUnsupported
	}
	wt := c.app.Load().wt
	id, err := wt.allocate()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, WebTransportOpenTimeout)
	defer cancel()
	defer context.AfterFunc(c.ctx, cancel)()

	req := &msgDataTransportStartRequest{
		msgRequest: msgRequest{RequestId: msgRequestId(c.nextRequestID())},
		ExchangeId: id,
	}
	control, err := c.openStream(ctx, req)
	if err != nil {
		return nil, err
	}

	if err := awaitTransportStartResponse(ctx, control, req.RequestId); err != nil {
		// Abandon a session with no response by resetting the control
		// stream; a refusal ended it already.
		wt.failed(id)
		abortStream(control)
		if c.ctx.Err() != nil {
			return nil, c.err()
		}
		return nil, err
	}
	c.log.Debug("receive", "msg", "msgDataTransportStartResponse", "exchange-id", id)

	s := newWebTransportSession(c, id, control)
	wt.open(s)
	s.start()
	return s, nil
}

// awaitTransportStartResponse waits for the data-transport-start-response to
// request reqID on the control stream.
func awaitTransportStartResponse(ctx context.Context, control ApplicationStream, reqID msgRequestId) error {
	type result struct {
		msg interface{}
		err error
	}
	ch := make(chan result, 1)
	go func() {
		msg, err := readMessage(control)
		ch <- result{msg, err}
	}()
	var res result
	select {
	case res = <-ch:
	case <-ctx.Done():
		return fmt.Errorf("awaiting data-transport-start-response: %w", ctx.Err())
	}
	if res.err != nil {
		return fmt.Errorf("awaiting data-transport-start-response: %w", res.err)
	}
	resp, ok := res.msg.(*msgDataTransportStartResponse)
	if !ok {
		return fmt.Errorf("awaiting data-transport-start-response: got %s", msgName(res.msg))
	}
	if resp.RequestId != reqID {
		return fmt.Errorf("data-transport-start-response to request %d, want %d", resp.RequestId, reqID)
	}
	if resp.Result != ResultSuccess {
		return fmt.Errorf("%w: result %d", ErrWebTransportRefused, resp.Result)
	}
	return nil
}

// handleTransportStart handles a data-transport-start-request received on
// stream s and hands the session to AcceptWebTransport.
func (c *baseConnection) handleTransportStart(req *msgDataTransportStartRequest, s ApplicationStream) {
	wt := c.app.Load().wt
	wt.mu.Lock()
	ok := req.ExchangeId%2 == wt.peerParity() && !wt.used[req.ExchangeId]
	if ok {
		wt.used[req.ExchangeId] = true
	}
	wt.mu.Unlock()
	if req.SessionBinding != nil {
		c.log.Debug("WebTransport: ignoring session-binding on a pooled connection", "exchange-id", req.ExchangeId)
	}
	inc := &IncomingWebTransport{conn: c, id: req.ExchangeId, control: s, requestID: req.RequestId}
	if !ok {
		c.log.Debug("WebTransport: refusing session with a bad exchange-id", "exchange-id", req.ExchangeId)
		_ = inc.Refuse()
		return
	}
	deliver(c, wt.accept, inc, inc.Refuse)
}

// IncomingWebTransport is a WebTransport session the peer opened, waiting
// to be accepted or refused.
type IncomingWebTransport struct {
	conn      *baseConnection
	id        uint64
	control   ApplicationStream
	requestID msgRequestId
	once      sync.Once
}

// ExchangeID is the session's exchange-id.
func (i *IncomingWebTransport) ExchangeID() uint64 { return i.id }

func (i *IncomingWebTransport) respond(result msgResult) error {
	resp := &msgDataTransportStartResponse{
		msgResponse: msgResponse{RequestId: i.requestID},
		Result:      result,
	}
	i.conn.log.Debug("send", "msg", msgName(resp), "exchange-id", i.id, "result", result)
	return writeMessage(resp, i.control)
}

// Accept accepts the session: it answers success and returns the open
// session. The peer may already have opened streams on it.
func (i *IncomingWebTransport) Accept() (*WebTransportSession, error) {
	var s *WebTransportSession
	var err error
	i.once.Do(func() {
		if err = i.respond(ResultSuccess); err != nil {
			abortStream(i.control)
			return
		}
		s = newWebTransportSession(i.conn, i.id, i.control)
		i.conn.app.Load().wt.open(s)
		s.start()
	})
	if s == nil && err == nil {
		err = errors.New("WebTransport session already answered")
	}
	return s, err
}

// Refuse refuses the session: it answers permanent-error and ends the
// control stream.
func (i *IncomingWebTransport) Refuse() error {
	var err error
	i.once.Do(func() {
		err = i.respond(ResultPermanentError)
		if cerr := i.control.Close(); err == nil {
			err = cerr
		}
	})
	return err
}

// WebTransportSession is an open WebTransport session pooled on an OSP
// connection.
type WebTransportSession struct {
	conn    *baseConnection
	id      uint64
	control ApplicationStream

	acceptBidi chan *WebTransportStream
	acceptUni  chan *WebTransportStream
	datagrams  chan []byte
	draining   chan struct{}
	done       chan struct{}

	mu        sync.Mutex
	closed    bool
	closeInfo WebTransportCloseInfo
	closeErr  error // Set for an abrupt close.
	streams   map[*WebTransportStream]struct{}
	drainOnce sync.Once
}

func newWebTransportSession(c *baseConnection, id uint64, control ApplicationStream) *WebTransportSession {
	return &WebTransportSession{
		conn:       c,
		id:         id,
		control:    control,
		acceptBidi: make(chan *WebTransportStream),
		acceptUni:  make(chan *WebTransportStream),
		datagrams:  make(chan []byte, webTransportDatagramQueue),
		draining:   make(chan struct{}),
		done:       make(chan struct{}),
		streams:    make(map[*WebTransportStream]struct{}),
	}
}

// start reads the control stream until the session closes.
func (s *WebTransportSession) start() {
	if !s.conn.goroutine(s.readControl) {
		s.abort(s.conn.err())
	}
}

func (s *WebTransportSession) readControl() {
	for {
		msg, err := readMessage(s.control)
		if err == io.EOF {
			// FIN without data-transport-close: code 0, empty reason.
			s.peerClosed(WebTransportCloseInfo{})
			return
		} else if err != nil {
			// Reset, or the connection closed: abrupt.
			if s.conn.ctx.Err() != nil {
				err = s.conn.err()
			}
			s.abort(err)
			return
		}
		switch m := msg.(type) {
		case *msgDataTransportClose:
			s.conn.log.Debug("receive", "msg", msgName(m), "exchange-id", s.id, "code", m.CloseCode)
			s.peerClosed(WebTransportCloseInfo{CloseCode: uint32(m.CloseCode), Reason: m.Reason})
			return
		case *msgDataTransportDrain:
			s.conn.log.Debug("receive", "msg", msgName(m), "exchange-id", s.id)
			s.drainOnce.Do(func() { close(s.draining) })
		default:
			s.abort(fmt.Errorf("unexpected %s on the control stream", msgName(msg)))
			return
		}
	}
}

// ExchangeID is the session's exchange-id.
func (s *WebTransportSession) ExchangeID() uint64 { return s.id }

// Done is closed once the session is closed, cleanly or not.
func (s *WebTransportSession) Done() <-chan struct{} { return s.done }

// Closed waits until the session is closed. It returns the close info of a
// clean close (data-transport-close, or FIN on the control stream), or the
// error of an abrupt one (control stream reset, connection closed).
func (s *WebTransportSession) Closed(ctx context.Context) (WebTransportCloseInfo, error) {
	select {
	case <-ctx.Done():
		return WebTransportCloseInfo{}, ctx.Err()
	case <-s.done:
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closeInfo, s.closeErr
}

// Draining is closed once the peer asked to finish using the session.
func (s *WebTransportSession) Draining() <-chan struct{} { return s.draining }

// Drain asks the peer to finish using the session and close it.
func (s *WebTransportSession) Drain() error {
	s.mu.Lock()
	closed := s.closed
	s.mu.Unlock()
	if closed {
		return ErrWebTransportClosed
	}
	msg := &msgDataTransportDrain{}
	s.conn.log.Debug("send", "msg", msgName(msg), "exchange-id", s.id)
	return writeMessage(msg, s.control)
}

// Close closes the session with info: it sends data-transport-close, ends
// the control stream and resets all streams of the session with
// session-gone. The reason must be at most 1024 bytes.
func (s *WebTransportSession) Close(info WebTransportCloseInfo) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.closeInfo = info
	s.mu.Unlock()

	msg := &msgDataTransportClose{CloseCode: uint64(info.CloseCode), Reason: info.Reason}
	s.conn.log.Debug("send", "msg", msgName(msg), "exchange-id", s.id, "code", info.CloseCode)
	err := writeMessage(msg, s.control)
	if cerr := s.control.Close(); err == nil {
		err = cerr
	}
	s.teardown()
	return err
}

// peerClosed handles a clean close by the peer.
func (s *WebTransportSession) peerClosed(info WebTransportCloseInfo) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	s.closeInfo = info
	s.mu.Unlock()
	_ = s.control.Close()
	s.teardown()
}

// abort closes the session abruptly.
func (s *WebTransportSession) abort(err error) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	if err == nil {
		err = ErrWebTransportClosed
	}
	s.closeErr = fmt.Errorf("%w: %w", ErrWebTransportClosed, err)
	s.mu.Unlock()
	abortStream(s.control)
	s.teardown()
}

// teardown resets the session's streams and reports the session closed.
func (s *WebTransportSession) teardown() {
	s.mu.Lock()
	streams := make([]*WebTransportStream, 0, len(s.streams))
	for st := range s.streams {
		streams = append(streams, st)
	}
	s.streams = nil
	s.mu.Unlock()
	for _, st := range streams {
		st.sessionGone()
	}
	s.conn.app.Load().wt.remove(s)
	close(s.done)
}

// addStream registers a stream of the session. It reports false if the
// session is closed.
func (s *WebTransportSession) addStream(st *WebTransportStream) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return false
	}
	s.streams[st] = struct{}{}
	return true
}

func (s *WebTransportSession) removeStream(st *WebTransportStream) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.streams, st)
}

// deliverStream hands a stream the peer opened to AcceptStream or
// AcceptUniStream, or refuses it if the session is closed first.
func (s *WebTransportSession) deliverStream(st *WebTransportStream) {
	st.session = s
	if !s.addStream(st) {
		st.sessionGone()
		return
	}
	ch := s.acceptBidi
	if st.uni {
		ch = s.acceptUni
	}
	select {
	case ch <- st:
	case <-s.done:
		st.sessionGone()
	case <-s.conn.ctx.Done():
		st.sessionGone()
	}
}

func (s *WebTransportSession) acceptStream(ctx context.Context, ch chan *WebTransportStream) (*WebTransportStream, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-s.done:
		return nil, s.closeError()
	case <-s.conn.ctx.Done():
		return nil, s.conn.err()
	case st := <-ch:
		return st, nil
	}
}

func (s *WebTransportSession) closeError() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closeErr != nil {
		return s.closeErr
	}
	return ErrWebTransportClosed
}

// AcceptStream returns the next bidirectional stream the peer opened.
func (s *WebTransportSession) AcceptStream(ctx context.Context) (*WebTransportStream, error) {
	return s.acceptStream(ctx, s.acceptBidi)
}

// AcceptUniStream returns the next unidirectional stream the peer opened,
// which can only be read.
func (s *WebTransportSession) AcceptUniStream(ctx context.Context) (*WebTransportStream, error) {
	return s.acceptStream(ctx, s.acceptUni)
}

// OpenStreamSync opens a bidirectional stream on the session.
func (s *WebTransportSession) OpenStreamSync(ctx context.Context) (*WebTransportStream, error) {
	return s.openStream(ctx, false)
}

// OpenUniStreamSync opens a unidirectional stream on the session, which can
// only be written.
func (s *WebTransportSession) OpenUniStreamSync(ctx context.Context) (*WebTransportStream, error) {
	return s.openStream(ctx, true)
}

func (s *WebTransportSession) openStream(ctx context.Context, uni bool) (*WebTransportStream, error) {
	s.mu.Lock()
	closed := s.closed
	s.mu.Unlock()
	if closed {
		return nil, s.closeError()
	}
	appConn := s.conn.app.Load().appConn
	var stream ApplicationStream
	var err error
	if uni {
		u, ok := appConn.(UniStreamOpener)
		if !ok {
			return nil, errors.New("transport has no unidirectional streams")
		}
		stream, err = u.OpenUniStreamSync(ctx)
	} else {
		stream, err = appConn.OpenStreamSync(ctx)
	}
	if err != nil {
		return nil, err
	}
	if err := writeMessage(&msgDataTransportStream{ExchangeId: s.id}, stream); err != nil {
		abortStream(stream)
		return nil, err
	}
	st := newWebTransportStream(s, stream, uni)
	if !s.addStream(st) {
		st.sessionGone()
		return nil, s.closeError()
	}
	return st, nil
}

// SupportsDatagrams reports whether the session delivers datagrams: it is
// "supports-unreliable" if so and "reliable-only" otherwise.
func (s *WebTransportSession) SupportsDatagrams() bool {
	dc, ok := s.conn.app.Load().appConn.(datagramConnection)
	return ok && dc.SupportsDatagrams()
}

// SendDatagram sends p as a datagram of the session: a QUIC DATAGRAM frame
// with the exchange-id as a varint before p.
func (s *WebTransportSession) SendDatagram(p []byte) error {
	s.mu.Lock()
	closed := s.closed
	s.mu.Unlock()
	if closed {
		return ErrWebTransportClosed
	}
	dc, ok := s.conn.app.Load().appConn.(datagramConnection)
	if !ok || !dc.SupportsDatagrams() {
		return ErrWebTransportNoDatagrams
	}
	buf := make([]byte, 0, varuintLen(s.id)+len(p))
	w := bytes.NewBuffer(buf)
	if err := writeVaruint(s.id, w); err != nil {
		return err
	}
	w.Write(p)
	err := dc.SendDatagram(w.Bytes())
	if err != nil && err.Error() == "message too large" { // quic-go has no typed error for it.
		return fmt.Errorf("%w: %d bytes", ErrWebTransportDatagramTooLarge, len(p))
	}
	return err
}

// ReceiveDatagram returns the next datagram received for the session.
func (s *WebTransportSession) ReceiveDatagram(ctx context.Context) ([]byte, error) {
	select {
	case p := <-s.datagrams:
		return p, nil
	default:
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-s.done:
		return nil, s.closeError()
	case p := <-s.datagrams:
		return p, nil
	}
}

// ExportKeyingMaterial derives keying material of the given length that
// both sides of the session derive identically and no other session yields:
// the TLS exporter of the QUIC connection with the label
// "EXPORTER-WebTransport" and the WebTransport Exporter Context of
// draft-ietf-webtrans-http3 section 4.8, with the exchange-id as session ID,
// label and context inside it. Label and context are at most 255 bytes.
func (s *WebTransportSession) ExportKeyingMaterial(label string, context []byte, length int) ([]byte, error) {
	if len(label) > 255 || len(context) > 255 {
		return nil, errors.New("exporter label and context must be at most 255 bytes")
	}
	// WebTransport Exporter Context {
	//   WebTransport Session ID (64),
	//   Label Length (8), Label (..),
	//   Context Length (8), Context (..),
	// }
	ctx := make([]byte, 0, 8+1+len(label)+1+len(context))
	ctx = binary.BigEndian.AppendUint64(ctx, s.id)
	ctx = append(ctx, byte(len(label)))
	ctx = append(ctx, label...)
	ctx = append(ctx, byte(len(context)))
	ctx = append(ctx, context...)
	cs := s.conn.netConn.ConnectionState()
	return cs.ExportKeyingMaterial("EXPORTER-WebTransport", ctx, length)
}

// WebTransportStream is a stream of a WebTransportSession: a QUIC stream of
// the same directionality. Reads and writes fail with a
// *WebTransportStreamError when the stream was reset or stopped with a
// WebTransport error code, and with ErrWebTransportSessionGone when it was
// reset or stopped because the session is gone.
type WebTransportStream struct {
	session *WebTransportSession
	stream  ApplicationStream
	uni     bool
	local   bool // Opened locally.

	mu        sync.Mutex
	readDone  bool
	writeDone bool
}

func newWebTransportStream(s *WebTransportSession, stream ApplicationStream, uni bool) *WebTransportStream {
	return &WebTransportStream{session: s, stream: stream, uni: uni}
}

// Unidirectional reports whether the stream has only one direction: it can
// only be written if opened locally, only read if opened by the peer.
func (st *WebTransportStream) Unidirectional() bool { return st.uni }

// StreamID is the QUIC stream ID.
func (st *WebTransportStream) StreamID() int64 {
	if s, ok := st.stream.(interface{ streamID() quic.StreamID }); ok {
		return int64(s.streamID())
	}
	return -1
}

// mapStreamError maps a transport error to the WebTransport one.
func mapStreamError(err error) error {
	if err == nil {
		return nil
	}
	var se *quic.StreamError
	if errors.As(err, &se) {
		if uint64(se.ErrorCode) > MaxWebTransportStreamErrorCode {
			return ErrWebTransportSessionGone
		}
		return &WebTransportStreamError{Code: uint32(se.ErrorCode), Remote: se.Remote}
	}
	return err
}

func (st *WebTransportStream) Read(p []byte) (int, error) {
	n, err := st.stream.Read(p)
	if err != nil {
		st.half(true)
	}
	return n, mapStreamError(err)
}

func (st *WebTransportStream) Write(p []byte) (int, error) {
	n, err := st.stream.Write(p)
	return n, mapStreamError(err)
}

// Close ends the sending side of the stream with FIN.
func (st *WebTransportStream) Close() error {
	err := st.stream.Close()
	st.half(false)
	return mapStreamError(err)
}

// CancelRead stops receiving with a WebTransport error code (STOP_SENDING).
func (st *WebTransportStream) CancelRead(code uint32) {
	if c, ok := st.stream.(streamCanceler); ok {
		c.cancelRead(uint64(code))
	}
	st.half(true)
}

// CancelWrite aborts sending with a WebTransport error code (RESET_STREAM).
func (st *WebTransportStream) CancelWrite(code uint32) {
	if c, ok := st.stream.(streamCanceler); ok {
		c.cancelWrite(uint64(code))
	}
	st.half(false)
}

// sessionGone resets and stops the stream with session-gone.
func (st *WebTransportStream) sessionGone() {
	if c, ok := st.stream.(streamCanceler); ok {
		c.cancelRead(WebTransportSessionGone)
		c.cancelWrite(WebTransportSessionGone)
	} else {
		abortStream(st.stream)
	}
	st.half(true)
	st.half(false)
}

// half records that one direction is finished and deregisters the stream
// from its session once both are.
func (st *WebTransportStream) half(read bool) {
	st.mu.Lock()
	if read {
		st.readDone = true
	} else {
		st.writeDone = true
	}
	done := st.readDone && st.writeDone
	if st.uni {
		done = st.readDone || st.writeDone
	}
	st.mu.Unlock()
	if done && st.session != nil {
		st.session.removeStream(st)
	}
}
