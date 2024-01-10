package ospc

import (
	"context"
	"crypto/tls"

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

	accept chan quic.Connection
	close  chan struct{}
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
	close := l.close
	accept := l.accept

	select {
	case <-close:
		return nil, ErrListenerClosed
	case <-ctx.Done():
		return nil, ctx.Err()
	case conn := <-accept:
		return conn, nil
	}
}

func (l *ALPNListener) doVerifyConnection(cs tls.ConnectionState) error {
	if l.config == nil || l.config.VerifyConnection == nil {
		return nil
	}
	return l.config.VerifyConnection(cs)
}

func (l *ALPNListener) dispatch(conn quic.Connection) {
	close := l.close
	accept := l.accept

	select {
	case accept <- conn:
	case <-close:
	}
}

// func (l *Listener) Addr() net.Addr {
// 	return l.parent.Addr()
// }

func (l *ALPNListener) Close() error {
	close(l.close)
	l.parent.removeALPNListener(l)
	return nil
}
