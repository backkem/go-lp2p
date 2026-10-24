package lp2p

import (
	"context"
	"sync"
)

// LP2PRequestOptions configures an LP2PRequest.
type LP2PRequestOptions struct {
	// Nickname the page presents itself with to the peer.
	Nickname string
}

// LP2PRequest requests a connection to a peer the user picks.
type LP2PRequest struct {
	window  *Window
	options LP2PRequestOptions

	mu sync.Mutex
	// listener receives transports on the connections this request makes.
	listener *LP2PQuicTransportListener
}

// NewLP2PRequest creates an LP2PRequest in this window. The user agent
// starts discovering peers right away, so the picker is populated by the
// time Start is called.
func (w *Window) NewLP2PRequest(options LP2PRequestOptions) (*LP2PRequest, error) {
	if err := w.UserAgent.StartDiscovery(); err != nil {
		return nil, toDOMException(err)
	}
	return &LP2PRequest{window: w, options: options}, nil
}

// Start shows the device picker and connects to the peer the user picks,
// pairing with it if needed. It blocks until connected or failed. Every
// call makes a new connection.
//
// It fails with an AbortError DOMException if the user dismisses the picker
// and a NotAllowedError if pairing fails.
func (r *LP2PRequest) Start() (*LP2PConnection, error) {
	u := r.window.UserAgent
	conn, err := u.Connect(context.Background(), r.window.Origin, r.options.Nickname)
	if err != nil {
		return nil, toDOMException(err)
	}
	return newLP2PConnection(conn, r.transportListener), nil
}

func (r *LP2PRequest) transportListener() *LP2PQuicTransportListener {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.listener
}

func (r *LP2PRequest) registerLP2PQuicTransportListener(l *LP2PQuicTransportListener) error {
	r.mu.Lock()
	r.listener = l
	r.mu.Unlock()
	return nil
}
