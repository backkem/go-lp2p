package lp2p

import (
	"github.com/backkem/go-lp2p/ospc"
)

var userAgent = &mockUserAgent{}

type LP2PReceiverConfig struct {
	Nickname string
}

// LP2PReceiver advertises itself and receives incoming peer connections.
type LP2PReceiver struct {
	config LP2PReceiverConfig
	l      *uaPeerListener
}

// NewLP2Receiver
func NewLP2Receiver(config LP2PReceiverConfig) (*LP2PReceiver, error) {
	return &LP2PReceiver{}, nil
}

// Start advertising and receiving peers.
func (r *LP2PReceiver) Start() error {
	var err error
	r.l, err = userAgent.PeerManager().Listen(r.config.Nickname)
	if err != nil {
		return err
	}

	// TODO: event wiring

	return nil
}

type OnConnectionEvent struct {
	Connection *LP2PConnection
}

func (r *LP2PReceiver) OnConnection(callback func(e OnConnectionEvent)) {
	// TODO: event wiring
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
	_ = userAgent.PeerManager()

	return &LP2PRequest{
		config: config,
	}, nil
}

// Start the request.
func (r *LP2PRequest) Start() (*LP2PConnection, error) {
	pm := userAgent.PeerManager()
	conn, err := pm.PickAndDial(r.config.Nickname)

	return &LP2PConnection{
		conn: conn,
	}, err
}

// LP2PConnection
type LP2PConnection struct {
	// TODO: Rethink attributes
	// PeerId   string
	// Nickname string

	conn *ospc.Connection
}
