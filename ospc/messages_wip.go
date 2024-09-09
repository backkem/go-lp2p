package ospc

import "fmt"

////
// WIP messages not yet part of any CDDL
///

// Backup of old auth flow
// TODO: rework for w3c/openscreenprotocol/pull/294

const (
	// Transport Stream
	typeKeyAuthSpake2NeedPskDeprecated      TypeKey = 99001
	typeKeyAuthSpake2HandshakeDeprecated    TypeKey = 99002
	typeKeyAuthSpake2ConfirmationDeprecated TypeKey = 99003
)

// auth-spake2-need-psk
type msgAuthSpake2NeedPskDeprecated struct {
	AuthInitiationToken string `codec:"0"`
}

// auth-spake2-handshake
type msgAuthSpake2HandshakeDeprecated struct {
	AuthInitiationToken string `codec:"0"`
	Payload             []byte `codec:"1"`
}

// auth-spake2-confirmation
type msgAuthSpake2ConfirmationDeprecated struct {
	Payload []byte `codec:"1"`
}

// DataChannel

// DataEncoding represents pre-agreed EncodingIds used in exchange-data.
type DataEncoding uint64

const (
	DataEncodingBinary = iota
	DataEncodingString
	DataEncodingArrayBuffer
)

// WebTransport Pooled

const (
	// Transport Stream
	typeKeyDataTransportStartRequest   TypeKey = 1201
	typeKeyDataTransportStartResponse  TypeKey = 1202
	typeKeyDataTransportStreamRequest  TypeKey = 1203
	typeKeyDataTransportStreamResponse TypeKey = 1204
)

func newMessageByTypeWIP(key TypeKey) (interface{}, error) {
	switch key {
	case typeKeyDataTransportStartRequest:
		return &msgDataTransportStartRequest{}, nil

	case typeKeyDataTransportStartResponse:
		return &msgDataTransportStartResponse{}, nil

	case typeKeyDataTransportStreamRequest:
		return &msgDataTransportStreamRequest{}, nil

	case typeKeyDataTransportStreamResponse:
		return &msgDataTransportStreamResponse{}, nil

	case typeKeyAuthSpake2NeedPskDeprecated:
		return &msgAuthSpake2NeedPskDeprecated{}, nil

	case typeKeyAuthSpake2HandshakeDeprecated:
		return &msgAuthSpake2HandshakeDeprecated{}, nil

	case typeKeyAuthSpake2ConfirmationDeprecated:
		return &msgAuthSpake2ConfirmationDeprecated{}, nil

	default:
		return nil, fmt.Errorf("unknown type key: %d", key)
	}
}

func typeKeyByMessageWIP(msg interface{}) (TypeKey, error) {
	switch msg.(type) {
	case *msgDataTransportStartRequest:
		return typeKeyDataTransportStartRequest, nil

	case *msgDataTransportStartResponse:
		return typeKeyDataTransportStartResponse, nil

	case *msgDataTransportStreamRequest:
		return typeKeyDataTransportStreamRequest, nil

	case *msgDataTransportStreamResponse:
		return typeKeyDataTransportStreamResponse, nil

	case *msgAuthSpake2NeedPskDeprecated:
		return typeKeyAuthSpake2NeedPskDeprecated, nil

	case *msgAuthSpake2HandshakeDeprecated:
		return typeKeyAuthSpake2HandshakeDeprecated, nil

	case *msgAuthSpake2ConfirmationDeprecated:
		return typeKeyAuthSpake2ConfirmationDeprecated, nil

	default:
		return 0, fmt.Errorf("unknown message type: %T", msg)
	}
}

// data-transport-start-request
type msgDataTransportStartRequest struct {
	RequestID  uint64 `codec:"0"`
	ExchangeId uint64 `codec:"1"`
}

// data-transport-start-response
type msgDataTransportStartResponse struct {
	RequestID uint64    `codec:"0"`
	Result    msgResult `codec:"1"`
}

// data-transport-stream-request
type msgDataTransportStreamRequest struct {
	RequestID  uint64 `codec:"0"`
	ExchangeId uint64 `codec:"1"`
}

// data-transport-stream-response
type msgDataTransportStreamResponse struct {
	RequestID uint64    `codec:"0"`
	Result    msgResult `codec:"1"`
}
