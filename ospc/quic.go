package ospc

import (
	"crypto/tls"
	"net"

	"github.com/quic-go/quic-go"
)

type ospConnectionIDGenerator struct {
}

func (g *ospConnectionIDGenerator) GenerateConnectionID() (quic.ConnectionID, error) {
	return quic.ConnectionID{}, nil
}

func (g *ospConnectionIDGenerator) ConnectionIDLen() int {
	return 0
}

func listenUDP(addr string) (*net.UDPConn, error) {
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, err
	}
	return net.ListenUDP("udp", udpAddr)
}

// ListenAddr is a version of quic.ListenAddr that overwrites the
// ConnectionID behavior to match the OSP zero-length requirement.
func ListenAddr(addr string, tlsConf *tls.Config, config *quic.Config) (*quic.Listener, error) {
	conn, err := listenUDP(addr)
	if err != nil {
		return nil, err
	}
	return (&quic.Transport{
		Conn:                  conn,
		ConnectionIDGenerator: &ospConnectionIDGenerator{},
	}).Listen(tlsConf, config)
}
