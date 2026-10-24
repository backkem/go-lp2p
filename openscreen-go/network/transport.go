package ospc

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
)

var ErrTransportClosed = errors.New("transport closed")
var ErrTransportHandedOff = errors.New("transport handed off")

func NewNetworkTransport(typ AgentTransport) (NetworkTransport, error) {
	return newAgentTransport(typ, KeepAliveConfig{}, nil)
}

func newAgentTransport(typ AgentTransport, keepAlive KeepAliveConfig, log *slog.Logger) (NetworkTransport, error) {
	switch typ {
	case AgentTransportQUIC:
		return &QuicTransport{KeepAlive: keepAlive, Logger: log}, nil

	case AgentTransportWebRTC:
		return &DTLSTransport{}, nil

	default:
		return nil, fmt.Errorf("unknown transport type: %d", typ)
	}
}

type NetworkTransport interface {
	DialAddr(ctx context.Context, addr string, tlsConf *tls.Config) (NetworkConnection, error)
	ListenAddr(addr string, tlsConf *tls.Config) (NetworkListener, error)
}

type NetworkListener interface {
	Accept(ctx context.Context) (NetworkConnection, error)
	Addr() net.Addr
	// Close stops listening. Accepted connections are not closed.
	Close() error
}

// Abstract connection for the network protocol, responsible for getting
// extended agent capabilities and performing the authentication ceremony.
type NetworkConnection interface {
	// Write sends a single serialized message.
	io.WriteCloser
	// ReadMessage returns the next received message. Transports with multiple
	// streams read them independently: ordering is only preserved for
	// messages within the same stream.
	ReadMessage() (interface{}, error)
	// Determines if this connection supports reliable delivery. If not, the
	// network protocol agent needs to perform timeout & retransmission.
	IsReliable() bool
	// Upgrades the connection to an application connection. Any proceeding
	// calls to the network connection will fail.
	IntoApplicationConnection() (ApplicationConnection, error)

	ConnectionState() tls.ConnectionState
}

// Abstract connection for the application protocol.
type ApplicationConnection interface {
	AcceptStream(context.Context) (ApplicationStream, error)
	OpenStreamSync(context.Context) (ApplicationStream, error)
	Close() error
}

// UniStreamOpener is implemented by application connections that support
// unidirectional streams, as used for OSP messages (application.bs). The
// returned stream can only be written to.
type UniStreamOpener interface {
	OpenUniStreamSync(context.Context) (ApplicationStream, error)
}

// Abstract stream for the application protocol.
type ApplicationStream interface {
	io.ReadWriteCloser
}

func listenUDP(addr string) (*net.UDPConn, error) {
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, err
	}
	return net.ListenUDP("udp", udpAddr)
}
