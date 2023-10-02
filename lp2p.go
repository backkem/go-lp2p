package lp2p

import (
	"context"
	"sync"

	"github.com/backkem/go-lp2p/ospc"
)

var DefaultUserAgent = &mockUserAgent{
	Consumer:  CLICollector,
	Presenter: CLIPresenter,
}

type LP2PReceiverConfig struct {
	Nickname string
}

// LP2PReceiver advertises itself and receives incoming peer connections.
type LP2PReceiver struct {
	config LP2PReceiverConfig
	l      *uaPeerListener

	mu sync.Mutex

	cbOnConnection func(e OnConnectionEvent)
}

// NewLP2Receiver
func NewLP2Receiver(config LP2PReceiverConfig) (*LP2PReceiver, error) {
	return &LP2PReceiver{
		config: config,
	}, nil
}

// Start advertising and receiving peers.
func (r *LP2PReceiver) Start() error {
	var err error
	r.l, err = DefaultUserAgent.PeerManager().Listen(r.config.Nickname)
	if err != nil {
		return err
	}

	r.run()

	return nil
}

func (r *LP2PReceiver) run() error {
	go func() {
		for {
			oConn, err := r.l.Accept(context.Background())
			if err != nil {
				return
			}

			conn := &LP2PConnection{
				mu:   sync.Mutex{},
				conn: oConn,
			}

			conn.run()

			r.onConnection(conn)
		}
	}()

	return nil
}

type OnConnectionEvent struct {
	Connection *LP2PConnection
}

func (r *LP2PReceiver) OnConnection(callback func(e OnConnectionEvent)) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.cbOnConnection = callback
}

func (r *LP2PReceiver) onConnection(connection *LP2PConnection) {
	e := OnConnectionEvent{
		Connection: connection,
	}
	r.mu.Lock()
	cb := r.cbOnConnection
	r.mu.Unlock()

	cb(e)
}

type LP2PRequestConfig struct {
	Nickname string
	// TODO: Some device type filters (name/type/PeerID)
}

// LP2PRequest
// Rename to LP2PConnectionRequest?
type LP2PRequest struct {
	config LP2PRequestConfig
}

// NewLP2PRequest
func NewLP2PRequest(config LP2PRequestConfig) (*LP2PRequest, error) {
	// Discover early
	_ = DefaultUserAgent.PeerManager()

	return &LP2PRequest{
		config: config,
	}, nil
}

// Start the request.
func (r *LP2PRequest) Start() (*LP2PConnection, error) {
	pm := DefaultUserAgent.PeerManager()
	oConn, err := pm.PickAndDial(r.config.Nickname)
	if err != nil {
		return nil, err
	}

	conn := &LP2PConnection{
		mu:   sync.Mutex{},
		conn: oConn,
	}

	conn.run()

	return conn, nil
}

// LP2PConnection
type LP2PConnection struct {
	// TODO: Rethink attributes
	// PeerId   string
	// Nickname string

	conn *ospc.Connection

	mu              sync.Mutex
	cbOnDataChannel func(e OnDataChannelEvent)
}
