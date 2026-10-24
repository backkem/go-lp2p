package ospc

import "errors"

// InteropQuirks relaxes the protocol to interoperate with implementations
// that deviate from the Open Screen Network Protocol (network.bs), such as
// older builds of Chromium's osp_demo. Every quirk breaks a rule of the spec
// and is off by default. Enable them only for interop testing and debugging,
// never in production: several of them remove security guarantees.
//
// Only behavior that violates the spec is gated here. Spec-compatible
// leniency (e.g. dialing an agent that doesn't advertise the non-standard
// "sn" TXT key, or discovering an agent that omits "mv") is always on.
type InteropQuirks struct {
	// AcceptCertificateFingerprint accepts an "fp" TXT value computed as the
	// base64 SHA-256 of the whole DER-encoded certificate, in addition to the
	// spec's SPKI fingerprint, when dialing.
	//
	// Violates network.bs "Computing the Agent Fingerprint" (fp is the
	// RFC 7469 SPKI fingerprint) and "TLS 1.3" (the fingerprint calculated for
	// the received certificate must match fp). Older osp_demo builds advertise
	// the whole-certificate hash.
	AcceptCertificateFingerprint bool

	// AdvertiseCertificateFingerprint advertises "fp" as the base64 SHA-256
	// of the whole DER-encoded certificate instead of the SPKI fingerprint,
	// so peers that pin the whole-certificate hash (older osp_demo
	// controllers) accept the listener's certificate. Spec-compliant peers
	// will reject the listener.
	//
	// Violates network.bs "Computing the Agent Fingerprint". The PeerID and
	// the SPAKE2 identities are still the SPKI fingerprint.
	AdvertiseCertificateFingerprint bool

	// TolerateMissingAgentInfo lets Dial and Accept proceed when the peer
	// doesn't send agent-info-response and auth-capabilities within a short
	// grace period. The remote agent's Info is then nil and its
	// authentication capabilities are assumed equal to the local ones.
	//
	// Violates network.bs "Metadata Discovery" and "Authentication": agents
	// must answer agent-info-request, and authentication starts from the
	// exchanged auth-capabilities. Older osp_demo builds implement neither.
	TolerateMissingAgentInfo bool

	// AllowSkipAuthentication enables
	// UnauthenticatedConnection.SkipAuthentication, which hands the
	// connection to the application protocol WITHOUT authenticating the
	// peer. Application messages the peer sent before the hand-off are
	// delivered rather than discarded. Every skipped authentication is logged
	// at warning level.
	//
	// INSECURE. Violates network.bs "Authentication": agents must
	// authenticate with SPAKE2 before exchanging application messages. Only
	// for debugging against peers without authentication.
	AllowSkipAuthentication bool
}

// Any reports whether any quirk is enabled.
func (q InteropQuirks) Any() bool {
	return q != InteropQuirks{}
}

// ErrQuirkDisabled is returned when calling an API that requires an interop
// quirk that is not enabled in the AgentConfig.
var ErrQuirkDisabled = errors.New("interop quirk not enabled")
