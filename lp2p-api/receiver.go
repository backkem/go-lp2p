package lp2p

import (
	"context"
	"sync"

	ua "github.com/backkem/go-lp2p/lp2p-api/internal/useragent"
	"github.com/backkem/go-lp2p/web-api"
)

// LP2PReceiver advertises itself and receives incoming peer connections.
type LP2PReceiver struct {
	config       LP2PReceiverConfig
	peerListener *ua.PeerListener

	OnConnection        web.CallbackSetter[OnConnectionEvent]
	onConnectionHandler *web.EventHandler[OnConnectionEvent]

	mu                sync.Mutex
	transportListener *LP2PQuicTransportListener
}

// NewLP2Receiver
func NewLP2Receiver(config LP2PReceiverConfig) (*LP2PReceiver, error) {
	onConnectionHandler := web.NewEventHandler[OnConnectionEvent]()
	return &LP2PReceiver{
		config:              config,
		OnConnection:        onConnectionHandler.SetCallback,
		onConnectionHandler: onConnectionHandler,
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

			r.mu.Lock()
			transportListener := r.transportListener
			r.mu.Unlock()

			conn := newLP2PConnection(oConn)
			conn.run(transportListener)

			r.onConnectionHandler.OnCallback(OnConnectionEvent{
				Connection: conn,
			})
		}
	}()

	return nil
}

func (r *LP2PReceiver) registerLP2PQuicTransportListener(listener *LP2PQuicTransportListener) error {
	r.mu.Lock()
	r.transportListener = listener
	r.mu.Unlock()

	// Ensure LP2PReceiver is started
	return r.Start()
}
