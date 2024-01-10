package ospc

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/pion/dtls/v2/pkg/crypto/fingerprint"
	"github.com/quic-go/quic-go"
)

type AgentRole int

const (
	AgentRoleClient AgentRole = 0
	AgentRoleServer AgentRole = 1
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

	// AuthInfo
	PSKConfig PSKConfig
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

func (c *AgentConfig) WithModelName(easeOfInput, entropy int) {
	c.PSKConfig = PSKConfig{
		EaseOfInput: easeOfInput,
		Entropy:     entropy,
	}
}

func (c *AgentConfig) WithCertificateSNBase(snBase uint32) {
	c.CertificateSNBase = snBase
}

// PeerID is a unique identifier for a peer, it is equal tot the
// OSP agent fingerprint (FP) of the peer.
type PeerID string

type Agent struct {
	PeerID PeerID

	Certificate          *tls.Certificate
	CertificateSNBase    uint32
	CertificateSNCounter uint32

	mu sync.Mutex

	info               *AgentInfo
	authenticationInfo *AgentAuthenticationInfo

	knownPeers map[PeerID]knownPeer
}

type knownPeer struct {
	Agent         *Agent
	Authenticated bool
}

type AgentAuthenticationInfo struct {
	PSKConfig PSKConfig
}

type AgentInfo struct {
	DisplayName string
	ModelName   string
	// Capabilities []agentCapability
	// StateToken string // TODO: State token
	Locales []string
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

	return agent, nil
}

func (a *Agent) NewRemoteAgent(conn quic.Connection) (*Agent, error) {

	certs := conn.ConnectionState().TLS.PeerCertificates
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

	if agent, ok := a.knownPeers[peerID]; ok {
		// TODO: avoid re-authenticating known peers.
		return agent.Agent, nil
	}

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
		DisplayName: a.info.DisplayName,
		ModelName:   a.info.ModelName,
		Locales:     a.info.Locales,
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
	privKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		return nil, err
	}
	pubKey := privKey.Public()

	serialNumber := uint64(certificateSNBase)<<32 | uint64(certificateSNCounter)

	cn := fmt.Sprintf("%d._openscreen._udp", serialNumber) // TODO: openscreenprotocol#293
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
	// fp                     = hash-func SP fingerprint
	// hash-func              =  "sha-256" / "sha-512"
	// fingerprint            =  2UHEX *(":" 2UHEX)
	//                           ; Each byte in upper-case hex, separated
	//                           ; by colons.
	// UHEX                   =  DIGIT / %x41-46 ; A-F uppercase

	hashAlgo := crypto.SHA512
	hashName := "sha-512"

	fp, err := fingerprint.Fingerprint(cert, hashAlgo)
	if err != nil {
		return "", fmt.Errorf("failed to create hash: %v", err)
	}

	return fmt.Sprintf("%s %s", hashName, fp), nil
}
func validateFingerprint(fp string, remoteCerts []tls.Certificate) error {
	for _, cert := range remoteCerts {
		n := strings.IndexRune(fp, ' ')
		if n < 0 {
			return errors.New("failed to find fingerprint algo")
		}
		algo := fp[:n]
		fpValue := fp[n+1:]

		hashAlgo, err := fingerprint.HashFromString(algo)
		if err != nil {
			return err
		}

		remoteValue, err := fingerprint.Fingerprint(cert.Leaf, hashAlgo)
		if err != nil {
			return err
		}

		if strings.EqualFold(remoteValue, fpValue) {
			return nil
		}
	}

	return errors.New("no certificate matching fingerprint")
}

type PSKConfig struct {
	EaseOfInput int // 0-100
	Entropy     int // 20-60 bits
}
