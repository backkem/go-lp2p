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

	// Decode base64 serial number to get expected certificate serial number
	expectedSNBytes, err := base64.StdEncoding.DecodeString(snBase64)
	if err != nil {
		return nil, fmt.Errorf("failed to decode sn: %v", err)
	}
	expectedSN := new(big.Int).SetBytes(expectedSNBytes)

	// Build OpenScreen-compliant hostname from discovered agent info
	instanceName := ra.info.ServiceRecord.Instance
	domain := MdnsDomain
	cn := buildAgentHostname(snBase64, instanceName, domain)

	tlsConfig := &tls.Config{
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

	t, err := NewNetworkTransport(transportType)
	if err != nil {
		return nil, err
	}
	nc, err := t.DialAddr(ctx, addr, tlsConfig)
	if err != nil {
		return nil, err
	}

	remoteAgent, err := la.NewRemoteAgent(nc)
	if err != nil {
		return nil, err
	}
	bConn := newBaseConnection(
		nc,
		la,
		remoteAgent,
		AgentRoleClient,
	)

	bConn.runNetwork()

	pendingCh := make(chan exchangeInfoResult)
	err = bConn.exchangeInfo(ctx, pendingCh)
	if err != nil {
		return nil, err
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res := <-pendingCh: // TODO: handle meta-discovery failure.
		if res.err != nil {
			return nil, res.err
		}
	}

	return &UnauthenticatedConnection{
		base: bConn,
	}, nil
}

func getMdnsHost(entry *mdns.ServiceEntry) string {
	for _, ipv6 := range entry.AddrIPv6 {
		// log.Printf("Choosing IPv6 address [%s]\n", ipv6)
		return fmt.Sprintf("[%s]", ipv6)
	}
	for _, ipv4 := range entry.AddrIPv4 {
		// log.Printf("Choosing IPv4 address %s\n", ipv4)
		return ipv4.String()
	}
	// log.Printf("No IP address found. Falling back to hostname %s\n", entry.HostName)
	return entry.HostName
}
