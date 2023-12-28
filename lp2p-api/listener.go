package lp2p

import (
	"context"
	"errors"
	"fmt"

	"github.com/backkem/go-lp2p/ospc"
	"github.com/backkem/go-lp2p/webtransport-api"
)

type uaPeerListener struct {
	m            *uaPeerManager
	connListener *ospc.Listener

	acceptTransport chan *LP2PQuicTransport
	close           chan struct{}
}

// Listen starts the OSPC listener
func (m *uaPeerManager) ListenConnection(nickname string) (*uaPeerListener, error) {
	c := ospc.AgentConfig{
		Nickname: nickname,
	}

	err := m.consentListen(nickname)
	if err != nil {
		return nil, err
	}

	listener, err := ospc.Listen(c)
	if err != nil {
		return nil, fmt.Errorf("failed to listen: %s", err)
	}

	return &uaPeerListener{
		m:               m,
		connListener:    listener,
		acceptTransport: make(chan *LP2PQuicTransport),
		close:           make(chan struct{}),
	}, nil
}

// AcceptConnection new connections
func (l *uaPeerListener) AcceptConnection(ctx context.Context) (*ospc.Connection, error) {
	uConn, err := l.connListener.Accept(ctx)
	if err != nil {
		return nil, err
	}
	defer uConn.Close() // Cleanup of not authenticated

	err = l.m.consentAccept(uConn.RemoteConfig().Nickname)
	if err != nil {
		return nil, err
	}

	conn, err := l.m.authenticatePSK(ctx, uConn)
	if err != nil {
		return nil, err
	}

	return conn, nil
}

// Close the listener
func (l *uaPeerListener) Close() error {
	return l.connListener.Close()
}

func (m *uaPeerManager) dial(ctx context.Context, agent *ospc.RemoteAgent, localNickname string) (*ospc.Connection, error) {
	uConn, err := agent.Dial(context.Background(),
		ospc.AgentConfig{
			Nickname: localNickname,
		})
	if err != nil {
		return nil, err
	}
	defer uConn.Close() // Cleanup of not authenticated

	conn, err := m.authenticatePSK(ctx, uConn)
	if err != nil {
		return nil, err
	}

	return conn, nil
}

func (l *uaPeerListener) listenPooledTransport(c *LP2PConnection) {
	go func() {
	listenLoop:
		for {
			t, err := c.conn.AcceptTransport(context.Background())
			if err != nil {
				return
			}

			s := webtransport.NewSessionAdaptor(t)

			tp, err := createLP2PQuicTransport(s,
				LP2PWebTransportOptions{
					AllowPooling: true,
				})
			if err != nil {
				return
			}

			close := l.close
			accept := l.acceptTransport

			select {
			case <-close:
			case accept <- tp:
				break listenLoop
			}
		}
	}()
}

// AcceptTransport new Transports
func (l *uaPeerListener) AcceptTransport(ctx context.Context) (*LP2PQuicTransport, error) {
	acceptCh := l.acceptTransport
	closeCh := l.close

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case a := <-acceptCh:
		return a, nil
	case <-closeCh:
		return nil, errors.New("closed")
	}
}
