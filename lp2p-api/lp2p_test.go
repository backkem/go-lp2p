package lp2p

import (
	"bytes"
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
	return &browser{ui: ui, ua: u, window: NewWindow(u, "https://"+name+".test")}
}

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
	t.Cleanup(func() { _ = r.Close() })
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

	dc, err := reqConn.CreateDataChannel("chat", &LP2PDataChannelInit{Protocol: "text", ID: 7})
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
	if rxDc.Label() != "chat" || rxDc.Protocol() != "text" || rxDc.ID() != 7 {
		t.Errorf("remote channel: label %q protocol %q id %d", rxDc.Label(), rxDc.Protocol(), rxDc.ID())
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
	_, err = req.Start()
	wantDOMException(t, err, AbortError)
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
	// The requester isn't told. Usually its user gives up waiting for a
	// PIN (AbortError); if the refusal lands while ospc is still dialing,
	// the dial only ends with the handshake timeout (TimeoutError).
	_, err = req.Start()
	var de *DOMException
	if !errors.As(err, &de) || (de.Name != AbortError && de.Name != TimeoutError) {
		t.Fatalf("got %v, want an AbortError or TimeoutError", err)
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
	r, err := rx.window.NewLP2PReceiver(LP2PReceiverOptions{Nickname: name})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.Close() })
	listener, err := NewLP2PQuicTransportListener(r, LP2PQuicTransportListenerInit{}) // starts r
	if err != nil {
		t.Fatal(err)
	}

	// Receiver: echo the first stream of the first transport.
	echoed := make(chan error, 1)
	go func() {
		echoed <- func() error {
			res, err := listener.IncomingTransports.GetReader(nil).(streams.ReadableStreamDefaultReader[*LP2PQuicTransport]).Read()
			if err != nil || res.Done {
				return fmt.Errorf("incoming transport: %v", err)
			}
			sres, err := res.Val.IncomingBidirectionalStreams.GetReader(nil).(streams.ReadableStreamDefaultReader[webtransport.WebTransportBidirectionalStream]).Read()
			if err != nil || sres.Done {
				return fmt.Errorf("incoming stream: %v", err)
			}
			msg, err := readLine(sres.Val.Readable)
			if err != nil {
				return err
			}
			return sres.Val.Writable.GetWriter().Write([]byte("echo: " + msg + "\n"))
		}()
	}()

	req, err := tx.window.NewLP2PRequest(LP2PRequestOptions{Nickname: "Requester"})
	if err != nil {
		t.Fatal(err)
	}
	transport, err := NewLP2PQuicTransport(req, LP2PQuicTransportInit{AllowPooling: true})
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close(webtransport.WebTransportCloseInfo{})
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
	if err := recv(t, echoed, "echo"); err != nil {
		t.Fatal(err)
	}

	listener.Close()
	res, err := listener.IncomingTransports.GetReader(nil).(streams.ReadableStreamDefaultReader[*LP2PQuicTransport]).Read()
	if err != nil || !res.Done {
		t.Errorf("read after Close: done=%v err=%v, want done", res.Done, err)
	}
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

func TestDedicatedTransportNotSupported(t *testing.T) {
	_, err := NewLP2PQuicTransport(&LP2PConnection{}, LP2PQuicTransportInit{})
	wantDOMException(t, err, NotSupportedError)
	_, err = NewLP2PQuicTransport("nope", LP2PQuicTransportInit{AllowPooling: true})
	wantDOMException(t, err, NotSupportedError)
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
