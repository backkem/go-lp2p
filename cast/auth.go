package cast

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"errors"
	"fmt"
	"time"

	// Register hash implementations used by device auth.
	_ "crypto/sha1"
	_ "crypto/sha256"
)

// Device authentication (urn:x-cast:com.google.cast.tp.deviceauth).
//
// The sender sends an AuthChallenge carrying a fresh nonce. The receiver
// answers with an AuthResponse: its device certificate chain and a signature,
// made with the device key, over sender_nonce || receiver TLS certificate DER.
// Verifying the chain proves the device is genuine; verifying the signature
// binds that identity to the TLS session the sender is talking over.

// NonceSize is the size of the sender nonce (kNonceSizeInBytes).
const NonceSize = 16

// DefaultMaxTLSCertLifetime is the longest validity period accepted for a
// receiver's self-signed TLS certificate (kMaxSelfSignedCertLifetimeInDays).
const DefaultMaxTLSCertLifetime = 4 * 24 * time.Hour

// ErrAuthFailed wraps all device authentication failures.
var ErrAuthFailed = errors.New("cast: device authentication failed")

// AuthErrorResponse is returned when the receiver answers with an AuthError.
type AuthErrorResponse struct{ Type AuthErrorType }

func (e *AuthErrorResponse) Error() string {
	return fmt.Sprintf("cast: receiver returned auth error %d", e.Type)
}

func (e *AuthErrorResponse) Unwrap() error { return ErrAuthFailed }

// Verifier checks an AuthResponse. nonce is the challenge nonce and peerCert
// is the receiver's TLS leaf certificate as seen by the sender.
type Verifier interface {
	Verify(resp *AuthResponse, nonce []byte, peerCert *x509.Certificate) error
}

// VerifierFunc adapts a function to Verifier.
type VerifierFunc func(resp *AuthResponse, nonce []byte, peerCert *x509.Certificate) error

// Verify implements Verifier.
func (f VerifierFunc) Verify(resp *AuthResponse, nonce []byte, peerCert *x509.Certificate) error {
	return f(resp, nonce, peerCert)
}

// InsecureVerifier accepts every AuthResponse. Use only for testing or with
// receivers you trust by other means.
type InsecureVerifier struct{}

// Verify implements Verifier.
func (InsecureVerifier) Verify(*AuthResponse, []byte, *x509.Certificate) error { return nil }

// CertVerifier ports the checks of openscreen's AuthenticateChallengeReply /
// VerifyCredentials: TLS certificate lifetime, nonce echo, device
// certificate chain (path building and validation as in openscreen's
// FindCertificatePath, see certpath.go), hash policy and the signature
// binding the device key to the TLS certificate.
//
// Genuine Cast devices chain to Google's Cast root CAs: use
// GoogleCastRoots(). CRL checking is not implemented: a CRL in the response
// is ignored (openscreen senders treat CRLs as optional too).
//
// openscreen does not enforce the sender nonce echo by default
// (enforce_nonce_checking=false); CertVerifier always does.
type CertVerifier struct {
	// Roots are the trust anchors, matched by subject name like
	// openscreen's TrustStore. Required unless InsecureSkipChainVerify is
	// set.
	Roots []*x509.Certificate
	// InsecureSkipChainVerify trusts any device certificate but still
	// verifies the signature binding (useful for self-made test devices).
	InsecureSkipChainVerify bool
	// RequireSHA256 rejects SHA1 signatures.
	RequireSHA256 bool
	// MaxTLSCertLifetime bounds the receiver TLS certificate validity
	// period; zero means DefaultMaxTLSCertLifetime, negative disables.
	MaxTLSCertLifetime time.Duration
	// Now returns the verification time; defaults to time.Now.
	Now func() time.Time
}

// Verify implements Verifier.
func (v *CertVerifier) Verify(resp *AuthResponse, nonce []byte, peerCert *x509.Certificate) error {
	fail := func(format string, args ...any) error {
		return fmt.Errorf("%w: %s", ErrAuthFailed, fmt.Sprintf(format, args...))
	}
	now := time.Now()
	if v.Now != nil {
		now = v.Now()
	}
	if peerCert == nil {
		return fail("no peer TLS certificate")
	}
	if now.Before(peerCert.NotBefore) || now.After(peerCert.NotAfter) {
		return fail("peer TLS certificate not valid at %v", now)
	}
	maxLife := v.MaxTLSCertLifetime
	if maxLife == 0 {
		maxLife = DefaultMaxTLSCertLifetime
	}
	if maxLife > 0 && peerCert.NotAfter.Sub(peerCert.NotBefore) > maxLife {
		return fail("peer TLS certificate lifetime exceeds %v", maxLife)
	}
	if !bytes.Equal(resp.SenderNonce, nonce) {
		return fail("sender nonce mismatch")
	}

	leaf, err := x509.ParseCertificate(resp.ClientAuthCertificate)
	if err != nil {
		return fail("parse device certificate: %v", err)
	}
	if !v.InsecureSkipChainVerify {
		if len(v.Roots) == 0 {
			return fail("no trusted roots configured")
		}
		var inter []*x509.Certificate
		for _, der := range resp.IntermediateCertificate {
			c, err := x509.ParseCertificate(der)
			if err != nil {
				return fail("parse intermediate certificate: %v", err)
			}
			inter = append(inter, c)
		}
		if _, err := verifyDeviceCertPath(leaf, inter, v.Roots, now); err != nil {
			return fail("device certificate chain: %v", err)
		}
	}

	var h crypto.Hash
	switch resp.Hash() {
	case HashSHA1:
		if v.RequireSHA256 {
			return fail("SHA1 signature rejected")
		}
		h = crypto.SHA1
	case HashSHA256:
		h = crypto.SHA256
	default:
		return fail("unsupported hash algorithm %d", resp.Hash())
	}
	pub, ok := leaf.PublicKey.(*rsa.PublicKey)
	if !ok {
		return fail("device key is %T, want RSA", leaf.PublicKey)
	}
	digest := signedDigest(h, resp.SenderNonce, peerCert.Raw)
	sigAlg := SignatureRSASSAPKCS1v15
	if resp.SignatureAlgorithm != nil {
		sigAlg = *resp.SignatureAlgorithm
	}
	switch sigAlg {
	case SignatureRSASSAPKCS1v15, SignatureUnspecified:
		err = rsa.VerifyPKCS1v15(pub, h, digest, resp.Signature)
	case SignatureRSASSAPSS:
		err = rsa.VerifyPSS(pub, h, digest, resp.Signature, &rsa.PSSOptions{SaltLength: rsa.PSSSaltLengthAuto})
	default:
		return fail("unsupported signature algorithm %d", sigAlg)
	}
	if err != nil {
		return fail("signature: %v", err)
	}
	return nil
}

func signedDigest(h crypto.Hash, nonce, tlsCertDER []byte) []byte {
	w := h.New()
	w.Write(nonce)
	w.Write(tlsCertDER)
	return w.Sum(nil)
}

// Authenticate runs the sender side of device authentication on a started
// channel: it sends an AuthChallenge (SHA256, as openscreen's sender does)
// from sender-0 to receiver-0 and verifies the reply with v. It temporarily
// owns the AuthNamespace handler and must not be called from a Handler.
func Authenticate(ctx context.Context, ch *Channel, v Verifier) error {
	peer := ch.Socket().PeerCertificate()
	if peer == nil {
		return fmt.Errorf("%w: channel is not TLS", ErrAuthFailed)
	}
	nonce := make([]byte, NonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	replies := make(chan *CastMessage, 1)
	ch.Handle(AuthNamespace, func(_ *Channel, m *CastMessage) {
		select {
		case replies <- m:
		default:
		}
	})
	defer ch.Handle(AuthNamespace, nil)

	sha256 := HashSHA256
	challenge := &DeviceAuthMessage{Challenge: &AuthChallenge{SenderNonce: nonce, HashAlgorithm: &sha256}}
	if err := ch.Send(NewBinaryMessage(PlatformSenderID, PlatformReceiverID, AuthNamespace, challenge.Marshal())); err != nil {
		return err
	}
	var reply *CastMessage
	select {
	case reply = <-replies:
	case <-ctx.Done():
		return ctx.Err()
	case <-ch.Done():
		return ErrChannelClosed
	}
	if reply.PayloadType != PayloadBinary {
		return fmt.Errorf("%w: reply payload is not binary", ErrAuthFailed)
	}
	var dam DeviceAuthMessage
	if err := dam.Unmarshal(reply.PayloadBinary); err != nil {
		return fmt.Errorf("%w: %v", ErrAuthFailed, err)
	}
	if dam.Error != nil {
		return &AuthErrorResponse{Type: dam.Error.ErrorType}
	}
	if dam.Response == nil {
		return fmt.Errorf("%w: reply has no response", ErrAuthFailed)
	}
	return v.Verify(dam.Response, nonce, peer)
}

// DeviceAuthResponder is the receiver side of device authentication
// (openscreen's DeviceAuthNamespaceHandler). Register Handler() for
// AuthNamespace on receiver channels.
type DeviceAuthResponder struct {
	// Chain is the device certificate chain, leaf first (DER).
	Chain [][]byte
	// Key is the device private key (RSA) matching Chain[0].
	Key crypto.Signer
	// CRL is the serialized CRL sent along, if any.
	CRL []byte
}

// Handler returns the AuthNamespace handler.
func (r *DeviceAuthResponder) Handler() Handler {
	return func(ch *Channel, msg *CastMessage) {
		if msg.PayloadType != PayloadBinary {
			return
		}
		var dam DeviceAuthMessage
		if dam.Unmarshal(msg.PayloadBinary) != nil || dam.Challenge == nil ||
			dam.Response != nil || dam.Error != nil {
			return
		}
		reply := func(m *DeviceAuthMessage) {
			_ = ch.Send(NewBinaryMessage(msg.DestinationID, msg.SourceID, AuthNamespace, m.Marshal()))
		}
		replyErr := func(t AuthErrorType) { reply(&DeviceAuthMessage{Error: &AuthError{ErrorType: t}}) }

		resp, errType, ok := r.respond(ch.Socket(), dam.Challenge)
		if !ok {
			replyErr(errType)
			return
		}
		reply(&DeviceAuthMessage{Response: resp})
	}
}

func (r *DeviceAuthResponder) respond(sock *Socket, c *AuthChallenge) (*AuthResponse, AuthErrorType, bool) {
	if _, isTLS := sock.ConnectionState(); !isTLS {
		return nil, AuthErrorNoTLS, false
	}
	if c.SignatureAlgorithm != nil && *c.SignatureAlgorithm != SignatureUnspecified &&
		*c.SignatureAlgorithm != SignatureRSASSAPKCS1v15 {
		return nil, AuthErrorSignatureAlgorithmUnavailable, false
	}
	var h crypto.Hash
	switch c.Hash() {
	case HashSHA1:
		h = crypto.SHA1
	case HashSHA256:
		h = crypto.SHA256
	default:
		return nil, AuthErrorSignatureAlgorithmUnavailable, false
	}
	tlsDER := sock.LocalCertificate()
	if len(tlsDER) == 0 || len(r.Chain) == 0 || r.Key == nil {
		return nil, AuthErrorInternal, false
	}
	if _, isRSA := r.Key.Public().(*rsa.PublicKey); !isRSA {
		return nil, AuthErrorInternal, false // we only advertise RSASSA_PKCS1v15
	}
	sig, err := r.Key.Sign(rand.Reader, signedDigest(h, c.SenderNonce, tlsDER), h)
	if err != nil {
		return nil, AuthErrorInternal, false
	}
	pkcs1, hashAlg := SignatureRSASSAPKCS1v15, c.Hash()
	resp := &AuthResponse{
		Signature:             sig,
		ClientAuthCertificate: r.Chain[0],
		SignatureAlgorithm:    &pkcs1,
		HashAlgorithm:         &hashAlg,
		CRL:                   r.CRL,
	}
	if len(r.Chain) > 1 {
		resp.IntermediateCertificate = r.Chain[1:]
	}
	if c.SenderNonce != nil {
		resp.SenderNonce = c.SenderNonce
	}
	if resp.CRL == nil {
		resp.CRL = []byte{} // openscreen always sets crl
	}
	return resp, 0, true
}
