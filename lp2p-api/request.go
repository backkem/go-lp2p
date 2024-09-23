package lp2p

import "sync"

type LP2PReceiverConfig struct {
	Nickname string
}

type LP2PRequestConfig struct {
	Nickname string
	// TODO: Some device type filters (name/type/PeerID)
}

// LP2PRequest
// Rename to LP2PConnectionRequest?
type LP2PRequest struct {
	config LP2PRequestConfig

	mu                sync.Mutex
	transportListener *LP2PQuicTransportListener
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

	r.mu.Lock()
	transportListener := r.transportListener
	r.mu.Unlock()

	conn := newLP2PConnection(oConn)
	conn.run(transportListener)

	return conn, nil
}

func (r *LP2PRequest) registerLP2PQuicTransportListener(listener *LP2PQuicTransportListener) error {
	r.mu.Lock()
	r.transportListener = listener
	r.mu.Unlock()

	// Ensure LP2PRequest is started
	_, err := r.Start()
	return err
}
