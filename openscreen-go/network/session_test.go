package ospc

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"
)

const testOrigin = "https://page.example"

func newSessionTestAgent(t *testing.T, name string, session bool) *Agent {
	t.Helper()
	conf := AgentConfig{
		DisplayName:      name,
		InteropQuirks:    InteropQuirks{Spake2SuiteP256: true},
		KeepAlive:        KeepAliveConfig{Disabled: true},
		SessionExtension: session,
	}
	if os.Getenv("OSPC_TEST_LOG") != "" {
		conf.Logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})).With("agent", name)
	}
	a, err := NewAgent(conf)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

// sessionServer is what the test's server does with an accepted connection.
type sessionServer func(ctx context.Context, uc *UnauthenticatedConnection) (*Connection, error)

// sessionClient is what the test's client does after DialSession.
type sessionClient func(ctx context.Context, uc *UnauthenticatedConnection) (*Connection, error)

// runSessionPair dials a session from a client agent to a listening server
// agent and runs both sides' steps.
func runSessionPair(t *testing.T, serverAgent, clientAgent *Agent, server sessionServer, client sessionClient) (serverRes, clientRes authResult) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	l, ra := startTestListener(t, serverAgent)

	serverCh := make(chan authResult, 1)
	go func() {
		uc, err := l.Accept(ctx)
		if err != nil {
			serverCh <- authResult{err: err}
			return
		}
		defer uc.Close()
		conn, err := server(ctx, uc)
		serverCh <- authResult{conn, err}
	}()

	clientCh := make(chan authResult, 1)
	go func() {
		uc, err := ra.DialSession(ctx, AgentTransportQUIC, clientAgent, testOrigin)
		if err != nil {
			clientCh <- authResult{err: err}
			return
		}
		defer uc.Close()
		conn, err := client(ctx, uc)
		clientCh <- authResult{conn, err}
	}()

	serverRes, clientRes = <-serverCh, <-clientCh
	for _, r := range []authResult{serverRes, clientRes} {
		if r.conn != nil {
			conn := r.conn
			t.Cleanup(func() { conn.Close() })
		}
	}
	return serverRes, clientRes
}

const sessionPSK = "123456"

// serverCeremony is a server that asks the client to authenticate and
// accepts the session afterwards.
func serverCeremony(t *testing.T, wantOrigin string) sessionServer {
	return func(ctx context.Context, uc *UnauthenticatedConnection) (*Connection, error) {
		origin, ok := uc.RequestingOrigin()
		if !ok || origin != wantOrigin {
			return nil, errors.New("wrong requesting origin " + origin)
		}
		if err := uc.RequestAuthentication(); err != nil {
			return nil, err
		}
		if _, err := uc.AcceptAuthenticate(ctx); err != nil {
			return nil, err
		}
		if _, err := uc.AuthenticatePSK(ctx, []byte(sessionPSK)); err != nil {
			return nil, err
		}
		return uc.CompleteSession(ctx)
	}
}

// clientCeremony is a client that expects to be asked to authenticate.
func clientCeremony(ctx context.Context, uc *UnauthenticatedConnection) (*Connection, error) {
	cue, err := uc.AwaitSession(ctx)
	if err != nil {
		return nil, err
	}
	if cue != SessionCueAuthenticate {
		return nil, errors.New("unexpected cue " + cue.String())
	}
	if _, err := uc.AcceptAuthenticate(ctx); err != nil {
		return nil, err
	}
	if _, err := uc.AuthenticatePSK(ctx, []byte(sessionPSK)); err != nil {
		return nil, err
	}
	return uc.CompleteSession(ctx)
}

// checkDataChannel opens a data channel from client to server and sends a
// message on it, to check the session is usable.
func checkDataChannel(t *testing.T, server, client *Connection) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	got := make(chan error, 1)
	go func() {
		dc, err := server.AcceptDataChannel(ctx)
		if err != nil {
			got <- err
			return
		}
		defer dc.Close()
		msg, err := dc.ReceiveMessage()
		if err == nil && string(msg) != "hi" {
			err = errors.New("unexpected message")
		}
		got <- err
	}()
	dc, err := client.OpenDataChannel(ctx, DataChannelInit{Label: "t"})
	if err != nil {
		t.Fatal(err)
	}
	defer dc.Close()
	if err := dc.SendMessage([]byte("hi")); err != nil {
		t.Fatal(err)
	}
	if err := <-got; err != nil {
		t.Fatal(err)
	}
}

func TestSessionAcceptWithCeremony(t *testing.T) {
	checkNoLeaks(t)
	server, client := runSessionPair(t,
		newSessionTestAgent(t, "Server", true), newSessionTestAgent(t, "Client", false),
		serverCeremony(t, testOrigin), clientCeremony)
	if server.err != nil || client.err != nil {
		t.Fatalf("session failed: server %v, client %v", server.err, client.err)
	}
	if !server.conn.Authenticated() || !client.conn.Authenticated() {
		t.Error("connections not authenticated")
	}
	checkDataChannel(t, server.conn, client.conn)
}

func TestSessionRefusedOnHint(t *testing.T) {
	checkNoLeaks(t)
	server, client := runSessionPair(t,
		newSessionTestAgent(t, "Server", true), newSessionTestAgent(t, "Client", false),
		func(ctx context.Context, uc *UnauthenticatedConnection) (*Connection, error) {
			origin, ok := uc.RequestingOrigin()
			if !ok || origin != testOrigin {
				return nil, errors.New("wrong requesting origin " + origin)
			}
			// The hint isn't allowed: refuse before any authentication.
			return nil, uc.RefuseSession()
		},
		func(ctx context.Context, uc *UnauthenticatedConnection) (*Connection, error) {
			_, err := uc.AwaitSession(ctx)
			if err == nil {
				err = errors.New("session not refused")
			}
			// The server sent no authentication message at all.
			if uc.RemoteAgent().HasAuthenticationInfo() {
				return nil, errors.New("server sent auth-capabilities before refusing")
			}
			return nil, err
		})
	if server.err != nil {
		t.Errorf("server: %v", server.err)
	}
	if !errors.Is(client.err, ErrSessionRefused) {
		t.Errorf("client: got %v, want ErrSessionRefused", client.err)
	}
}

func TestSessionTrustedPeers(t *testing.T) {
	checkNoLeaks(t)
	// Both agents trust each other's fingerprint: no auth-capabilities, no
	// SPAKE2, just success.
	server, client := runSessionPair(t,
		newSessionTestAgent(t, "Server", true), newSessionTestAgent(t, "Client", false),
		func(ctx context.Context, uc *UnauthenticatedConnection) (*Connection, error) {
			if _, ok := uc.RequestingOrigin(); !ok {
				return nil, errors.New("no requesting origin")
			}
			if err := uc.TrustPeer(); err != nil {
				return nil, err
			}
			return uc.CompleteSession(ctx)
		},
		func(ctx context.Context, uc *UnauthenticatedConnection) (*Connection, error) {
			cue, err := uc.AwaitSession(ctx)
			if err != nil {
				return nil, err
			}
			if cue != SessionCueAccepted {
				return nil, errors.New("unexpected cue " + cue.String())
			}
			if uc.RemoteAgent().HasAuthenticationInfo() {
				return nil, errors.New("server sent auth-capabilities to a trusted client")
			}
			if err := uc.TrustPeer(); err != nil {
				return nil, err
			}
			return uc.CompleteSession(ctx)
		})
	if server.err != nil || client.err != nil {
		t.Fatalf("session failed: server %v, client %v", server.err, client.err)
	}
	if !server.conn.Authenticated() || !client.conn.Authenticated() {
		t.Error("connections not authenticated")
	}
	checkDataChannel(t, server.conn, client.conn)
}

// TestSessionClientHoldsEarlyStreams: the server opens a data channel right
// after sending success; the client, which treats the session as accepted
// only later, holds it until then.
func TestSessionClientHoldsEarlyStreams(t *testing.T) {
	checkNoLeaks(t)
	var serverConn *Connection
	opened := make(chan error, 1)
	server, client := runSessionPair(t,
		newSessionTestAgent(t, "Server", true), newSessionTestAgent(t, "Client", false),
		func(ctx context.Context, uc *UnauthenticatedConnection) (*Connection, error) {
			conn, err := serverCeremony(t, testOrigin)(ctx, uc)
			if err != nil {
				return nil, err
			}
			serverConn = conn
			go func() {
				// Outlives the pair's context, which ends when both sides returned.
				octx, cancel := context.WithTimeout(context.Background(), testTimeout)
				defer cancel()
				dc, err := conn.OpenDataChannel(octx, DataChannelInit{Label: "early"})
				if err == nil {
					err = dc.SendMessage([]byte("hi"))
				}
				opened <- err
			}()
			return conn, nil
		},
		func(ctx context.Context, uc *UnauthenticatedConnection) (*Connection, error) {
			if _, err := uc.AwaitSession(ctx); err != nil {
				return nil, err
			}
			if _, err := uc.AcceptAuthenticate(ctx); err != nil {
				return nil, err
			}
			if _, err := uc.AuthenticatePSK(ctx, []byte(sessionPSK)); err != nil {
				return nil, err
			}
			// Let the server's data channel arrive before the session is
			// treated as accepted.
			time.Sleep(300 * time.Millisecond)
			if uc.base.app.Load() != nil {
				return nil, errors.New("client handed off before CompleteSession")
			}
			return uc.CompleteSession(ctx)
		})
	if server.err != nil || client.err != nil {
		t.Fatalf("session failed: server %v, client %v", server.err, client.err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	dc, err := client.conn.AcceptDataChannel(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer dc.Close()
	if msg, err := dc.ReceiveMessage(); err != nil || string(msg) != "hi" {
		t.Fatalf("received %q, %v", msg, err)
	}
	if err := <-opened; err != nil {
		t.Fatal(err)
	}
	_ = serverConn
}

// TestSessionLateCeremonyFails: a server that accepted on trust answers a
// ceremony the client starts afterwards, and closes the connection when it
// fails.
func TestSessionLateCeremonyFails(t *testing.T) {
	checkNoLeaks(t)
	server, client := runSessionPair(t,
		newSessionTestAgent(t, "Server", true), newSessionTestAgent(t, "Client", false),
		func(ctx context.Context, uc *UnauthenticatedConnection) (*Connection, error) {
			if err := uc.TrustPeer(); err != nil {
				return nil, err
			}
			if _, err := uc.CompleteSession(ctx); err != nil {
				return nil, err
			}
			_, err := uc.AuthenticatePSK(ctx, []byte("111111"))
			return nil, err
		},
		func(ctx context.Context, uc *UnauthenticatedConnection) (*Connection, error) {
			if _, err := uc.AwaitSession(ctx); err != nil {
				return nil, err
			}
			_, err := uc.AuthenticatePSK(ctx, []byte("222222"))
			return nil, err
		})
	if !errors.Is(server.err, ErrAuthenticationFailed) && !errors.Is(client.err, ErrAuthenticationFailed) {
		t.Fatalf("late ceremony with different PSKs succeeded: server %v, client %v", server.err, client.err)
	}
}

// TestSessionAcceptedUntrustedServer: the server trusts the client and
// accepts right away, but the client doesn't trust the server, so it
// authenticates after the success response; the server only answers.
func TestSessionAcceptedUntrustedServer(t *testing.T) {
	checkNoLeaks(t)
	server, client := runSessionPair(t,
		newSessionTestAgent(t, "Server", true), newSessionTestAgent(t, "Client", false),
		func(ctx context.Context, uc *UnauthenticatedConnection) (*Connection, error) {
			if err := uc.TrustPeer(); err != nil {
				return nil, err
			}
			conn, err := uc.CompleteSession(ctx)
			if err != nil {
				return nil, err
			}
			// Answer the client's authentication, as the ua would.
			if _, err := uc.AuthenticatePSK(ctx, []byte(sessionPSK)); err != nil {
				return nil, err
			}
			return conn, nil
		},
		func(ctx context.Context, uc *UnauthenticatedConnection) (*Connection, error) {
			cue, err := uc.AwaitSession(ctx)
			if err != nil {
				return nil, err
			}
			if cue != SessionCueAccepted {
				return nil, errors.New("unexpected cue " + cue.String())
			}
			if _, err := uc.CompleteSession(ctx); !errors.Is(err, ErrSessionNotVerified) {
				return nil, errors.New("CompleteSession before verifying the server succeeded")
			}
			if _, err := uc.AuthenticatePSK(ctx, []byte(sessionPSK)); err != nil {
				return nil, err
			}
			return uc.CompleteSession(ctx)
		})
	if server.err != nil || client.err != nil {
		t.Fatalf("session failed: server %v, client %v", server.err, client.err)
	}
	checkDataChannel(t, server.conn, client.conn)
}

func TestSessionSecondRequestCloses(t *testing.T) {
	checkNoLeaks(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	l, ra := startTestListener(t, newSessionTestAgent(t, "Server", true))

	// The server closes the connection on the second request, usually
	// before the listener hands it to Accept; if Accept does return it, the
	// connection is closed with ErrSessionProtocol.
	accepted := make(chan *UnauthenticatedConnection, 1)
	go func() {
		uc, err := l.Accept(ctx)
		if err != nil {
			uc = nil
		}
		accepted <- uc
	}()

	uc, err := ra.DialSession(ctx, AgentTransportQUIC, newSessionTestAgent(t, "Client", false), testOrigin)
	if err != nil {
		t.Fatal(err)
	}
	defer uc.Close()
	s, err := uc.base.netConn.(networkStreamOpener).OpenNetworkStream(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeMessage(&msgLp2pSessionRequest{RequestingOrigin: testOrigin}, s); err != nil {
		t.Fatal(err)
	}
	if _, err := uc.AwaitSession(ctx); err == nil {
		t.Fatal("session survived a second lp2p-session-request")
	}
	select {
	case <-uc.base.ctx.Done():
	case <-ctx.Done():
		t.Fatal("client connection not closed")
	}

	l.Close() // Unblocks Accept if the listener never handed the connection out.
	if serverConn := <-accepted; serverConn != nil {
		defer serverConn.Close()
		select {
		case <-serverConn.base.ctx.Done():
		case <-ctx.Done():
			t.Fatal("server connection not closed")
		}
		if err := serverConn.base.err(); !errors.Is(err, ErrSessionProtocol) {
			t.Errorf("server: got %v, want ErrSessionProtocol", err)
		}
	}
}

// TestSessionLegacyServer: an agent without the session extension refuses
// sessions, so a client that needs one fails right away.
func TestSessionLegacyServer(t *testing.T) {
	checkNoLeaks(t)
	server, client := runSessionPair(t,
		newSessionTestAgent(t, "Server", false), newSessionTestAgent(t, "Client", false),
		func(ctx context.Context, uc *UnauthenticatedConnection) (*Connection, error) {
			select {
			case <-uc.base.ctx.Done():
				return nil, uc.base.err()
			case <-ctx.Done():
				return nil, errors.New("connection not closed")
			}
		},
		func(ctx context.Context, uc *UnauthenticatedConnection) (*Connection, error) {
			// A plain OSP server sends auth-capabilities right away, which
			// is a valid cue; the refusal follows.
			if _, err := uc.AwaitSession(ctx); err != nil {
				return nil, err
			}
			select {
			case <-uc.base.ctx.Done():
				return nil, uc.base.err()
			case <-ctx.Done():
				return nil, errors.New("connection not closed")
			}
		})
	if !errors.Is(server.err, ErrSessionProtocol) {
		t.Errorf("server: got %v, want ErrSessionProtocol", server.err)
	}
	if !errors.Is(client.err, ErrSessionRefused) {
		t.Errorf("client: got %v, want ErrSessionRefused", client.err)
	}
}

// TestSessionLegacyClient: a listener with the session extension closes a
// connection from a client that sends no lp2p-session-request.
func TestSessionLegacyClient(t *testing.T) {
	checkNoLeaks(t)
	defer func(d time.Duration) { infoExchangeTimeout = d }(infoExchangeTimeout)
	infoExchangeTimeout = 500 * time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	l, ra := startTestListener(t, newSessionTestAgent(t, "Server", true))
	accepted := make(chan error, 1)
	go func() {
		_, err := l.Accept(ctx)
		accepted <- err
	}()

	// The server withholds auth-capabilities, so the plain Dial times out
	// too; either way the connection ends without being accepted.
	uc, err := ra.Dial(ctx, AgentTransportQUIC, newSessionTestAgent(t, "Client", false))
	if err == nil {
		defer uc.Close()
		select {
		case <-uc.base.ctx.Done():
		case <-ctx.Done():
			t.Fatal("connection without lp2p-session-request not closed")
		}
	} else if !errors.Is(err, ErrNoAgentInfo) {
		t.Fatalf("Dial: got %v, want ErrNoAgentInfo", err)
	}
	select {
	case err := <-accepted:
		t.Fatalf("Accept returned %v for a client without lp2p-session-request", err)
	case <-time.After(infoExchangeTimeout):
	}
	l.Close()
	if err := <-accepted; !errors.Is(err, ErrListenerClosed) {
		t.Errorf("Accept: got %v, want ErrListenerClosed", err)
	}
}

func TestSessionUnsupportedPeer(t *testing.T) {
	checkNoLeaks(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	// A DTLS transport can't open the session stream.
	serverAgent := newSessionTestAgent(t, "Server", true)
	l := NewListener(serverAgent, AgentTransportWebRTC)
	if err := l.start("127.0.0.1:0", false); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	ra := discoveredAgentFor(t, serverAgent, l.netListener.Addr(), l.txt)
	go func() {
		if uc, err := l.Accept(ctx); err == nil {
			uc.Close()
		}
	}()
	_, err := ra.DialSession(ctx, AgentTransportWebRTC, newSessionTestAgent(t, "Client", false), testOrigin)
	if !errors.Is(err, ErrSessionUnsupported) {
		t.Fatalf("got %v, want ErrSessionUnsupported", err)
	}
}
