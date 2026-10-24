package cast

import (
	"encoding/json"
	"strings"
)

// Namespaces and well-known endpoint ids, from openscreen
// cast/common/channel/message_util.h.
const (
	CastInternalNamespacePrefix = "urn:x-cast:com.google.cast."
	TransportNamespacePrefix    = "urn:x-cast:com.google.cast.tp."

	AuthNamespace       = "urn:x-cast:com.google.cast.tp.deviceauth"
	HeartbeatNamespace  = "urn:x-cast:com.google.cast.tp.heartbeat"
	ConnectionNamespace = "urn:x-cast:com.google.cast.tp.connection"
	ReceiverNamespace   = "urn:x-cast:com.google.cast.receiver"
	BroadcastNamespace  = "urn:x-cast:com.google.cast.broadcast"
	MediaNamespace      = "urn:x-cast:com.google.cast.media"

	// PlatformSenderID and PlatformReceiverID are the default endpoint ids
	// of the sender and receiver platforms.
	PlatformSenderID   = "sender-0"
	PlatformReceiverID = "receiver-0"
	// BroadcastID addresses every endpoint.
	BroadcastID = "*"
)

// IsTransportNamespace reports whether ns is a transport ("tp.") namespace.
// Transport messages are routed without an established virtual connection.
func IsTransportNamespace(ns string) bool {
	return strings.HasPrefix(ns, TransportNamespacePrefix)
}

// JSON message types used on the transport namespaces.
const (
	MessageTypeConnect   = "CONNECT"
	MessageTypeClose     = "CLOSE"
	MessageTypeConnected = "CONNECTED"
	MessageTypePing      = "PING"
	MessageTypePong      = "PONG"
)

// ConnectionType is VirtualConnection::Type.
type ConnectionType int

const (
	ConnectionStrong    ConnectionType = 0
	ConnectionWeak      ConnectionType = 1
	ConnectionInvisible ConnectionType = 2
)

// CloseReason is VirtualConnection::CloseReason; it is carried as
// "reasonCode" in CLOSE messages.
type CloseReason int

const (
	CloseUnknown CloseReason = iota
	CloseTransportClosed
	CloseTransportAborted
	CloseTransportInvalidMessage
	CloseTransportTooLongInactive
	CloseClosedBySelf
	CloseClosedByPeer
)

func (r CloseReason) String() string {
	switch r {
	case CloseTransportClosed:
		return "transport closed"
	case CloseTransportAborted:
		return "transport aborted"
	case CloseTransportInvalidMessage:
		return "transport invalid message"
	case CloseTransportTooLongInactive:
		return "transport too long inactive"
	case CloseClosedBySelf:
		return "closed by self"
	case CloseClosedByPeer:
		return "closed by peer"
	default:
		return "unknown"
	}
}

// UserAgent is the userAgent / version string openscreen reports.
const UserAgent = "Unknown (Open Screen)"

// SenderInfo is the "senderInfo" object of a CONNECT message. The "skdType"
// key spelling is intentional: it matches openscreen's kMessageKeySdkType.
type SenderInfo struct {
	SdkType        int    `json:"skdType"`
	Version        string `json:"version"`
	BrowserVersion string `json:"browserVersion"`
	Platform       int    `json:"platform"`
	ConnectionType int    `json:"connectionType"`
}

// ConnectMessage is the payload of a CONNECT request.
type ConnectMessage struct {
	Type                string          `json:"type"`
	ProtocolVersion     *int            `json:"protocolVersion,omitempty"`
	ProtocolVersionList []int           `json:"protocolVersionList,omitempty"`
	UserAgent           string          `json:"userAgent,omitempty"`
	ConnType            *ConnectionType `json:"connType,omitempty"`
	Origin              *struct{}       `json:"origin,omitempty"`
	SenderInfo          *SenderInfo     `json:"senderInfo,omitempty"`
}

// connectedMessage is the payload of a CONNECTED response.
type connectedMessage struct {
	Type            string `json:"type"`
	ProtocolVersion int    `json:"protocolVersion"`
}

// closeMessage is the payload of a CLOSE message.
type closeMessage struct {
	Type       string       `json:"type"`
	ReasonCode *CloseReason `json:"reasonCode,omitempty"`
}

type typedMessage struct {
	Type string `json:"type"`
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

// MakeConnectMessage builds a CONNECT request as openscreen's
// MakeConnectMessage does.
func MakeConnectMessage(source, destination string) *CastMessage {
	strong := ConnectionStrong
	payload := ConnectMessage{
		Type:                MessageTypeConnect,
		ProtocolVersionList: []int{int(CastV2_1_0), int(CastV2_1_1), int(CastV2_1_2), int(CastV2_1_3)},
		UserAgent:           UserAgent,
		ConnType:            &strong,
		Origin:              &struct{}{},
		SenderInfo: &SenderInfo{
			SdkType:        2,
			Version:        UserAgent,
			BrowserVersion: UserAgent,
			Platform:       6, // Linux
			ConnectionType: 1, // LOCAL
		},
	}
	return NewStringMessage(source, destination, ConnectionNamespace, mustJSON(payload))
}

// MakeCloseMessage builds a CLOSE message. openscreen omits the reason code
// for self-initiated closes; pass CloseUnknown to omit it as well.
func MakeCloseMessage(source, destination string, reason CloseReason) *CastMessage {
	m := closeMessage{Type: MessageTypeClose}
	if reason != CloseUnknown {
		m.ReasonCode = &reason
	}
	return NewStringMessage(source, destination, ConnectionNamespace, mustJSON(m))
}

func makeTypedMessage(source, destination, ns, typ string) *CastMessage {
	return NewStringMessage(source, destination, ns, mustJSON(typedMessage{Type: typ}))
}

// maxProtocolVersion mirrors FindMaxProtocolVersion: it returns the highest
// valid version in protocolVersion / protocolVersionList, or ok=false when
// the sender did not advertise any version.
func (m *ConnectMessage) maxProtocolVersion() (int, bool) {
	valid := func(v int) bool { return v >= int(CastV2_1_0) && v <= int(CastV2_1_3) }
	found, max := false, int(CastV2_1_0)
	if m.ProtocolVersion != nil {
		found = true
		if valid(*m.ProtocolVersion) && *m.ProtocolVersion > max {
			max = *m.ProtocolVersion
		}
	}
	for _, v := range m.ProtocolVersionList {
		found = true
		if valid(v) && v > max {
			max = v
		}
	}
	return max, found
}
