package ospc

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"sync"
	"time"
)

type AgentRole int

const (
	AgentRoleClient AgentRole = 0
	AgentRoleServer AgentRole = 1
)

type AgentTransport int

const (
	AgentTransportQUIC AgentTransport = iota + 1
	AgentTransportWebRTC
)

type AgentConfig struct {
	// ID
	PeerID            PeerID
	Certificate       *tls.Certificate
	CertificateSNBase uint32

	// Info
	DisplayName string
	ModelName   string
	Locales     []string

	// Capabilities are advertised in agent-info in addition to the ones ospc
	// itself implements (data channels, QUIC transport). application.bs
	// "Presentation API": a controlling user agent must support
	// control-presentation (CapabilityControlPresentation), a receiving user
	// agent receive-presentation (CapabilityReceivePresentation).
	Capabilities []AgentCapability

	// AuthInfo
	PSKConfig PSKConfig

	SupportedTransports []AgentTransport

	// KeepAlive configures the connection keep-alive.
	KeepAlive KeepAliveConfig

	// Logger receives the logs of the agent and its connections. Per-message
	// logs are at debug level. Nil discards all logs.
	Logger *slog.Logger

	// InteropQuirks enables deviations from the spec for interop with
	// non-conforming peers. All are off by default, see InteropQuirks.
	InteropQuirks InteropQuirks
}

func NewAgentConfig(nickname string) AgentConfig {
	return AgentConfig{
		DisplayName: nickname,
	}
}

func (c *AgentConfig) WithPSKConfig(easeOfInput, entropy int) {
	c.PSKConfig = PSKConfig{
		EaseOfInput: easeOfInput,
		Entropy:     entropy,
	}
}

func (c *AgentConfig) WithModelName(modelName string) {
	c.ModelName = modelName
}

// WithLogger sets the logger of the agent, see AgentConfig.Logger.
func (c *AgentConfig) WithLogger(logger *slog.Logger) {
	c.Logger = logger
}

func (c *AgentConfig) WithCertificateSNBase(snBase uint32) {
	c.CertificateSNBase = snBase
}

// PeerID is a unique identifier for a peer, it is equal tot the
// OSP agent fingerprint (FP) of the peer.
type PeerID string

// short returns an abbreviated PeerID for logging.
func (id PeerID) short() string {
	if len(id) > 8 {
		return string(id[:8])
	}
	return string(id)
}

// discardLogger is used when no logger is configured.
var discardLogger = slog.New(slog.DiscardHandler)

type Agent struct {
	PeerID PeerID

	Certificate          *tls.Certificate
	CertificateSNBase    uint32
	CertificateSNCounter uint32

	mu sync.Mutex

	// Discovery state, see discover_names.go and metadata.go.
	// For a local agent metadataVersion is the advertised mv value. For a
	// remote agent instanceName and metadataVersion are what it was
	// discovered with, if it was discovered.
	instanceName       *InstanceName
	metadataVersion    uint64
	hasMetadataVersion bool
	authenticated      bool

	info               *AgentInfo
	authenticationInfo *AgentAuthenticationInfo

	capabilities []AgentCapability // Only set for local agents, see AgentConfig.Capabilities.

	keepAlive KeepAliveConfig
	log       *slog.Logger  // Only set for local agents.
	quirks    InteropQuirks // Only set for local agents.

	// Local agent state, see request_id.go. Initialized lazily.
	stateToken string
	requestIDs *RequestIDAllocator
}

type AgentAuthenticationInfo struct {
	PSKConfig PSKConfig
}

type AgentInfo struct {
	DisplayName string
	ModelName   string
	// Capabilities as listed in agent-info. For a remote agent they are
	// what it advertised; for a local agent see AgentInfoMessage.
	Capabilities []AgentCapability
	StateToken   string // 8 characters in the range [0-9A-Za-z]
	Locales      []string
}

func NewAgent(c AgentConfig) (*Agent, error) {
	var agent *Agent

	certificateSNBase := uint32(0)
	if c.CertificateSNBase != 0 {
		certificateSNBase = c.CertificateSNBase
	} else {
		err := binary.Read(rand.Reader, binary.BigEndian, &certificateSNBase)
		if err != nil {
			return nil, err
		}
	}
	certificateSNCounter := uint32(0) // TODO: Manage counter state.

	var peerID PeerID
	var cert *tls.Certificate
	if c.Certificate != nil {
		peerID = c.PeerID
		cert = c.Certificate
	} else {
		var err error
		cert, err = generateCert(c.DisplayName, certificateSNBase, certificateSNCounter)
		if err != nil {
			return nil, err
		}
		rawPeerID, err := certificateFingerPrint(cert.Leaf)
		if err != nil {
			return nil, err
		}
		peerID = PeerID(rawPeerID)
	}

	agent = &Agent{
		PeerID:               peerID,
		Certificate:          cert,
		CertificateSNBase:    certificateSNBase,
		CertificateSNCounter: certificateSNCounter,
		mu:                   sync.Mutex{},
	}

	agent.info = &AgentInfo{
		DisplayName: c.DisplayName,
		ModelName:   "OSPC-GO",
		Locales:     c.Locales,
	}
	if len(c.DisplayName) != 0 {
		agent.info.DisplayName = c.DisplayName
	}
	if len(c.ModelName) != 0 {
		agent.info.ModelName = c.ModelName
	}

	agent.authenticationInfo = &AgentAuthenticationInfo{
		PSKConfig: PSKConfig{
			EaseOfInput: 0,
			Entropy:     20,
		},
	}
	if c.PSKConfig.EaseOfInput != 0 {
		agent.authenticationInfo.PSKConfig.EaseOfInput = c.PSKConfig.EaseOfInput
	}
	if c.PSKConfig.Entropy != 0 {
		agent.authenticationInfo.PSKConfig.Entropy = c.PSKConfig.Entropy
	}
	// network.bs "Authentication": "PSK ease of input is an integer in the
	// range from 0 to 100 inclusive" and psk-min-bits-of-entropy is "in the
	// range of 20 to 60 bits inclusive".
	if psk := agent.authenticationInfo.PSKConfig; psk.EaseOfInput < 0 || psk.EaseOfInput > 100 {
		return nil, fmt.Errorf("PSK ease of input %d out of range 0-100", psk.EaseOfInput)
	} else if psk.Entropy < minPSKEntropy || psk.Entropy > maxPSKEntropy {
		return nil, fmt.Errorf("PSK entropy %d bits out of range %d-%d", psk.Entropy, minPSKEntropy, maxPSKEntropy)
	}

	agent.capabilities = append([]AgentCapability(nil), c.Capabilities...)
	agent.keepAlive = c.KeepAlive
	agent.log = c.Logger
	agent.quirks = c.InteropQuirks
	if agent.quirks.Any() {
		agent.logger().Warn("interop quirks enabled, the agent deviates from the Open Screen Protocol", "quirks", fmt.Sprintf("%+v", agent.quirks))
	}

	return agent, nil
}

// logger returns the logger of a local agent.
func (a *Agent) logger() *slog.Logger {
	if a.log == nil {
		return discardLogger
	}
	return a.log
}

func (a *Agent) NewRemoteAgent(nc NetworkConnection) (*Agent, error) {
	certs := nc.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return nil, errors.New("no peer certificate")
	}
	cert := &tls.Certificate{
		Certificate: [][]byte{},
		Leaf:        certs[0],
	}
	for _, orig := range certs {
		cert.Certificate = append(cert.Certificate, orig.Raw)
	}
	rawPeerID, err := certificateFingerPrint(cert.Leaf)
	if err != nil {
		return nil, err
	}
	peerID := PeerID(rawPeerID)

	return &Agent{
		PeerID:               peerID,
		Certificate:          cert,
		CertificateSNBase:    0,
		CertificateSNCounter: 0,
		mu:                   sync.Mutex{},
	}, nil
}

func (a *Agent) setInfo(info AgentInfo) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.info = &info
}

func (a *Agent) HasInfo() bool {
	a.mu.Lock()
	defer a.mu.Unlock()

	return a.info != nil
}

func (a *Agent) Info() *AgentInfo {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.info == nil {
		return nil
	}

	return &AgentInfo{
		DisplayName:  a.info.DisplayName,
		ModelName:    a.info.ModelName,
		Capabilities: append([]AgentCapability(nil), a.info.Capabilities...),
		StateToken:   a.info.StateToken,
		Locales:      a.info.Locales,
	}
}

// HasCapability reports whether a remote agent listed cap in its agent-info.
// It is false until the agent's info is known.
func (a *Agent) HasCapability(cap AgentCapability) bool {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.info == nil {
		return false
	}
	for _, c := range a.info.Capabilities {
		if c == cap {
			return true
		}
	}
	return false
}

// AgentInfoMessage returns the agent-info a local agent sends in an
// agent-info-response: its display and model name, the capabilities ospc
// implements plus AgentConfig.Capabilities, its state token and locales.
func (a *Agent) AgentInfoMessage() MsgAgentInfo {
	info := a.Info()
	return MsgAgentInfo{
		DisplayName: info.DisplayName,
		ModelName:   info.ModelName,
		Capabilities: append([]AgentCapability{
			AgentCapabilityDataChannels,
			AgentCapabilityQuickTransport,
		}, a.capabilities...),
		StateToken: a.StateToken(),
		Locales:    info.Locales,
	}
}

func (a *Agent) setAuthenticationInfo(authenticationInfo AgentAuthenticationInfo) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.authenticationInfo = &authenticationInfo
}

func (a *Agent) HasAuthenticationInfo() bool {
	a.mu.Lock()
	defer a.mu.Unlock()

	return a.authenticationInfo != nil
}

func (a *Agent) AuthenticationInfo() *AgentAuthenticationInfo {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.authenticationInfo == nil {
		return nil
	}

	return &AgentAuthenticationInfo{
		PSKConfig: a.authenticationInfo.PSKConfig,
	}
}

func generateCert(displayName string, certificateSNBase, certificateSNCounter uint32) (*tls.Certificate, error) {
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	pubKey := privKey.Public()

	serialNumber := uint64(certificateSNBase)<<32 | uint64(certificateSNCounter)

	// Encode serial number as URL-safe base64 (no padding) for hostname.
	// Spec says RFC4648 base64, but standard base64 contains +/= which are
	// invalid in DNS labels. See: https://github.com/w3c/openscreenprotocol/issues/365
	snBig := new(big.Int).SetUint64(serialNumber)
	snBytes := snBig.Bytes()
	snBase64 := base64.RawURLEncoding.EncodeToString(snBytes)

	// Build OpenScreen-compliant hostname: serialNumber.encodedInstanceName.encodedDomain
	cn := buildAgentHostname(snBase64, displayName, MdnsDomain)
	names := []string{cn}

	keyUsage := x509.KeyUsageDigitalSignature // | x509.KeyUsageCertSign

	template := x509.Certificate{
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageClientAuth,
			x509.ExtKeyUsageServerAuth,
		},
		BasicConstraintsValid: true,
		NotBefore:             time.Now(),
		KeyUsage:              keyUsage,
		NotAfter:              time.Now().AddDate(0, 1, 0),
		SerialNumber:          new(big.Int).SetUint64(serialNumber),
		Version:               3,
		IsCA:                  true,
		DNSNames:              names,
		Issuer: pkix.Name{
			CommonName: displayName,
		},
		Subject: pkix.Name{
			CommonName: cn,
		},
	}

	raw, err := x509.CreateCertificate(rand.Reader, &template, &template, pubKey, privKey)
	if err != nil {
		return nil, err
	}

	leaf, err := x509.ParseCertificate(raw)
	if err != nil {
		return nil, err
	}

	return &tls.Certificate{
		Certificate: [][]byte{raw},
		PrivateKey:  any(privKey),
		Leaf:        leaf,
	}, nil
}

func (a *Agent) CertificateFingerPrint() (string, error) {
	return certificateFingerPrint(a.Certificate.Leaf)
}

func certificateFingerPrint(cert *x509.Certificate) (string, error) {
	// Per OpenScreen spec (network.bs - "Computing the Agent Fingerprint"):
	// 1. Compute the SPKI Fingerprint of the agent certificate
	//    according to RFC7469 using SHA-256 as the hash algorithm.
	// 2. base64 encode the result of Step 1 according to RFC4648.
	// Note: The resulting string will be 44 bytes in length.

	// RFC7469: SPKI fingerprint is the SHA-256 hash of the
	// DER-encoded SubjectPublicKeyInfo structure
	hash := crypto.SHA256.New()
	hash.Write(cert.RawSubjectPublicKeyInfo)
	spkiHash := hash.Sum(nil)

	return base64.StdEncoding.EncodeToString(spkiHash), nil
}

// wholeCertificateFingerPrint is the base64 SHA-256 of the whole DER
// certificate, as advertised by older osp_demo builds. Not the spec's agent
// fingerprint, see InteropQuirks.AcceptCertificateFingerprint.
func wholeCertificateFingerPrint(cert *x509.Certificate) string {
	sum := sha256.Sum256(cert.Raw)
	return base64.StdEncoding.EncodeToString(sum[:])
}

// advertisedFingerPrint is the fp TXT value the agent advertises.
func (a *Agent) advertisedFingerPrint() (string, error) {
	if a.quirks.AdvertiseCertificateFingerprint {
		return wholeCertificateFingerPrint(a.Certificate.Leaf), nil
	}
	return a.CertificateFingerPrint()
}

// ErrFingerprintMismatch is returned when the certificate presented by an
// advertising agent doesn't match its advertised fp.
var ErrFingerprintMismatch = errors.New("agent certificate doesn't match advertised fingerprint")

// verifyFingerprint checks the agent certificate against the fp advertised by
// the advertising agent. Per network.bs "TLS 1.3": "The agent fingerprint
// calculated for the received agent certificate must match the fp mDNS TXT
// record as advertised by the advertising agent." With the
// AcceptCertificateFingerprint quirk the whole-certificate hash is accepted
// too.
func verifyFingerprint(fp string, cert *x509.Certificate, quirks InteropQuirks) error {
	spki, err := certificateFingerPrint(cert)
	if err != nil {
		return err
	}
	if spki == fp {
		return nil
	}
	if quirks.AcceptCertificateFingerprint && wholeCertificateFingerPrint(cert) == fp {
		return nil
	}
	return fmt.Errorf("%w: fp %q, SPKI fingerprint %q", ErrFingerprintMismatch, fp, spki)
}

type PSKConfig struct {
	EaseOfInput int // 0-100
	Entropy     int // 20-60 bits
}
