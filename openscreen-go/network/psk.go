package ospc

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"errors"
	"fmt"
	"math/big"
	"strings"

	spake2 "github.com/backkem/spake2-go"
)

// PSK entropy bounds of auth-capabilities psk-min-bits-of-entropy.
// network.bs: "in the range of 20 to 60 bits inclusive, with a default of 20".
const (
	minPSKEntropy = 20
	maxPSKEntropy = 60
)

// ErrInvalidPSK is returned for a PSK that is not a base-10 numeric PSK.
var ErrInvalidPSK = errors.New("invalid PSK")

// ParsePSK decodes a base-10 numeric PSK as typed or displayed (network.bs
// Appendix B "Base-10 Numeric"): "Remove dashes and leading zeros from N.
// Parse N as a base-10 decimal number to obtain P." Whitespace is ignored as
// well. It returns the canonical decimal text of P, which is the SPAKE2
// password pw: e.g. "0614-8854-8833" and "61488548833" both yield
// "61488548833". This is the encoding Chromium's openscreen uses too.
func ParsePSK(s string) (string, error) {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '-' || r == ' ' || r == '\t' || r == '\n' || r == '\r':
			continue
		case r < '0' || r > '9':
			return "", fmt.Errorf("%w: %q is not a base-10 numeric PSK", ErrInvalidPSK, s)
		case r == '0' && b.Len() == 0:
			continue // Leading zero.
		}
		b.WriteRune(r)
	}
	if b.Len() == 0 {
		return "", fmt.Errorf("%w: %q has no non-zero digits", ErrInvalidPSK, s)
	}
	return b.String(), nil
}

// FormatPSK encodes a numeric PSK for display to the user (network.bs
// Appendix B "Base-10 Numeric"): groups of three digits separated by dashes
// below 9 digits, groups of four otherwise, zero-padded on the left to fill
// the first group. The input may be in any form ParsePSK accepts.
//
// The spec's padding rule ("Zero-pad N on the left with 3 - len(N) mod 3
// digits") would add a whole group of zeros when len(N) is already a multiple
// of the group size, and it doesn't cover exactly 9 digits; like Chromium's
// openscreen, padding is only added when needed and 9 digits use groups of
// four. Leading zeros are dropped when decoding, so this doesn't affect
// interoperability.
func FormatPSK(psk string) (string, error) {
	n, err := ParsePSK(psk)
	if err != nil {
		return "", err
	}
	group := 4
	if len(n) < 9 {
		group = 3
	}
	if r := len(n) % group; r != 0 {
		n = strings.Repeat("0", group-r) + n
	}
	var b strings.Builder
	for i := 0; i < len(n); i += group {
		if i > 0 {
			b.WriteByte('-')
		}
		b.WriteString(n[i : i+group])
	}
	return b.String(), nil
}

// generatePSK returns a random non-zero PSK of the given bits of entropy, as
// canonical decimal text.
func generatePSK(bits int) ([]byte, error) {
	limit := new(big.Int).Lsh(big.NewInt(1), uint(bits))
	for {
		n, err := rand.Int(rand.Reader, limit)
		if err != nil {
			return nil, err
		}
		if n.Sign() != 0 {
			return []byte(n.String()), nil
		}
	}
}

// ErrSpake2SuiteUnsupported is returned when authenticating without
// InteropQuirks.Spake2SuiteP256: the spec's edwards25519 suite is not
// implemented.
var ErrSpake2SuiteUnsupported = errors.New("SPAKE2 edwards25519 suite (network.bs) is not implemented; enable InteropQuirks.Spake2SuiteP256 to use the P-256 suite")

// newSpake2 creates the SPAKE2 state for one authentication. alice selects
// the role: the initiator is Alice (network.bs: "The PSK presenter or the PSK
// consumer may initiate authentication (assuming the role of Alice in
// SPAKE2)"). identityA and identityB are the agent fingerprints of the client
// and server. pw is the canonical PSK, see ParsePSK.
//
// The suite is RFC 9382 SPAKE2-P256-SHA256-HKDF-HMAC (see
// InteropQuirks.Spake2SuiteP256) with the network.bs password hash: "Password
// hash function is SHA-512", i.e. w = SHA-512(pw) mod p.
func newSpake2(alice bool, pw, identityA, identityB []byte) *spake2.SPAKE2 {
	// spake2-go uses Ciphersuite.Hash both for the password hash, which it
	// only evaluates in NewClient/NewServer, and for the transcript hash,
	// evaluated later through the same *Ciphersuite. Construct with SHA-512
	// and then switch this instance's ciphersuite back to SHA-256. Pinned by
	// TestSpake2PasswordHashSHA512.
	cs := spake2.DefaultCiphersuite()
	cs.Hash = sha512.New
	opts := &spake2.Options{
		Ciphersuite: cs,
		IdentityA:   identityA,
		IdentityB:   identityB,
	}
	var s *spake2.SPAKE2
	if alice {
		s = spake2.NewClient(pw, opts)
	} else {
		s = spake2.NewServer(pw, opts)
	}
	cs.Hash = sha256.New
	return s
}
