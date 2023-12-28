package lp2p

import (
	"context"
	"sync"

	"github.com/backkem/go-lp2p/ospc"
	"github.com/backkem/go-lp2p/web-api"
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
	config       LP2PReceiverConfig
	peerListener *uaPeerListener

	OnConnection        web.CallbackSetter[OnConnectionEvent]
	onConnectionHandler *web.EventHandler[OnConnectionEvent]

	OnTransport        web.CallbackSetter[OnTransportEvent]
	onTransportHandler *web.EventHandler[OnTransportEvent]
}

// NewLP2Receiver
func NewLP2Receiver(config LP2PReceiverConfig) (*LP2PReceiver, error) {
	onConnectionHandler := web.NewEventHandler[OnConnectionEvent]()
	onTransportHandler := web.NewEventHandler[OnTransportEvent]()
	return &LP2PReceiver{
		config:              config,
		OnConnection:        onConnectionHandler.SetCallback,
		onConnectionHandler: onConnectionHandler,
		OnTransport:         onTransportHandler.SetCallback,
		onTransportHandler:  onTransportHandler,
	}, nil
}

// Start advertising and receiving peers.
func (r *LP2PReceiver) Start() error {
	var err error
	r.peerListener, err = DefaultUserAgent.PeerManager().ListenConnection(r.config.Nickname)
	if err != nil {
		return err
	}

	r.run()

	return nil
}

func (r *LP2PReceiver) run() error {
	// Connection
	go func() {
		for {
			oConn, err := r.peerListener.AcceptConnection(context.Background())
			if err != nil {
				return
			}

			conn := &LP2PConnection{
				conn: oConn,
			}

			r.peerListener.listenPooledTransport(conn)
			conn.run()

			r.onConnectionHandler.OnCallback(OnConnectionEvent{
				Connection: conn,
			})
		}
	}()

	// Transport
	go func() {
		for {
			t, err := r.peerListener.AcceptTransport(context.Background())
			if err != nil {
				return
			}

			r.onTransportHandler.OnCallback(OnTransportEvent{
				Transport: t,
			})
		}
	}()

	return nil
}

type OnConnectionEvent struct {
	Connection *LP2PConnection
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

type OnTransportEvent struct {
	Transport *LP2PQuicTransport
}
