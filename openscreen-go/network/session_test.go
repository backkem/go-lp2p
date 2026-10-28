package ospc

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/fxamacker/cbor/v2"
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

// ceremony runs the PSK exchange on one side with psk: the consumer first
// asks for the PSK to be shown, as a user agent does before asking its user.
func ceremony(ctx context.Context, uc *UnauthenticatedConnection, psk string) error {
	if err := uc.StartAuthentication(); err != nil {
		return err
	}
	if _, err := uc.AcceptAuthenticate(ctx); err != nil {
		return err
	}
	_, err := uc.AuthenticatePSK(ctx, []byte(psk))
	return err
}

// serverFlow is a server driver: it trusts the client if trustClient, runs
// a ceremony on each authentication cue, and accepts the session if allow
// accepts the requesting origin. cues receives every cue in order.
func serverFlow(trustClient bool, allow func(origin string) bool, cues *[]SessionCue) sessionServer {
	return func(ctx context.Context, uc *UnauthenticatedConnection) (*Connection, error) {
		for {
			cue, err := uc.AwaitSession(ctx)
			if err != nil {
				return nil, err
			}
			if cues != nil {
				*cues = append(*cues, cue)
			}
			switch cue {
			case SessionCueAuthenticate:
				if err := ceremony(ctx, uc, sessionPSK); err != nil {
					return nil, err
				}
			case SessionCueRequested:
				if _, ok := uc.RequestingOrigin(); ok && !uc.RemoteAgent().Authenticated() {
					return nil, errors.New("requesting origin exposed before the client was verified")
				}
				if trustClient {
					if err := uc.TrustPeer(); err != nil {
						return nil, err
					}
				} else if !uc.RemoteAgent().Authenticated() {
					if err := uc.RequireAuthentication(); err != nil {
						return nil, err
					}
					if cue, err := uc.AwaitSession(ctx); err != nil || cue != SessionCueAuthenticate {
						return nil, errors.New("no authentication cue after authentication-required")
					}
					if cues != nil {
						*cues = append(*cues, SessionCueAuthenticate)
					}
					if err := ceremony(ctx, uc, sessionPSK); err != nil {
						return nil, err
					}
				}
			}
			if !uc.RemoteAgent().Authenticated() {
				continue
			}
			origin, ok := uc.RequestingOrigin()
			if !ok {
				continue // Verified, but no request yet.
			}
			if !allow(origin) {
				return nil, uc.RefuseSession()
			}
			return uc.CompleteSession(ctx)
		}
	}
}

func allowAll(string) bool { return true }

// clientFlow is a client driver: it trusts the server if trustServer, else
// runs the ceremony first; then requests the session and follows the cues.
func clientFlow(trustServer bool, cues *[]SessionCue) sessionClient {
	return func(ctx context.Context, uc *UnauthenticatedConnection) (*Connection, error) {
		if trustServer {
			if err := uc.TrustPeer(); err != nil {
				return nil, err
			}
		} else {
			if err := uc.RequestSession(ctx); !errors.Is(err, ErrSessionNotVerified) {
				return nil, errors.New("lp2p-session-request allowed before verifying the server")
			}
			if err := ceremony(ctx, uc, sessionPSK); err != nil {
				return nil, err
			}
		}
		if err := uc.RequestSession(ctx); err != nil {
			return nil, err
		}
		for {
			cue, err := uc.AwaitSession(ctx)
			if err != nil {
				return nil, err
			}
			if cues != nil {
				*cues = append(*cues, cue)
			}
			switch cue {
			case SessionCueAuthenticate:
				if err := ceremony(ctx, uc, sessionPSK); err != nil {
					return nil, err
				}
			case SessionCueAccepted:
				return uc.CompleteSession(ctx)
			}
		}
	}
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

func cuesEqual(a []SessionCue, b ...SessionCue) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// consumerRequest returns the consumer's psk-needs-presentation the server
// received, which must carry no public-value.
func consumerRequest(t *testing.T, uc *UnauthenticatedConnection) {
	t.Helper()
	uc.base.mu.Lock()
	defer uc.base.mu.Unlock()
	hs := uc.base.authenticationState.remoteHandshake
	if hs == nil || hs.PskStatus != AuthSpake2PskStatusPskNeedsPresentation {
		t.Errorf("server got %+v, want the consumer's psk-needs-presentation", hs)
	} else if hs.PublicValue != nil {
		t.Errorf("psk-needs-presentation carries a public-value of %d bytes", len(hs.PublicValue))
	}
}

// TestConsumerRequestHasNoPublicValue checks the wire form of a consumer's
// psk-needs-presentation: key 2 is absent.
func TestConsumerRequestHasNoPublicValue(t *testing.T) {
	var buf bytes.Buffer
	if err := writeMessage(&msgAuthSpake2Handshake{PskStatus: AuthSpake2PskStatusPskNeedsPresentation}, &buf); err != nil {
		t.Fatal(err)
	}
	r := bytes.NewReader(buf.Bytes())
	if key, err := readTypeKey(r); err != nil || key != typeKeyAuthSpake2Handshake {
		t.Fatalf("type key %d, %v", key, err)
	}
	var m map[uint64]cbor.RawMessage
	if err := cbor.Unmarshal(buf.Bytes()[buf.Len()-r.Len():], &m); err != nil {
		t.Fatal(err)
	}
	if _, ok := m[2]; ok {
		t.Errorf("public-value (key 2) present: %x", buf.Bytes())
	}
	if _, ok := m[1]; !ok {
		t.Errorf("psk-status (key 1) missing: %x", buf.Bytes())
	}
	// And with pA (the presenter's psk-shown) it is present.
	buf.Reset()
	if err := writeMessage(&msgAuthSpake2Handshake{PskStatus: AuthSpake2PskStatusPskShown, PublicValue: []byte{1, 2, 3}}, &buf); err != nil {
		t.Fatal(err)
	}
	r = bytes.NewReader(buf.Bytes())
	readTypeKey(r)
	m = nil
	if err := cbor.Unmarshal(buf.Bytes()[buf.Len()-r.Len():], &m); err != nil {
		t.Fatal(err)
	}
	if _, ok := m[2]; !ok {
		t.Errorf("public-value (key 2) missing from psk-shown: %x", buf.Bytes())
	}
}

// TestSessionNeitherTrusts: the client, as PSK consumer, asks the server to
// show the PSK, verifies it, and only then sends the origin.
func TestSessionNeitherTrusts(t *testing.T) {
	checkNoLeaks(t)
	var serverCues, clientCues []SessionCue
	server, client := runSessionPair(t,
		newSessionTestAgent(t, "Server", true), newSessionTestAgent(t, "Client", false),
		func(ctx context.Context, uc *UnauthenticatedConnection) (*Connection, error) {
			conn, err := serverFlow(false, allowAll, &serverCues)(ctx, uc)
			if err == nil {
				consumerRequest(t, uc)
			}
			return conn, err
		}, clientFlow(false, &clientCues))
	if server.err != nil || client.err != nil {
		t.Fatalf("session failed: server %v, client %v", server.err, client.err)
	}
	// The request may already be there once the server's ceremony is done.
	if len(serverCues) == 0 || serverCues[0] != SessionCueAuthenticate {
		t.Errorf("server cues %v, want authenticate first", serverCues)
	}
	if !cuesEqual(clientCues, SessionCueAccepted) {
		t.Errorf("client cues %v, want [accepted]", clientCues)
	}
	checkDataChannel(t, server.conn, client.conn)
}

func TestSessionMutualTrust(t *testing.T) {
	checkNoLeaks(t)
	var serverCues, clientCues []SessionCue
	server, client := runSessionPair(t,
		newSessionTestAgent(t, "Server", true), newSessionTestAgent(t, "Client", false),
		serverFlow(true, allowAll, &serverCues), clientFlow(true, &clientCues))
	if server.err != nil || client.err != nil {
		t.Fatalf("session failed: server %v, client %v", server.err, client.err)
	}
	if !cuesEqual(serverCues, SessionCueRequested) || !cuesEqual(clientCues, SessionCueAccepted) {
		t.Errorf("cues: server %v, client %v; want no ceremony", serverCues, clientCues)
	}
	for _, r := range []authResult{server, client} {
		r.conn.base.mu.Lock()
		ran := r.conn.base.authenticationState != nil
		r.conn.base.mu.Unlock()
		if ran {
			t.Error("a ceremony ran between peers that trust each other")
		}
	}
	checkDataChannel(t, server.conn, client.conn)
}

// TestSessionServerForgotClient: the client trusts the server, so it sends
// the request right away; the server answers authentication-required and
// the client, as consumer, asks for the PSK and authenticates.
func TestSessionServerForgotClient(t *testing.T) {
	checkNoLeaks(t)
	var serverCues, clientCues []SessionCue
	server, client := runSessionPair(t,
		newSessionTestAgent(t, "Server", true), newSessionTestAgent(t, "Client", false),
		func(ctx context.Context, uc *UnauthenticatedConnection) (*Connection, error) {
			conn, err := serverFlow(false, allowAll, &serverCues)(ctx, uc)
			if err == nil {
				consumerRequest(t, uc)
			}
			return conn, err
		}, clientFlow(true, &clientCues))
	if server.err != nil || client.err != nil {
		t.Fatalf("session failed: server %v, client %v", server.err, client.err)
	}
	if !cuesEqual(serverCues, SessionCueRequested, SessionCueAuthenticate) {
		t.Errorf("server cues %v, want [requested authenticate]", serverCues)
	}
	if !cuesEqual(clientCues, SessionCueAuthenticate, SessionCueAccepted) {
		t.Errorf("client cues %v, want [authenticate accepted]", clientCues)
	}
	checkDataChannel(t, server.conn, client.conn)
}

// TestSessionClientForgotServer: the server trusts the client, but the
// client authenticates first anyway, before sending the origin.
func TestSessionClientForgotServer(t *testing.T) {
	checkNoLeaks(t)
	var serverCues, clientCues []SessionCue
	server, client := runSessionPair(t,
		newSessionTestAgent(t, "Server", true), newSessionTestAgent(t, "Client", false),
		serverFlow(true, allowAll, &serverCues), clientFlow(false, &clientCues))
	if server.err != nil || client.err != nil {
		t.Fatalf("session failed: server %v, client %v", server.err, client.err)
	}
	if len(serverCues) == 0 || serverCues[0] != SessionCueAuthenticate {
		t.Errorf("server cues %v, want authenticate first", serverCues)
	}
	if !cuesEqual(clientCues, SessionCueAccepted) {
		t.Errorf("client cues %v, want [accepted]", clientCues)
	}
	checkDataChannel(t, server.conn, client.conn)
}

// TestSessionClientPresents: with a lower ease of input the client is the
// presenter and initiates with psk-shown itself.
func TestSessionClientPresents(t *testing.T) {
	checkNoLeaks(t)
	clientAgent := newSessionTestAgent(t, "Client", false)
	clientAgent.authenticationInfo.PSKConfig.EaseOfInput = 0
	serverAgent := newSessionTestAgent(t, "Server", true)
	serverAgent.authenticationInfo.PSKConfig.EaseOfInput = 90
	server, client := runSessionPair(t, serverAgent, clientAgent,
		func(ctx context.Context, uc *UnauthenticatedConnection) (*Connection, error) {
			conn, err := serverFlow(false, allowAll, nil)(ctx, uc)
			if err == nil {
				uc.base.mu.Lock()
				hs := uc.base.authenticationState.remoteHandshake
				uc.base.mu.Unlock()
				if hs == nil || hs.PskStatus != AuthSpake2PskStatusPskShown || hs.PublicValue == nil {
					t.Errorf("server got %+v, want the client's psk-shown with pA", hs)
				}
			}
			return conn, err
		}, clientFlow(false, nil))
	if server.err != nil || client.err != nil {
		t.Fatalf("session failed: server %v, client %v", server.err, client.err)
	}
}

func TestSessionRefusedOrigin(t *testing.T) {
	checkNoLeaks(t)
	server, client := runSessionPair(t,
		newSessionTestAgent(t, "Server", true), newSessionTestAgent(t, "Client", false),
		serverFlow(true, func(origin string) bool { return origin != testOrigin }, nil),
		clientFlow(true, nil))
	if server.err != nil {
		t.Errorf("server: %v", server.err)
	}
	if !errors.Is(client.err, ErrSessionRefused) {
		t.Errorf("client: got %v, want ErrSessionRefused", client.err)
	}
}

// TestSessionWrongPINOnCue: an unverified client never learns whether its
// origin is accepted: the ceremony after authentication-required fails and
// the connection closes without a final response.
func TestSessionWrongPINOnCue(t *testing.T) {
	checkNoLeaks(t)
	var clientUC *UnauthenticatedConnection
	server, client := runSessionPair(t,
		newSessionTestAgent(t, "Server", true), newSessionTestAgent(t, "Client", false),
		func(ctx context.Context, uc *UnauthenticatedConnection) (*Connection, error) {
			if cue, err := uc.AwaitSession(ctx); err != nil || cue != SessionCueRequested {
				return nil, errors.New("want the request first")
			}
			if err := uc.RequireAuthentication(); err != nil {
				return nil, err
			}
			if cue, err := uc.AwaitSession(ctx); err != nil || cue != SessionCueAuthenticate {
				return nil, errors.New("want the authentication cue")
			}
			return nil, ceremony(ctx, uc, "111111")
		},
		func(ctx context.Context, uc *UnauthenticatedConnection) (*Connection, error) {
			clientUC = uc
			if err := uc.TrustPeer(); err != nil {
				return nil, err
			}
			if err := uc.RequestSession(ctx); err != nil {
				return nil, err
			}
			if cue, err := uc.AwaitSession(ctx); err != nil || cue != SessionCueAuthenticate {
				return nil, errors.New("want the authentication cue")
			}
			if err := ceremony(ctx, uc, "222222"); err != nil {
				return nil, err
			}
			return uc.CompleteSession(ctx)
		})
	if server.err == nil || client.err == nil {
		t.Fatalf("wrong PIN accepted: server %v, client %v", server.err, client.err)
	}
	if errors.Is(client.err, ErrSessionRefused) {
		t.Error("client got a final response despite failing the ceremony")
	}
	select {
	case <-clientUC.base.session.accepted:
		t.Error("client got success despite failing the ceremony")
	default:
	}
}

// TestSessionClientHoldsEarlyStreams: the server opens a data channel right
// after sending success; the client, which treats the session as accepted
// only later, holds it until then.
func TestSessionClientHoldsEarlyStreams(t *testing.T) {
	checkNoLeaks(t)
	opened := make(chan error, 1)
	server, client := runSessionPair(t,
		newSessionTestAgent(t, "Server", true), newSessionTestAgent(t, "Client", false),
		func(ctx context.Context, uc *UnauthenticatedConnection) (*Connection, error) {
			conn, err := serverFlow(true, allowAll, nil)(ctx, uc)
			if err != nil {
				return nil, err
			}
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
			if err := uc.TrustPeer(); err != nil {
				return nil, err
			}
			if err := uc.RequestSession(ctx); err != nil {
				return nil, err
			}
			if cue, err := uc.AwaitSession(ctx); err != nil || cue != SessionCueAccepted {
				return nil, errors.New("want accepted")
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
}

// TestSessionLateCeremonyFails: a server still answers a ceremony the
// client starts after success, and closes the connection when it fails.
func TestSessionLateCeremonyFails(t *testing.T) {
	checkNoLeaks(t)
	server, client := runSessionPair(t,
		newSessionTestAgent(t, "Server", true), newSessionTestAgent(t, "Client", false),
		func(ctx context.Context, uc *UnauthenticatedConnection) (*Connection, error) {
			if _, err := serverFlow(true, allowAll, nil)(ctx, uc); err != nil {
				return nil, err
			}
			if cue, err := uc.AwaitSession(ctx); err != nil || cue != SessionCueAuthenticate {
				return nil, errors.New("want the late authentication cue")
			}
			return nil, ceremony(ctx, uc, "111111")
		},
		func(ctx context.Context, uc *UnauthenticatedConnection) (*Connection, error) {
			if _, err := clientFlow(true, nil)(ctx, uc); err != nil {
				return nil, err
			}
			return nil, ceremony(ctx, uc, "222222")
		})
	if !errors.Is(server.err, ErrAuthenticationFailed) && !errors.Is(client.err, ErrAuthenticationFailed) {
		t.Fatalf("late ceremony with different PSKs succeeded: server %v, client %v", server.err, client.err)
	}
}

func TestSessionSecondRequestCloses(t *testing.T) {
	checkNoLeaks(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	l, ra := startTestListener(t, newSessionTestAgent(t, "Server", true))

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
	if err := uc.TrustPeer(); err != nil {
		t.Fatal(err)
	}
	if err := uc.RequestSession(ctx); err != nil {
		t.Fatal(err)
	}
	s, err := uc.base.netConn.(networkStreamOpener).OpenNetworkStream(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeMessage(&msgLp2pSessionRequest{RequestingOrigin: testOrigin}, s); err != nil {
		t.Fatal(err)
	}
	select {
	case <-uc.base.ctx.Done():
	case <-ctx.Done():
		t.Fatal("client connection not closed")
	}

	// The server usually closes the connection before the listener hands
	// it to Accept; if Accept does return it, it is closed with
	// ErrSessionProtocol.
	l.Close()
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
			if err := uc.TrustPeer(); err != nil {
				return nil, err
			}
			if err := uc.RequestSession(ctx); err != nil {
				return nil, err
			}
			_, err := uc.AwaitSession(ctx)
			return nil, err
		})
	if !errors.Is(server.err, ErrSessionProtocol) {
		t.Errorf("server: got %v, want ErrSessionProtocol", server.err)
	}
	if !errors.Is(client.err, ErrSessionRefused) {
		t.Errorf("client: got %v, want ErrSessionRefused", client.err)
	}
}

// TestSessionLegacyClient: a plain OSP client reaches a listener with the
// session extension as an unauthenticated connection; its SPAKE2 is
// answered like a ceremony, but no session is ever requested.
func TestSessionLegacyClient(t *testing.T) {
	checkNoLeaks(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	l, ra := startTestListener(t, newSessionTestAgent(t, "Server", true))
	serverCh := make(chan error, 1)
	go func() {
		uc, err := l.Accept(ctx)
		if err != nil {
			serverCh <- err
			return
		}
		defer uc.Close()
		cue, err := uc.AwaitSession(ctx)
		if err != nil {
			serverCh <- err
			return
		}
		if cue != SessionCueAuthenticate {
			serverCh <- errors.New("unexpected cue " + cue.String())
			return
		}
		if err := ceremony(ctx, uc, sessionPSK); err != nil {
			serverCh <- err
			return
		}
		short, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
		defer cancel()
		if _, err := uc.AwaitSession(short); !errors.Is(err, context.DeadlineExceeded) {
			serverCh <- errors.New("a plain client requested a session")
			return
		}
		serverCh <- nil
	}()

	uc, err := ra.Dial(ctx, AgentTransportQUIC, newSessionTestAgent(t, "Client", false))
	if err != nil {
		t.Fatal(err)
	}
	defer uc.Close()
	conn, err := uc.AuthenticatePSK(ctx, []byte(sessionPSK))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := <-serverCh; err != nil {
		t.Fatal(err)
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
	uc, err := ra.DialSession(ctx, AgentTransportWebRTC, newSessionTestAgent(t, "Client", false), testOrigin)
	if err != nil {
		t.Fatal(err)
	}
	defer uc.Close()
	if err := uc.TrustPeer(); err != nil {
		t.Fatal(err)
	}
	if err := uc.RequestSession(ctx); !errors.Is(err, ErrSessionUnsupported) {
		t.Fatalf("got %v, want ErrSessionUnsupported", err)
	}
}
