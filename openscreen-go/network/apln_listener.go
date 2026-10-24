package ospc

import (
	"context"
	"crypto/tls"
	"sync"

	"github.com/quic-go/quic-go"
)

type ALPNListenerConfig struct {
	VerifyConnection func(cs tls.ConnectionState) error
}

// ALPNListener allows listening for application protocols
// on the same port as OSP.
type ALPNListener struct {
	parent *Listener
	config *ALPNListenerConfig

	accept    chan quic.Connection
	close     chan struct{}
	closeOnce sync.Once
}

func newALPNListener(
	parent *Listener,
	config *ALPNListenerConfig,
) *ALPNListener {
	return &ALPNListener{
		parent: parent,
		config: config,
		accept: make(chan quic.Connection),
		close:  make(chan struct{}),
	}
}

func (l *ALPNListener) Accept(ctx context.Context) (quic.Connection, error) {
	select {
	case <-l.close:
		return nil, ErrListenerClosed
	case <-l.parent.ctx.Done():
		return nil, ErrListenerClosed
	case <-ctx.Done():
		return nil, ctx.Err()
	case conn := <-l.accept:
		return conn, nil
	}
}

func (l *ALPNListener) doVerifyConnection(cs tls.ConnectionState) error {
	if l.config == nil || l.config.VerifyConnection == nil {
		return nil
	}
	return l.config.VerifyConnection(cs)
}

// dispatch hands conn to Accept. If either listener is closed first, conn
// is closed.
func (l *ALPNListener) dispatch(ctx context.Context, conn quic.Connection) {
	select {
	case l.accept <- conn:
	case <-l.close:
		_ = conn.CloseWithError(QuicErrorCodeNoLongerNeeded, "listener closed")
	case <-ctx.Done():
		_ = conn.CloseWithError(QuicErrorCodeNoLongerNeeded, "listener closed")
	}
}

// Close stops accepting connections for the ALPN. It is idempotent.
func (l *ALPNListener) Close() error {
	l.closeOnce.Do(func() {
		close(l.close)
		l.parent.removeALPNListener(l)
	})
	return nil
}
