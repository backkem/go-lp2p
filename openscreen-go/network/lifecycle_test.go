package ospc

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"net"
	"runtime"
	"strings"
	"testing"
	"time"

	mdns "github.com/grandcat/zeroconf"
)

const testTimeout = 5 * time.Second

// checkNoLeaks fails the test if goroutines of this package or of quic-go
// are still running once the test and its cleanups are done. Register it
// first so it runs after all other cleanups.
func checkNoLeaks(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		deadline := time.Now().Add(3 * time.Second)
		for {
			leaked := leakedGoroutines()
			if len(leaked) == 0 {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("%d leaked goroutines:\n\n%s", len(leaked), strings.Join(leaked, "\n\n"))
			}
			time.Sleep(20 * time.Millisecond)
		}
	})
}

func leakedGoroutines() []string {
	buf := make([]byte, 1<<20)
	buf = buf[:runtime.Stack(buf, true)]
	var leaked []string
	for _, g := range strings.Split(string(buf), "\n\n") {
		if strings.Contains(g, "testing.tRunner") || strings.Contains(g, "leakedGoroutines") {
			continue
		}
		if strings.Contains(g, "openscreen-go/network.") || strings.Contains(g, "quic-go") {
			leaked = append(leaked, g)
		}
	}
	return leaked
}

func newTestAgent(t *testing.T, name string) *Agent {
	t.Helper()
	a, err := NewAgent(AgentConfig{
		DisplayName: name,
		// P-256 SPAKE2 suite: the spec's edwards25519 is not implemented (non-spec opt-in).
		InteropQuirks: InteropQuirks{Spake2SuiteP256: true},
		KeepAlive:     KeepAliveConfig{Disabled: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	return a
}

// discoveredAgentFor returns the DiscoveredAgent a listening agent advertising
// txt on addr would be discovered as.
func discoveredAgentFor(t *testing.T, a *Agent, addr net.Addr, txt TXTRecordSet) DiscoveredAgent {
	t.Helper()
	udpAddr := addr.(*net.UDPAddr)
	entry := mdns.NewServiceEntry(escapeDNSSD(instanceNameForDisplayName(a.Info().DisplayName)), MdnsServiceType, MdnsDomain)
	entry.AddrIPv4 = []net.IP{net.IPv4(127, 0, 0, 1)}
	entry.Port = udpAddr.Port
	entry.Text = txt.ToSlice()
	ra, err := newDiscoveredAgent(entry)
	if err != nil {
		t.Fatal(err)
	}
	return *ra
}

// startTestListener starts a listener on loopback without mDNS.
func startTestListener(t *testing.T, a *Agent) (*Listener, DiscoveredAgent) {
	t.Helper()
	l := NewListener(a, AgentTransportQUIC)
	if err := l.start("127.0.0.1:0", false); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	return l, discoveredAgentFor(t, a, l.netListener.Addr(), l.txt)
}

type authResult struct {
	conn *Connection
	err  error
}

// authenticatePair connects a client to a listening server and runs the
// authentication with the given PSKs, as the server presents and the client
// consumes it with the default PSK config.
func authenticatePair(t *testing.T, serverPSK, clientPSK []byte) (server, client authResult) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	serverAgent := newTestAgent(t, "Server")
	clientAgent := newTestAgent(t, "Client")
	l, ra := startTestListener(t, serverAgent)

	serverCh := make(chan authResult, 1)
	go func() {
		uConn, err := l.Accept(ctx)
		if err != nil {
			serverCh <- authResult{err: err}
			return
		}
		defer uConn.Close()
		role, err := uConn.AcceptAuthenticate(ctx)
		if err != nil {
			serverCh <- authResult{err: err}
			return
		}
		if role != AuthenticationRolePresenter {
			serverCh <- authResult{err: errors.New("server is not the presenter")}
			return
		}
		conn, err := uConn.AuthenticatePSK(ctx, serverPSK)
		serverCh <- authResult{conn, err}
	}()

	clientCh := make(chan authResult, 1)
	go func() {
		uConn, err := ra.Dial(ctx, AgentTransportQUIC, clientAgent)
		if err != nil {
			clientCh <- authResult{err: err}
			return
		}
		defer uConn.Close()
		if uConn.RemoteAgent().Info().DisplayName != "Server" {
			clientCh <- authResult{err: errors.New("wrong remote display name")}
			return
		}
		conn, err := uConn.AuthenticatePSK(ctx, clientPSK)
		clientCh <- authResult{conn, err}
	}()

	server, client = <-serverCh, <-clientCh
	for _, r := range []authResult{server, client} {
		if r.conn != nil {
			conn := r.conn
			t.Cleanup(func() { conn.Close() })
		}
	}
	return server, client
}

func TestAuthenticateAndClose(t *testing.T) {
	checkNoLeaks(t)

	server, client := authenticatePair(t, []byte("1234"), []byte("1234"))
	if server.err != nil || client.err != nil {
		t.Fatalf("authentication failed: server: %v, client: %v", server.err, client.err)
	}
	if !client.conn.RemoteAgent().Authenticated() {
		t.Fatal("remote agent not marked authenticated")
	}

	if err := client.conn.Close(); err != nil {
		t.Fatal(err)
	}
	// Close is idempotent.
	if err := client.conn.Close(); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(client.conn.Err(), ErrConnectionClosed) {
		t.Fatalf("unexpected close reason: %v", client.conn.Err())
	}

	// The remote side notices the close.
	select {
	case <-server.conn.Done():
	case <-time.After(testTimeout):
		t.Fatal("server connection not closed after remote close")
	}
	if server.conn.Err() == nil {
		t.Fatal("no close reason")
	}
	_, err := server.conn.AcceptDataChannel(context.Background())
	if err == nil {
		t.Fatal("AcceptDataChannel on closed connection succeeded")
	}
}

func TestAuthenticateWrongPSK(t *testing.T) {
	checkNoLeaks(t)

	server, client := authenticatePair(t, []byte("1234"), []byte("4321"))
	if server.err == nil || client.err == nil {
		t.Fatalf("authentication succeeded: server: %v, client: %v", server.err, client.err)
	}
	if !errors.Is(server.err, ErrAuthenticationFailed) && !errors.Is(client.err, ErrAuthenticationFailed) {
		t.Fatalf("neither side reports ErrAuthenticationFailed: server: %v, client: %v", server.err, client.err)
	}
}

// TestRemoteAuthFailure checks that both agents close the connection when
// one reports a non-authenticated auth-status, instead of hanging.
func TestRemoteAuthFailure(t *testing.T) {
	checkNoLeaks(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	l, ra := startTestListener(t, newTestAgent(t, "Server"))
	accepted := make(chan *UnauthenticatedConnection, 1)
	go func() {
		uConn, _ := l.Accept(ctx)
		accepted <- uConn
	}()

	uConn, err := ra.Dial(ctx, AgentTransportQUIC, newTestAgent(t, "Client"))
	if err != nil {
		t.Fatal(err)
	}
	defer uConn.Close()
	clientErr := make(chan error, 1)
	go func() {
		_, err := uConn.AuthenticatePSK(ctx, []byte("1234"))
		clientErr <- err
	}()

	serverConn := <-accepted
	if serverConn == nil {
		t.Fatal("accept failed")
	}
	defer serverConn.Close()
	if _, err := serverConn.AcceptAuthenticate(ctx); err != nil {
		t.Fatal(err)
	}
	// Instead of authenticating, the server reports a failure.
	if err := serverConn.base.send(&msgAuthStatus{Result: AuthStatusResultProofInvalid}); err != nil {
		t.Fatal(err)
	}

	if err := <-clientErr; !errors.Is(err, ErrAuthenticationFailed) {
		t.Fatalf("client: unexpected error: %v", err)
	}
	select {
	case <-serverConn.base.ctx.Done():
	case <-ctx.Done():
		t.Fatal("server connection not closed")
	}
}

func TestCloseDuringHandshake(t *testing.T) {
	checkNoLeaks(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	serverAgent := newTestAgent(t, "Server")
	l, ra := startTestListener(t, serverAgent)

	serverErr := make(chan error, 1)
	go func() {
		uConn, err := l.Accept(ctx)
		if err != nil {
			serverErr <- err
			return
		}
		defer uConn.Close()
		// The client closes before the PSK is entered on the server.
		_, err = uConn.AcceptAuthenticate(ctx)
		if err != nil {
			serverErr <- err
			return
		}
		select {
		case <-uConn.base.ctx.Done():
			serverErr <- uConn.base.err()
		case <-ctx.Done():
			serverErr <- nil
		}
	}()

	uConn, err := ra.Dial(ctx, AgentTransportQUIC, newTestAgent(t, "Client"))
	if err != nil {
		t.Fatal(err)
	}
	clientErr := make(chan error, 1)
	go func() {
		_, err := uConn.AuthenticatePSK(ctx, []byte("1234"))
		clientErr <- err
	}()

	time.Sleep(50 * time.Millisecond)
	if err := uConn.Close(); err != nil {
		t.Fatal(err)
	}

	if err := <-clientErr; !errors.Is(err, ErrConnectionClosed) {
		t.Fatalf("client: unexpected error: %v", err)
	}
	if err := <-serverErr; err == nil || ctx.Err() != nil {
		t.Fatalf("server: unexpected result: %v (ctx: %v)", err, ctx.Err())
	}
}

func TestListenerCloseDuringHandshake(t *testing.T) {
	checkNoLeaks(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	l, ra := startTestListener(t, newTestAgent(t, "Server"))

	// Nobody calls Accept: the connection stays pending in the listener.
	uConn, err := ra.Dial(ctx, AgentTransportQUIC, newTestAgent(t, "Client"))
	if err != nil {
		t.Fatal(err)
	}
	defer uConn.Close()

	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Accept(ctx); !errors.Is(err, ErrListenerClosed) {
		t.Fatalf("unexpected Accept error: %v", err)
	}

	// The pending connection was closed.
	_, err = uConn.AuthenticatePSK(ctx, []byte("1234"))
	if err == nil || ctx.Err() != nil {
		t.Fatalf("unexpected result: %v (ctx: %v)", err, ctx.Err())
	}
}

// TestDialContextCancel dials an agent that accepts the QUIC connection but
// never answers, and checks Dial returns on cancel without leaking.
func TestDialContextCancel(t *testing.T) {
	checkNoLeaks(t)

	serverAgent := newTestAgent(t, "Server")
	tlsConf := &tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{*serverAgent.Certificate},
		NextProtos:   []string{ALPN_OSP},
		ClientAuth:   tls.RequireAnyClientCert,
	}
	nl, err := (&QuicTransport{KeepAlive: KeepAliveConfig{Disabled: true}}).ListenAddr("127.0.0.1:0", tlsConf)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { nl.Close() })
	acceptedNC := make(chan NetworkConnection, 1)
	go func() {
		nc, _ := nl.Accept(context.Background())
		acceptedNC <- nc
	}()
	t.Cleanup(func() {
		if nc := <-acceptedNC; nc != nil {
			nc.Close()
		}
	})

	fp, err := serverAgent.CertificateFingerPrint()
	if err != nil {
		t.Fatal(err)
	}
	txt := TXTRecordSet{}
	txt.Set(txtKeyFingerprint, fp)
	txt.Set("sn", base64.RawURLEncoding.EncodeToString(serverAgent.Certificate.Leaf.SerialNumber.Bytes()))
	ra := discoveredAgentFor(t, serverAgent, nl.Addr(), txt)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_, err = ra.Dial(ctx, AgentTransportQUIC, newTestAgent(t, "Client"))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestStreamRouting checks that application streams have a single owner: the
// connection dispatches data channels internally while streams of other
// protocols reach AcceptStream intact.
func TestStreamRouting(t *testing.T) {
	checkNoLeaks(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	server, client := authenticatePair(t, []byte("1234"), []byte("1234"))
	if server.err != nil || client.err != nil {
		t.Fatalf("authentication failed: server: %v, client: %v", server.err, client.err)
	}

	// Server: answer agent-info-requests on raw streams.
	serverErr := make(chan error, 1)
	go func() {
		for i := 0; i < 3; i++ {
			s, err := server.conn.AcceptStream(ctx)
			if err != nil {
				serverErr <- err
				return
			}
			var req MsgAgentInfoRequest
			if err := ReadTypedMessage(s, TypeKeyAgentInfoRequest, &req); err != nil {
				serverErr <- err
				return
			}
			err = WriteTypedMessage(s, TypeKeyAgentInfoResponse, &MsgAgentInfoResponse{
				RequestID: req.RequestID,
				AgentInfo: MsgAgentInfo{DisplayName: "Server"},
			})
			s.Close()
			if err != nil {
				serverErr <- err
				return
			}
		}
		serverErr <- nil
	}()

	// Interleave a data channel with the request/response exchanges.
	dcErr := make(chan error, 1)
	go func() {
		dc, err := server.conn.AcceptDataChannel(ctx)
		if err != nil {
			dcErr <- err
			return
		}
		msg, err := dc.ReceiveMessage()
		if err == nil && string(msg) != "hello" {
			err = errors.New("unexpected data channel message")
		}
		dcErr <- err
	}()

	dc, err := client.conn.OpenDataChannel(ctx, DataChannelParameters{Label: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if err := dc.SendMessage([]byte("hello")); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 3; i++ {
		s, err := client.conn.OpenStream(ctx)
		if err != nil {
			t.Fatal(err)
		}
		reqID := RequestID(i + 100)
		if err := WriteTypedMessage(s, TypeKeyAgentInfoRequest, &MsgAgentInfoRequest{RequestID: reqID}); err != nil {
			t.Fatal(err)
		}
		var resp MsgAgentInfoResponse
		if err := ReadTypedMessage(s, TypeKeyAgentInfoResponse, &resp); err != nil {
			t.Fatal(err)
		}
		s.Close()
		if resp.RequestID != reqID || resp.AgentInfo.DisplayName != "Server" {
			t.Fatalf("unexpected response: %+v", resp)
		}
	}

	if err := <-serverErr; err != nil {
		t.Fatal(err)
	}
	if err := <-dcErr; err != nil {
		t.Fatal(err)
	}
	dc.Close()
}
