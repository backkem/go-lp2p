package ospc

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"io"
	"math/big"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/fxamacker/cbor/v2"
	"github.com/quic-go/quic-go"
)

// osp_demo-like certificate: CN "Open Screen Certificate", no SAN, serial 1.
func newPlainCert(t *testing.T) *tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Open Screen Certificate"},
		Issuer:                pkix.Name{CommonName: "Open Screen Certificate"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	raw, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(raw)
	if err != nil {
		t.Fatal(err)
	}
	return &tls.Certificate{Certificate: [][]byte{raw}, PrivateKey: key, Leaf: leaf}
}

func newQuirkAgent(t *testing.T, name string, q InteropQuirks, cert *tls.Certificate) *Agent {
	t.Helper()
	conf := AgentConfig{
		DisplayName:   name,
		KeepAlive:     KeepAliveConfig{Disabled: true},
		InteropQuirks: q,
		Certificate:   cert,
	}
	if cert != nil {
		fp, err := certificateFingerPrint(cert.Leaf)
		if err != nil {
			t.Fatal(err)
		}
		conf.PeerID = PeerID(fp)
	}
	a, err := NewAgent(conf)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

// startDirectListener starts a listener without mDNS and returns it with a
// DiscoveredAgent for it, using the exported direct-dial API.
func startDirectListener(t *testing.T, a *Agent) (*Listener, *DiscoveredAgent) {
	t.Helper()
	l := NewListener(a, AgentTransportQUIC)
	if err := l.StartWithoutAdvertising("127.0.0.1:0"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	ra, err := DirectAgent(a.Info().DisplayName, l.Addr().(*net.UDPAddr), l.TXT())
	if err != nil {
		t.Fatal(err)
	}
	return l, ra
}

// acceptAsync accepts one connection on l.
func acceptAsync(t *testing.T, ctx context.Context, l *Listener) <-chan *UnauthenticatedConnection {
	ch := make(chan *UnauthenticatedConnection, 1)
	go func() {
		uc, err := l.Accept(ctx)
		if err != nil {
			t.Logf("accept: %v", err)
			ch <- nil
			return
		}
		ch <- uc
	}()
	return ch
}

func TestDialWithoutSN(t *testing.T) {
	checkNoLeaks(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	server := newTestAgent(t, "Server NoSN")
	l, ra := startDirectListener(t, server)
	delete(ra.TXT, "sn")
	if got, want := ra.serverName(), "Server-NoSN.local"; got != want {
		t.Fatalf("server name without sn: got %q, want %q", got, want)
	}

	accepted := acceptAsync(t, ctx, l)
	uc, err := ra.Dial(ctx, AgentTransportQUIC, newTestAgent(t, "Client"))
	if err != nil {
		t.Fatalf("Dial without sn: %v", err)
	}
	defer uc.Close()
	if sc := <-accepted; sc == nil {
		t.Fatal("listener didn't accept")
	} else {
		defer sc.Close()
	}
}

func TestVerifyFingerprint(t *testing.T) {
	cert := newPlainCert(t).Leaf
	other := newPlainCert(t).Leaf
	spki, _ := certificateFingerPrint(cert)
	der := wholeCertificateFingerPrint(cert)
	if spki == der {
		t.Fatal("SPKI and DER fingerprints should differ")
	}

	for _, tc := range []struct {
		name   string
		fp     string
		quirks InteropQuirks
		ok     bool
	}{
		{"spki", spki, InteropQuirks{}, true},
		{"spki with quirk", spki, InteropQuirks{AcceptCertificateFingerprint: true}, true},
		{"der", der, InteropQuirks{}, false},
		{"der with quirk", der, InteropQuirks{AcceptCertificateFingerprint: true}, true},
		{"other cert", wholeCertificateFingerPrint(other), InteropQuirks{AcceptCertificateFingerprint: true}, false},
		{"garbage", "AAAA", InteropQuirks{}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := verifyFingerprint(tc.fp, cert, tc.quirks)
			if (err == nil) != tc.ok {
				t.Fatalf("got %v, want ok=%v", err, tc.ok)
			}
			if err != nil && !errors.Is(err, ErrFingerprintMismatch) {
				t.Fatalf("expected ErrFingerprintMismatch, got %v", err)
			}
		})
	}
}

// TestDialFingerprintPinning dials a listener with an osp_demo-like
// certificate (no SAN, unrelated CN and serial): only fp is checked.
func TestDialFingerprintPinning(t *testing.T) {
	checkNoLeaks(t)
	plain := newPlainCert(t)
	derFP := wholeCertificateFingerPrint(plain.Leaf)

	for _, tc := range []struct {
		name   string
		fp     func(spki string) string
		quirks InteropQuirks
		ok     bool
	}{
		{"spki", func(spki string) string { return spki }, InteropQuirks{}, true},
		{"mismatch", func(string) string { return wholeCertificateFingerPrint(newPlainCert(t).Leaf) }, InteropQuirks{}, false},
		{"der without quirk", func(string) string { return derFP }, InteropQuirks{}, false},
		{"der with quirk", func(string) string { return derFP }, InteropQuirks{AcceptCertificateFingerprint: true}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
			defer cancel()

			server := newQuirkAgent(t, "Plain Server", InteropQuirks{}, plain)
			l, ra := startDirectListener(t, server)
			ra.TXT.Set(txtKeyFingerprint, tc.fp(ra.TXT.Get(txtKeyFingerprint)[0]))
			delete(ra.TXT, "sn")

			accepted := acceptAsync(t, ctx, l)
			client := newQuirkAgent(t, "Client", tc.quirks, nil)
			uc, err := ra.Dial(ctx, AgentTransportQUIC, client)
			if !tc.ok {
				if err == nil {
					uc.Close()
					t.Fatal("Dial succeeded despite fingerprint mismatch")
				}
				if !strings.Contains(err.Error(), "fingerprint") {
					t.Fatalf("unexpected error: %v", err)
				}
				l.Close()
				<-accepted
				return
			}
			if err != nil {
				t.Fatalf("Dial: %v", err)
			}
			defer uc.Close()
			if sc := <-accepted; sc == nil {
				t.Fatal("listener didn't accept")
			} else {
				defer sc.Close()
			}
		})
	}
}

// TestListenerAcceptsPlainClientCert checks the listener doesn't verify a
// hostname in the client certificate.
func TestListenerAcceptsPlainClientCert(t *testing.T) {
	checkNoLeaks(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	l, ra := startDirectListener(t, newTestAgent(t, "Server"))
	accepted := acceptAsync(t, ctx, l)
	uc, err := ra.Dial(ctx, AgentTransportQUIC, newQuirkAgent(t, "Plain Client", InteropQuirks{}, newPlainCert(t)))
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer uc.Close()
	sc := <-accepted
	if sc == nil {
		t.Fatal("listener rejected a SAN-less client certificate")
	}
	defer sc.Close()
}

func TestAdvertiseCertificateFingerprintQuirk(t *testing.T) {
	for _, on := range []bool{false, true} {
		a := newQuirkAgent(t, "Adv", InteropQuirks{AdvertiseCertificateFingerprint: on}, nil)
		fp, err := a.advertisedFingerPrint()
		if err != nil {
			t.Fatal(err)
		}
		spki, _ := a.CertificateFingerPrint()
		want := spki
		if on {
			want = wholeCertificateFingerPrint(a.Certificate.Leaf)
		}
		if fp != want {
			t.Fatalf("quirk=%v: advertised %q, want %q", on, fp, want)
		}
		if string(a.PeerID) != spki {
			t.Fatalf("quirk=%v: PeerID must stay the SPKI fingerprint", on)
		}
	}
}

// silentPeer is a QUIC server that completes the handshake but never sends
// agent-info or auth-capabilities, like osp_demo before metadata support.
// It sends the given messages on unidirectional streams right away.
type silentPeer struct {
	ln   *quic.Listener
	ra   *DiscoveredAgent
	done chan struct{}
}

func startSilentPeer(t *testing.T, early ...[]byte) *silentPeer {
	t.Helper()
	cert := newPlainCert(t)
	ln, err := quic.ListenAddr("127.0.0.1:0", &tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{*cert},
		NextProtos:   []string{ALPN_OSP},
		ClientAuth:   tls.RequireAnyClientCert,
	}, &quic.Config{MaxIdleTimeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	fp, _ := certificateFingerPrint(cert.Leaf)
	txt := TXTRecordSet{}
	txt.Set(txtKeyFingerprint, fp)
	txt.Set(txtKeyAuthToken, "abcdefgh")
	ra, err := DirectAgent("Silent", ln.Addr().(*net.UDPAddr), txt)
	if err != nil {
		t.Fatal(err)
	}
	p := &silentPeer{ln: ln, ra: ra, done: make(chan struct{})}
	go func() {
		defer close(p.done)
		conn, err := ln.Accept(context.Background())
		if err != nil {
			return
		}
		for _, msg := range early {
			s, err := conn.OpenUniStream()
			if err != nil {
				return
			}
			_, _ = s.Write(msg)
			_ = s.Close()
		}
		<-conn.Context().Done()
	}()
	t.Cleanup(func() {
		ln.Close()
		<-p.done
	})
	return p
}

func TestNoAgentInfo(t *testing.T) {
	checkNoLeaks(t)
	peer := startSilentPeer(t)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	uc, err := peer.ra.Dial(ctx, AgentTransportQUIC, newTestAgent(t, "Client"))
	if err == nil {
		uc.Close()
		t.Fatal("Dial succeeded without agent-info")
	}
	if !errors.Is(err, ErrNoAgentInfo) {
		t.Fatalf("expected ErrNoAgentInfo, got %v", err)
	}
	if !strings.Contains(err.Error(), "agent-info-response and auth-capabilities") {
		t.Fatalf("error doesn't say what's missing: %v", err)
	}
}

func TestTolerateMissingAgentInfoQuirk(t *testing.T) {
	checkNoLeaks(t)
	peer := startSilentPeer(t)

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	client := newQuirkAgent(t, "Client", InteropQuirks{TolerateMissingAgentInfo: true}, nil)
	uc, err := peer.ra.Dial(ctx, AgentTransportQUIC, client)
	if err != nil {
		t.Fatalf("Dial with TolerateMissingAgentInfo: %v", err)
	}
	defer uc.Close()
	if uc.RemoteAgent().Info() != nil {
		t.Fatal("remote info should be unknown")
	}
	if role := uc.GetAuthenticationRole(); role != AuthenticationRoleConsumer {
		t.Fatalf("dialing agent should consume the PSK with equal capabilities, got %v", role)
	}
}

func TestSkipAuthenticationQuirk(t *testing.T) {
	checkNoLeaks(t)

	// A presentation-url-availability-request sent before authentication.
	var early bytes.Buffer
	body := map[uint64]any{0: uint64(1), 1: []string{"https://example.com/"}, 2: uint64(1000), 3: uint64(1)}
	if err := WriteTypedMessage(&early, typeKeyPresentationUrlAvailabilityRequest, body); err != nil {
		t.Fatal(err)
	}

	t.Run("disabled", func(t *testing.T) {
		peer := startSilentPeer(t)
		ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
		defer cancel()
		client := newQuirkAgent(t, "Client", InteropQuirks{TolerateMissingAgentInfo: true}, nil)
		uc, err := peer.ra.Dial(ctx, AgentTransportQUIC, client)
		if err != nil {
			t.Fatal(err)
		}
		defer uc.Close()
		if _, err := uc.SkipAuthentication(); !errors.Is(err, ErrQuirkDisabled) {
			t.Fatalf("expected ErrQuirkDisabled, got %v", err)
		}
	})

	t.Run("enabled", func(t *testing.T) {
		peer := startSilentPeer(t, early.Bytes())
		ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
		defer cancel()
		client := newQuirkAgent(t, "Client", InteropQuirks{TolerateMissingAgentInfo: true, AllowSkipAuthentication: true}, nil)
		uc, err := peer.ra.Dial(ctx, AgentTransportQUIC, client)
		if err != nil {
			t.Fatal(err)
		}
		conn, err := uc.SkipAuthentication()
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		if conn.Authenticated() || conn.RemoteAgent().Authenticated() {
			t.Fatal("connection must not report authenticated")
		}

		// The message sent before the hand-off is delivered.
		s, err := conn.AcceptStream(ctx)
		if err != nil {
			t.Fatal(err)
		}
		got, err := io.ReadAll(s)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, early.Bytes()) {
			t.Fatalf("got %x, want %x", got, early.Bytes())
		}
		if _, err := s.Write([]byte{0}); !errors.Is(err, ErrStreamDirection) {
			t.Fatalf("writing to a received unidirectional stream: %v", err)
		}
	})
}

// TestUniStreamsAfterAuthentication sends application messages on
// unidirectional streams in both directions between authenticated agents.
func TestUniStreamsAfterAuthentication(t *testing.T) {
	checkNoLeaks(t)
	server, client := authenticatePair(t, []byte("1234"), []byte("1234"))
	if server.err != nil || client.err != nil {
		t.Fatalf("auth: server %v, client %v", server.err, client.err)
	}
	defer server.conn.Close()
	defer client.conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	for _, pair := range [][2]*Connection{{client.conn, server.conn}, {server.conn, client.conn}} {
		from, to := pair[0], pair[1]
		s, err := from.OpenUniStream(ctx)
		if err != nil {
			t.Fatal(err)
		}
		req := &MsgAgentInfoRequest{RequestID: 7}
		if err := WriteTypedMessage(s, TypeKeyAgentInfoRequest, req); err != nil {
			t.Fatal(err)
		}
		if err := WriteTypedMessage(s, TypeKeyAgentInfoRequest, req); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Read(make([]byte, 1)); !errors.Is(err, ErrStreamDirection) {
			t.Fatalf("reading an outgoing unidirectional stream: %v", err)
		}
		s.Close()

		in, err := to.AcceptStream(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 2; i++ {
			var got MsgAgentInfoRequest
			if err := ReadTypedMessage(in, TypeKeyAgentInfoRequest, &got); err != nil {
				t.Fatalf("message %d: %v", i, err)
			}
			if got.RequestID != 7 {
				t.Fatalf("got request-id %d", got.RequestID)
			}
		}
		in.Close()
	}
}

func TestDeterministicCBOR(t *testing.T) {
	// A map with keys inserted out of order and a struct with fields
	// declared out of order both encode with ascending keys.
	type outOfOrder struct {
		B uint64 `cbor:"2,keyasint"`
		A uint64 `cbor:"0,keyasint"`
		C uint64 `cbor:"1,keyasint"`
	}
	want := []byte{0xa3, 0x00, 0x01, 0x01, 0x02, 0x02, 0x03}
	for _, v := range []any{
		map[uint64]uint64{2: 3, 0: 1, 1: 2},
		outOfOrder{A: 1, C: 2, B: 3},
	} {
		for i := 0; i < 20; i++ { // Go map iteration is randomized.
			got, err := EncodeCBOR(v)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("%T: got %x, want %x", v, got, want)
			}
		}
	}

	var buf bytes.Buffer
	if err := WriteTypedMessage(&buf, 15, map[uint64]any{1: []uint64{0}, 0: uint64(9)}); err != nil {
		t.Fatal(err)
	}
	var decoded map[uint64]any
	if err := cbor.Unmarshal(buf.Bytes()[1:], &decoded); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(buf.Bytes(), []byte{15, 0xa2, 0x00, 0x09, 0x01, 0x81, 0x00}) {
		t.Fatalf("WriteTypedMessage: got %x", buf.Bytes())
	}
}
