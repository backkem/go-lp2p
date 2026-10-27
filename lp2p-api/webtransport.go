package lp2p

import (
	"fmt"

	ospc "github.com/backkem/go-lp2p/openscreen-go/network"
	"github.com/backkem/go-lp2p/webtransport-api"
)

// LP2PQuicTransportInit configures an LP2PQuicTransport.
type LP2PQuicTransportInit struct {
	// AllowPooling lets the transport share the peer connection with data
	// channels and other transports, as WebTransportOptions.allowPooling.
	// Only pooled transports are implemented so far.
	AllowPooling bool
}

// LP2PQuicTransport is a WebTransport to a peer.
type LP2PQuicTransport struct {
	*webtransport.Transport

	init LP2PQuicTransportInit
	// conn is closed with the transport when the transport created it.
	conn *LP2PConnection
}

// NewLP2PQuicTransport opens a transport to a peer. The source is either
//   - an *LP2PRequest: the user picks a peer and a connection is made for
//     the transport (closed with it), or
//   - an *LP2PConnection: the transport is opened on that connection.
//
// It blocks until the transport is ready.
func NewLP2PQuicTransport(source any, init LP2PQuicTransportInit) (*LP2PQuicTransport, error) {
	if !init.AllowPooling {
		return nil, newDOMException(NotSupportedError,
			"dedicated LP2PQuicTransports are not implemented, set AllowPooling", nil)
	}
	switch s := source.(type) {
	case *LP2PConnection:
		return openTransport(s, init, false)
	case *LP2PRequest:
		conn, err := s.Start()
		if err != nil {
			return nil, err
		}
		t, err := openTransport(conn, init, true)
		if err != nil {
			_ = conn.Close()
		}
		return t, err
	default:
		return nil, newDOMException(NotSupportedError,
			fmt.Sprintf("can't open an LP2PQuicTransport from %T", source), nil)
	}
}

func openTransport(c *LP2PConnection, init LP2PQuicTransportInit, owned bool) (*LP2PQuicTransport, error) {
	if c.isClosed() {
		return nil, errClosed("LP2PConnection")
	}
	s, err := c.conn.OpenWebTransport(c.ctx)
	if err != nil {
		return nil, toDOMException(err)
	}
	t, err := newLP2PQuicTransport(webtransport.NewSessionAdaptor(sessionAdaptor{s}), init)
	if err != nil {
		return nil, err
	}
	if owned {
		t.conn = c
	}
	return t, nil
}

func newLP2PQuicTransport(s webtransport.Session, init LP2PQuicTransportInit) (*LP2PQuicTransport, error) {
	t, err := webtransport.NewTransport(s)
	if err != nil {
		return nil, toDOMException(err)
	}
	return &LP2PQuicTransport{Transport: t, init: init}, nil
}

// Ready blocks until the transport is ready, as the ready promise. Transports
// are returned ready, so it returns right away.
func (t *LP2PQuicTransport) Ready() error { return nil }

// Close the transport, and the connection if the transport made it.
func (t *LP2PQuicTransport) Close(info webtransport.WebTransportCloseInfo) error {
	err := t.Transport.Close(info)
	if t.conn != nil {
		if cerr := t.conn.Close(); err == nil {
			err = cerr
		}
	}
	return err
}

// sessionAdaptor adapts an ospc session to webtransport.ConcreteSession.
type sessionAdaptor struct{ *ospc.WebTransportSession }

func (a sessionAdaptor) CloseWithError(code uint64, reason string) error {
	return a.Close(ospc.WebTransportCloseInfo{CloseCode: uint32(code), Reason: reason})
}
