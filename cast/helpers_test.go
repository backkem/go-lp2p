package cast

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"sort"
	"sync"
	"testing"
	"time"
)

var serial int64

func makeCert(t testing.TB, cn string, pub crypto.PublicKey, parent *x509.Certificate, parentKey crypto.Signer, isCA bool, life time.Duration) *x509.Certificate {
	t.Helper()
	serial++
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(serial),
		Subject:               pkix.Name{CommonName: cn},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(life),
		BasicConstraintsValid: true,
		IsCA:                  isCA,
		KeyUsage:              x509.KeyUsageDigitalSignature,
	}
	if isCA {
		tmpl.KeyUsage |= x509.KeyUsageCertSign
	}
	if parent == nil {
		parent = tmpl
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, parent, pub, parentKey)
	if err != nil {
		t.Fatal(err)
	}
	c, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

var (
	rsaKeysOnce sync.Once
	rsaKeys     [3]*rsa.PrivateKey
)

// testRSAKey returns one of a few cached 2048-bit keys (generation is slow).
func testRSAKey(t testing.TB, i int) *rsa.PrivateKey {
	rsaKeysOnce.Do(func() {
		for j := range rsaKeys {
			k, err := rsa.GenerateKey(rand.Reader, 2048)
			if err != nil {
				panic(err)
			}
			rsaKeys[j] = k
		}
	})
	return rsaKeys[i]
}

// deviceCreds is a fake Cast PKI: root -> intermediate -> device.
type deviceCreds struct {
	roots []*x509.Certificate
	chain [][]byte
	key   crypto.Signer
}

func newDeviceCreds(t testing.TB) *deviceCreds {
	rootKey, interKey, devKey := testRSAKey(t, 0), testRSAKey(t, 1), testRSAKey(t, 2)
	root := makeCert(t, "Test Cast Root CA", rootKey.Public(), nil, rootKey, true, 24*time.Hour)
	inter := makeCert(t, "Test Cast ICA", interKey.Public(), root, rootKey, true, 24*time.Hour)
	dev := makeCert(t, "test-device", devKey.Public(), inter, interKey, false, 24*time.Hour)
	return &deviceCreds{roots: []*x509.Certificate{root}, chain: [][]byte{dev.Raw, inter.Raw}, key: devKey}
}

// receiverTLSConfig returns a short-lived self-signed TLS certificate, as
// Cast receivers use.
func receiverTLSConfig(t testing.TB, life time.Duration) *tls.Config {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	c := makeCert(t, "receiver-tls", key.Public(), nil, key, false, life)
	return &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{c.Raw}, PrivateKey: key, Leaf: c}}}
}

// socketPair returns connected (sender, receiver) TLS sockets on loopback.
func socketPair(t testing.TB, tlsConf *tls.Config) (*Socket, *Socket) {
	t.Helper()
	if tlsConf == nil {
		tlsConf = receiverTLSConfig(t, 48*time.Hour)
	}
	ln, err := Listen("127.0.0.1:0", tlsConf)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	type res struct {
		s   *Socket
		err error
	}
	accepted := make(chan res, 1)
	go func() {
		s, err := ln.Accept(ctx)
		accepted <- res{s, err}
	}()
	snd, err := Dial(ctx, ln.Addr().String(), nil)
	if err != nil {
		t.Fatal(err)
	}
	r := <-accepted
	if r.err != nil {
		t.Fatal(r.err)
	}
	t.Cleanup(func() { _ = snd.Close(); _ = r.s.Close() })
	return snd, r.s
}

// fakeClock is a manually advanced Clock.
type fakeClock struct {
	mu     sync.Mutex
	now    time.Duration
	timers []*fakeTimer
	resets int
}

type fakeTimer struct {
	c      *fakeClock
	at     time.Duration
	f      func()
	active bool
}

func (c *fakeClock) AfterFunc(d time.Duration, f func()) Timer {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := &fakeTimer{c: c, at: c.now + d, f: f, active: true}
	c.timers = append(c.timers, t)
	return t
}

func (t *fakeTimer) Stop() bool {
	t.c.mu.Lock()
	defer t.c.mu.Unlock()
	was := t.active
	t.active = false
	return was
}

func (t *fakeTimer) Reset(d time.Duration) bool {
	t.c.mu.Lock()
	defer t.c.mu.Unlock()
	was := t.active
	t.at, t.active = t.c.now+d, true
	t.c.resets++
	return was
}

func (c *fakeClock) Resets() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.resets
}

// Advance moves time forward and runs expired timers in deadline order.
func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now += d
	var due []*fakeTimer
	for _, t := range c.timers {
		if t.active && t.at <= c.now {
			t.active = false
			due = append(due, t)
		}
	}
	c.mu.Unlock()
	sort.Slice(due, func(i, j int) bool { return due[i].at < due[j].at })
	for _, t := range due {
		t.f()
	}
}

func waitFor(t testing.TB, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func recvTimeout(t testing.TB, ch <-chan *CastMessage) *CastMessage {
	t.Helper()
	select {
	case m := <-ch:
		return m
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for message")
		return nil
	}
}
