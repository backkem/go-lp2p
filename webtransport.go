package lp2p

import (
	"errors"

	"github.com/quic-go/quic-go"
)

type LP2PQuicTransport struct {
	source *LP2PConnection
	conn   quic.Connection
	// TODO: WebTransport API
}

// TODO: Rethink API vs DataChannel
func NewLP2PQuicTransport(source *LP2PConnection) *LP2PQuicTransport {
	return &LP2PQuicTransport{
		source: source,
	}
}

// Start the LP2PQuicTransport
func (t *LP2PQuicTransport) Start() error {
	// TODO: Upgrade to WebTransport
	return errors.New("todo")
}