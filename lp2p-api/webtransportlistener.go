package lp2p

import (
	"sync"

	"github.com/backkem/go-lp2p/streams-api"
)

// LP2PQuicTransportListenerInit configures an LP2PQuicTransportListener.
// Its options apply to each transport the listener creates.
type LP2PQuicTransportListenerInit struct {
	LP2PQuicTransportInit
}

// incomingTransportsHighWaterMark is how many incoming transports a
// listener queues while the page isn't reading; further ones are refused.
const incomingTransportsHighWaterMark = 16

// LP2PQuicTransportListener receives the transports a peer opens on an
// LP2PConnection. There is at most one per connection at a time.
type LP2PQuicTransportListener struct {
	// IncomingTransports yields every transport the peer opens, already
	// ready. Reading it reports done once the listener is closed, after
	// the transports queued before.
	IncomingTransports streams.ReadableStream[*LP2PQuicTransport]

	conn *LP2PConnection
	init LP2PQuicTransportListenerInit

	queue chan *LP2PQuicTransport
	done  chan struct{}
	once  sync.Once

	mu       sync.Mutex
	reserved int // Transports accepted but not enqueued yet.
}

// NewLP2PQuicTransportListener listens for transports the peer opens on the
// connection. It fails with an InvalidStateError if the connection is
// closed or already has a listener; after Close, a new one can be
// constructed.
func NewLP2PQuicTransportListener(connection *LP2PConnection, init LP2PQuicTransportListenerInit) (*LP2PQuicTransportListener, error) {
	l := &LP2PQuicTransportListener{
		conn:  connection,
		init:  init,
		queue: make(chan *LP2PQuicTransport, incomingTransportsHighWaterMark),
		done:  make(chan struct{}),
	}
	l.IncomingTransports = streams.NewValueReadableStream[*LP2PQuicTransport](transportReader{l})
	if err := connection.setListener(l); err != nil {
		return nil, err
	}
	return l, nil
}

// Close stops listening: the connection's listener slot is freed,
// IncomingTransports reports done once the queued transports are read, and
// transports the peer opens from now on are refused.
func (l *LP2PQuicTransportListener) Close() {
	l.once.Do(func() {
		l.conn.clearListener(l)
		close(l.done)
	})
}

// reserve claims room in the queue for a transport about to be accepted. It
// reports false if the listener is closed or full.
func (l *LP2PQuicTransportListener) reserve() bool {
	select {
	case <-l.done:
		return false
	default:
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.queue)+l.reserved >= cap(l.queue) {
		return false
	}
	l.reserved++
	return true
}

func (l *LP2PQuicTransportListener) release() {
	l.mu.Lock()
	l.reserved--
	l.mu.Unlock()
}

// enqueue queues a reserved transport.
func (l *LP2PQuicTransportListener) enqueue(t *LP2PQuicTransport) {
	l.mu.Lock()
	l.reserved--
	l.mu.Unlock()
	l.queue <- t // Room was reserved.
}

type transportReader struct{ l *LP2PQuicTransportListener }

func (r transportReader) Read() (*LP2PQuicTransport, error) {
	// Transports queued before Close can still be read.
	select {
	case t := <-r.l.queue:
		return t, nil
	default:
	}
	select {
	case t := <-r.l.queue:
		return t, nil
	case <-r.l.done:
		select {
		case t := <-r.l.queue:
			return t, nil
		default:
			return nil, streams.ErrValueReaderDone
		}
	}
}
