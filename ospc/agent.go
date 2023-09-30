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
	"time"

	"github.com/pion/dtls/v2/pkg/crypto/fingerprint"
)

type AgentRole int

const (
	AgentRoleClient AgentRole = 0
	AgentRoleServer AgentRole = 1
)

type AgentConfig struct {
	Nickname  string
	ModelName string

	Certificate          *tls.Certificate
	CertificateSNBase    uint32
	CertificateSNCounter uint32

	PSKConfig PSKConfig
}

func NewAgentConfig(nickname string) AgentConfig {
	return AgentConfig{
		Nickname: nickname,
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

func (c *AgentConfig) normalize() (err error) {
	if len(c.ModelName) == 0 {
		c.ModelName = "OSPC-GO"
	}

	if c.PSKConfig.Entropy == 0 {
		c.PSKConfig.Entropy = 20
	}

	if c.CertificateSNBase == 0 {
		err = binary.Read(rand.Reader, binary.BigEndian, &c.CertificateSNBase)
		if err != nil {
			return err
		}
	}

	if c.Certificate == nil {
		c.Certificate, err = c.generateCert()
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *AgentConfig) generateCert() (*tls.Certificate, error) {
	privKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		return nil, err
	}
	pubKey := privKey.Public()

	serialNumber := uint64(c.CertificateSNBase)<<32 | uint64(c.CertificateSNCounter) // TODO: Manage counter state.

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
			CommonName: c.Nickname,
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

func (c *AgentConfig) CertificateFingerPrint() (string, error) {
	// fp                     = hash-func SP fingerprint
	// hash-func              =  "sha-256" / "sha-512"
	// fingerprint            =  2UHEX *(":" 2UHEX)
	//                           ; Each byte in upper-case hex, separated
	//                           ; by colons.
	// UHEX                   =  DIGIT / %x41-46 ; A-F uppercase

	hashAlgo := crypto.SHA512
	hashName := "sha-512"

	fp, err := fingerprint.Fingerprint(c.Certificate.Leaf, hashAlgo)
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
