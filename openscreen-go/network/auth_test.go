package ospc

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"testing"
	"time"

	spake2 "github.com/backkem/spake2-go"
)

func TestParsePSK(t *testing.T) {
	for in, want := range map[string]string{
		"61488548833":     "61488548833",
		"0614-8854-8833":  "61488548833",
		" 0614 8854-8833": "61488548833",
		"001":             "1",
		"000-123-456":     "123456",
	} {
		got, err := ParsePSK(in)
		if err != nil || got != want {
			t.Errorf("ParsePSK(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "-", "0", "00-00", "12a4", "0x12"} {
		if _, err := ParsePSK(bad); !errors.Is(err, ErrInvalidPSK) {
			t.Errorf("ParsePSK(%q) = %v, want ErrInvalidPSK", bad, err)
		}
	}
}

func TestFormatPSK(t *testing.T) {
	for in, want := range map[string]string{
		"61488548833": "0614-8854-8833", // network.bs Appendix B example.
		"1":           "001",
		"123456":      "123-456",
		"12345678":    "012-345-678",
		"123456789":   "0001-2345-6789",
		"000123":      "123",
	} {
		got, err := FormatPSK(in)
		if err != nil || got != want {
			t.Errorf("FormatPSK(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
}

func TestGeneratePSKEntropy(t *testing.T) {
	for i := 0; i < 100; i++ {
		psk, err := generatePSK(20)
		if err != nil {
			t.Fatal(err)
		}
		canonical, err := ParsePSK(string(psk))
		if err != nil || canonical != string(psk) {
			t.Fatalf("generated PSK %q is not canonical: %q, %v", psk, canonical, err)
		}
		if len(psk) > 7 { // 2^20 has 7 digits.
			t.Fatalf("generated PSK %q exceeds 20 bits", psk)
		}
	}
}

// TestSpake2PasswordHashSHA512 pins how newSpake2 gets spake2-go to hash the
// password with SHA-512 while keeping SHA-256 for the transcript.
func TestSpake2PasswordHashSHA512(t *testing.T) {
	pw, idA, idB := []byte("123456"), []byte("client-fp"), []byte("server-fp")

	run := func(alice, bob *spake2.SPAKE2) ([]byte, error) {
		pA, err := alice.Start()
		if err != nil {
			return nil, err
		}
		pB, err := bob.Exchange(pA)
		if err != nil {
			return nil, err
		}
		cA, err := alice.Finish(pB)
		if err != nil {
			return nil, err
		}
		if len(cA) != sha256.Size {
			t.Errorf("confirmation is %d bytes, want HMAC-SHA-256's %d", len(cA), sha256.Size)
		}
		cB, err := bob.Confirm(cA)
		if err != nil {
			return nil, err
		}
		if err := alice.Verify(cB); err != nil {
			return nil, err
		}
		return alice.SharedKey()
	}

	ke, err := run(newSpake2(true, pw, idA, idB), newSpake2(false, pw, idA, idB))
	if err != nil {
		t.Fatalf("SPAKE2 failed: %v", err)
	}
	// Ke is the first half of the transcript hash: 16 bytes for SHA-256.
	if len(ke) != sha256.Size/2 {
		t.Errorf("Ke is %d bytes, want %d: the transcript hash isn't SHA-256", len(ke), sha256.Size/2)
	}

	// A peer deriving w with SHA-256 must not match.
	sha256Bob := spake2.NewServer(pw, &spake2.Options{
		Ciphersuite: spake2.DefaultCiphersuite(),
		IdentityA:   idA,
		IdentityB:   idB,
	})
	if _, err := run(newSpake2(true, pw, idA, idB), sha256Bob); err == nil {
		t.Error("SPAKE2 with a SHA-256 password hash peer succeeded, w isn't SHA-512")
	}
}

type authPairConfig struct {
	serverEase, clientEase int
	serverDelay            time.Duration // Before the server calls AuthenticatePSK.
	clientDelay            time.Duration
	serverPSK, clientPSK   string
	serverP256, clientP256 bool
}

func newAuthTestAgent(t *testing.T, name string, ease int, p256 bool) *Agent {
	t.Helper()
	a, err := NewAgent(AgentConfig{
		DisplayName:   name,
		KeepAlive:     KeepAliveConfig{Disabled: true},
		PSKConfig:     PSKConfig{EaseOfInput: ease},
		InteropQuirks: InteropQuirks{Spake2SuiteP256: p256},
	})
	if err != nil {
		t.Fatal(err)
	}
	return a
}

// runAuthPair authenticates a dialing client with a listening server.
func runAuthPair(t *testing.T, cfg authPairConfig) (server, client authResult, serverRole, clientRole AuthenticationRole) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	l, ra := startTestListener(t, newAuthTestAgent(t, "Server", cfg.serverEase, cfg.serverP256))
	clientAgent := newAuthTestAgent(t, "Client", cfg.clientEase, cfg.clientP256)

	serverCh := make(chan authResult, 1)
	go func() {
		uConn, err := l.Accept(ctx)
		if err != nil {
			serverCh <- authResult{err: err}
			return
		}
		defer uConn.Close()
		if serverRole, err = uConn.AcceptAuthenticate(ctx); err != nil {
			serverCh <- authResult{err: err}
			return
		}
		time.Sleep(cfg.serverDelay)
		conn, err := uConn.AuthenticatePSK(ctx, []byte(cfg.serverPSK))
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
		if clientRole, err = uConn.AcceptAuthenticate(ctx); err != nil {
			clientCh <- authResult{err: err}
			return
		}
		time.Sleep(cfg.clientDelay)
		conn, err := uConn.AuthenticatePSK(ctx, []byte(cfg.clientPSK))
		clientCh <- authResult{conn, err}
	}()

	server, client = <-serverCh, <-clientCh
	for _, r := range []authResult{server, client} {
		if r.conn != nil {
			conn := r.conn
			t.Cleanup(func() { conn.Close() })
		}
	}
	return server, client, serverRole, clientRole
}

func TestAuthenticationFlows(t *testing.T) {
	const delay = 200 * time.Millisecond
	for _, tc := range []struct {
		name                   string
		cfg                    authPairConfig
		serverRole, clientRole AuthenticationRole
	}{{
		// Equal ease of input: the server presents.
		name:       "presenter initiates",
		cfg:        authPairConfig{clientDelay: delay},
		serverRole: AuthenticationRolePresenter, clientRole: AuthenticationRoleConsumer,
	}, {
		name:       "consumer initiates",
		cfg:        authPairConfig{serverDelay: delay},
		serverRole: AuthenticationRolePresenter, clientRole: AuthenticationRoleConsumer,
	}, {
		name:       "both initiate",
		cfg:        authPairConfig{},
		serverRole: AuthenticationRolePresenter, clientRole: AuthenticationRoleConsumer,
	}, {
		// The lower ease of input presents.
		name:       "client presents",
		cfg:        authPairConfig{clientEase: 10, serverEase: 90, serverDelay: delay},
		serverRole: AuthenticationRoleConsumer, clientRole: AuthenticationRolePresenter,
	}, {
		name:       "client presents, consumer initiates",
		cfg:        authPairConfig{clientEase: 10, serverEase: 90, clientDelay: delay},
		serverRole: AuthenticationRoleConsumer, clientRole: AuthenticationRolePresenter,
	}, {
		// Leading zeros and dashes don't change the PSK.
		name:       "PSK formatting",
		cfg:        authPairConfig{serverPSK: "001-234", clientPSK: "1234"},
		serverRole: AuthenticationRolePresenter, clientRole: AuthenticationRoleConsumer,
	}} {
		t.Run(tc.name, func(t *testing.T) {
			checkNoLeaks(t)
			cfg := tc.cfg
			cfg.serverP256, cfg.clientP256 = true, true
			if cfg.serverPSK == "" {
				cfg.serverPSK, cfg.clientPSK = "123456", "123-456"
			}
			server, client, serverRole, clientRole := runAuthPair(t, cfg)
			if server.err != nil || client.err != nil {
				t.Fatalf("authentication failed: server %v, client %v", server.err, client.err)
			}
			if serverRole != tc.serverRole || clientRole != tc.clientRole {
				t.Errorf("roles: server %v, client %v; want %v, %v", serverRole, clientRole, tc.serverRole, tc.clientRole)
			}
			if !server.conn.RemoteAgent().Authenticated() || !client.conn.RemoteAgent().Authenticated() {
				t.Error("remote agent not marked authenticated")
			}
		})
	}
}

func TestAuthenticationWrongPSK(t *testing.T) {
	checkNoLeaks(t)
	server, client, _, _ := runAuthPair(t, authPairConfig{
		serverPSK: "123456", clientPSK: "123457", serverP256: true, clientP256: true,
	})
	if server.err == nil || client.err == nil {
		t.Fatalf("authentication with different PSKs succeeded: server %v, client %v", server.err, client.err)
	}
}

func TestAuthenticationSuiteUnsupported(t *testing.T) {
	checkNoLeaks(t)
	server, client, _, _ := runAuthPair(t, authPairConfig{
		serverPSK: "123456", clientPSK: "123456", serverP256: true, clientP256: false,
	})
	if !errors.Is(client.err, ErrSpake2SuiteUnsupported) {
		t.Errorf("client: got %v, want ErrSpake2SuiteUnsupported", client.err)
	}
	if server.err == nil {
		t.Error("server authenticated with a client that can't")
	}
}

func TestAuthenticatePSKInvalid(t *testing.T) {
	checkNoLeaks(t)
	server, client, _, _ := runAuthPair(t, authPairConfig{
		serverPSK: "123456", clientPSK: "12a456", serverP256: true, clientP256: true,
	})
	if !errors.Is(client.err, ErrInvalidPSK) {
		t.Errorf("client: got %v, want ErrInvalidPSK", client.err)
	}
	_ = server // The server waits for the client until the test timeout.
}

func TestSpake2IdentitiesClientServer(t *testing.T) {
	c := &baseConnection{
		localAgent:  &Agent{PeerID: "local"},
		remoteAgent: &Agent{PeerID: "remote"},
	}
	c.agentRole = AgentRoleClient
	if a, b := c.spake2Identities(); !bytes.Equal(a, []byte("local")) || !bytes.Equal(b, []byte("remote")) {
		t.Errorf("client: A=%s B=%s", a, b)
	}
	c.agentRole = AgentRoleServer
	if a, b := c.spake2Identities(); !bytes.Equal(a, []byte("remote")) || !bytes.Equal(b, []byte("local")) {
		t.Errorf("server: A=%s B=%s", a, b)
	}
}
