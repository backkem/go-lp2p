package ospc

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"math/big"

	mdns "github.com/grandcat/zeroconf"
)

// Dial opens a connection to the remote agent.
func (ra DiscoveredAgent) Dial(ctx context.Context, transportType AgentTransport, la *Agent) (*UnauthenticatedConnection, error) {
	snBase64, err := ra.TXT.GetOne("sn")
	if err != nil {
		return nil, fmt.Errorf("failed to get sn record: %v", err)
	}
	fp, err := ra.TXT.GetOne("fp")
	if err != nil {
		return nil, fmt.Errorf("failed to get fp record: %v", err)
	}

	// Decode URL-safe base64 serial number to get expected certificate serial number.
	// See: https://github.com/w3c/openscreenprotocol/issues/365
	expectedSNBytes, err := base64.RawURLEncoding.DecodeString(snBase64)
	if err != nil {
		return nil, fmt.Errorf("failed to decode sn: %v", err)
	}
	expectedSN := new(big.Int).SetBytes(expectedSNBytes)

	// Build OpenScreen-compliant hostname from discovered agent info
	// Unescaped, without the truncation marker (e.g., "Test\ Receiver" -> "Test Receiver")
	instanceName := ra.instanceName.Name
	domain := MdnsDomain
	cn := buildAgentHostname(snBase64, instanceName, domain)

	tlsConfig := &tls.Config{
		MinVersion:         tls.VersionTLS13, // OpenScreen spec requires TLS 1.3
		MaxVersion:         tls.VersionTLS13,
		InsecureSkipVerify: true, // Manual verification in VerifyConnection
		VerifyConnection: func(cs tls.ConnectionState) error {
			if len(cs.PeerCertificates) == 0 {
				return errors.New("no peer certificate")
			}
			if len(cs.PeerCertificates) != 1 {
				return errors.New("didn't expect cert chain")
			}
			peerCert := cs.PeerCertificates[0]

			// Verify certificate serial number matches advertised value
			if peerCert.SerialNumber.Cmp(expectedSN) != 0 {
				return fmt.Errorf("certificate serial number mismatch: expected %s, got %s", expectedSN.String(), peerCert.SerialNumber.String())
			}

			roots := x509.NewCertPool()
			roots.AddCert(peerCert)

			opts := x509.VerifyOptions{
				DNSName: cn,
				Roots:   roots,
			}
			_, err := peerCert.Verify(opts)
			return err
		},
		NextProtos:   []string{ALPN_OSP}, // Application-Layer Protocol Negotiation
		ServerName:   cn,
		Certificates: []tls.Certificate{*la.Certificate},
		VerifyPeerCertificate: func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
			certs := []tls.Certificate{}
			for _, rawCert := range rawCerts {
				leaf, err := x509.ParseCertificate(rawCert)
				if err != nil {
					return err
				}

				cert := tls.Certificate{
					Certificate: [][]byte{rawCert},
					Leaf:        leaf,
				}
				certs = append(certs, cert)
			}

			return validateFingerprint(fp, certs)
		},
	}
	addr := fmt.Sprintf("%s:%d", getMdnsHost(ra.info), ra.info.Port)

	return ra.dial(ctx, transportType, la, addr, tlsConfig)
}

// dial connects to addr and runs the info exchange. On failure, including
// cancellation of ctx, everything set up so far is closed.
func (ra DiscoveredAgent) dial(ctx context.Context, transportType AgentTransport, la *Agent, addr string, tlsConfig *tls.Config) (*UnauthenticatedConnection, error) {
	t, err := newAgentTransport(transportType, la.keepAlive, la.logger())
	if err != nil {
		return nil, err
	}
	nc, err := t.DialAddr(ctx, addr, tlsConfig)
	if err != nil {
		return nil, err
	}

	remoteAgent, err := la.NewRemoteAgent(nc)
	if err != nil {
		_ = nc.Close()
		return nil, err
	}
	remoteAgent.setDiscovery(ra.instanceName, ra.metadataVersion, ra.hasMetadataVersion)

	// The at of the advertising agent, used as auth-initiation-token.
	at, err := ra.AuthToken()
	if err != nil {
		la.logger().Debug("dial: no usable at record", "err", err)
		at = ""
	}

	bConn := newBaseConnection(nc, la, remoteAgent, AgentRoleClient, at)
	bConn.runNetwork()

	// TODO: handle meta-discovery failure.
	if err := bConn.awaitInfo(ctx); err != nil {
		_ = bConn.Close()
		return nil, err
	}

	return &UnauthenticatedConnection{
		base: bConn,
	}, nil
}

func getMdnsHost(entry *mdns.ServiceEntry) string {
	for _, ipv6 := range entry.AddrIPv6 {
		return fmt.Sprintf("[%s]", ipv6)
	}
	for _, ipv4 := range entry.AddrIPv4 {
		return ipv4.String()
	}
	return entry.HostName
}
