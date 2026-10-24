package cast

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"errors"
	"testing"
	"time"
)

func authPair(t *testing.T, responder *DeviceAuthResponder, tlsLife time.Duration) *Channel {
	t.Helper()
	ss, rs := socketPair(t, receiverTLSConfig(t, tlsLife))
	s, r := NewChannel(ss, nil), NewChannel(rs, nil)
	r.Handle(AuthNamespace, responder.Handler())
	s.Start()
	r.Start()
	t.Cleanup(func() { _ = s.Close(); _ = r.Close() })
	return s
}

func authenticate(s *Channel, v Verifier) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return Authenticate(ctx, s, v)
}

func TestDeviceAuthSuccess(t *testing.T) {
	creds := newDeviceCreds(t)
	s := authPair(t, &DeviceAuthResponder{Chain: creds.chain, Key: creds.key}, 48*time.Hour)
	if err := authenticate(s, &CertVerifier{Roots: creds.roots, RequireSHA256: true}); err != nil {
		t.Fatal(err)
	}
	// The auth handler is released afterwards and can run again.
	if err := authenticate(s, &CertVerifier{Roots: creds.roots}); err != nil {
		t.Fatal(err)
	}
}

func TestDeviceAuthWrongRoot(t *testing.T) {
	creds, other := newDeviceCreds(t), x509.NewCertPool()
	otherKey := testRSAKey(t, 0)
	other.AddCert(makeCert(t, "Other Root", otherKey.Public(), nil, otherKey, true, time.Hour))
	s := authPair(t, &DeviceAuthResponder{Chain: creds.chain, Key: creds.key}, 48*time.Hour)
	err := authenticate(s, &CertVerifier{Roots: other})
	if !errors.Is(err, ErrAuthFailed) {
		t.Fatalf("got %v", err)
	}
	// Signature binding alone still verifies.
	if err := authenticate(s, &CertVerifier{InsecureSkipChainVerify: true}); err != nil {
		t.Fatal(err)
	}
}

func TestDeviceAuthWrongKey(t *testing.T) {
	creds := newDeviceCreds(t)
	// Signs with a key that does not match the device certificate.
	s := authPair(t, &DeviceAuthResponder{Chain: creds.chain, Key: testRSAKey(t, 0)}, 48*time.Hour)
	if err := authenticate(s, &CertVerifier{Roots: creds.roots}); !errors.Is(err, ErrAuthFailed) {
		t.Fatalf("got %v", err)
	}
	if err := authenticate(s, InsecureVerifier{}); err != nil {
		t.Fatalf("insecure verifier: %v", err)
	}
}

func TestDeviceAuthTLSCertLifetime(t *testing.T) {
	creds := newDeviceCreds(t)
	s := authPair(t, &DeviceAuthResponder{Chain: creds.chain, Key: creds.key}, 30*24*time.Hour)
	if err := authenticate(s, &CertVerifier{Roots: creds.roots}); !errors.Is(err, ErrAuthFailed) {
		t.Fatalf("long-lived TLS cert accepted: %v", err)
	}
	if err := authenticate(s, &CertVerifier{Roots: creds.roots, MaxTLSCertLifetime: -1}); err != nil {
		t.Fatal(err)
	}
}

func TestDeviceAuthErrorResponse(t *testing.T) {
	ec, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	creds := newDeviceCreds(t)
	s := authPair(t, &DeviceAuthResponder{Chain: creds.chain, Key: ec}, 48*time.Hour)
	err := authenticate(s, InsecureVerifier{})
	var ae *AuthErrorResponse
	if !errors.As(err, &ae) || ae.Type != AuthErrorInternal || !errors.Is(err, ErrAuthFailed) {
		t.Fatalf("got %v", err)
	}
}

func TestDeviceAuthResponderAlgorithms(t *testing.T) {
	creds := newDeviceCreds(t)
	_, rs := socketPair(t, nil)
	r := &DeviceAuthResponder{Chain: creds.chain, Key: creds.key}
	nonce := []byte("0123456789abcdef")

	// Default challenge: SHA1, PKCS#1 v1.5.
	resp, _, ok := r.respond(rs, &AuthChallenge{SenderNonce: nonce})
	if !ok || resp.Hash() != HashSHA1 || len(resp.IntermediateCertificate) != 1 {
		t.Fatalf("resp %+v", resp)
	}
	pub := creds.key.Public().(*rsa.PublicKey)
	if err := rsa.VerifyPKCS1v15(pub, crypto.SHA1, signedDigest(crypto.SHA1, nonce, rs.LocalCertificate()), resp.Signature); err != nil {
		t.Fatal(err)
	}
	if err := (&CertVerifier{Roots: creds.roots, RequireSHA256: true}).Verify(resp, nonce, x509Leaf(t, rs.LocalCertificate())); !errors.Is(err, ErrAuthFailed) {
		t.Fatalf("SHA1 accepted with RequireSHA256: %v", err)
	}
	if err := (&CertVerifier{Roots: creds.roots}).Verify(resp, nonce, x509Leaf(t, rs.LocalCertificate())); err != nil {
		t.Fatal(err)
	}
	if err := (&CertVerifier{Roots: creds.roots}).Verify(resp, []byte("other nonce....."), x509Leaf(t, rs.LocalCertificate())); !errors.Is(err, ErrAuthFailed) {
		t.Fatalf("nonce mismatch accepted: %v", err)
	}

	pss := SignatureRSASSAPSS
	if _, et, ok := r.respond(rs, &AuthChallenge{SenderNonce: nonce, SignatureAlgorithm: &pss}); ok || et != AuthErrorSignatureAlgorithmUnavailable {
		t.Fatalf("PSS: ok=%v type=%v", ok, et)
	}
	bad := HashAlgorithm(9)
	if _, et, ok := r.respond(rs, &AuthChallenge{HashAlgorithm: &bad}); ok || et != AuthErrorSignatureAlgorithmUnavailable {
		t.Fatalf("bad hash: ok=%v type=%v", ok, et)
	}
}

func TestCertVerifierPSS(t *testing.T) {
	creds := newDeviceCreds(t)
	tlsLeaf := x509Leaf(t, receiverTLSConfig(t, time.Hour).Certificates[0].Certificate[0])
	nonce := []byte("0123456789abcdef")
	digest := signedDigest(crypto.SHA256, nonce, tlsLeaf.Raw)
	sig, err := rsa.SignPSS(rand.Reader, creds.key.(*rsa.PrivateKey), crypto.SHA256, digest, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp := &AuthResponse{Signature: sig, ClientAuthCertificate: creds.chain[0],
		IntermediateCertificate: creds.chain[1:], SignatureAlgorithm: ptr(SignatureRSASSAPSS),
		SenderNonce: nonce, HashAlgorithm: ptr(HashSHA256)}
	if err := (&CertVerifier{Roots: creds.roots}).Verify(resp, nonce, tlsLeaf); err != nil {
		t.Fatal(err)
	}
}

func x509Leaf(t *testing.T, der []byte) *x509.Certificate {
	t.Helper()
	c, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return c
}
