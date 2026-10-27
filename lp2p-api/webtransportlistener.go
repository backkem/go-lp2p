package lp2p

import (
	"context"
	"fmt"
	"sync"

	ospc "github.com/backkem/go-lp2p/openscreen-go/network"
	"github.com/backkem/go-lp2p/streams-api"
	"github.com/backkem/go-lp2p/webtransport-api"
)

// LP2PQuicTransportListenerInit configures an LP2PQuicTransportListener.
type LP2PQuicTransportListenerInit struct{}

// LP2PQuicTransportListener receives the transports peers open.
type LP2PQuicTransportListener struct {
	// IncomingTransports yields every transport a peer opens. Reading it
	// reports done once the listener is closed.
	IncomingTransports streams.ReadableStream[*LP2PQuicTransport]

	queue chan *LP2PQuicTransport
	done  chan struct{}
	once  sync.Once
}

// transportSource is implemented by the objects a listener can listen on.
type transportSource interface {
	registerLP2PQuicTransportListener(listener *LP2PQuicTransportListener) error
}

var (
	_ transportSource = (*LP2PReceiver)(nil)
	_ transportSource = (*LP2PRequest)(nil)
	_ transportSource = (*LP2PConnection)(nil)
)

// NewLP2PQuicTransportListener listens for transports opened by peers on the
// connections of source: an *LP2PReceiver (which is started if needed), an
// *LP2PRequest or a single *LP2PConnection. A newer listener on the same
// source replaces the older one.
func NewLP2PQuicTransportListener(source any, init LP2PQuicTransportListenerInit) (*LP2PQuicTransportListener, error) {
	src, ok := source.(transportSource)
	if !ok {
		return nil, newDOMException(NotSupportedError,
			fmt.Sprintf("can't listen for transports on %T", source), nil)
	}
	l := &LP2PQuicTransportListener{
		queue: make(chan *LP2PQuicTransport),
		done:  make(chan struct{}),
	}
	l.IncomingTransports = streams.NewValueReadableStream[*LP2PQuicTransport](transportReader{l})
	if err := src.registerLP2PQuicTransportListener(l); err != nil {
		l.Close()
		return nil, err
	}
	return l, nil
}

// Ready blocks until the listener is ready, as the ready promise. Listeners
// are returned ready, so it returns right away.
func (l *LP2PQuicTransportListener) Ready() error { return nil }

// Close stops listening: IncomingTransports reports done and transports
// opened by peers from now on are refused. Go-only.
func (l *LP2PQuicTransportListener) Close() error {
	l.once.Do(func() { close(l.done) })
	return nil
}

// deliver hands an incoming transport to the reader. It reports false if the
// listener is closed or ctx is done before anyone read it.
func (l *LP2PQuicTransportListener) deliver(ctx context.Context, pt *ospc.WebTransportSession) bool {
	t, err := newLP2PQuicTransport(webtransport.NewSessionAdaptor(sessionAdaptor{pt}), LP2PQuicTransportInit{AllowPooling: true})
	if err != nil {
		return false
	}
	select {
	case l.queue <- t:
		return true
	case <-l.done:
	case <-ctx.Done():
	}
	return false
}

type transportReader struct{ l *LP2PQuicTransportListener }

func (r transportReader) Read() (*LP2PQuicTransport, error) {
	select {
	case t := <-r.l.queue:
		return t, nil
	case <-r.l.done:
		return nil, streams.ErrValueReaderDone
	}
}
