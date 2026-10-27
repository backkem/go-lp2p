package lp2p

import (
	"context"
	"errors"
	"sync"

	ospc "github.com/backkem/go-lp2p/openscreen-go/network"
	"github.com/backkem/go-lp2p/webtransport-api"
)

// LP2PQuicTransportInit configures an LP2PQuicTransport, with the meaning
// of the WebTransportOptions members of the same name. Options that concern
// HTTP or a server (allowPooling, serverCertificateHashes, protocols) are
// left out: every transport is pooled on its connection.
type LP2PQuicTransportInit struct {
	// RequireUnreliable fails the transport unless its session delivers
	// datagrams unreliably, i.e. QUIC datagrams were negotiated.
	RequireUnreliable bool
	// CongestionControl is "default", "throughput" or "low-latency". It is
	// accepted but not acted on.
	CongestionControl string
	// Anticipated numbers of concurrent incoming streams, or nil. Accepted
	// but not acted on.
	AnticipatedConcurrentIncomingUnidirectionalStreams *uint16
	AnticipatedConcurrentIncomingBidirectionalStreams  *uint16
}

// Reliability values of a transport, as WebTransport.reliability.
const (
	ReliabilityPending            = "pending"
	ReliabilityReliableOnly       = "reliable-only"
	ReliabilitySupportsUnreliable = "supports-unreliable"
)

// LP2PQuicTransport is a WebTransport to a peer, over an LP2PConnection.
// Each construction opens a new WebTransport session on the connection; the
// peer receives it with an LP2PQuicTransportListener.
type LP2PQuicTransport struct {
	*webtransport.Transport

	conn    *LP2PConnection
	session *sessionAdaptor
	init    LP2PQuicTransportInit
}

// NewLP2PQuicTransport opens a transport on the connection. It blocks until
// the peer accepted the session, as awaiting the ready promise would.
//
// It fails with an InvalidStateError if the connection is closed, and with
// a WebTransportError if opening fails, the peer refuses, or
// RequireUnreliable is set and the session is reliable-only.
func NewLP2PQuicTransport(connection *LP2PConnection, init LP2PQuicTransportInit) (*LP2PQuicTransport, error) {
	if connection.isClosed() {
		return nil, errClosed("LP2PConnection")
	}
	s, err := connection.conn.OpenWebTransport(connection.ctx)
	if err != nil {
		if connection.isClosed() {
			return nil, errClosed("LP2PConnection")
		}
		return nil, newDOMException(WebTransportError, "opening the session failed: "+err.Error(), err)
	}
	if init.RequireUnreliable && !s.SupportsDatagrams() {
		_ = s.Close(ospc.WebTransportCloseInfo{})
		return nil, newDOMException(WebTransportError, "the session is reliable-only", nil)
	}
	t := newLP2PQuicTransport(connection, s, init)
	if !connection.addTransport(t) {
		t.fail(errClosed("LP2PConnection"))
		return nil, errClosed("LP2PConnection")
	}
	return t, nil
}

func newLP2PQuicTransport(c *LP2PConnection, s *ospc.WebTransportSession, init LP2PQuicTransportInit) *LP2PQuicTransport {
	a := &sessionAdaptor{session: s, failed: make(chan struct{})}
	wt, err := webtransport.NewTransport(a)
	if err != nil {
		panic(err) // only for a nil session
	}
	t := &LP2PQuicTransport{Transport: wt, conn: c, session: a, init: init}
	go func() {
		<-s.Done()
		c.removeTransport(t)
	}()
	return t
}

// Connection returns the connection the transport is pooled on, as the
// connection attribute.
func (t *LP2PQuicTransport) Connection() *LP2PConnection { return t.conn }

// Ready blocks until the transport is ready, as the ready promise. Transports
// are returned ready, so it returns right away.
func (t *LP2PQuicTransport) Ready() error { return nil }

// Reliability is "supports-unreliable" if the session delivers datagrams,
// "reliable-only" otherwise.
func (t *LP2PQuicTransport) Reliability() string {
	if t.session.session.SupportsDatagrams() {
		return ReliabilitySupportsUnreliable
	}
	return ReliabilityReliableOnly
}

// Close the transport with a close code and reason, which the peer's
// transport is closed with. The connection stays open.
func (t *LP2PQuicTransport) Close(info webtransport.WebTransportCloseInfo) error {
	if info.CloseCode > ospc.MaxWebTransportCloseCode {
		return newDOMException(TypeError, "closeCode is not an unsigned long", nil)
	}
	return t.Transport.Close(info)
}

// Closed blocks until the transport is closed, as the closed promise: it
// returns the close info of a clean close, by Close or by the peer, or a
// WebTransportError if the session failed, including because the connection
// closed.
func (t *LP2PQuicTransport) Closed() (webtransport.WebTransportCloseInfo, error) {
	return t.Transport.Closed(context.Background())
}

// Draining blocks until the peer asks to finish using the transport, as the
// draining promise. It fails if the transport closes first.
func (t *LP2PQuicTransport) Draining() error {
	return toDOMException(t.Transport.Draining(context.Background()))
}

// fail fails the transport abruptly with a WebTransportError, when its
// connection closes.
func (t *LP2PQuicTransport) fail(cause error) {
	t.session.fail(cause)
}

// sessionAdaptor adapts an ospc session to webtransport.Session and its
// optional capabilities.
type sessionAdaptor struct {
	session *ospc.WebTransportSession

	failOnce sync.Once
	failed   chan struct{}
	failErr  error
}

var (
	_ webtransport.Session          = (*sessionAdaptor)(nil)
	_ webtransport.SessionCloser    = (*sessionAdaptor)(nil)
	_ webtransport.SessionDrainer   = (*sessionAdaptor)(nil)
	_ webtransport.SessionExporter  = (*sessionAdaptor)(nil)
	_ webtransport.SessionDatagrams = (*sessionAdaptor)(nil)
)

func (a *sessionAdaptor) fail(cause error) {
	a.failOnce.Do(func() {
		a.failErr = newDOMException(WebTransportError, "the session was terminated: "+cause.Error(), cause)
		close(a.failed)
	})
}

func (a *sessionAdaptor) AcceptStream(ctx context.Context) (webtransport.Stream, error) {
	s, err := a.session.AcceptStream(ctx)
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (a *sessionAdaptor) OpenStreamSync(ctx context.Context) (webtransport.Stream, error) {
	s, err := a.session.OpenStreamSync(ctx)
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (a *sessionAdaptor) CloseWithError(code uint64, reason string) error {
	return a.session.Close(ospc.WebTransportCloseInfo{CloseCode: uint32(code), Reason: reason})
}

func (a *sessionAdaptor) Closed(ctx context.Context) (webtransport.WebTransportCloseInfo, error) {
	select {
	case <-ctx.Done():
		return webtransport.WebTransportCloseInfo{}, ctx.Err()
	case <-a.failed:
		return webtransport.WebTransportCloseInfo{}, a.failErr
	case <-a.session.Done():
	}
	info, err := a.session.Closed(ctx)
	if err != nil {
		select {
		case <-a.failed:
			return webtransport.WebTransportCloseInfo{}, a.failErr
		default:
		}
		return webtransport.WebTransportCloseInfo{}, newDOMException(WebTransportError, "the session was terminated: "+err.Error(), err)
	}
	return webtransport.WebTransportCloseInfo{CloseCode: uint64(info.CloseCode), Reason: info.Reason}, nil
}

func (a *sessionAdaptor) Draining() <-chan struct{} { return a.session.Draining() }

func (a *sessionAdaptor) ExportKeyingMaterial(label string, context []byte, length int) ([]byte, error) {
	return a.session.ExportKeyingMaterial(label, context, length)
}

func (a *sessionAdaptor) SendDatagram(p []byte) error {
	err := a.session.SendDatagram(p)
	if errors.Is(err, ospc.ErrWebTransportNoDatagrams) {
		return webtransport.ErrNotSupported
	}
	return err
}

func (a *sessionAdaptor) ReceiveDatagram(ctx context.Context) ([]byte, error) {
	return a.session.ReceiveDatagram(ctx)
}
