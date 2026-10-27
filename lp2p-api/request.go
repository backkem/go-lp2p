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

	mu      sync.Mutex
	pending bool
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
// pairing with it if needed. It blocks until the peer accepted the session
// or something failed. After a failure, Start can be called again.
//
// Every failure is a NotAllowedError DOMException, so that a page can't
// tell whether the user dismissed the picker, pairing failed or the peer
// refused the session; the user agent's reason is available with errors.Is
// (ua.ErrCanceled, ua.ErrAuthenticationFailed, ua.ErrRefused, ...).
func (r *LP2PRequest) Start() (*LP2PConnection, error) {
	r.mu.Lock()
	if r.pending {
		r.mu.Unlock()
		return nil, newDOMException(InvalidStateError, "LP2PRequest.Start is pending", nil)
	}
	r.pending = true
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		r.pending = false
		r.mu.Unlock()
	}()

	u := r.window.UserAgent
	conn, err := u.Connect(context.Background(), r.window.Origin, r.options.Nickname)
	if err != nil {
		return nil, newDOMException(NotAllowedError, "connection not allowed: "+err.Error(), err)
	}
	c := newLP2PConnection(conn)
	c.expose()
	return c, nil
}
