package lp2p

import (
	"context"

	"github.com/backkem/go-lp2p/webtransport-api"
)

type LP2PQuicTransport struct {
	*webtransport.Transport

	opts LP2PWebTransportOptions
}

type LP2PWebTransportOptions struct {
	// AllowPooling when set to true, the WebTransport session
	// can be pooled, that is, its underlying connection can be
	// shared with other WebTransport sessions or the LP2P
	// transport.
	AllowPooling bool
}

// NewLP2PQuicTransport
func (r *LP2PRequest) NewLP2PQuicTransport(options LP2PWebTransportOptions) (*LP2PQuicTransport, error) {
	// Ensure request is started
	// TODO: avoid double start
	c, err := r.Start()
	if err != nil {
		return nil, err
	}

	return c.NewLP2PQuicTransport(options)
}

// NewLP2PQuicTransport
func (c *LP2PConnection) NewLP2PQuicTransport(options LP2PWebTransportOptions) (*LP2PQuicTransport, error) {
	var t webtransport.Session

	if options.AllowPooling {
		s, err := c.conn.NewTransport(context.Background())
		if err != nil {
			return nil, err
		}
		t = webtransport.NewSessionAdaptor(s)
	} else {
		// TODO: Transport over dedicated Quic conn
		panic("todo")
	}

	return createLP2PQuicTransport(t, options)
}

func createLP2PQuicTransport(s webtransport.Session, options LP2PWebTransportOptions) (*LP2PQuicTransport, error) {
	conn, err := webtransport.NewTransport(s)
	if err != nil {
		return nil, err
	}
	return &LP2PQuicTransport{
		opts:      options,
		Transport: conn,
	}, nil
}
