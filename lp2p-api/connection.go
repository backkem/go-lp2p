package lp2p

import (
	"context"
	"sync"

	ua "github.com/backkem/go-lp2p/lp2p-api/useragent"
)

// LP2PConnection is an authenticated connection to a peer, the user granted
// to the page.
type LP2PConnection struct {
	conn *ua.Connection

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	once   sync.Once

	onDataChannel eventSlot[LP2PDataChannelEvent]
	onClose       eventSlot[OnCloseEvent]

	mu sync.Mutex
	// listener receives the peer's transports: this connection's own
	// listener if set, else the one of the receiver or request it came from.
	listener       *LP2PQuicTransportListener
	parentListener func() *LP2PQuicTransportListener
}

// LP2PConnectionEvent is fired at an LP2PReceiver (onconnection) for every
// connection it receives.
type LP2PConnectionEvent struct {
	Connection *LP2PConnection
}

func newLP2PConnection(conn *ua.Connection, parentListener func() *LP2PQuicTransportListener) *LP2PConnection {
	ctx, cancel := context.WithCancel(context.Background())
	c := &LP2PConnection{
		conn:           conn,
		ctx:            ctx,
		cancel:         cancel,
		parentListener: parentListener,
	}
	c.wg.Add(2)
	go c.acceptDataChannels()
	go c.acceptTransports()
	return c
}

// OnDataChannel sets the ondatachannel event handler.
func (c *LP2PConnection) OnDataChannel(cb func(LP2PDataChannelEvent)) { c.onDataChannel.set(cb) }

// OnClose sets a handler fired once when the connection is closed, locally
// or by the peer. Go-only: the proposal has no connection teardown yet.
func (c *LP2PConnection) OnClose(cb func(OnCloseEvent)) { c.onClose.set(cb) }

// Close the connection, its data channels and transports. It doesn't wait
// for them to wind down, so it is safe to call from event handlers; onclose
// fires once they have. Go-only: the proposal has no connection teardown yet.
func (c *LP2PConnection) Close() error {
	var err error
	c.once.Do(func() {
		c.cancel()
		err = c.conn.Close()
		go func() {
			c.wg.Wait()
			c.onClose.fire(OnCloseEvent{})
		}()
	})
	return err
}

func (c *LP2PConnection) isClosed() bool { return c.ctx.Err() != nil }

func (c *LP2PConnection) acceptDataChannels() {
	defer c.wg.Done()
	for {
		oDc, err := c.conn.AcceptDataChannel(c.ctx)
		if err != nil {
			c.peerGone()
			return
		}
		dc := c.adoptDataChannel(oDc, oDc.DataChannelParameters)
		c.onDataChannel.fire(LP2PDataChannelEvent{Channel: dc})
	}
}

func (c *LP2PConnection) acceptTransports() {
	defer c.wg.Done()
	for {
		t, err := c.conn.AcceptTransport(c.ctx)
		if err != nil {
			c.peerGone()
			return
		}
		l := c.transportListener()
		if l == nil || !l.deliver(c.ctx, t) {
			_ = t.CloseWithError(0, "no LP2PQuicTransportListener")
		}
	}
}

// peerGone closes the connection when the peer went away, so channels and
// waiters are released.
func (c *LP2PConnection) peerGone() {
	_ = c.Close()
}

func (c *LP2PConnection) transportListener() *LP2PQuicTransportListener {
	c.mu.Lock()
	l := c.listener
	c.mu.Unlock()
	if l == nil && c.parentListener != nil {
		l = c.parentListener()
	}
	return l
}

func (c *LP2PConnection) registerLP2PQuicTransportListener(l *LP2PQuicTransportListener) error {
	if c.isClosed() {
		return errClosed("LP2PConnection")
	}
	c.mu.Lock()
	c.listener = l
	c.mu.Unlock()
	return nil
}
