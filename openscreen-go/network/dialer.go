package ospc

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"

	mdns "github.com/grandcat/zeroconf"
)

// Dial opens a connection to the remote agent.
//
// The advertising agent is verified as network.bs "TLS 1.3" requires: the
// agent fingerprint of the certificate it presents must match its advertised
// fp TXT record. Nothing else about the certificate is checked, its identity
// is established by authentication.
//
// The non-standard sn TXT key, if present, is used to compute the agent
// hostname sent as TLS server_name. Without it, which is all the spec defines
// (see w3c/openscreenprotocol#293), the hostname can't be computed and the
// server_name is the encoded Instance Name and domain only.
//
// Dial returns once the peer completed metadata discovery (agent-info and
// auth-capabilities), see ErrNoAgentInfo.
func (ra DiscoveredAgent) Dial(ctx context.Context, transportType AgentTransport, la *Agent) (*UnauthenticatedConnection, error) {
	fp, err := ra.TXT.GetOne(txtKeyFingerprint)
	if err != nil {
		return nil, fmt.Errorf("failed to get fp record: %v", err)
	}

	tlsConfig := dialTLSConfig(fp, ra.serverName(), la)
	addr := fmt.Sprintf("%s:%d", getMdnsHost(ra.info), ra.info.Port)

	return ra.dial(ctx, transportType, la, addr, tlsConfig)
}

// serverName returns the TLS server_name for dialing the agent: its agent
// hostname if the advertised sn allows computing it, otherwise the agent
// hostname without the serial number label.
func (ra DiscoveredAgent) serverName() string {
	instanceName := ra.instanceName.Name
	if sn, err := ra.TXT.GetOne("sn"); err == nil && sn != "" {
		// URL-safe base64, see w3c/openscreenprotocol#365.
		return buildAgentHostname(sn, instanceName, MdnsDomain)
	}
	return encodeHostnamePart(hostnameInstanceName(instanceName)) + "." + encodeHostnamePart(MdnsDomain)
}

// dialTLSConfig returns the TLS configuration for dialing an advertising
// agent that advertises fp.
func dialTLSConfig(fp, serverName string, la *Agent) *tls.Config {
	return &tls.Config{
		MinVersion: tls.VersionTLS13, // OpenScreen spec requires TLS 1.3
		MaxVersion: tls.VersionTLS13,
		// The certificate is self-signed and pinned by fp instead, see
		// VerifyConnection.
		InsecureSkipVerify: true,
		VerifyConnection: func(cs tls.ConnectionState) error {
			if len(cs.PeerCertificates) == 0 {
				return errors.New("no peer certificate")
			}
			return verifyFingerprint(fp, cs.PeerCertificates[0], la.quirks)
		},
		NextProtos:   []string{ALPN_OSP}, // Application-Layer Protocol Negotiation
		ServerName:   serverName,
		Certificates: []tls.Certificate{*la.Certificate},
	}
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
