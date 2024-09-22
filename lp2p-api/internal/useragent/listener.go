package ua

import (
	"context"
	"fmt"

	"github.com/backkem/go-lp2p/ospc"
)

type PeerListener struct {
	m            *ConnectionManager
	connListener *ospc.Listener

	close chan struct{}
}

// Listen starts the OSPC listener
func (m *ConnectionManager) ListenConnection(nickname string) (*PeerListener, error) {
	c := ospc.AgentConfig{
		DisplayName: nickname,
	}
	a, err := ospc.NewAgent(c)
	if err != nil {
		return nil, err
	}

	err = m.consentListen(nickname)
	if err != nil {
		return nil, err
	}

	listener, err := ospc.Listen(ospc.AgentTransportQUIC, a)
	if err != nil {
		return nil, fmt.Errorf("failed to listen: %s", err)
	}

	return &PeerListener{
		m:            m,
		connListener: listener,
		close:        make(chan struct{}),
	}, nil
}

// AcceptConnection new connections
func (l *PeerListener) AcceptConnection(ctx context.Context) (*ospc.Connection, error) {
	uConn, err := l.connListener.Accept(ctx)
	if err != nil {
		return nil, err
	}
	defer uConn.Close() // Cleanup of not authenticated

	err = l.m.consentAccept(uConn.RemoteAgent().Info().DisplayName)
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
func (l *PeerListener) Close() error {
	return l.connListener.Close()
}

func (m *ConnectionManager) dial(ctx context.Context, agent *ospc.DiscoveredAgent, localNickname string) (*ospc.Connection, error) {
	// TODO: manage agent
	c := ospc.AgentConfig{
		DisplayName: localNickname,
	}
	a, err := ospc.NewAgent(c)
	if err != nil {
		return nil, err
	}

	uConn, err := agent.Dial(context.Background(), ospc.AgentTransportQUIC, a)
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
