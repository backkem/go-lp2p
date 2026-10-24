package cast

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

var (
	// ErrChannelClosed is returned by operations on a closed Channel.
	ErrChannelClosed = errors.New("cast: channel closed")
	// ErrHeartbeatTimeout closes a Channel whose peer stopped sending.
	ErrHeartbeatTimeout = errors.New("cast: heartbeat timeout")
	// ErrNoConnection is returned by Send when the virtual connection for a
	// non-transport namespace has not been established.
	ErrNoConnection = errors.New("cast: no virtual connection")
	// ErrConnectionRefused is returned by Connect when the peer answers a
	// CONNECT with CLOSE.
	ErrConnectionRefused = errors.New("cast: virtual connection refused")
)

// VirtualConnection identifies a logical connection between a local and a
// remote endpoint multiplexed over one Channel. Messages from PeerID to
// LocalID (and vice versa) on non-transport namespaces are only delivered
// while the connection exists.
type VirtualConnection struct {
	LocalID string
	PeerID  string
}

// ConnectionInfo is the data associated with an open virtual connection.
type ConnectionInfo struct {
	Type            ConnectionType
	UserAgent       string
	SenderInfo      *SenderInfo
	ProtocolVersion ProtocolVersion // max negotiated version
}

// Handler receives messages for one namespace. Handlers run on the Channel's
// read goroutine: they may call Send, but must not block on further incoming
// messages (e.g. by calling Connect or Authenticate).
type Handler func(ch *Channel, msg *CastMessage)

// HeartbeatConfig controls the tp.heartbeat keep-alive. openscreen leaves
// heartbeat policy to the embedder; the defaults follow Chromium's
// cast_channel KeepAliveDelegate (ping after 5s idle, give up after 10s).
type HeartbeatConfig struct {
	// Interval after the last received message at which a PING is sent.
	// Zero disables sending PINGs (the channel still answers PINGs).
	Interval time.Duration
	// Timeout after the last received message at which the channel is
	// closed with ErrHeartbeatTimeout. Zero disables the liveness check.
	Timeout time.Duration
	// SourceID / DestinationID of PING messages; default sender-0 /
	// receiver-0.
	SourceID, DestinationID string
}

// DefaultHeartbeat is the sender-side heartbeat used by Chromium.
var DefaultHeartbeat = HeartbeatConfig{Interval: 5 * time.Second, Timeout: 10 * time.Second}

// Config configures a Channel. The zero value is valid: no heartbeat, all
// incoming CONNECTs accepted.
type Config struct {
	Heartbeat HeartbeatConfig
	// Clock drives heartbeat timers; defaults to SystemClock.
	Clock Clock
	// AllowConnection decides whether an incoming CONNECT is accepted
	// (openscreen's VirtualConnectionPolicy). Nil accepts all.
	AllowConnection func(vc VirtualConnection, req *ConnectMessage) bool
	// OnConnectionOpened / OnConnectionClosed observe virtual connection
	// lifetime. They run on the read goroutine (or the closing goroutine).
	OnConnectionOpened func(vc VirtualConnection, info ConnectionInfo)
	OnConnectionClosed func(vc VirtualConnection, reason CloseReason)
}

type pendingConnect struct {
	done chan struct{}
	err  error
}

// Channel runs the Cast channel protocol on one Socket: it routes incoming
// messages by namespace, implements the tp.connection virtual connection
// protocol and the tp.heartbeat keep-alive. It is the Go counterpart of
// openscreen's VirtualConnectionRouter + ConnectionNamespaceHandler for a
// single socket.
type Channel struct {
	sock *Socket
	cfg  Config

	mu        sync.Mutex
	started   bool
	handlers  map[string]Handler
	conns     map[VirtualConnection]ConnectionInfo
	pending   map[VirtualConnection]*pendingConnect
	pingTimer Timer
	liveTimer Timer
	err       error

	done chan struct{}
}

// NewChannel creates a Channel on sock. Register handlers, then call Start.
func NewChannel(sock *Socket, cfg *Config) *Channel {
	c := &Channel{
		sock:     sock,
		handlers: map[string]Handler{},
		conns:    map[VirtualConnection]ConnectionInfo{},
		pending:  map[VirtualConnection]*pendingConnect{},
		done:     make(chan struct{}),
	}
	if cfg != nil {
		c.cfg = *cfg
	}
	if c.cfg.Clock == nil {
		c.cfg.Clock = SystemClock{}
	}
	if c.cfg.Heartbeat.SourceID == "" {
		c.cfg.Heartbeat.SourceID = PlatformSenderID
	}
	if c.cfg.Heartbeat.DestinationID == "" {
		c.cfg.Heartbeat.DestinationID = PlatformReceiverID
	}
	return c
}

// Socket returns the underlying socket.
func (c *Channel) Socket() *Socket { return c.sock }

// Handle registers h for namespace ns, replacing any previous handler; a nil
// h removes it. The connection and heartbeat namespaces are handled
// internally and cannot be overridden.
func (c *Channel) Handle(ns string, h Handler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if h == nil {
		delete(c.handlers, ns)
	} else {
		c.handlers[ns] = h
	}
}

// Start begins reading from the socket and arms the heartbeat timers.
func (c *Channel) Start() {
	c.mu.Lock()
	if c.started {
		c.mu.Unlock()
		return
	}
	c.started = true
	hb := c.cfg.Heartbeat
	if hb.Interval > 0 {
		c.pingTimer = c.cfg.Clock.AfterFunc(hb.Interval, c.onPingTimer)
	}
	if hb.Timeout > 0 {
		c.liveTimer = c.cfg.Clock.AfterFunc(hb.Timeout, c.onLivenessTimeout)
	}
	c.mu.Unlock()
	go c.readLoop()
}

// Done is closed when the channel shuts down.
func (c *Channel) Done() <-chan struct{} { return c.done }

// Err returns why the channel shut down, or nil while it is open.
func (c *Channel) Err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.err
}

// Close sends CLOSE for every open virtual connection (best effort) and
// closes the socket.
func (c *Channel) Close() error {
	c.mu.Lock()
	vcs := make([]VirtualConnection, 0, len(c.conns))
	for vc := range c.conns {
		vcs = append(vcs, vc)
	}
	closed := c.err != nil
	c.mu.Unlock()
	if closed {
		return nil
	}
	_ = c.sock.Conn().SetWriteDeadline(time.Now().Add(time.Second))
	for _, vc := range vcs {
		_ = c.sock.Send(MakeCloseMessage(vc.LocalID, vc.PeerID, CloseUnknown))
	}
	c.shutdown(ErrChannelClosed, CloseClosedBySelf)
	return nil
}

// Send transmits msg. Messages on non-transport namespaces require an open
// virtual connection from msg.SourceID to msg.DestinationID, unless the
// destination is BroadcastID.
func (c *Channel) Send(msg *CastMessage) error {
	c.mu.Lock()
	if c.err != nil {
		c.mu.Unlock()
		return ErrChannelClosed
	}
	if !IsTransportNamespace(msg.Namespace) && msg.DestinationID != BroadcastID {
		if _, ok := c.conns[VirtualConnection{msg.SourceID, msg.DestinationID}]; !ok {
			c.mu.Unlock()
			return ErrNoConnection
		}
	}
	c.mu.Unlock()
	return c.sock.Send(msg)
}

// SendString is a convenience for sending a UTF-8 payload over vc.
func (c *Channel) SendString(vc VirtualConnection, ns, payload string) error {
	return c.Send(NewStringMessage(vc.LocalID, vc.PeerID, ns, payload))
}

// Connect opens the virtual connection vc by sending CONNECT and waiting for
// the peer's CONNECTED (openscreen only answers senders that advertise a
// protocol version, which MakeConnectMessage always does). It returns nil
// immediately if vc is already open. It must not be called from a Handler.
func (c *Channel) Connect(ctx context.Context, vc VirtualConnection) error {
	c.mu.Lock()
	if c.err != nil {
		c.mu.Unlock()
		return ErrChannelClosed
	}
	if _, ok := c.conns[vc]; ok {
		c.mu.Unlock()
		return nil
	}
	p, inflight := c.pending[vc]
	if !inflight {
		p = &pendingConnect{done: make(chan struct{})}
		c.pending[vc] = p
	}
	c.mu.Unlock()

	if !inflight {
		if err := c.sock.Send(MakeConnectMessage(vc.LocalID, vc.PeerID)); err != nil {
			c.resolvePending(vc, err)
			return err
		}
	}
	select {
	case <-p.done:
		return p.err
	case <-ctx.Done():
		c.mu.Lock()
		if c.pending[vc] == p {
			delete(c.pending, vc)
		}
		c.mu.Unlock()
		return ctx.Err()
	}
}

// Disconnect closes the virtual connection vc and notifies the peer.
func (c *Channel) Disconnect(vc VirtualConnection) error {
	if !c.removeConn(vc, CloseClosedBySelf) {
		return ErrNoConnection
	}
	return c.sock.Send(MakeCloseMessage(vc.LocalID, vc.PeerID, CloseUnknown))
}

// Connections returns the open virtual connections.
func (c *Channel) Connections() map[VirtualConnection]ConnectionInfo {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make(map[VirtualConnection]ConnectionInfo, len(c.conns))
	for k, v := range c.conns {
		out[k] = v
	}
	return out
}

// IsConnected reports whether vc is open.
func (c *Channel) IsConnected(vc VirtualConnection) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.conns[vc]
	return ok
}

func (c *Channel) readLoop() {
	for {
		msg, err := c.sock.Receive()
		if err != nil {
			c.shutdown(err, closeReasonFor(err))
			return
		}
		c.onActivity()
		c.dispatch(msg)
	}
}

func closeReasonFor(err error) CloseReason {
	var ne net.Error
	switch {
	case errors.Is(err, io.EOF):
		return CloseTransportClosed
	case errors.Is(err, ErrMessageTooLarge), errors.Is(err, ErrMissingRequiredField),
		errors.Is(err, errTruncated), errors.Is(err, errOverflow):
		return CloseTransportInvalidMessage
	case errors.As(err, &ne):
		return CloseTransportAborted
	default:
		return CloseTransportInvalidMessage
	}
}

func (c *Channel) dispatch(msg *CastMessage) {
	switch msg.Namespace {
	case ConnectionNamespace:
		c.handleConnectionMessage(msg)
		return
	case HeartbeatNamespace:
		c.handleHeartbeat(msg)
		return
	}
	c.mu.Lock()
	if msg.DestinationID != BroadcastID && !IsTransportNamespace(msg.Namespace) {
		if _, ok := c.conns[VirtualConnection{msg.DestinationID, msg.SourceID}]; !ok {
			c.mu.Unlock()
			return // drop: no route (matches VirtualConnectionRouter)
		}
	}
	h := c.handlers[msg.Namespace]
	c.mu.Unlock()
	if h != nil {
		h(c, msg)
	}
}

func (c *Channel) handleConnectionMessage(msg *CastMessage) {
	if msg.DestinationID == BroadcastID || msg.SourceID == BroadcastID ||
		msg.PayloadType != PayloadString {
		return
	}
	var req ConnectMessage
	if err := json.Unmarshal([]byte(msg.Payload()), &req); err != nil {
		return
	}
	vc := VirtualConnection{LocalID: msg.DestinationID, PeerID: msg.SourceID}
	switch req.Type {
	case MessageTypeConnect:
		c.handleConnect(vc, &req)
	case MessageTypeClose:
		var cm closeMessage
		_ = json.Unmarshal([]byte(msg.Payload()), &cm)
		reason := CloseClosedByPeer
		if cm.ReasonCode != nil && *cm.ReasonCode >= CloseUnknown && *cm.ReasonCode <= CloseClosedByPeer {
			reason = *cm.ReasonCode
		}
		c.removeConn(vc, reason)
		c.resolvePending(vc, ErrConnectionRefused)
	case MessageTypeConnected:
		var cm connectedMessage
		_ = json.Unmarshal([]byte(msg.Payload()), &cm)
		c.mu.Lock()
		_, waiting := c.pending[vc]
		c.mu.Unlock()
		if !waiting {
			return
		}
		v := ProtocolVersion(cm.ProtocolVersion)
		if v < CastV2_1_0 || v > CastV2_1_3 {
			v = CastV2_1_0
		}
		c.addConn(vc, ConnectionInfo{Type: ConnectionStrong, ProtocolVersion: v})
		c.resolvePending(vc, nil)
	}
}

func (c *Channel) handleConnect(vc VirtualConnection, req *ConnectMessage) {
	if c.cfg.AllowConnection != nil && !c.cfg.AllowConnection(vc, req) {
		_ = c.sock.Send(MakeCloseMessage(vc.LocalID, vc.PeerID, CloseUnknown))
		return
	}
	info := ConnectionInfo{Type: ConnectionStrong, UserAgent: req.UserAgent, SenderInfo: req.SenderInfo}
	if req.ConnType != nil {
		if *req.ConnType < ConnectionStrong || *req.ConnType > ConnectionInvisible {
			_ = c.sock.Send(MakeCloseMessage(vc.LocalID, vc.PeerID, CloseUnknown))
			return
		}
		info.Type = *req.ConnType
	}
	v, advertised := req.maxProtocolVersion()
	info.ProtocolVersion = ProtocolVersion(v)
	// Only answer senders that advertised a version, for compatibility with
	// older senders that do not expect a response.
	// Register before answering so the sender can use the connection as
	// soon as it sees CONNECTED.
	c.addConn(vc, info)
	if advertised {
		_ = c.sock.Send(NewStringMessage(vc.LocalID, vc.PeerID, ConnectionNamespace,
			mustJSON(connectedMessage{Type: MessageTypeConnected, ProtocolVersion: v})))
	}
}

func (c *Channel) addConn(vc VirtualConnection, info ConnectionInfo) {
	c.mu.Lock()
	_, exists := c.conns[vc]
	if !exists {
		c.conns[vc] = info
	}
	c.mu.Unlock()
	if !exists && c.cfg.OnConnectionOpened != nil {
		c.cfg.OnConnectionOpened(vc, info)
	}
}

func (c *Channel) removeConn(vc VirtualConnection, reason CloseReason) bool {
	c.mu.Lock()
	_, ok := c.conns[vc]
	delete(c.conns, vc)
	c.mu.Unlock()
	if ok && c.cfg.OnConnectionClosed != nil {
		c.cfg.OnConnectionClosed(vc, reason)
	}
	return ok
}

func (c *Channel) resolvePending(vc VirtualConnection, err error) {
	c.mu.Lock()
	p, ok := c.pending[vc]
	delete(c.pending, vc)
	c.mu.Unlock()
	if ok {
		p.err = err
		close(p.done)
	}
}

func (c *Channel) handleHeartbeat(msg *CastMessage) {
	if msg.PayloadType != PayloadString {
		return
	}
	var tm typedMessage
	if json.Unmarshal([]byte(msg.Payload()), &tm) != nil {
		return
	}
	if tm.Type == MessageTypePing {
		_ = c.sock.Send(makeTypedMessage(msg.DestinationID, msg.SourceID, HeartbeatNamespace, MessageTypePong))
	}
	// PONG needs no action: any received message already counts as liveness.
}

// onActivity re-arms both heartbeat timers after a received message.
func (c *Channel) onActivity() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err != nil {
		return
	}
	if c.pingTimer != nil {
		c.pingTimer.Reset(c.cfg.Heartbeat.Interval)
	}
	if c.liveTimer != nil {
		c.liveTimer.Reset(c.cfg.Heartbeat.Timeout)
	}
}

func (c *Channel) onPingTimer() {
	c.mu.Lock()
	if c.err != nil {
		c.mu.Unlock()
		return
	}
	hb := c.cfg.Heartbeat
	c.pingTimer.Reset(hb.Interval)
	c.mu.Unlock()
	_ = c.sock.Send(makeTypedMessage(hb.SourceID, hb.DestinationID, HeartbeatNamespace, MessageTypePing))
}

func (c *Channel) onLivenessTimeout() {
	c.shutdown(ErrHeartbeatTimeout, CloseTransportTooLongInactive)
}

// shutdown tears the channel down once; later calls are no-ops.
func (c *Channel) shutdown(err error, reason CloseReason) {
	c.mu.Lock()
	if c.err != nil {
		c.mu.Unlock()
		return
	}
	c.err = err
	if c.pingTimer != nil {
		c.pingTimer.Stop()
	}
	if c.liveTimer != nil {
		c.liveTimer.Stop()
	}
	conns := c.conns
	c.conns = map[VirtualConnection]ConnectionInfo{}
	pending := c.pending
	c.pending = map[VirtualConnection]*pendingConnect{}
	c.mu.Unlock()

	_ = c.sock.Close()
	for _, p := range pending {
		p.err = fmt.Errorf("%w: %v", ErrChannelClosed, err)
		close(p.done)
	}
	if c.cfg.OnConnectionClosed != nil {
		for vc := range conns {
			c.cfg.OnConnectionClosed(vc, reason)
		}
	}
	close(c.done)
}
