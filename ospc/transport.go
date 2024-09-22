package ospc

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
)

var ErrTransportClosed = errors.New("transport closed")
var ErrTransportHandedOff = errors.New("transport handed off")

func NewNetworkTransport(typ AgentTransport) (NetworkTransport, error) {
	switch typ {
	case AgentTransportQUIC:
		return &QuicTransport{}, nil
	default:
		return nil, fmt.Errorf("unknown transport type: %T", typ)
	}
}

type NetworkTransport interface {
	DialAddr(ctx context.Context, addr string, tlsConf *tls.Config) (NetworkConnection, error)
	ListenAddr(addr string, tlsConf *tls.Config) (NetworkListener, error)
}

type NetworkListener interface {
	Accept(ctx context.Context) (NetworkConnection, error)
	Addr() net.Addr
}

// Abstract connection for the network protocol, responsible for getting
// extended agent capabilities and performing the authentication ceremony.
type NetworkConnection interface {
	io.ReadWriteCloser
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

// Abstract stream for the application protocol.
type ApplicationStream interface {
	io.ReadWriteCloser
}
