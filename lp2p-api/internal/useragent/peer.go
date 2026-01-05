package ua

import (
	"github.com/backkem/go-lp2p/openscreen-go/network"
)

// PeerContext represents the context of a local peer.
type PeerContext struct {
	m         *ConnectionManager
	LocalPeer *ospc.Agent
}

// OriginPeerGrant represents a peer grant to an origin.
type OriginPeerGrant struct {
	ID     ospc.PeerID
	Origin string
}

// GrantedConnection represents a connection to a remote peer
// that has been granted to an origin by the user agent.
type GrantedConnection struct {
	Conn *ospc.Connection
}
