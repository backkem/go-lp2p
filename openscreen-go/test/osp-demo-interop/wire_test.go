//go:build interop

package ospdemointerop

// Raw wire-level OSP helpers. They use ospc's exported framing (QUIC varint
// type key followed by one CBOR item) but generic CBOR maps for message
// bodies, so the probes can send and inspect exactly what osp_demo expects
// without going through ospc's connection state machine.

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"fmt"
	"math/big"
	"net"
	"sync"
	"testing"
	"time"

	ospc "github.com/backkem/go-lp2p/openscreen-go/network"
	"github.com/fxamacker/cbor/v2"
	"github.com/quic-go/quic-go"
)

// Type keys used by the probes (network.bs / application.bs appendix; the
// same values are generated into osp_demo's osp_messages.h).
const (
	tkAgentInfoRequest                  = 10
	tkAgentInfoResponse                 = 11
	tkAgentStatusRequest                = 12
	tkAgentStatusResponse               = 13
	tkPresentationURLAvailabilityReq    = 14
	tkPresentationURLAvailabilityResp   = 15
	tkPresentationConnectionMessage     = 16
	tkPresentationURLAvailabilityEvent  = 103
	tkPresentationStartRequest          = 104
	tkPresentationStartResponse         = 105
	tkPresentationTerminationRequest    = 106
	tkPresentationTerminationResponse   = 107
	tkPresentationConnectionOpenRequest = 109
	tkAuthCapabilities                  = 1001
)

// fingerprintSPKI is the agent fingerprint as specified by network.bs:
// base64(SHA-256(SubjectPublicKeyInfo)). ospc uses this.
func fingerprintSPKI(c *x509.Certificate) string {
	h := sha256.Sum256(c.RawSubjectPublicKeyInfo)
	return base64.StdEncoding.EncodeToString(h[:])
}

// fingerprintDER is what osp_demo advertises and verifies:
// base64(SHA-256(DER certificate)).
func fingerprintDER(c *x509.Certificate) string {
	h := sha256.Sum256(c.Raw)
	return base64.StdEncoding.EncodeToString(h[:])
}

// newProbeCert creates a self-signed ECDSA P-256 certificate with the given
// validity, for the raw probes.
func newProbeCert(t *testing.T, cn string, validity time.Duration) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	tmpl := x509.Certificate{
		SerialNumber:          big.NewInt(now.UnixNano()),
		Subject:               pkix.Name{CommonName: cn},
		Issuer:                pkix.Name{CommonName: cn},
		DNSNames:              []string{cn},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(validity - time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}
	raw, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(raw)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{raw}, PrivateKey: key, Leaf: leaf}
}

// wireMsg is a received OSP message with a generically decoded body.
type wireMsg struct {
	Type   uint64
	Body   map[uint64]any
	Stream quic.StreamID
}

func (m wireMsg) String() string {
	return fmt.Sprintf("type=%d stream=%d body=%v", m.Type, m.Stream, m.Body)
}

// detEnc encodes map keys in ascending order. CBOR maps are unordered, but
// osp_demo's generated tinycbor decoders read fields in key order and reject
// anything else (url_availability_requester.cc: "parse error: -519").
// ospc encodes structs in field order, so it isn't affected.
var detEnc = func() cbor.EncMode {
	em, err := cbor.CoreDetEncOptions().EncMode()
	if err != nil {
		panic(err)
	}
	return em
}()

// encodeWire frames a message: varint type key + CBOR body.
func encodeWire(typeKey uint64, body map[uint64]any) ([]byte, error) {
	var buf bytes.Buffer
	if err := ospc.WriteTypeKey(ospc.TypeKey(typeKey), &buf); err != nil {
		return nil, err
	}
	b, err := detEnc.Marshal(body)
	if err != nil {
		return nil, err
	}
	buf.Write(b)
	return buf.Bytes(), nil
}

// wireConn sends each message on its own unidirectional stream and reads
// messages from every incoming stream (uni and bidi). It answers
// agent-status-request and, if info is set, agent-info-request itself.
type wireConn struct {
	t    *testing.T
	name string
	conn quic.Connection

	// agentInfo, if set, is used to answer agent-info-request.
	agentInfo map[uint64]any

	msgs chan wireMsg

	mu   sync.Mutex
	seen []wireMsg
}

// newWireConn binds the connection to t: it is closed when t ends, so pass
// the test that spans all stages using it, not a stage subtest.
func newWireConn(t *testing.T, name string, conn quic.Connection, agentInfo map[uint64]any) *wireConn {
	w := &wireConn{
		t:         t,
		name:      name,
		conn:      conn,
		agentInfo: agentInfo,
		msgs:      make(chan wireMsg, 64),
	}
	ctx := conn.Context()
	go func() {
		for {
			s, err := conn.AcceptUniStream(ctx)
			if err != nil {
				return
			}
			go w.readStream(s, s.StreamID())
		}
	}()
	go func() {
		for {
			s, err := conn.AcceptStream(ctx)
			if err != nil {
				return
			}
			go w.readStream(s, s.StreamID())
		}
	}()
	t.Cleanup(func() { _ = conn.CloseWithError(0, "test done") })
	return w
}

func (w *wireConn) readStream(r interface{ Read([]byte) (int, error) }, id quic.StreamID) {
	for {
		tk, err := ospc.ReadTypeKey(r)
		if err != nil {
			return
		}
		var v any
		if err := ospc.DecodeCBOR(r, &v); err != nil {
			w.t.Logf("[%s] stream %d: type %d: cbor decode error: %v", w.name, id, tk, err)
			return
		}
		m := wireMsg{Type: uint64(tk), Body: normalizeMap(v), Stream: id}
		w.t.Logf("[%s] <-- %s", w.name, m)
		w.mu.Lock()
		w.seen = append(w.seen, m)
		w.mu.Unlock()

		switch m.Type {
		case tkAgentStatusRequest:
			_ = w.Send(tkAgentStatusResponse, map[uint64]any{0: m.Body[0]})
			continue
		case tkAgentInfoRequest:
			if w.agentInfo != nil {
				_ = w.Send(tkAgentInfoResponse, map[uint64]any{0: m.Body[0], 1: w.agentInfo})
			}
		}
		select {
		case w.msgs <- m:
		default:
			w.t.Logf("[%s] dropping message, queue full: %s", w.name, m)
		}
	}
}

// Send writes one message on a new unidirectional stream.
func (w *wireConn) Send(typeKey uint64, body map[uint64]any) error {
	b, err := encodeWire(typeKey, body)
	if err != nil {
		return err
	}
	s, err := w.conn.OpenUniStream()
	if err != nil {
		return err
	}
	w.t.Logf("[%s] --> type=%d stream=%d body=%v", w.name, typeKey, s.StreamID(), body)
	if _, err := s.Write(b); err != nil {
		return err
	}
	return s.Close()
}

// Expect waits for a message of the given type. Other messages are logged
// and discarded.
func (w *wireConn) Expect(typeKey uint64, timeout time.Duration) (wireMsg, error) {
	ctx, cancel := context.WithTimeout(w.conn.Context(), timeout)
	defer cancel()
	for {
		select {
		case m := <-w.msgs:
			if m.Type == typeKey {
				return m, nil
			}
		case <-ctx.Done():
			if w.conn.Context().Err() != nil {
				return wireMsg{}, fmt.Errorf("connection closed while waiting for type %d: %w", typeKey, context.Cause(w.conn.Context()))
			}
			return wireMsg{}, fmt.Errorf("timeout waiting for type %d", typeKey)
		}
	}
}

// normalizeMap converts a decoded CBOR map with integer keys to
// map[uint64]any. Non-map values return nil.
func normalizeMap(v any) map[uint64]any {
	in, ok := v.(map[any]any)
	if !ok {
		return nil
	}
	out := make(map[uint64]any, len(in))
	for k, val := range in {
		switch k := k.(type) {
		case uint64:
			out[k] = val
		case int64:
			out[uint64(k)] = val
		}
	}
	return out
}

func asUint(v any) (uint64, bool) {
	switch v := v.(type) {
	case uint64:
		return v, true
	case int64:
		if v >= 0 {
			return uint64(v), true
		}
	}
	return 0, false
}

// firstIPv4 returns a non-loopback IPv4 address of this host, osp_demo binds
// its publisher to such an address.
func firstIPv4(t *testing.T) net.IP {
	t.Helper()
	ifaces, err := net.Interfaces()
	if err != nil {
		t.Fatal(err)
	}
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := ifc.Addrs()
		for _, a := range addrs {
			if ipn, ok := a.(*net.IPNet); ok && ipn.IP.To4() != nil {
				return ipn.IP
			}
		}
	}
	t.Skip("no non-loopback IPv4 interface")
	return nil
}
