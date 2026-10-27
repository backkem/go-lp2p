package ospc

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/quic-go/quic-go"
)

// wtPair returns an authenticated connection pair.
func wtPair(t *testing.T) (server, client *Connection) {
	t.Helper()
	s, c := authenticatePair(t, []byte("1234"), []byte("1234"))
	if s.err != nil || c.err != nil {
		t.Fatalf("authentication failed: server %v, client %v", s.err, c.err)
	}
	return s.conn, c.conn
}

// acceptSession accepts the next incoming session on conn.
func acceptSession(t *testing.T, ctx context.Context, conn *Connection) *WebTransportSession {
	t.Helper()
	inc, err := conn.AcceptWebTransport(ctx)
	if err != nil {
		t.Fatalf("AcceptWebTransport: %v", err)
	}
	s, err := inc.Accept()
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}
	return s
}

// openSessionPair opens a session from client to server and accepts it.
func openSessionPair(t *testing.T, ctx context.Context, server, client *Connection) (serverSide, clientSide *WebTransportSession) {
	t.Helper()
	accepted := make(chan *WebTransportSession, 1)
	go func() {
		inc, err := server.AcceptWebTransport(ctx)
		if err != nil {
			accepted <- nil
			return
		}
		s, err := inc.Accept()
		if err != nil {
			accepted <- nil
			return
		}
		accepted <- s
	}()
	clientSide, err := client.OpenWebTransport(ctx)
	if err != nil {
		t.Fatalf("OpenWebTransport: %v", err)
	}
	serverSide = <-accepted
	if serverSide == nil {
		t.Fatal("server failed to accept the session")
	}
	return serverSide, clientSide
}

func readAll(t *testing.T, r io.Reader) []byte {
	t.Helper()
	b, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	return b
}

func TestWebTransportOpenAndStreams(t *testing.T) {
	checkNoLeaks(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	server, client := wtPair(t)
	if !server.SupportsDatagrams() || !client.SupportsDatagrams() {
		t.Fatal("QUIC datagrams not negotiated")
	}

	// The accepting side opens a stream right after accepting, which may
	// reach the opener before the success response: it is held.
	early := make(chan error, 1)
	go func() {
		inc, err := server.AcceptWebTransport(ctx)
		if err != nil {
			early <- err
			return
		}
		s, err := inc.Accept()
		if err != nil {
			early <- err
			return
		}
		st, err := s.OpenUniStreamSync(ctx)
		if err != nil {
			early <- err
			return
		}
		_, err = st.Write([]byte("early"))
		if cerr := st.Close(); err == nil {
			err = cerr
		}
		early <- err
	}()
	cs, err := client.OpenWebTransport(ctx)
	if err != nil {
		t.Fatalf("OpenWebTransport: %v", err)
	}
	if cs.ExchangeID() != 0 {
		t.Errorf("client exchange-id %d, want 0", cs.ExchangeID())
	}
	if err := <-early; err != nil {
		t.Fatal(err)
	}
	uni, err := cs.AcceptUniStream(ctx)
	if err != nil {
		t.Fatalf("AcceptUniStream: %v", err)
	}
	if !uni.Unidirectional() {
		t.Error("stream not unidirectional")
	}
	if got := readAll(t, uni); string(got) != "early" {
		t.Errorf("early stream: got %q", got)
	}
	if _, err := uni.Write([]byte("x")); err == nil {
		t.Error("write on a receive-only stream succeeded")
	}

	// Echo on a bidirectional stream opened by the client.
	ss := server.base.app.Load().wt.get(0)
	if ss == nil {
		t.Fatal("server has no session 0")
	}
	go func() {
		st, err := ss.AcceptStream(ctx)
		if err != nil {
			return
		}
		b, _ := io.ReadAll(st)
		st.Write(append([]byte("echo: "), b...))
		st.Close()
	}()
	st, err := cs.OpenStreamSync(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	st.Close()
	if got := readAll(t, st); string(got) != "echo: hello" {
		t.Errorf("echo: got %q", got)
	}

	// A second session from the client gets the next even id.
	_, cs2 := openSessionPair(t, ctx, server, client)
	if cs2.ExchangeID() != 2 {
		t.Errorf("second client exchange-id %d, want 2", cs2.ExchangeID())
	}
}

func TestWebTransportRefuse(t *testing.T) {
	checkNoLeaks(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	server, client := wtPair(t)
	go func() {
		inc, err := server.AcceptWebTransport(ctx)
		if err == nil {
			inc.Refuse()
		}
	}()
	_, err := client.OpenWebTransport(ctx)
	if !errors.Is(err, ErrWebTransportRefused) {
		t.Fatalf("got %v, want ErrWebTransportRefused", err)
	}
}

func TestWebTransportGlare(t *testing.T) {
	checkNoLeaks(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	server, client := wtPair(t)
	for _, conn := range []*Connection{server, client} {
		conn := conn
		go func() {
			inc, err := conn.AcceptWebTransport(ctx)
			if err == nil {
				inc.Accept()
			}
		}()
	}
	type res struct {
		s   *WebTransportSession
		err error
	}
	results := make(chan res, 2)
	for _, conn := range []*Connection{server, client} {
		conn := conn
		go func() {
			s, err := conn.OpenWebTransport(ctx)
			results <- res{s, err}
		}()
	}
	ids := map[uint64]bool{}
	for i := 0; i < 2; i++ {
		r := <-results
		if r.err != nil {
			t.Fatal(r.err)
		}
		ids[r.s.ExchangeID()] = true
	}
	if !ids[0] || !ids[1] {
		t.Errorf("exchange-ids %v, want 0 (client) and 1 (server)", ids)
	}
	if n := len(server.base.app.Load().wt.sessions); n != 2 {
		t.Errorf("server has %d sessions, want 2", n)
	}
}

func TestWebTransportClose(t *testing.T) {
	checkNoLeaks(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	server, client := wtPair(t)
	ss, cs := openSessionPair(t, ctx, server, client)

	// A stream open in both directions when the session closes.
	st, err := cs.OpenStreamSync(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Write([]byte("x")); err != nil {
		t.Fatal(err)
	}
	sst, err := ss.AcceptStream(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n, err := sst.Read(make([]byte, 1)); n != 1 || err != nil {
		t.Fatalf("read: %d, %v", n, err)
	}

	info := WebTransportCloseInfo{CloseCode: 42, Reason: "bye"}
	if err := cs.Close(info); err != nil {
		t.Fatal(err)
	}
	got, err := ss.Closed(ctx)
	if err != nil || got != info {
		t.Fatalf("peer closed with %+v, %v; want %+v", got, err, info)
	}
	if lgot, err := cs.Closed(ctx); err != nil || lgot != info {
		t.Fatalf("local closed with %+v, %v; want %+v", lgot, err, info)
	}
	// Streams of the session are reset and stopped with session-gone.
	if _, err := sst.Read(make([]byte, 1)); !errors.Is(err, ErrWebTransportSessionGone) {
		t.Errorf("read on the peer's stream: %v, want ErrWebTransportSessionGone", err)
	}
	if _, err := st.Read(make([]byte, 1)); !errors.Is(err, ErrWebTransportSessionGone) {
		t.Errorf("read on the local stream: %v, want ErrWebTransportSessionGone", err)
	}
	if _, err := ss.OpenStreamSync(ctx); !errors.Is(err, ErrWebTransportClosed) {
		t.Errorf("open on a closed session: %v", err)
	}
	if _, err := ss.AcceptStream(ctx); !errors.Is(err, ErrWebTransportClosed) {
		t.Errorf("accept on a closed session: %v", err)
	}
	if err := cs.Close(WebTransportCloseInfo{}); err != nil {
		t.Errorf("second close: %v", err)
	}

	// A stream for the closed session is refused with session-gone.
	raw, err := client.OpenStream(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeMessage(&msgDataTransportStream{ExchangeId: cs.ExchangeID()}, raw); err != nil {
		t.Fatal(err)
	}
	_, err = raw.Read(make([]byte, 1))
	var se *quic.StreamError
	if !errors.As(err, &se) || uint64(se.ErrorCode) != WebTransportSessionGone {
		t.Errorf("stream for a closed session: %v, want reset with session-gone", err)
	}

	// FIN on the control stream without data-transport-close: code 0.
	ss2, cs2 := openSessionPair(t, ctx, server, client)
	if err := cs2.control.Close(); err != nil {
		t.Fatal(err)
	}
	if got, err := ss2.Closed(ctx); err != nil || got != (WebTransportCloseInfo{}) {
		t.Errorf("closed by FIN with %+v, %v", got, err)
	}
	<-cs2.Done()

	// A reset control stream closes abruptly.
	ss3, cs3 := openSessionPair(t, ctx, server, client)
	abortStream(cs3.control)
	if _, err := ss3.Closed(ctx); !errors.Is(err, ErrWebTransportClosed) {
		t.Errorf("closed by reset: %v, want ErrWebTransportClosed", err)
	}
	<-cs3.Done()
}

func TestWebTransportStreamErrorCodes(t *testing.T) {
	checkNoLeaks(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	server, client := wtPair(t)
	ss, cs := openSessionPair(t, ctx, server, client)
	st, err := cs.OpenStreamSync(ctx)
	if err != nil {
		t.Fatal(err)
	}
	sst, err := ss.AcceptStream(ctx)
	if err != nil {
		t.Fatal(err)
	}
	st.CancelWrite(7)
	_, err = io.ReadAll(sst)
	var we *WebTransportStreamError
	if !errors.As(err, &we) || we.Code != 7 || !we.Remote {
		t.Errorf("reset with code 7 read as %v", err)
	}
	st2, err := cs.OpenStreamSync(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st2.Write([]byte("x")); err != nil {
		t.Fatal(err)
	}
	sst2, err := ss.AcceptStream(ctx)
	if err != nil {
		t.Fatal(err)
	}
	sst2.CancelRead(MaxWebTransportStreamErrorCode)
	for {
		_, err = st2.Write([]byte("x"))
		if err != nil {
			break
		}
	}
	if !errors.As(err, &we) || we.Code != MaxWebTransportStreamErrorCode || !we.Remote {
		t.Errorf("stopped with code 2^32-1 written as %v", err)
	}
	cs.Close(WebTransportCloseInfo{})
	<-ss.Done()
}

func TestWebTransportDrain(t *testing.T) {
	checkNoLeaks(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	server, client := wtPair(t)
	ss, cs := openSessionPair(t, ctx, server, client)
	select {
	case <-ss.Draining():
		t.Fatal("draining before drain")
	default:
	}
	if err := cs.Drain(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ss.Draining():
	case <-ctx.Done():
		t.Fatal("peer not draining")
	}
	// The session still works, and closes cleanly.
	if _, err := ss.OpenStreamSync(ctx); err != nil {
		t.Fatal(err)
	}
	ss.Close(WebTransportCloseInfo{CloseCode: 1})
	if got, err := cs.Closed(ctx); err != nil || got.CloseCode != 1 {
		t.Errorf("closed with %+v, %v", got, err)
	}
}

func TestWebTransportExporter(t *testing.T) {
	checkNoLeaks(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	server, client := wtPair(t)
	ss, cs := openSessionPair(t, ctx, server, client)
	ss2, cs2 := openSessionPair(t, ctx, server, client)

	export := func(s *WebTransportSession, label string, context []byte) []byte {
		t.Helper()
		b, err := s.ExportKeyingMaterial(label, context, 32)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	a, b := export(ss, "l", []byte("c")), export(cs, "l", []byte("c"))
	if !bytes.Equal(a, b) {
		t.Error("exporter differs between the two sides")
	}
	if bytes.Equal(a, export(ss2, "l", []byte("c"))) || !bytes.Equal(export(ss2, "l", []byte("c")), export(cs2, "l", []byte("c"))) {
		t.Error("exporter not distinct per session, or differs between sides of the second session")
	}
	if bytes.Equal(a, export(ss, "l2", []byte("c"))) || bytes.Equal(a, export(ss, "l", []byte("c2"))) {
		t.Error("exporter not distinct per label and context")
	}
	if _, err := ss.ExportKeyingMaterial(string(make([]byte, 256)), nil, 32); err == nil {
		t.Error("256-byte label accepted")
	}
}

func TestWebTransportDatagrams(t *testing.T) {
	checkNoLeaks(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	server, client := wtPair(t)
	ss, cs := openSessionPair(t, ctx, server, client)
	if !ss.SupportsDatagrams() || !cs.SupportsDatagrams() {
		t.Fatal("datagrams not supported")
	}
	// Datagrams may be lost, even on loopback; send a few and expect one.
	for i := 0; i < 5; i++ {
		if err := cs.SendDatagram([]byte("ping")); err != nil {
			t.Fatal(err)
		}
	}
	got, err := ss.ReceiveDatagram(ctx)
	if err != nil || string(got) != "ping" {
		t.Fatalf("received %q, %v", got, err)
	}
	for i := 0; i < 5; i++ {
		if err := ss.SendDatagram([]byte("pong")); err != nil {
			t.Fatal(err)
		}
	}
	if got, err := cs.ReceiveDatagram(ctx); err != nil || string(got) != "pong" {
		t.Fatalf("received %q, %v", got, err)
	}
	if err := cs.SendDatagram(make([]byte, 65536)); !errors.Is(err, ErrWebTransportDatagramTooLarge) {
		t.Errorf("oversized datagram: %v", err)
	}
	cs.Close(WebTransportCloseInfo{})
	<-ss.Done()
	// Datagrams received before the close can still be read; then it fails.
	for {
		_, err := ss.ReceiveDatagram(ctx)
		if err != nil {
			if !errors.Is(err, ErrWebTransportClosed) {
				t.Errorf("receive after close: %v", err)
			}
			break
		}
	}
}

func TestWebTransportUnsupportedPeer(t *testing.T) {
	checkNoLeaks(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	_, client := wtPair(t)
	info := *client.RemoteAgent().Info()
	info.Capabilities = []AgentCapability{AgentCapabilityDataChannels}
	client.base.remoteAgent.setInfo(info)
	if _, err := client.OpenWebTransport(ctx); !errors.Is(err, ErrWebTransportUnsupported) {
		t.Fatalf("got %v, want ErrWebTransportUnsupported", err)
	}
}

func TestWebTransportConnectionClose(t *testing.T) {
	checkNoLeaks(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	server, client := wtPair(t)
	ss, cs := openSessionPair(t, ctx, server, client)
	st, err := cs.OpenStreamSync(ctx)
	if err != nil {
		t.Fatal(err)
	}
	client.Close()
	if _, err := cs.Closed(ctx); !errors.Is(err, ErrWebTransportClosed) {
		t.Errorf("local session after connection close: %v", err)
	}
	if _, err := ss.Closed(ctx); !errors.Is(err, ErrWebTransportClosed) {
		t.Errorf("peer session after connection close: %v", err)
	}
	if _, err := st.Read(make([]byte, 1)); err == nil {
		t.Error("read on a stream of a closed connection succeeded")
	}
	select {
	case <-server.Done():
	case <-time.After(testTimeout):
		t.Fatal("server connection not closed")
	}
}
