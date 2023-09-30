package ospc

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log"
	"sync"

	mdns "github.com/grandcat/zeroconf"
	"github.com/quic-go/quic-go"
)

// Dial opens a connection to the remote agent.
func (r RemoteAgent) Dial(ctx context.Context, c AgentConfig) (*UnauthenticatedConnection, error) {
	err := c.normalize()
	if err != nil {
		return nil, err
	}

	txt := TXTRecordSet{}
	err = txt.FromSlice(r.info.Text)
	if err != nil {
		return nil, err
	}

	sn, err := txt.GetOne("sn")
	if err != nil {
		return nil, fmt.Errorf("failed to get sn record: %v", err)
	}
	fp, err := txt.GetOne("fp")
	if err != nil {
		return nil, fmt.Errorf("failed to get sn record: %v", err)
	}

	cn := fmt.Sprintf("%s._openscreen._udp", sn) // TODO: openscreenprotocol#293

	tlsConfig := &tls.Config{
		InsecureSkipVerify: true,
		VerifyConnection: func(cs tls.ConnectionState) error {
			if len(cs.PeerCertificates) != 1 {
				return errors.New("didn't expect cert chain")
			}
			peerCert := cs.PeerCertificates[0]
			roots := x509.NewCertPool()
			roots.AddCert(peerCert)

			opts := x509.VerifyOptions{
				DNSName: cn,
				Roots:   roots,
			}
			_, err := peerCert.Verify(opts)
			return err
		},
		NextProtos:   []string{"OSP"}, // Application-Layer Protocol Negotiation
		ServerName:   cn,
		Certificates: []tls.Certificate{*c.Certificate},
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
	addr := fmt.Sprintf("%s:%d", getMdnsHost(r.info), r.info.Port)

	fmt.Println("Remote addr:", addr)
	qConn, err := quic.DialAddr(ctx, addr, tlsConfig, nil)
	if err != nil {
		return nil, err
	}

	bConn := &baseConnection{
		mu:         sync.Mutex{},
		agentRole:  AgentRoleClient,
		agentState: newAgentState(), // TODO: reconnect
		localInfo:  c,
		conn:       qConn,
		close:      make(chan struct{}),
		done:       make(chan struct{}),
	}

	bConn.run()

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
		log.Printf("Choosing IPv6 address [%s]\n", ipv6)
		return fmt.Sprintf("[%s]", ipv6)
	}
	for _, ipv4 := range entry.AddrIPv4 {
		log.Printf("Choosing IPv4 address %s\n", ipv4)
		return ipv4.String()
	}
	// This shouldn't happen
	log.Printf("No IP address found. Falling back to hostname %s\n", entry.HostName)
	return entry.HostName
}
