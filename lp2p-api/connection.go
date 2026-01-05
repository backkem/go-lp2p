package lp2p

import (
	"github.com/backkem/go-lp2p/openscreen-go/network"
	"github.com/backkem/go-lp2p/web-api"
)

// LP2PConnection
type LP2PConnection struct {
	// TODO: Rethink attributes
	// PeerId   string
	// Nickname string

	conn *ospc.Connection

	OnDataChannel        web.CallbackSetter[OnDataChannelEvent]
	onDataChannelHandler *web.EventHandler[OnDataChannelEvent]
}

func newLP2PConnection(conn *ospc.Connection) *LP2PConnection {

	onDataChannelHandler := web.NewEventHandler[OnDataChannelEvent]()
	return &LP2PConnection{
		conn: conn,

		OnDataChannel:        onDataChannelHandler.SetCallback,
		onDataChannelHandler: onDataChannelHandler,
	}
}

type OnConnectionEvent struct {
	Connection *LP2PConnection
}
