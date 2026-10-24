package cast

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"
)

const testNS = "urn:x-cast:com.example.test"

var (
	senderVC   = VirtualConnection{LocalID: PlatformSenderID, PeerID: PlatformReceiverID}
	receiverVC = VirtualConnection{LocalID: PlatformReceiverID, PeerID: PlatformSenderID}
)

type closeEvent struct {
	vc     VirtualConnection
	reason CloseReason
}

type observer struct {
	opened chan VirtualConnection
	closed chan closeEvent
}

func newObserver(cfg *Config) *observer {
	o := &observer{opened: make(chan VirtualConnection, 16), closed: make(chan closeEvent, 16)}
	cfg.OnConnectionOpened = func(vc VirtualConnection, _ ConnectionInfo) { o.opened <- vc }
	cfg.OnConnectionClosed = func(vc VirtualConnection, r CloseReason) { o.closed <- closeEvent{vc, r} }
	return o
}

func (o *observer) waitClosed(t *testing.T) closeEvent {
	t.Helper()
	select {
	case e := <-o.closed:
		return e
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for close")
		return closeEvent{}
	}
}

func channelPair(t *testing.T, sCfg, rCfg *Config) (*Channel, *Channel) {
	t.Helper()
	ss, rs := socketPair(t, nil)
	s, r := NewChannel(ss, sCfg), NewChannel(rs, rCfg)
	t.Cleanup(func() { _ = s.Close(); _ = r.Close() })
	return s, r
}

func collect(ch *Channel, ns string) <-chan *CastMessage {
	out := make(chan *CastMessage, 16)
	ch.Handle(ns, func(_ *Channel, m *CastMessage) { out <- m })
	return out
}

func TestVirtualConnectionLifecycle(t *testing.T) {
	var rCfg Config
	obs := newObserver(&rCfg)
	s, r := channelPair(t, nil, &rCfg)
	sIn, rIn := collect(s, testNS), collect(r, testNS)
	s.Start()
	r.Start()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := s.SendString(senderVC, testNS, "early"); !errors.Is(err, ErrNoConnection) {
		t.Fatalf("send before connect: %v", err)
	}
	if err := s.Connect(ctx, senderVC); err != nil {
		t.Fatal(err)
	}
	if got := <-obs.opened; got != receiverVC {
		t.Fatalf("receiver opened %v", got)
	}
	if info := r.Connections()[receiverVC]; info.ProtocolVersion != CastV2_1_3 ||
		info.UserAgent != UserAgent || info.SenderInfo == nil || info.SenderInfo.SdkType != 2 {
		t.Fatalf("receiver connection info %+v", info)
	}
	if info := s.Connections()[senderVC]; info.ProtocolVersion != CastV2_1_3 {
		t.Fatalf("sender connection info %+v", info)
	}
	// Idempotent.
	if err := s.Connect(ctx, senderVC); err != nil {
		t.Fatal(err)
	}

	if err := s.SendString(senderVC, testNS, "hello"); err != nil {
		t.Fatal(err)
	}
	if m := recvTimeout(t, rIn); m.Payload() != "hello" || m.SourceID != PlatformSenderID {
		t.Fatalf("receiver got %v", m)
	}
	if err := r.SendString(receiverVC, testNS, "world"); err != nil {
		t.Fatal(err)
	}
	if m := recvTimeout(t, sIn); m.Payload() != "world" {
		t.Fatalf("sender got %v", m)
	}

	// A message for an unknown virtual connection is dropped; bypass the
	// Send check by writing to the socket directly.
	if err := s.Socket().Send(NewStringMessage("stranger", PlatformReceiverID, testNS, "drop me")); err != nil {
		t.Fatal(err)
	}
	// Broadcasts are delivered without a connection.
	if err := s.Socket().Send(NewStringMessage("stranger", BroadcastID, testNS, "broadcast")); err != nil {
		t.Fatal(err)
	}
	if m := recvTimeout(t, rIn); m.Payload() != "broadcast" {
		t.Fatalf("expected broadcast, got %v", m)
	}

	if err := s.Disconnect(senderVC); err != nil {
		t.Fatal(err)
	}
	if e := obs.waitClosed(t); e.vc != receiverVC || e.reason != CloseClosedByPeer {
		t.Fatalf("close event %+v", e)
	}
	if err := s.SendString(senderVC, testNS, "late"); !errors.Is(err, ErrNoConnection) {
		t.Fatalf("send after disconnect: %v", err)
	}
	if err := s.Disconnect(senderVC); !errors.Is(err, ErrNoConnection) {
		t.Fatalf("double disconnect: %v", err)
	}
}

func TestCloseReasonCode(t *testing.T) {
	var rCfg Config
	obs := newObserver(&rCfg)
	s, r := channelPair(t, nil, &rCfg)
	s.Start()
	r.Start()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.Connect(ctx, senderVC); err != nil {
		t.Fatal(err)
	}
	<-obs.opened
	if err := s.Socket().Send(MakeCloseMessage(PlatformSenderID, PlatformReceiverID, CloseTransportAborted)); err != nil {
		t.Fatal(err)
	}
	if e := obs.waitClosed(t); e.reason != CloseTransportAborted {
		t.Fatalf("reason %v", e.reason)
	}
}

func TestConnectRefused(t *testing.T) {
	rCfg := Config{AllowConnection: func(vc VirtualConnection, _ *ConnectMessage) bool {
		return vc.PeerID == "sender-allowed"
	}}
	s, r := channelPair(t, nil, &rCfg)
	s.Start()
	r.Start()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.Connect(ctx, senderVC); !errors.Is(err, ErrConnectionRefused) {
		t.Fatalf("got %v", err)
	}
	if err := s.Connect(ctx, VirtualConnection{"sender-allowed", PlatformReceiverID}); err != nil {
		t.Fatal(err)
	}
}

func TestConnectInvalidConnType(t *testing.T) {
	var rCfg Config
	obs := newObserver(&rCfg)
	s, r := channelPair(t, nil, &rCfg)
	r.Start()
	payload := `{"type":"CONNECT","connType":7,"protocolVersion":0}`
	if err := s.Socket().Send(NewStringMessage(PlatformSenderID, PlatformReceiverID, ConnectionNamespace, payload)); err != nil {
		t.Fatal(err)
	}
	reply, err := s.Socket().Receive()
	if err != nil {
		t.Fatal(err)
	}
	var tm typedMessage
	_ = json.Unmarshal([]byte(reply.Payload()), &tm)
	if tm.Type != MessageTypeClose {
		t.Fatalf("want CLOSE, got %v", reply)
	}
	select {
	case vc := <-obs.opened:
		t.Fatalf("connection %v opened", vc)
	default:
	}
}

func TestLegacyConnectWithoutVersionGetsNoReply(t *testing.T) {
	var rCfg Config
	obs := newObserver(&rCfg)
	s, r := channelPair(t, nil, &rCfg)
	r.Start()
	if err := s.Socket().Send(NewStringMessage(PlatformSenderID, PlatformReceiverID, ConnectionNamespace, `{"type":"CONNECT"}`)); err != nil {
		t.Fatal(err)
	}
	<-obs.opened
	// Next message from the receiver must be our PING's PONG, not CONNECTED.
	if err := s.Socket().Send(makeTypedMessage("a", "b", HeartbeatNamespace, MessageTypePing)); err != nil {
		t.Fatal(err)
	}
	m, err := s.Socket().Receive()
	if err != nil {
		t.Fatal(err)
	}
	if m.Namespace != HeartbeatNamespace || m.SourceID != "b" || m.DestinationID != "a" ||
		m.Payload() != `{"type":"PONG"}` {
		t.Fatalf("got %v", m)
	}
	if r.Connections()[receiverVC].ProtocolVersion != CastV2_1_0 {
		t.Fatal("legacy sender must get CASTV2_1_0")
	}
}

func TestHeartbeatPingAnswered(t *testing.T) {
	clk := &fakeClock{}
	sCfg := Config{Heartbeat: DefaultHeartbeat, Clock: clk}
	s, r := channelPair(t, &sCfg, nil)
	s.Start()
	r.Start()

	before := clk.Resets()
	clk.Advance(5 * time.Second) // ping fires; receiver answers PONG
	// The PONG re-arms both timers via onActivity (+1 for the ping re-arm).
	waitFor(t, "PONG", func() bool { return clk.Resets() >= before+3 })
	clk.Advance(9 * time.Second) // < 10s since PONG: still alive
	waitFor(t, "second PONG", func() bool { return clk.Resets() >= before+6 })
	if s.Err() != nil {
		t.Fatalf("channel closed: %v", s.Err())
	}
}

func TestHeartbeatTimeout(t *testing.T) {
	clk := &fakeClock{}
	sCfg := Config{Heartbeat: DefaultHeartbeat, Clock: clk}
	obs := newObserver(&sCfg)
	ss, rs := socketPair(t, nil)
	s := NewChannel(ss, &sCfg)
	s.Start()
	defer s.Close()

	// A silent peer that answers CONNECT but never PING.
	var wg sync.WaitGroup
	pings := make(chan *CastMessage, 4)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			m, err := rs.Receive()
			if err != nil {
				return
			}
			switch m.Namespace {
			case ConnectionNamespace:
				_ = rs.Send(NewStringMessage(m.DestinationID, m.SourceID, ConnectionNamespace, `{"type":"CONNECTED","protocolVersion":3}`))
			case HeartbeatNamespace:
				pings <- m
			}
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.Connect(ctx, senderVC); err != nil {
		t.Fatal(err)
	}
	<-obs.opened

	clk.Advance(5 * time.Second)
	p := recvTimeout(t, pings)
	if p.SourceID != PlatformSenderID || p.DestinationID != PlatformReceiverID || p.Payload() != `{"type":"PING"}` {
		t.Fatalf("ping %v", p)
	}
	clk.Advance(5 * time.Second) // 10s without traffic
	select {
	case <-s.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("channel did not close")
	}
	if !errors.Is(s.Err(), ErrHeartbeatTimeout) {
		t.Fatalf("err %v", s.Err())
	}
	if e := obs.waitClosed(t); e.vc != senderVC || e.reason != CloseTransportTooLongInactive {
		t.Fatalf("close event %+v", e)
	}
	if err := s.SendString(senderVC, testNS, "x"); !errors.Is(err, ErrChannelClosed) {
		t.Fatalf("send after close: %v", err)
	}
	wg.Wait()
}

func TestPeerCloseShutsDownChannel(t *testing.T) {
	var rCfg Config
	obs := newObserver(&rCfg)
	s, r := channelPair(t, nil, &rCfg)
	s.Start()
	r.Start()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.Connect(ctx, senderVC); err != nil {
		t.Fatal(err)
	}
	<-obs.opened
	_ = s.Close() // sends CLOSE for senderVC, then closes the socket
	if e := obs.waitClosed(t); e.reason != CloseClosedByPeer {
		t.Fatalf("close event %+v", e)
	}
	select {
	case <-r.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("receiver channel did not close")
	}
}

func TestConnectUnblocksOnShutdown(t *testing.T) {
	ss, _ := socketPair(t, nil) // peer never answers
	s := NewChannel(ss, nil)
	s.Start()
	errc := make(chan error, 1)
	go func() { errc <- s.Connect(context.Background(), senderVC) }()
	waitFor(t, "pending connect", func() bool {
		s.mu.Lock()
		defer s.mu.Unlock()
		return len(s.pending) == 1
	})
	_ = s.Close()
	if err := <-errc; !errors.Is(err, ErrChannelClosed) {
		t.Fatalf("got %v", err)
	}
}

func TestInvalidFrameClosesChannel(t *testing.T) {
	var sCfg Config
	ss, rs := socketPair(t, nil)
	s := NewChannel(ss, &sCfg)
	s.Start()
	if _, err := rs.Conn().Write([]byte{0x58, 0x58, 0x58, 0x58}); err != nil {
		t.Fatal(err)
	}
	<-s.Done()
	if !errors.Is(s.Err(), ErrMessageTooLarge) {
		t.Fatalf("err %v", s.Err())
	}
}
