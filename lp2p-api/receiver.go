package lp2p

import (
	"context"
	"fmt"
	"sync"

	ua "github.com/backkem/go-lp2p/lp2p-api/useragent"
)

// LP2PReceiverOptions configures an LP2PReceiver.
type LP2PReceiverOptions struct {
	// Nickname the device is advertised under.
	Nickname string
	// AllowedOrigins are URLs whose origins, in addition to the page's own
	// origin, the receiver accepts sessions requested by. By default only
	// same-origin sessions are delivered. An entry that doesn't parse makes
	// the constructor fail with a TypeError; one with an opaque origin,
	// such as a data: URL, never matches.
	AllowedOrigins []string
}

type receiverState int

const (
	receiverInitial receiverState = iota
	receiverAdvertising
	receiverStopped
)

// LP2PReceiver makes the device discoverable and receives connections from
// peers. It is used once: after Stop it cannot be started again.
type LP2PReceiver struct {
	window         *Window
	options        LP2PReceiverOptions
	allowedOrigins []string

	onConnection eventSlot[LP2PConnectionEvent]

	startMu sync.Mutex // held while starting
	mu      sync.Mutex
	state   receiverState
	advert  *ua.Advertisement
	cancel  context.CancelFunc
}

// NewLP2PReceiver creates an LP2PReceiver in this window. It fails with a
// TypeError if an entry of AllowedOrigins doesn't parse as a URL.
func (w *Window) NewLP2PReceiver(options LP2PReceiverOptions) (*LP2PReceiver, error) {
	r := &LP2PReceiver{window: w, options: options}
	for _, entry := range options.AllowedOrigins {
		origin, err := ua.OriginOf(entry)
		if err != nil {
			return nil, newDOMException(TypeError, fmt.Sprintf("allowedOrigins: %q is not a URL", entry), err)
		}
		r.allowedOrigins = append(r.allowedOrigins, origin)
	}
	return r, nil
}

// OnConnection sets the onconnection event handler.
func (r *LP2PReceiver) OnConnection(cb func(LP2PConnectionEvent)) { r.onConnection.set(cb) }

// Start asks the user for permission to advertise and, once granted,
// advertises the device and receives connections. It blocks until the user
// decided. Calling Start on a started receiver does nothing.
//
// It fails with a NotAllowedError DOMException if the user denies or
// advertising fails, an InvalidStateError after Stop, and an AbortError if
// Stop is called while the user decides.
func (r *LP2PReceiver) Start() error {
	r.startMu.Lock()
	defer r.startMu.Unlock()

	r.mu.Lock()
	state := r.state
	r.mu.Unlock()
	switch state {
	case receiverStopped:
		return newDOMException(InvalidStateError, "LP2PReceiver is stopped", nil)
	case receiverAdvertising:
		return nil
	}

	u := r.window.UserAgent
	advert, err := u.Advertise(context.Background(), ua.AdvertiseOptions{
		Origin:         r.window.Origin,
		Nickname:       r.options.Nickname,
		AllowedOrigins: r.allowedOrigins,
	})
	if err != nil {
		return newDOMException(NotAllowedError, "advertising not allowed: "+err.Error(), err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	r.mu.Lock()
	if r.state == receiverStopped { // stopped while the user was deciding
		r.mu.Unlock()
		cancel()
		_ = advert.Close()
		return newDOMException(AbortError, "LP2PReceiver stopped while starting", nil)
	}
	r.state = receiverAdvertising
	r.advert, r.cancel = advert, cancel
	r.mu.Unlock()

	go r.acceptConnections(ctx, advert)
	return nil
}

func (r *LP2PReceiver) acceptConnections(ctx context.Context, advert *ua.Advertisement) {
	for {
		conn, err := advert.Accept(ctx)
		if err != nil {
			return
		}
		c := newLP2PConnection(conn)
		r.onConnection.fire(LP2PConnectionEvent{Connection: c})
		c.expose()
	}
}

// Stop stops advertising and receiving connections. Connections already
// delivered stay open. A stopped receiver cannot be started again.
func (r *LP2PReceiver) Stop() {
	r.mu.Lock()
	if r.state == receiverStopped {
		r.mu.Unlock()
		return
	}
	r.state = receiverStopped
	advert, cancel := r.advert, r.cancel
	r.advert, r.cancel = nil, nil
	r.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if advert != nil {
		_ = advert.Close()
	}
}
