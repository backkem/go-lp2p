package lp2p

import (
	"context"
	"sync"
	"time"

	ua "github.com/backkem/go-lp2p/lp2p-api/useragent"
	ospc "github.com/backkem/go-lp2p/openscreen-go/network"
)

// listenerGrace is how long an incoming transport waits for the page to
// construct an LP2PQuicTransportListener before it is refused. In a browser
// the task that exposes the connection runs to completion before incoming
// sessions are handled; Go has no such ordering, so a listener constructed
// right after start() returns or in the connection event handler is given
// this long. Go-only.
const listenerGrace = 100 * time.Millisecond

// LP2PConnection is an authenticated connection to a peer, the user granted
// to the page.
type LP2PConnection struct {
	conn *ua.Connection

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	onDataChannel eventSlot[LP2PDataChannelEvent]
	onClose       eventSlot[OnCloseEvent]

	// exposed is closed once the connection is handed to the page: when
	// LP2PRequest.Start returns or the receiver's connection event fired.
	// Incoming transports are held until then.
	exposed    chan struct{}
	exposeOnce sync.Once

	mu       sync.Mutex
	closed   bool
	finalize sync.Once
	// listener is the connection's [[Listener]]: at most one at a time.
	listener *LP2PQuicTransportListener
	// listenerSet is closed and replaced whenever a listener is set.
	listenerSet chan struct{}
	// transports is the connection's [[Transports]].
	transports map[*LP2PQuicTransport]struct{}
}

// LP2PConnectionEvent is fired at an LP2PReceiver (onconnection) for every
// connection it receives.
type LP2PConnectionEvent struct {
	Connection *LP2PConnection
}

func newLP2PConnection(conn *ua.Connection) *LP2PConnection {
	ctx, cancel := context.WithCancel(context.Background())
	c := &LP2PConnection{
		conn:        conn,
		ctx:         ctx,
		cancel:      cancel,
		exposed:     make(chan struct{}),
		listenerSet: make(chan struct{}),
		transports:  make(map[*LP2PQuicTransport]struct{}),
	}
	c.wg.Add(2)
	go c.acceptDataChannels()
	go c.acceptTransports()
	go func() {
		// The peer closed, or the transport failed.
		<-conn.Done()
		c.doFinalize()
	}()
	return c
}

// expose marks the connection as handed to the page.
func (c *LP2PConnection) expose() {
	c.exposeOnce.Do(func() { close(c.exposed) })
}

// OnDataChannel sets the ondatachannel event handler.
func (c *LP2PConnection) OnDataChannel(cb func(LP2PDataChannelEvent)) { c.onDataChannel.set(cb) }

// OnClose sets the onclose event handler, fired once when the connection is
// closed: by Close, by the peer, or because the transport failed.
func (c *LP2PConnection) OnClose(cb func(OnCloseEvent)) { c.onClose.set(cb) }

// Close the connection, its data channels and its transports. Transports
// fail with a WebTransportError and the listener is closed before onclose
// fires. Closing a closed connection does nothing.
func (c *LP2PConnection) Close() error {
	c.mu.Lock()
	closed := c.closed
	c.mu.Unlock()
	if closed {
		return nil
	}
	err := c.conn.Close()
	c.doFinalize()
	return err
}

// doFinalize runs the connection's finalize steps once.
func (c *LP2PConnection) doFinalize() {
	c.finalize.Do(func() {
		c.mu.Lock()
		c.closed = true
		listener := c.listener
		transports := make([]*LP2PQuicTransport, 0, len(c.transports))
		for t := range c.transports {
			transports = append(transports, t)
		}
		c.transports = nil
		c.mu.Unlock()

		c.cancel()
		c.expose()
		for _, t := range transports {
			t.fail(errClosed("LP2PConnection"))
		}
		if listener != nil {
			listener.Close()
		}
		go func() {
			c.wg.Wait()
			c.onClose.fire(OnCloseEvent{})
		}()
	})
}

func (c *LP2PConnection) isClosed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closed
}

func (c *LP2PConnection) acceptDataChannels() {
	defer c.wg.Done()
	for {
		oDc, err := c.conn.AcceptDataChannel(c.ctx)
		if err != nil {
			return
		}
		dc := c.adoptDataChannel(oDc, oDc.DataChannelParameters)
		c.onDataChannel.fire(LP2PDataChannelEvent{Channel: dc})
	}
}

// acceptTransports handles the peer's incoming WebTransport sessions, see
// index.bs "incoming WebTransport session": held until the connection is
// exposed, then refused unless a listener with room exists.
func (c *LP2PConnection) acceptTransports() {
	defer c.wg.Done()
	select {
	case <-c.exposed:
	case <-c.ctx.Done():
		return
	}
	for {
		inc, err := c.conn.AcceptWebTransport(c.ctx)
		if err != nil {
			return
		}
		l := c.awaitListener()
		if l == nil || (l.init.RequireUnreliable && !c.conn.SupportsDatagrams()) || !l.reserve() {
			_ = inc.Refuse()
			continue
		}
		s, err := inc.Accept()
		if err != nil {
			l.release()
			continue
		}
		t := newLP2PQuicTransport(c, s, l.init.LP2PQuicTransportInit)
		if !c.addTransport(t) {
			l.release()
			t.fail(errClosed("LP2PConnection"))
			continue
		}
		l.enqueue(t)
	}
}

// awaitListener returns the connection's listener, waiting a little for one
// to be constructed, see listenerGrace.
func (c *LP2PConnection) awaitListener() *LP2PQuicTransportListener {
	c.mu.Lock()
	l, set := c.listener, c.listenerSet
	c.mu.Unlock()
	if l != nil {
		return l
	}
	select {
	case <-set:
	case <-time.After(listenerGrace):
	case <-c.ctx.Done():
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.listener
}

// addTransport appends t to [[Transports]]. It reports false if the
// connection is closed.
func (c *LP2PConnection) addTransport(t *LP2PQuicTransport) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return false
	}
	c.transports[t] = struct{}{}
	return true
}

func (c *LP2PConnection) removeTransport(t *LP2PQuicTransport) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.transports, t)
}

// setListener sets [[Listener]]. It fails with an InvalidStateError if the
// connection is closed or already has one.
func (c *LP2PConnection) setListener(l *LP2PQuicTransportListener) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return errClosed("LP2PConnection")
	}
	if c.listener != nil {
		return newDOMException(InvalidStateError, "LP2PConnection already has an LP2PQuicTransportListener", nil)
	}
	c.listener = l
	close(c.listenerSet)
	c.listenerSet = make(chan struct{})
	return nil
}

func (c *LP2PConnection) clearListener(l *LP2PQuicTransportListener) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.listener == l {
		c.listener = nil
	}
}

// ospcConnection returns the underlying ospc connection, for the examples
// and tests. Go-only.
func (c *LP2PConnection) ospcConnection() *ospc.Connection { return c.conn.Connection }
