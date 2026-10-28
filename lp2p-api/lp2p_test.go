package lp2p

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"testing"
	"time"

	ua "github.com/backkem/go-lp2p/lp2p-api/useragent"
	"github.com/backkem/go-lp2p/streams-api"
	"github.com/backkem/go-lp2p/webtransport-api"
)

// These tests run real peers over loopback, discovered with mDNS. Nicknames
// get a random suffix so concurrent runs and other agents on the network
// don't interfere.

func uniqueName(t *testing.T, base string) string {
	b := make([]byte, 3)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%s-%x", base, b)
}

type browser struct {
	ui     *ua.Scripted
	ua     *ua.UserAgent
	window *Window
}

func newBrowser(t *testing.T, name string, ui *ua.Scripted) *browser {
	t.Helper()
	return newBrowserTimeout(t, name, ui, 10*time.Second)
}

func newBrowserTimeout(t *testing.T, name string, ui *ua.Scripted, handshake time.Duration) *browser {
	t.Helper()
	ui.Name = name
	if testing.Verbose() {
		ui.Out = os.Stderr
	}
	u, err := ua.New(ua.Config{Name: name, UI: ui, HandshakeTimeout: handshake})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = u.Close() })
	return &browser{ui: ui, ua: u, window: NewWindow(u, testOrigin)}
}

// testOrigin is the origin of the page on every browser: the same web app
// runs on both devices, and by default only same-origin sessions are
// delivered.
const testOrigin = "https://app.test"

// pair returns a receiving and a requesting browser that pair with each
// other through a PIN relay; the requester picks receiverName.
func pair(t *testing.T, receiverName string) (rx, tx *browser) {
	relay := ua.NewPINRelay()
	rx = newBrowser(t, "rx", &ua.Scripted{Present: relay.Present, Enter: relay.Enter})
	tx = newBrowser(t, "tx", &ua.Scripted{
		Present: relay.Present, Enter: relay.Enter,
		Pick: ua.PickByName(receiverName),
	})
	return rx, tx
}

func startReceiver(t *testing.T, b *browser, nickname string) (*LP2PReceiver, <-chan *LP2PConnection) {
	t.Helper()
	r, err := b.window.NewLP2PReceiver(LP2PReceiverOptions{Nickname: nickname})
	if err != nil {
		t.Fatal(err)
	}
	conns := make(chan *LP2PConnection, 4)
	r.OnConnection(func(e LP2PConnectionEvent) { conns <- e.Connection })
	if err := r.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(r.Stop)
	return r, conns
}

func connect(t *testing.T, b *browser, nickname string) *LP2PConnection {
	t.Helper()
	req, err := b.window.NewLP2PRequest(LP2PRequestOptions{Nickname: nickname})
	if err != nil {
		t.Fatal(err)
	}
	c, err := req.Start()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func recv[T any](t *testing.T, ch <-chan T, what string) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(10 * time.Second):
		t.Fatalf("timed out waiting for %s", what)
		panic("unreachable")
	}
}

func wantDOMException(t *testing.T, err error, name string) *DOMException {
	t.Helper()
	var de *DOMException
	if !errors.As(err, &de) || de.Name != name {
		t.Fatalf("got error %v, want a %s DOMException", err, name)
	}
	return de
}

func TestDataChannel(t *testing.T) {
	name := uniqueName(t, "Receiver")
	rx, tx := pair(t, name)
	_, conns := startReceiver(t, rx, name)
	reqConn := connect(t, tx, "Requester")
	rxConn := recv(t, conns, "connection")

	// The receiver echoes every message back, upper-cased by type.
	rxChannels := make(chan *LP2PDataChannel, 1)
	rxConn.OnDataChannel(func(e LP2PDataChannelEvent) {
		ch := e.Channel
		ch.OnMessage(func(e OnMessageEvent) {
			switch p := e.Payload.(type) {
			case PayloadString:
				_ = ch.SendText("echo: " + p.String())
			case PayloadBinary:
				_ = ch.Send(append([]byte{0xff}, p.Data...))
			}
		})
		rxChannels <- ch
	})

	dc, err := reqConn.CreateDataChannel("chat", &LP2PDataChannelInit{Protocol: "text", ID: ptr[uint64](4)})
	if err != nil {
		t.Fatal(err)
	}
	opened := make(chan struct{})
	dc.OnOpen(func(OnOpenEvent) { close(opened) })
	recv(t, opened, "onopen")

	got := make(chan OnMessageEvent, 16)
	dc.OnMessage(func(e OnMessageEvent) { got <- e })

	const n = 5
	for i := 0; i < n; i++ {
		if err := dc.SendText(fmt.Sprintf("msg %d", i)); err != nil {
			t.Fatal(err)
		}
	}
	if err := dc.Send([]byte{1, 2, 3}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < n; i++ {
		e := recv(t, got, "echo")
		if s, ok := e.Payload.(PayloadString); !ok || s.String() != fmt.Sprintf("echo: msg %d", i) {
			t.Fatalf("message %d: got %#v", i, e.Payload)
		}
	}
	if b, ok := recv(t, got, "binary echo").Payload.(PayloadBinary); !ok || !bytes.Equal(b.Data, []byte{0xff, 1, 2, 3}) {
		t.Fatalf("binary echo: got %#v", b)
	}

	rxDc := recv(t, rxChannels, "ondatachannel")
	if rxDc.Label() != "chat" || rxDc.Protocol() != "text" || rxDc.ID() != 4 {
		t.Errorf("remote channel: label %q protocol %q id %d", rxDc.Label(), rxDc.Protocol(), rxDc.ID())
	}

	// Channel IDs: by default the requester side takes the lowest free even
	// ID and the receiver side the lowest free odd one; an explicit one must
	// be free, of the local parity and in range.
	var de *DOMException
	if _, err := reqConn.CreateDataChannel("dup", &LP2PDataChannelInit{ID: ptr[uint64](4)}); !errors.As(err, &de) || de.Name != OperationError {
		t.Errorf("CreateDataChannel with an ID in use: %v, want OperationError", err)
	}
	if _, err := reqConn.CreateDataChannel("odd", &LP2PDataChannelInit{ID: ptr[uint64](5)}); !errors.As(err, &de) || de.Name != OperationError {
		t.Errorf("CreateDataChannel with the peer's parity: %v, want OperationError", err)
	}
	if _, err := reqConn.CreateDataChannel("big", &LP2PDataChannelInit{ID: ptr[uint64](65535)}); !errors.As(err, &de) || de.Name != TypeError {
		t.Errorf("CreateDataChannel with ID 65535: %v, want TypeError", err)
	}
	var autos []*LP2PDataChannel
	for _, want := range []uint64{0, 2} {
		auto, err := reqConn.CreateDataChannel("auto", nil)
		if err != nil {
			t.Fatal(err)
		}
		if auto.ID() != want {
			t.Errorf("allocated ID %d, want %d", auto.ID(), want)
		}
		rx := recv(t, rxChannels, "ondatachannel")
		if rx.ID() != want {
			t.Errorf("remote allocated ID %d, want %d", rx.ID(), want)
		}
		autos = append(autos, auto)
	}
	// The receiver side allocates odd IDs.
	back, err := rxConn.CreateDataChannel("back", nil)
	if err != nil {
		t.Fatal(err)
	}
	if back.ID() != 1 {
		t.Errorf("peer allocated ID %d, want 2", back.ID())
	}
	_ = back.Close()
	for _, auto := range autos {
		_ = auto.Close()
	}

	// Closing is a handshake: both sides end up closed.
	rxClosed := make(chan struct{})
	rxDc.OnClose(func(OnCloseEvent) { close(rxClosed) })
	txClosed := make(chan struct{})
	dc.OnClose(func(OnCloseEvent) { close(txClosed) })
	if err := dc.Close(); err != nil {
		t.Fatal(err)
	}
	recv(t, rxClosed, "remote onclose")
	recv(t, txClosed, "local onclose")
	if s := dc.ReadyState(); s != LP2PDataChannelStateClosed {
		t.Errorf("readyState %s, want closed", s)
	}
	wantDOMException(t, dc.SendText("too late"), InvalidStateError)
}

func TestConnectionClose(t *testing.T) {
	name := uniqueName(t, "Receiver")
	rx, tx := pair(t, name)
	_, conns := startReceiver(t, rx, name)
	reqConn := connect(t, tx, "Requester")
	rxConn := recv(t, conns, "connection")

	rxChannels := make(chan *LP2PDataChannel, 1)
	rxConn.OnDataChannel(func(e LP2PDataChannelEvent) { rxChannels <- e.Channel })
	dc, err := reqConn.CreateDataChannel("c", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := dc.SendText("hi"); err != nil { // makes the channel known to the peer
		t.Fatal(err)
	}
	rxDc := recv(t, rxChannels, "ondatachannel")
	dcClosed := make(chan struct{})
	dc.OnClose(func(OnCloseEvent) { close(dcClosed) })
	rxDcClosed := make(chan struct{})
	rxDc.OnClose(func(OnCloseEvent) { close(rxDcClosed) })
	connClosed := make(chan struct{})
	reqConn.OnClose(func(OnCloseEvent) { close(connClosed) })

	if err := reqConn.Close(); err != nil {
		t.Fatal(err)
	}
	recv(t, dcClosed, "channel onclose")
	recv(t, connClosed, "connection onclose")
	// The peer's channels close with the connection. (The peer's
	// LP2PConnection itself isn't told: ospc doesn't surface a remote close
	// to AcceptDataChannel/AcceptTransport yet.)
	recv(t, rxDcClosed, "peer channel onclose")
	if _, err := reqConn.CreateDataChannel("late", nil); err == nil {
		t.Fatal("CreateDataChannel on a closed connection succeeded")
	}
	if err := reqConn.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
}

func TestPickerChoosesByName(t *testing.T) {
	names := []string{uniqueName(t, "Kitchen"), uniqueName(t, "Living-room"), uniqueName(t, "Office")}
	want := names[1]
	rx, tx := pair(t, want)
	conns := map[string]<-chan *LP2PConnection{}
	for _, n := range names {
		_, conns[n] = startReceiver(t, rx, n)
	}
	connect(t, tx, "Requester")
	recv(t, conns[want], "connection on "+want)
	for _, n := range names {
		select {
		case <-conns[n]:
			if n != want {
				t.Errorf("%s got a connection", n)
			}
		default:
		}
	}
}

func TestAdvertisePermissionDenied(t *testing.T) {
	b := newBrowser(t, "rx", &ua.Scripted{Permission: ua.Deny(ua.PermissionAdvertise)})
	r, err := b.window.NewLP2PReceiver(LP2PReceiverOptions{Nickname: uniqueName(t, "Receiver")})
	if err != nil {
		t.Fatal(err)
	}
	de := wantDOMException(t, r.Start(), NotAllowedError)
	if !errors.Is(de, ua.ErrPermissionDenied) {
		t.Errorf("%v doesn't wrap ua.ErrPermissionDenied", de)
	}
}

func TestPickerDismissed(t *testing.T) {
	b := newBrowser(t, "tx", &ua.Scripted{}) // no Pick: the user dismisses the picker
	req, err := b.window.NewLP2PRequest(LP2PRequestOptions{Nickname: "Requester"})
	if err != nil {
		t.Fatal(err)
	}
	// Every failure of start() is a NotAllowedError; the reason is only
	// available to Go callers.
	_, err = req.Start()
	de := wantDOMException(t, err, NotAllowedError)
	if !errors.Is(de, ua.ErrCanceled) {
		t.Errorf("%v doesn't wrap ua.ErrCanceled", de)
	}
}

func TestWrongPIN(t *testing.T) {
	name := uniqueName(t, "Receiver")
	rx := newBrowser(t, "rx", &ua.Scripted{Enter: ua.FixedPIN("0")})
	tx := newBrowser(t, "tx", &ua.Scripted{Enter: ua.FixedPIN("0"), Pick: ua.PickByName(name)})
	_, conns := startReceiver(t, rx, name)

	req, err := tx.window.NewLP2PRequest(LP2PRequestOptions{Nickname: "Requester"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = req.Start()
	de := wantDOMException(t, err, NotAllowedError)
	if !errors.Is(de, ua.ErrAuthenticationFailed) {
		t.Errorf("%v doesn't wrap ua.ErrAuthenticationFailed", de)
	}
	select {
	case <-conns:
		t.Error("receiver got a connection despite the wrong PIN")
	case <-time.After(200 * time.Millisecond):
	}
}

func TestIncomingConnectionDenied(t *testing.T) {
	name := uniqueName(t, "Receiver")
	relay := ua.NewPINRelay()
	rx := newBrowser(t, "rx", &ua.Scripted{
		Permission: ua.Deny(ua.PermissionAcceptConnection),
		Present:    relay.Present, Enter: relay.Enter,
	})
	tx := newBrowserTimeout(t, "tx", &ua.Scripted{
		Present: relay.Present, Enter: relay.EnterWithin(time.Second), Pick: ua.PickByName(name),
	}, 3*time.Second)
	_, conns := startReceiver(t, rx, name)

	req, err := tx.window.NewLP2PRequest(LP2PRequestOptions{Nickname: "Requester"})
	if err != nil {
		t.Fatal(err)
	}
	// The receiver pairs first, then asks its user, who declines: the
	// session is refused and the requester is told.
	_, err = req.Start()
	de := wantDOMException(t, err, NotAllowedError)
	if !errors.Is(de, ua.ErrRefused) {
		t.Errorf("%v doesn't wrap ua.ErrRefused", de)
	}
	select {
	case <-conns:
		t.Error("receiver got a denied connection")
	default:
	}
}

func TestQuicTransport(t *testing.T) {
	name := uniqueName(t, "Receiver")
	rx, tx := pair(t, name)
	_, conns := startReceiver(t, rx, name)
	reqConn := connect(t, tx, "Requester")
	rxConn := recv(t, conns, "connection")

	// At most one listener per connection; after Close a new one.
	listener, err := NewLP2PQuicTransportListener(rxConn, LP2PQuicTransportListenerInit{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = NewLP2PQuicTransportListener(rxConn, LP2PQuicTransportListenerInit{})
	wantDOMException(t, err, InvalidStateError)
	listener.Close()
	listener, err = NewLP2PQuicTransportListener(rxConn, LP2PQuicTransportListenerInit{})
	if err != nil {
		t.Fatal(err)
	}

	// Receiver: echo the first stream of the first transport, then report
	// how the transport was closed.
	type closed struct {
		info webtransport.WebTransportCloseInfo
		err  error
	}
	echoed := make(chan error, 1)
	peerClosed := make(chan closed, 1)
	go func() {
		echoed <- func() error {
			res, err := listener.IncomingTransports.GetReader(nil).(streams.ReadableStreamDefaultReader[*LP2PQuicTransport]).Read()
			if err != nil || res.Done {
				return fmt.Errorf("incoming transport: %v", err)
			}
			transport := res.Val
			go func() {
				info, err := transport.Closed()
				peerClosed <- closed{info, err}
			}()
			sres, err := transport.IncomingBidirectionalStreams.GetReader(nil).(streams.ReadableStreamDefaultReader[webtransport.WebTransportBidirectionalStream]).Read()
			if err != nil || sres.Done {
				return fmt.Errorf("incoming stream: %v", err)
			}
			msg, err := readLine(sres.Val.Readable)
			if err != nil {
				return err
			}
			if err := sres.Val.Writable.GetWriter().Write([]byte("echo: " + msg + "\n")); err != nil {
				return err
			}
			// Datagrams and the exporter work on the receiver's side too.
			d, err := transport.Datagrams.Readable.GetReader(nil).(streams.ReadableStreamDefaultReader[[]byte]).Read()
			if err != nil || d.Done || string(d.Val) != "dgram" {
				return fmt.Errorf("datagram: %q, done=%v, %v", d.Val, d.Done, err)
			}
			km, err := transport.ExportKeyingMaterial("label", []byte("ctx"), 16)
			if err != nil {
				return err
			}
			return transport.Datagrams.Writable.GetWriter().Write(km)
		}()
	}()

	transport, err := NewLP2PQuicTransport(reqConn, LP2PQuicTransportInit{RequireUnreliable: true})
	if err != nil {
		t.Fatal(err)
	}
	if transport.Connection() != reqConn {
		t.Error("transport.Connection() is not the connection it was opened on")
	}
	if transport.Reliability() != ReliabilitySupportsUnreliable {
		t.Errorf("reliability %s", transport.Reliability())
	}
	s, err := transport.CreateBidirectionalStream()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Writable.GetWriter().Write([]byte("hello\n")); err != nil {
		t.Fatal(err)
	}
	got, err := readLine(s.Readable)
	if err != nil {
		t.Fatal(err)
	}
	if got != "echo: hello" {
		t.Errorf("got %q", got)
	}
	// Datagrams may be lost; send a few.
	for i := 0; i < 5; i++ {
		if err := transport.Datagrams.Writable.GetWriter().Write([]byte("dgram")); err != nil {
			t.Fatal(err)
		}
	}
	if err := recv(t, echoed, "echo"); err != nil {
		t.Fatal(err)
	}
	km, err := transport.ExportKeyingMaterial("label", []byte("ctx"), 16)
	if err != nil {
		t.Fatal(err)
	}
	d, err := transport.Datagrams.Readable.GetReader(nil).(streams.ReadableStreamDefaultReader[[]byte]).Read()
	if err != nil || d.Done || !bytes.Equal(d.Val, km) {
		t.Errorf("keying material differs: got %x, want %x (%v)", d.Val, km, err)
	}

	// Close reaches the peer with code and reason.
	info := webtransport.WebTransportCloseInfo{CloseCode: 7, Reason: "done"}
	if err := transport.Close(info); err != nil {
		t.Fatal(err)
	}
	if c := recv(t, peerClosed, "peer closed"); c.err != nil || c.info != info {
		t.Errorf("peer closed with %+v, %v; want %+v", c.info, c.err, info)
	}
	if got, err := transport.Closed(); err != nil || got != info {
		t.Errorf("closed with %+v, %v", got, err)
	}

	// Without a listener, the peer's transports are refused.
	listener.Close()
	res, err := listener.IncomingTransports.GetReader(nil).(streams.ReadableStreamDefaultReader[*LP2PQuicTransport]).Read()
	if err != nil || !res.Done {
		t.Errorf("read after Close: done=%v err=%v, want done", res.Done, err)
	}
	time.Sleep(2 * listenerGrace)
	_, err = NewLP2PQuicTransport(reqConn, LP2PQuicTransportInit{})
	wantDOMException(t, err, WebTransportError)

	// Closing the connection fails its transports and closes its listener
	// before onclose.
	listener, err = NewLP2PQuicTransportListener(rxConn, LP2PQuicTransportListenerInit{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewLP2PQuicTransportListener(reqConn, LP2PQuicTransportListenerInit{}); err != nil {
		t.Fatal(err)
	}
	transport, err = NewLP2PQuicTransport(rxConn, LP2PQuicTransportInit{})
	if err != nil {
		t.Fatal(err)
	}
	closedAt := make(chan struct{})
	rxConn.OnClose(func(OnCloseEvent) {
		if _, err := transport.Closed(); err == nil {
			t.Error("transport not failed before onclose")
		}
		if _, err := NewLP2PQuicTransportListener(rxConn, LP2PQuicTransportListenerInit{}); err == nil {
			t.Error("listener constructed on a closed connection")
		}
		close(closedAt)
	})
	_ = rxConn.Close()
	recv(t, closedAt, "onclose")
	if _, err := transport.Closed(); err == nil {
		t.Error("transport survived the connection close")
	} else {
		wantDOMException(t, err, WebTransportError)
	}
	if res, err := listener.IncomingTransports.GetReader(nil).(streams.ReadableStreamDefaultReader[*LP2PQuicTransport]).Read(); err != nil || !res.Done {
		t.Errorf("listener not closed with the connection: done=%v err=%v", res.Done, err)
	}
	_, err = NewLP2PQuicTransport(rxConn, LP2PQuicTransportInit{})
	wantDOMException(t, err, InvalidStateError)
}

func TestTransportGlare(t *testing.T) {
	name := uniqueName(t, "Receiver")
	rx, tx := pair(t, name)
	_, conns := startReceiver(t, rx, name)
	reqConn := connect(t, tx, "Requester")
	rxConn := recv(t, conns, "connection")
	for _, c := range []*LP2PConnection{reqConn, rxConn} {
		if _, err := NewLP2PQuicTransportListener(c, LP2PQuicTransportListenerInit{}); err != nil {
			t.Fatal(err)
		}
	}
	// Both sides open a transport: there are two sessions.
	for _, c := range []*LP2PConnection{reqConn, rxConn} {
		if _, err := NewLP2PQuicTransport(c, LP2PQuicTransportInit{}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestAllowedOrigins(t *testing.T) {
	name := uniqueName(t, "Receiver")
	rx, tx := pair(t, name)

	_, err := rx.window.NewLP2PReceiver(LP2PReceiverOptions{Nickname: name, AllowedOrigins: []string{"://bad"}})
	wantDOMException(t, err, TypeError)

	prompts := 0
	rx.ui.Permission = func(req ua.PermissionRequest) bool {
		if req.Permission == ua.PermissionAcceptConnection {
			prompts++
		}
		return true
	}
	pins := 0
	present := rx.ui.Present
	rx.ui.Present = func(p ua.PINPresentation) { pins++; present(p) }
	r, err := rx.window.NewLP2PReceiver(LP2PReceiverOptions{
		Nickname:       name,
		AllowedOrigins: []string{"HTTPS://Partner.test:443/path", "data:text/plain,opaque"},
	})
	if err != nil {
		t.Fatal(err)
	}
	conns := make(chan *LP2PConnection, 4)
	r.OnConnection(func(e LP2PConnectionEvent) { conns <- e.Connection })
	if err := r.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(r.Stop)

	// The requester's origin isn't allowed: refused once the requester is
	// verified, so after a PIN ceremony but before any grant prompt.
	other := NewWindow(tx.ua, "https://other.test")
	req, err := other.NewLP2PRequest(LP2PRequestOptions{Nickname: "Requester"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = req.Start()
	de := wantDOMException(t, err, NotAllowedError)
	if !errors.Is(de, ua.ErrRefused) {
		t.Errorf("%v doesn't wrap ua.ErrRefused", de)
	}
	if prompts != 0 || pins != 1 {
		t.Errorf("receiver prompted %d times and showed %d PINs for a refused origin, want 0 and 1", prompts, pins)
	}

	// A page of an allowed origin on the same browser gets through; the
	// browsers trust each other now, so without another PIN.
	partner := NewWindow(tx.ua, "https://partner.test")
	req, err = partner.NewLP2PRequest(LP2PRequestOptions{Nickname: "Partner"})
	if err != nil {
		t.Fatal(err)
	}
	c, err := req.Start()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	recv(t, conns, "connection from the partner origin")
	if prompts != 1 || pins != 1 {
		t.Errorf("receiver prompted %d times and showed %d PINs for an allowed origin, want 1 and 1", prompts, pins)
	}
}

// TestSessionGoesToAllowingReceiver: the client picked a receiver that
// doesn't allow its origin; another receiver of the same browser does, so
// the session is delivered there.
func TestSessionGoesToAllowingReceiver(t *testing.T) {
	name := uniqueName(t, "Receiver")
	rx, tx := pair(t, name)
	// The PIN is shown by the receiving page, not the picked one, so the
	// relay can't match on nicknames: the user reads whatever PIN is shown.
	pins := make(chan string, 1)
	rx.ui.Present = func(p ua.PINPresentation) { pins <- p.PIN }
	tx.ui.Enter = func(ctx context.Context, p ua.PINPrompt) (string, error) {
		select {
		case pin := <-pins:
			return pin, nil
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	_, picked := startReceiver(t, rx, name) // same-origin only

	partnerPage := NewWindow(rx.ua, "https://partner.test")
	r, err := partnerPage.NewLP2PReceiver(LP2PReceiverOptions{
		Nickname:       uniqueName(t, "Other"),
		AllowedOrigins: []string{"https://other.test"},
	})
	if err != nil {
		t.Fatal(err)
	}
	conns := make(chan *LP2PConnection, 1)
	r.OnConnection(func(e LP2PConnectionEvent) { conns <- e.Connection })
	if err := r.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(r.Stop)

	other := NewWindow(tx.ua, "https://other.test")
	req, err := other.NewLP2PRequest(LP2PRequestOptions{Nickname: "Requester"})
	if err != nil {
		t.Fatal(err)
	}
	c, err := req.Start()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	recv(t, conns, "connection on the receiver that allows the origin")
	select {
	case <-picked:
		t.Error("the picked receiver got a connection it doesn't allow")
	default:
	}
	if g := rx.ua.PeerGrants("https://partner.test"); len(g) != 1 {
		t.Errorf("grants of the receiving origin: %v", g)
	}
}

// TestStartAbort: aborting Start through its signal fails it with the abort
// reason and takes down the picker or the session being set up.
func TestStartAbort(t *testing.T) {
	name := uniqueName(t, "Receiver")
	rx, tx := pair(t, name)
	_, conns := startReceiver(t, rx, name)

	// Already aborted: fails right away, with a DOMException cause as is.
	req, err := tx.window.NewLP2PRequest(LP2PRequestOptions{Nickname: "Requester"})
	if err != nil {
		t.Fatal(err)
	}
	reason := newDOMException("TimeoutError", "took too long", nil)
	ctx, cancel := context.WithCancelCause(context.Background())
	cancel(reason)
	if _, err := req.Start(LP2PRequestStartOptions{Signal: ctx}); err != reason {
		t.Fatalf("already aborted: got %v, want the abort reason", err)
	}

	// Abort during the picker: the picker blocks until aborted.
	tx.ui.Pick = func(ctx context.Context, p *ua.Picker) (ua.Peer, error) {
		<-ctx.Done()
		return ua.Peer{}, ctx.Err()
	}
	ctx, cancel = context.WithCancelCause(context.Background())
	// A second Start while pending returns the pending result.
	second := make(chan error, 1)
	go func() {
		time.Sleep(50 * time.Millisecond)
		_, err := req.Start()
		second <- err
	}()
	go func() {
		time.Sleep(200 * time.Millisecond)
		cancel(errors.New("user navigated away"))
	}()
	_, err = req.Start(LP2PRequestStartOptions{Signal: ctx})
	de := wantDOMException(t, err, AbortError)
	if de.Err == nil || de.Err.Error() != "user navigated away" {
		t.Errorf("abort reason: %v", de.Err)
	}
	if err2 := recv(t, second, "second Start"); err2 != err {
		t.Errorf("second Start got %v, want the pending result %v", err2, err)
	}

	// Abort during the session: the PIN is shown, the requester's user
	// never types it, the page aborts; the receiver gets no connection.
	tx.ui.Pick = ua.PickByName(name)
	shown := make(chan struct{}, 1)
	rx.ui.Present = func(p ua.PINPresentation) { shown <- struct{}{} }
	tx.ui.Enter = func(ctx context.Context, p ua.PINPrompt) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	}
	ctx, cancel = context.WithCancelCause(context.Background())
	go func() {
		recv(t, shown, "PIN shown")
		cancel(nil)
	}()
	_, err = req.Start(LP2PRequestStartOptions{Signal: ctx})
	wantDOMException(t, err, AbortError)
	select {
	case <-conns:
		t.Error("receiver got a connection from an aborted request")
	case <-time.After(300 * time.Millisecond):
	}

	// After a failure Start starts over; after success, aborting does
	// nothing and Start returns the same connection.
	relay := ua.NewPINRelay()
	rx.ui.Present, tx.ui.Enter = relay.Present, relay.Enter
	ctx, cancel = context.WithCancelCause(context.Background())
	c, err := req.Start(LP2PRequestStartOptions{Signal: ctx})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	recv(t, conns, "connection")
	cancel(nil)
	if again, err := req.Start(); err != nil || again != c {
		t.Errorf("Start after success: %v, %v; want the same connection", again, err)
	}
	if c.isClosed() {
		t.Error("abort after success closed the connection")
	}
}

// TestEarlyDataChannelHeld: a data channel the requester opens right after
// start() is delivered to the receiver's page only once its connection
// event handler has run, so that ondatachannel can be set there.
func TestEarlyDataChannelHeld(t *testing.T) {
	name := uniqueName(t, "Receiver")
	rx, tx := pair(t, name)
	r, err := rx.window.NewLP2PReceiver(LP2PReceiverOptions{Nickname: name})
	if err != nil {
		t.Fatal(err)
	}
	handlerDone := make(chan struct{})
	got := make(chan bool, 1)
	r.OnConnection(func(e LP2PConnectionEvent) {
		e.Connection.OnDataChannel(func(LP2PDataChannelEvent) {
			select {
			case <-handlerDone:
				got <- true
			default:
				got <- false
			}
		})
		time.Sleep(200 * time.Millisecond) // The channel arrives meanwhile.
		close(handlerDone)
	})
	if err := r.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(r.Stop)
	c := connect(t, tx, "Requester")
	if _, err := c.CreateDataChannel("early", nil); err != nil {
		t.Fatal(err)
	}
	if !recv(t, got, "ondatachannel") {
		t.Error("data channel delivered before the connection event handler finished")
	}
}

func TestTrustedPeerSkipsPairing(t *testing.T) {
	name := uniqueName(t, "Receiver")
	rx, tx := pair(t, name)
	var pins, prompts int
	present := rx.ui.Present
	rx.ui.Present = func(p ua.PINPresentation) { pins++; present(p) }
	rx.ui.Permission = func(req ua.PermissionRequest) bool {
		if req.Permission == ua.PermissionAcceptConnection {
			prompts++
		}
		return true
	}
	_, conns := startReceiver(t, rx, name)

	// First session: PIN ceremony and grant prompt.
	c1 := connect(t, tx, "Requester")
	recv(t, conns, "first connection")
	if pins != 1 || prompts != 1 {
		t.Fatalf("first session: %d PINs, %d prompts; want 1 and 1", pins, prompts)
	}
	if !rx.ua.Trusted(tx.ua.PeerID()) || !tx.ua.Trusted(rx.ua.PeerID()) {
		t.Error("peers don't trust each other after pairing")
	}
	if g := rx.ua.PeerGrants(rx.window.Origin); len(g) != 1 || g[0] != tx.ua.PeerID() {
		t.Errorf("receiver grants %v", g)
	}
	_ = c1.Close()

	// Second session: both trust each other and the origin has its grant,
	// so neither a PIN nor a prompt.
	connect(t, tx, "Requester")
	recv(t, conns, "second connection")
	if pins != 1 || prompts != 1 {
		t.Errorf("second session: %d PINs, %d prompts; want no more", pins, prompts)
	}

	// Removing the anchor on the receiver makes it ask for a PIN again.
	rx.ua.RemoveTrustAnchor(tx.ua.PeerID())
	connect(t, tx, "Requester")
	recv(t, conns, "third connection")
	if pins != 2 || prompts != 2 {
		t.Errorf("after removing the anchor: %d PINs, %d prompts; want 2 and 2", pins, prompts)
	}
}

func TestReceiverStop(t *testing.T) {
	name := uniqueName(t, "Receiver")
	rx, tx := pair(t, name)
	r, conns := startReceiver(t, rx, name)
	c := connect(t, tx, "Requester")
	rxConn := recv(t, conns, "connection")

	r.Stop()
	wantDOMException(t, r.Start(), InvalidStateError)
	r.Stop() // idempotent

	// Delivered connections stay open.
	dc, err := c.CreateDataChannel("after-stop", nil)
	if err != nil {
		t.Fatal(err)
	}
	got := make(chan *LP2PDataChannel, 1)
	rxConn.OnDataChannel(func(e LP2PDataChannelEvent) { got <- e.Channel })
	if err := dc.SendText("still here"); err != nil {
		t.Fatal(err)
	}
	recv(t, got, "data channel after stop")
}

func TestLegacySessionFlag(t *testing.T) {
	name := uniqueName(t, "Receiver")
	relay := ua.NewPINRelay()
	mk := func(n string, pick func(context.Context, *ua.Picker) (ua.Peer, error)) *browser {
		ui := &ua.Scripted{Name: n, Present: relay.Present, Enter: relay.Enter, Pick: pick}
		u, err := ua.New(ua.Config{Name: n, UI: ui, HandshakeTimeout: 10 * time.Second, DisableSessionExtension: true})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = u.Close() })
		return &browser{ui: ui, ua: u, window: NewWindow(u, testOrigin)}
	}
	rx, tx := mk("rx", nil), mk("tx", ua.PickByName(name))
	_, conns := startReceiver(t, rx, name)
	c := connect(t, tx, "Requester")
	rxConn := recv(t, conns, "connection")
	got := make(chan *LP2PDataChannel, 1)
	rxConn.OnDataChannel(func(e LP2PDataChannelEvent) { got <- e.Channel })
	dc, err := c.CreateDataChannel("legacy", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := dc.SendText("hi"); err != nil {
		t.Fatal(err)
	}
	recv(t, got, "data channel")
}

// readLine reads up to a newline. The streams shim returns fixed-size
// zero-padded chunks, so NUL bytes are dropped.
func readLine(rs streams.ReadableStream[[]byte]) (string, error) {
	r := rs.GetReader(nil).(streams.ReadableStreamDefaultReader[[]byte])
	var buf []byte
	for {
		res, err := r.Read()
		if err != nil {
			return "", err
		}
		if res.Done {
			return "", io.ErrUnexpectedEOF
		}
		buf = append(buf, bytes.Trim(res.Val, "\x00")...)
		if i := bytes.IndexByte(buf, '\n'); i >= 0 {
			return string(buf[:i]), nil
		}
	}
}

func TestEventSlotBuffersInOrder(t *testing.T) {
	var s eventSlot[int]
	for i := 0; i < 100; i++ {
		s.fire(i)
	}
	var mu sync.Mutex
	var got []int
	done := make(chan struct{})
	s.set(func(i int) {
		mu.Lock()
		got = append(got, i)
		n := len(got)
		mu.Unlock()
		if n == 100 {
			// Replacing the handler from within itself must not deadlock.
			s.set(func(int) {})
			close(done)
		}
	})
	<-done
	for i, v := range got {
		if v != i {
			t.Fatalf("event %d delivered as %d", i, v)
		}
	}
}

func ptr[T any](v T) *T { return &v }
