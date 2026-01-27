package ospc

import "fmt"

////
// WIP messages not yet part of any CDDL
///

// Backup of old auth flow
// TODO: rework for w3c/openscreenprotocol/pull/294

const (
	// Transport Stream
	typeKeyAuthSpake2NeedPskDeprecated TypeKey = 99001
)

// auth-spake2-need-psk
type msgAuthSpake2NeedPskDeprecated struct {
	AuthInitiationToken string `cbor:"0,keyasint"`
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

	default:
		return 0, fmt.Errorf("unknown message type: %T", msg)
	}
}

// data-transport-start-request
type msgDataTransportStartRequest struct {
	RequestID  uint64 `cbor:"0,keyasint"`
	ExchangeId uint64 `cbor:"1,keyasint"`
}

// data-transport-start-response
type msgDataTransportStartResponse struct {
	RequestID uint64    `cbor:"0,keyasint"`
	Result    msgResult `cbor:"1,keyasint"`
}

// data-transport-stream-request
type msgDataTransportStreamRequest struct {
	RequestID  uint64 `cbor:"0,keyasint"`
	ExchangeId uint64 `cbor:"1,keyasint"`
}

// data-transport-stream-response
type msgDataTransportStreamResponse struct {
	RequestID uint64    `cbor:"0,keyasint"`
	Result    msgResult `cbor:"1,keyasint"`
}
