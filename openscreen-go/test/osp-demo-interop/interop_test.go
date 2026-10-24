//go:build interop

package ospdemointerop

// Stage-by-stage interop tests against Chromium Open Screen's osp_demo.
//
// Each top-level test runs one direction. Its numbered subtests are the
// stages; a stage passes when it interoperates, and is skipped with the
// reason (prefixed "BLOCKER") at a known incompatibility, so `go test -v`
// output doubles as a progress gauge. Later stages skip when an earlier one
// they depend on didn't pass.
//
// This file holds the raw probe, kept as a fallback gauge: quic-go plus
// ospc's exported framing with generic CBOR bodies, no ospc connection state
// machine and no authentication. It shows how far the wire protocol
// interoperates regardless of ospc. The real ospc Dial/Listen paths are in
// ospc_test.go.

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"math/big"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/backkem/go-lp2p/openscreen-go/application"
	ospc "github.com/backkem/go-lp2p/openscreen-go/network"
	"github.com/grandcat/zeroconf"
	"github.com/quic-go/quic-go"
)

const (
	discoveryTimeout = 20 * time.Second
	stepTimeout      = 5 * time.Second
	testURL          = "https://example.com/"
)

func randSuffix() string {
	n, _ := rand.Int(rand.Reader, big.NewInt(1<<20))
	return fmt.Sprintf("%05x", n)
}

func randToken(n int) string {
	const chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	var b strings.Builder
	for i := 0; i < n; i++ {
		k, _ := rand.Int(rand.Reader, big.NewInt(int64(len(chars))))
		b.WriteByte(chars[k.Int64()])
	}
	return b.String()
}

// requireStage skips t unless the named earlier stage passed.
func requireStage(t *testing.T, ok bool, stage string) {
	t.Helper()
	if !ok {
		t.Skipf("depends on stage %q, which did not pass", stage)
	}
}

// browseInstance resolves an _openscreen._udp instance with zeroconf.
func browseInstance(ctx context.Context, instance string) (*zeroconf.ServiceEntry, error) {
	r, err := zeroconf.NewResolver(nil)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	entries := make(chan *zeroconf.ServiceEntry)
	if err := r.Browse(ctx, ospc.MdnsServiceType, ospc.MdnsDomain, entries); err != nil {
		return nil, err
	}
	for {
		select {
		case e, ok := <-entries:
			if !ok {
				return nil, ctx.Err()
			}
			if e.Instance == instance && e.Port != 0 && (len(e.AddrIPv4) > 0 || len(e.AddrIPv6) > 0) {
				return e, nil
			}
		case <-ctx.Done():
			return nil, fmt.Errorf("instance %q not found: %w", instance, ctx.Err())
		}
	}
}

// cidBlocker is the skip reason when osp_demo's QUIC server died on the
// connection ID bug, see INTEROP.md.
const cidBlocker = "BLOCKER (Chromium side): osp_demo's QUIC server advertised an empty connection ID " +
	"(NEW_CONNECTION_ID, generator length 0) but still parses short headers with the original DCID length; " +
	"quic-go switches to the empty CID after the handshake, so osp_demo misparses later packets and hits " +
	"QUICHE_DCHECK quic_connection.cc:1075 (debug build aborts, release would drop them)"

// skipIfDemoCrashed skips t if osp_demo exited, quoting its fatal log line.
func skipIfDemoCrashed(t *testing.T, d *ospDemo) {
	t.Helper()
	if d.Running() {
		return
	}
	fatal := d.Grep(`Check failed|FATAL|exited:`)
	if len(d.Grep(`unexpected server connection ID`)) > 0 {
		t.Skipf("%s. osp_demo: %q", cidBlocker, fatal)
	}
	t.Skipf("BLOCKER: osp_demo exited: %q", fatal)
}

func txtValue(txt []string, key string) (string, bool) {
	for _, kv := range txt {
		if k, v, ok := strings.Cut(kv, "="); ok && k == key {
			return v, true
		}
	}
	return "", false
}

// TestRawProbePublisher: osp_demo advertises as a presentation receiver
// ("publisher"), Go discovers and connects to it as a controller.
func TestRawProbePublisher(t *testing.T) {
	top := t // Owns the connection across stages.
	name := "GoIopRecv" + randSuffix()
	pub := startPublisher(t, name)
	t.Cleanup(pub.DumpLog)

	if _, ok := pub.WaitFor(`Running publisher demo|publisher started`, 5*time.Second); !ok || !pub.Running() {
		t.Fatalf("osp_demo publisher did not start")
	}

	localAgent, err := ospc.NewAgent(ospc.AgentConfig{DisplayName: "GoIopController" + randSuffix()})
	if err != nil {
		t.Fatal(err)
	}

	var discovered *ospc.DiscoveredAgent
	defer func() { _ = discovered }()
	t.Run("01-mdns-ospc-discover", func(t *testing.T) {
		d := ospc.NewDiscoverer()
		d.WithNickname(name)
		if err := d.Start(); err != nil {
			t.Fatal(err)
		}
		defer d.Close()
		a, err := d.Accept(withTimeout(t, discoveryTimeout))
		if err != nil {
			t.Fatalf("ospc did not discover %q: %v", name, err)
		}
		discovered = a
		t.Logf("discovered instance %q, TXT %v", a.Nickname(), a.TXT)
		for _, k := range []string{"fp", "at"} {
			if len(a.TXT.Get(k)) != 1 {
				t.Errorf("TXT %q missing", k)
			}
		}
		if _, ok := a.MetadataVersion(); !ok {
			t.Logf("note: TXT \"mv\" missing (required by network.bs, osp_demo never sets it)")
		}
	})

	var entry *zeroconf.ServiceEntry
	t.Run("03-raw-mdns-resolve", func(t *testing.T) {
		e, err := browseInstance(withTimeout(t, discoveryTimeout), name)
		if err != nil {
			t.Fatal(err)
		}
		entry = e
		t.Logf("resolved %s: host %s port %d v4 %v v6 %v TXT %q", e.Instance, e.HostName, e.Port, e.AddrIPv4, e.AddrIPv6, e.Text)
	})

	var wc *wireConn
	t.Run("04-raw-quic-tls", func(t *testing.T) {
		requireStage(t, entry != nil, "03-raw-mdns-resolve")
		fp, _ := txtValue(entry.Text, "fp")

		var addr string
		if len(entry.AddrIPv4) > 0 {
			addr = net.JoinHostPort(entry.AddrIPv4[0].String(), fmt.Sprint(entry.Port))
		} else {
			addr = net.JoinHostPort(entry.AddrIPv6[0].String(), fmt.Sprint(entry.Port))
		}
		var peer *x509.Certificate
		tlsConf := &tls.Config{
			MinVersion:         tls.VersionTLS13,
			NextProtos:         []string{ospc.ALPN_OSP},
			ServerName:         name + ".local",
			Certificates:       []tls.Certificate{*localAgent.Certificate},
			InsecureSkipVerify: true, // Checked below against both fingerprint definitions.
			VerifyPeerCertificate: func(raw [][]byte, _ [][]*x509.Certificate) error {
				if len(raw) == 0 {
					return errors.New("no peer certificate")
				}
				c, err := x509.ParseCertificate(raw[0])
				if err != nil {
					return err
				}
				peer = c
				return nil
			},
		}
		conn, err := quic.DialAddr(withTimeout(t, stepTimeout), addr, tlsConf, &quic.Config{MaxIdleTimeout: 30 * time.Second})
		if err != nil {
			t.Fatalf("QUIC dial %s: %v", addr, err)
		}
		cs := conn.ConnectionState().TLS
		t.Logf("QUIC handshake complete with %s: TLS 0x%04x, ALPN %q", addr, cs.Version, cs.NegotiatedProtocol)
		if cs.NegotiatedProtocol != ospc.ALPN_OSP {
			t.Errorf("ALPN = %q, want %q", cs.NegotiatedProtocol, ospc.ALPN_OSP)
		}
		spki, der := fingerprintSPKI(peer), fingerprintDER(peer)
		t.Logf("peer cert: subject %q, SAN %v, serial %v, valid %s..%s", peer.Subject, peer.DNSNames, peer.SerialNumber, peer.NotBefore.Format(time.DateOnly), peer.NotAfter.Format(time.DateOnly))
		t.Logf("TXT fp=%s; SPKI fp (spec, ospc)=%s; DER-cert fp (osp_demo)=%s", fp, spki, der)
		switch fp {
		case spki:
			t.Logf("fp matches the spec definition (SPKI)")
		case der:
			t.Logf("SPEC DRIFT (Chromium side): fp is SHA-256 over the whole DER certificate, not the SPKI as network.bs requires; ospc's Dial would reject it")
		default:
			t.Errorf("fp matches neither fingerprint definition")
		}
		wc = newWireConn(top, "go-ctrl", conn, nil)
	})

	t.Run("05-raw-agent-info", func(t *testing.T) {
		requireStage(t, wc != nil, "04-raw-quic-tls")
		if err := wc.Send(tkAgentInfoRequest, map[uint64]any{0: uint64(1)}); err != nil {
			t.Fatal(err)
		}
		m, err := wc.Expect(tkAgentInfoResponse, 2*time.Second)
		if err != nil {
			unhandled := pub.Grep(`no message handler matched`)
			if len(unhandled) == 0 {
				skipIfDemoCrashed(t, pub)
			}
			t.Skipf("BLOCKER (Chromium side): no agent-info-response (%v). The raw probe sends its request once, right after the handshake; older osp_demo builds have no handler for it, newer ones drop a request that arrives before their network phase starts. Unhandled: %q", err, unhandled)
		}
		t.Logf("agent-info-response: %v", m.Body)
	})

	t.Run("06-raw-auth", func(t *testing.T) {
		requireStage(t, wc != nil, "04-raw-quic-tls")
		err := wc.Send(tkAuthCapabilities, map[uint64]any{0: uint64(0), 1: []uint64{0}, 2: uint64(20)})
		if err != nil {
			t.Fatal(err)
		}
		_, err = wc.Expect(tkAuthCapabilities, 2*time.Second)
		skipIfDemoCrashed(t, pub)
		if wc.conn.Context().Err() != nil {
			t.Fatalf("osp_demo closed the connection after auth-capabilities: %v", context.Cause(wc.conn.Context()))
		}
		if err != nil {
			t.Skipf("BLOCKER: no auth-capabilities from osp_demo in reply (%v); the raw probe doesn't authenticate and continues unauthenticated", err)
		}
	})

	presentationID := application.NewPresentationID()
	var connectionID uint64
	availOK, startOK := false, false

	t.Run("07-presentation-url-availability", func(t *testing.T) {
		requireStage(t, wc != nil, "04-raw-quic-tls")
		err := wc.Send(tkPresentationURLAvailabilityReq, map[uint64]any{
			0: uint64(10),
			1: []string{testURL},
			2: uint64(60_000_000),
			3: uint64(1),
		})
		if err != nil {
			t.Fatal(err)
		}
		m, err := wc.Expect(tkPresentationURLAvailabilityResp, stepTimeout)
		if err != nil {
			skipIfDemoCrashed(t, pub)
			if len(pub.Grep(`(?i)holding back message|network phase`)) > 0 {
				t.Skipf("BLOCKER (by design): no presentation-url-availability-response (%v); osp_demo holds back application messages until authentication, which the raw probe doesn't do", err)
			}
			t.Fatal(err)
		}
		if id, _ := asUint(m.Body[0]); id != 10 {
			t.Errorf("request-id = %v, want 10", m.Body[0])
		}
		avail, _ := m.Body[1].([]any)
		if len(avail) != 1 {
			t.Fatalf("url-availabilities = %v, want one entry", m.Body[1])
		}
		if v, _ := asUint(avail[0]); v != 0 {
			t.Fatalf("url-availability = %v, want 0 (available)", avail[0])
		}
		if l, ok := pub.WaitFor(`got availability request for`, stepTimeout); ok {
			t.Logf("osp_demo: %s", l)
		}
		availOK = true
	})

	t.Run("08-presentation-start", func(t *testing.T) {
		requireStage(t, availOK, "07-presentation-url-availability")
		err := wc.Send(tkPresentationStartRequest, map[uint64]any{
			0: uint64(11),
			1: presentationID,
			2: testURL,
			3: []any{},
		})
		if err != nil {
			t.Fatal(err)
		}
		m, err := wc.Expect(tkPresentationStartResponse, stepTimeout)
		if err != nil {
			t.Fatal(err)
		}
		if r, _ := asUint(m.Body[1]); r != 1 {
			t.Fatalf("result = %v, want 1 (success)", m.Body[1])
		}
		connectionID, _ = asUint(m.Body[2])
		t.Logf("presentation %q started, connection-id %d", presentationID, connectionID)
		startOK = true
	})

	t.Run("09-presentation-connection-message", func(t *testing.T) {
		requireStage(t, startOK, "08-presentation-start")
		const text = "hello from go"
		err := wc.Send(tkPresentationConnectionMessage, map[uint64]any{0: connectionID, 1: text})
		if err != nil {
			t.Fatal(err)
		}
		if l, ok := pub.WaitFor(`got message: `+text, stepTimeout); ok {
			t.Logf("osp_demo: %s", l)
		} else {
			t.Errorf("osp_demo did not log the message")
		}
		m, err := wc.Expect(tkPresentationConnectionMessage, stepTimeout)
		if err != nil {
			t.Fatal(err)
		}
		if got := fmt.Sprint(m.Body[1]); got != "--echo-- "+text {
			t.Errorf("echo = %q, want %q", got, "--echo-- "+text)
		}
	})

	t.Run("10-presentation-termination", func(t *testing.T) {
		requireStage(t, startOK, "08-presentation-start")
		err := wc.Send(tkPresentationTerminationRequest, map[uint64]any{
			0: uint64(12),
			1: presentationID,
			2: uint64(1), // application-request
		})
		if err != nil {
			t.Fatal(err)
		}
		m, err := wc.Expect(tkPresentationTerminationResponse, stepTimeout)
		if err != nil {
			t.Fatal(err)
		}
		if r, _ := asUint(m.Body[1]); r != 1 {
			t.Errorf("result = %v, want 1 (success)", m.Body[1])
		}
	})
}

// TestRawProbeController: the raw probe advertises as a receiver, osp_demo's
// controller discovers and connects to it. It advertises the spec's SPKI
// fingerprint first and falls back to the whole-certificate hash older
// osp_demo builds pin.
func TestRawProbeController(t *testing.T) {
	connected := false
	t.Run("raw-receiver-spki", func(t *testing.T) { connected = testRawReceiverToController(t, false) })
	if !connected {
		t.Run("raw-receiver-der", func(t *testing.T) { testRawReceiverToController(t, true) })
	}
}

// testRawReceiverToController advertises with zeroconf and accepts with
// quic-go, using an osp_demo-compatible fingerprint, to find out how far the
// protocol goes past discovery and TLS.
func testRawReceiverToController(t *testing.T, derFingerprint bool) (connected bool) {
	top := t // Owns the connection across stages.
	name := "GoIopRaw" + randSuffix()
	cert := newProbeCert(t, name+".local", 365*24*time.Hour)

	var clientCert *x509.Certificate
	tlsConf := &tls.Config{
		MinVersion:   tls.VersionTLS13,
		NextProtos:   []string{ospc.ALPN_OSP},
		Certificates: []tls.Certificate{cert},
		ClientAuth:   tls.RequestClientCert,
		VerifyPeerCertificate: func(raw [][]byte, _ [][]*x509.Certificate) error {
			if len(raw) > 0 {
				clientCert, _ = x509.ParseCertificate(raw[0])
			}
			return nil
		},
	}
	ql, err := quic.ListenAddr(":0", tlsConf, &quic.Config{MaxIdleTimeout: 30 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ql.Close() })
	port := ql.Addr().(*net.UDPAddr).Port

	conns := make(chan quic.Connection, 4)
	go func() {
		for {
			c, err := ql.Accept(context.Background())
			if err != nil {
				return
			}
			conns <- c
		}
	}()

	fp := fingerprintSPKI(cert.Leaf)
	if derFingerprint {
		fp = fingerprintDER(cert.Leaf)
	}
	txt := []string{"fp=" + fp, "at=" + randToken(16)}
	srv, err := zeroconf.Register(name, ospc.MdnsServiceType, ospc.MdnsDomain, port, txt, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Shutdown)
	t.Logf("advertising %q on port %d with TXT %q (DER-cert fp: %v)", name, port, txt, derFingerprint)

	ctrl := startController(t)
	t.Cleanup(ctrl.DumpLog)

	found := false
	t.Run("01-mdns-cpp-discovers-go", func(t *testing.T) {
		l, ok := ctrl.WaitFor(`found! `+name, discoveryTimeout)
		if !ok {
			t.Fatalf("osp_demo did not discover %q", name)
		}
		t.Logf("osp_demo: %s", l)
		found = true
	})

	var wc *wireConn
	t.Run("02-quic-tls", func(t *testing.T) {
		requireStage(t, found, "01-mdns-cpp-discovers-go")
		ctrl.Send("connect " + name)
		var conn quic.Connection
		select {
		case conn = <-conns:
		case <-time.After(2 * stepTimeout):
			t.Fatalf("no QUIC connection from osp_demo; lines: %q", ctrl.Grep(`(?i)connect|handshake|fail|error|fingerprint`))
		}
		connected = true
		cs := conn.ConnectionState().TLS
		t.Logf("accepted QUIC connection from %s: ALPN %q, SNI %q", conn.RemoteAddr(), cs.NegotiatedProtocol, cs.ServerName)
		if cs.NegotiatedProtocol != ospc.ALPN_OSP {
			t.Errorf("ALPN = %q, want %q", cs.NegotiatedProtocol, ospc.ALPN_OSP)
		}
		if clientCert != nil {
			t.Logf("client cert: subject %q, SAN %v, SPKI fp %s, DER fp %s", clientCert.Subject, clientCert.DNSNames, fingerprintSPKI(clientCert), fingerprintDER(clientCert))
		} else {
			t.Logf("osp_demo presented no client certificate (network.bs requires both agents to)")
		}
		if l, ok := ctrl.WaitFor(`succeed to build|(?i)fail`, stepTimeout); ok {
			t.Logf("osp_demo: %s", l)
		}
		wc = newWireConn(top, "go-recv", conn, map[uint64]any{
			0: name, 1: "go-lp2p raw probe", 2: []uint64{3}, 3: randToken(8), 4: []string{"en"},
		})
	})

	availOK := false
	t.Run("03-presentation-url-availability", func(t *testing.T) {
		requireStage(t, wc != nil, "02-quic-tls")
		ctrl.Send("avail " + testURL)
		m, err := wc.Expect(tkPresentationURLAvailabilityReq, stepTimeout)
		if err != nil {
			for _, l := range ctrl.Grep(`(?i)stream|availability|write`) {
				t.Logf("osp_demo: %s", l)
			}
			t.Skipf("BLOCKER (Chromium side): no presentation-url-availability-request from osp_demo: %v", err)
		}
		if err := wc.Send(tkPresentationURLAvailabilityResp, map[uint64]any{0: m.Body[0], 1: []uint64{0}}); err != nil {
			t.Fatal(err)
		}
		l, ok := ctrl.WaitFor(`available! |unavailable! |request failed`, stepTimeout)
		if !ok || !strings.Contains(l, "available! "+name) || strings.Contains(l, "unavailable") {
			t.Fatalf("osp_demo did not accept the availability response: %q", l)
		}
		t.Logf("osp_demo: %s", l)
		availOK = true
	})

	var connectionID uint64 = 7
	startOK := false
	t.Run("04-presentation-start", func(t *testing.T) {
		requireStage(t, availOK, "03-presentation-url-availability")
		ctrl.Send("start " + testURL + " " + name)
		m, err := wc.Expect(tkPresentationStartRequest, stepTimeout)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("presentation-start-request: %v", m.Body)
		if err := wc.Send(tkPresentationStartResponse, map[uint64]any{0: m.Body[0], 1: uint64(1), 2: connectionID}); err != nil {
			t.Fatal(err)
		}
		l, ok := ctrl.WaitFor(`request successful|on request error|presentation connection connected`, stepTimeout)
		if !ok || strings.Contains(l, "error") {
			t.Fatalf("osp_demo did not accept the start response: %q", l)
		}
		t.Logf("osp_demo: %s", l)
		startOK = true
	})

	t.Run("05-presentation-connection-message", func(t *testing.T) {
		requireStage(t, startOK, "04-presentation-start")
		ctrl.Send("msg hello from cpp")
		m, err := wc.Expect(tkPresentationConnectionMessage, stepTimeout)
		if err != nil {
			t.Fatal(err)
		}
		if got := fmt.Sprint(m.Body[1]); got != "hello from cpp" {
			t.Errorf("message = %q", got)
		}
		if err := wc.Send(tkPresentationConnectionMessage, map[uint64]any{0: connectionID, 1: "hello from go"}); err != nil {
			t.Fatal(err)
		}
		if l, ok := ctrl.WaitFor(`got message: hello from go`, stepTimeout); ok {
			t.Logf("osp_demo: %s", l)
		} else {
			t.Errorf("osp_demo did not log our message")
		}
	})

	t.Run("06-network-messages-and-auth", func(t *testing.T) {
		requireStage(t, wc != nil, "02-quic-tls")
		wc.mu.Lock()
		var types []uint64
		for _, m := range wc.seen {
			types = append(types, m.Type)
		}
		wc.mu.Unlock()
		t.Logf("message types received from osp_demo over the whole connection: %v", types)
		for _, tk := range types {
			if tk == tkAgentInfoRequest || (tk >= tkAuthCapabilities && tk <= 1005) {
				t.Logf("osp_demo sent network-protocol message type %d", tk)
				return
			}
		}
		t.Skipf("BLOCKER (Chromium side): osp_demo sent no agent-info-request and no auth-* message; " +
			"it exchanged application messages without authenticating, so SPAKE2 interop can't be tested")
	})
	return connected
}
