package lp2p

import (
	"context"
	"sync"

	ua "github.com/backkem/go-lp2p/lp2p-api/useragent"
)

// LP2PReceiverOptions configures an LP2PReceiver.
type LP2PReceiverOptions struct {
	// Nickname the device is advertised under.
	Nickname string
}

// LP2PReceiver makes the device discoverable and receives connections from
// peers.
type LP2PReceiver struct {
	window  *Window
	options LP2PReceiverOptions

	onConnection eventSlot[LP2PConnectionEvent]

	startMu sync.Mutex // held while starting
	mu      sync.Mutex
	advert  *ua.Advertisement
	closed  bool
	cancel  context.CancelFunc
	// listener receives transports on the connections this receiver
	// accepts.
	listener *LP2PQuicTransportListener
}

// NewLP2PReceiver creates an LP2PReceiver in this window.
func (w *Window) NewLP2PReceiver(options LP2PReceiverOptions) (*LP2PReceiver, error) {
	return &LP2PReceiver{window: w, options: options}, nil
}

// OnConnection sets the onconnection event handler.
func (r *LP2PReceiver) OnConnection(cb func(LP2PConnectionEvent)) { r.onConnection.set(cb) }

// Start asks the user for permission to advertise and, once granted,
// advertises the device and receives connections. It blocks until the user
// decided. Calling Start on a started receiver does nothing.
//
// It fails with a NotAllowedError DOMException if the user denies.
func (r *LP2PReceiver) Start() error {
	r.startMu.Lock()
	defer r.startMu.Unlock()

	r.mu.Lock()
	started, closed := r.advert != nil, r.closed
	r.mu.Unlock()
	if closed {
		return errClosed("LP2PReceiver")
	}
	if started {
		return nil
	}

	u := r.window.UserAgent
	advert, err := u.Advertise(context.Background(), r.window.Origin, r.options.Nickname)
	if err != nil {
		return toDOMException(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	r.mu.Lock()
	if r.closed { // closed while the user was deciding
		r.mu.Unlock()
		cancel()
		_ = advert.Close()
		return errClosed("LP2PReceiver")
	}
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
		c := newLP2PConnection(conn, r.transportListener)
		r.onConnection.fire(LP2PConnectionEvent{Connection: c})
	}
}

// Close stops advertising and receiving connections. Connections already
// received stay open. Go-only: the proposal has no way to stop a receiver
// yet.
func (r *LP2PReceiver) Close() error {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil
	}
	r.closed = true
	advert, cancel := r.advert, r.cancel
	r.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if advert != nil {
		return advert.Close()
	}
	return nil
}

func (r *LP2PReceiver) transportListener() *LP2PQuicTransportListener {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.listener
}

func (r *LP2PReceiver) registerLP2PQuicTransportListener(l *LP2PQuicTransportListener) error {
	r.mu.Lock()
	r.listener = l
	r.mu.Unlock()
	// Listening for transports implies receiving connections.
	return r.Start()
}
