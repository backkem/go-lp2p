package ospc

import (
	"errors"
	"fmt"
)

// Messages of the Local Peer-to-Peer OSP binding (local-peer-to-peer
// index.bs, Appendix A) that cddlc doesn't generate: the session extension
// and the WebTransport extension. Limits the CDDL puts on their values are
// checked on encode and decode, see validate.

// DataEncoding represents pre-agreed encoding IDs used in data-frame
// messages of a data channel.
type DataEncoding uint64

const (
	DataEncodingBinary = iota
	DataEncodingString
	DataEncodingArrayBuffer
)

const (
	typeKeyLp2pSessionRequest         TypeKey = 1110
	typeKeyLp2pSessionResponse        TypeKey = 1111
	typeKeyDataTransportStartRequest  TypeKey = 1201
	typeKeyDataTransportStartResponse TypeKey = 1202
	typeKeyDataTransportStream        TypeKey = 1203
	// Type key 1204 is reserved.
	typeKeyDataTransportClose TypeKey = 1205
	typeKeyDataTransportDrain TypeKey = 1206
)

const (
	// MaxExchangeID is the largest exchange-id of a WebTransport session:
	// `exchange-id = uint .lt 4611686018427387904` (below 2^62).
	MaxExchangeID = 1<<62 - 1
	// MaxWebTransportCloseCode is the largest close-code of
	// data-transport-close: `uint .size 4`.
	MaxWebTransportCloseCode = 1<<32 - 1
	// MaxWebTransportReasonLen is the longest reason of data-transport-close
	// in bytes: `text .size (0..1024)`.
	MaxWebTransportReasonLen = 1024
	// sessionBindingLen is the size of data-transport-start-request's
	// session-binding: `bytes .size 32`.
	sessionBindingLen = 32
)

// ErrInvalidMessage is the cause of encode and decode errors for messages
// whose values are outside the limits their CDDL defines.
var ErrInvalidMessage = errors.New("invalid message")

// validator is implemented by messages with values to check beyond what CBOR
// decoding enforces.
type validator interface {
	validate() error
}

func validateMessage(msg interface{}) error {
	if v, ok := msg.(validator); ok {
		return v.validate()
	}
	return nil
}

func newMessageByTypeLP2P(key TypeKey) (interface{}, error) {
	switch key {
	case typeKeyLp2pSessionRequest:
		return &msgLp2pSessionRequest{}, nil
	case typeKeyLp2pSessionResponse:
		return &msgLp2pSessionResponse{}, nil
	case typeKeyDataTransportStartRequest:
		return &msgDataTransportStartRequest{}, nil
	case typeKeyDataTransportStartResponse:
		return &msgDataTransportStartResponse{}, nil
	case typeKeyDataTransportStream:
		return &msgDataTransportStream{}, nil
	case typeKeyDataTransportClose:
		return &msgDataTransportClose{}, nil
	case typeKeyDataTransportDrain:
		return &msgDataTransportDrain{}, nil
	default:
		return nil, fmt.Errorf("unknown type key: %d", key)
	}
}

func typeKeyByMessageLP2P(msg interface{}) (TypeKey, error) {
	switch msg.(type) {
	case *msgLp2pSessionRequest:
		return typeKeyLp2pSessionRequest, nil
	case *msgLp2pSessionResponse:
		return typeKeyLp2pSessionResponse, nil
	case *msgDataTransportStartRequest:
		return typeKeyDataTransportStartRequest, nil
	case *msgDataTransportStartResponse:
		return typeKeyDataTransportStartResponse, nil
	case *msgDataTransportStream:
		return typeKeyDataTransportStream, nil
	case *msgDataTransportClose:
		return typeKeyDataTransportClose, nil
	case *msgDataTransportDrain:
		return typeKeyDataTransportDrain, nil
	default:
		return 0, fmt.Errorf("unknown message type: %T", msg)
	}
}

// lp2p-session-request (type key 1110)
type msgLp2pSessionRequest struct {
	msgRequest
	RequestingOrigin string `cbor:"1,keyasint"`
}

// lp2p-session-response (type key 1111)
type msgLp2pSessionResponse struct {
	msgResponse
	Result msgLp2pSessionResult `cbor:"1,keyasint"`
}

// lp2p-session-result: the OSP result values plus authentication-required.
type msgLp2pSessionResult uint64

const (
	LP2PSessionResultSuccess                                     = msgLp2pSessionResult(ResultSuccess)
	LP2PSessionResultPermanentError                              = msgLp2pSessionResult(ResultPermanentError)
	LP2PSessionResultAuthenticationRequired msgLp2pSessionResult = 1110
)

// data-transport-start-request (type key 1201)
type msgDataTransportStartRequest struct {
	msgRequest
	ExchangeId     uint64 `cbor:"1,keyasint"`
	SessionBinding []byte `cbor:"2,keyasint,omitempty"`
}

func (m *msgDataTransportStartRequest) validate() error {
	if m.ExchangeId > MaxExchangeID {
		return fmt.Errorf("%w: exchange-id %d not below 2^62", ErrInvalidMessage, m.ExchangeId)
	}
	if m.SessionBinding != nil && len(m.SessionBinding) != sessionBindingLen {
		return fmt.Errorf("%w: session-binding is %d bytes, want %d", ErrInvalidMessage, len(m.SessionBinding), sessionBindingLen)
	}
	return nil
}

// data-transport-start-response (type key 1202)
type msgDataTransportStartResponse struct {
	msgResponse
	Result msgResult `cbor:"1,keyasint"`
}

// data-transport-stream (type key 1203)
type msgDataTransportStream struct {
	ExchangeId uint64 `cbor:"1,keyasint"`
}

func (m *msgDataTransportStream) validate() error {
	if m.ExchangeId > MaxExchangeID {
		return fmt.Errorf("%w: exchange-id %d not below 2^62", ErrInvalidMessage, m.ExchangeId)
	}
	return nil
}

// data-transport-close (type key 1205)
type msgDataTransportClose struct {
	CloseCode uint64 `cbor:"0,keyasint"`
	Reason    string `cbor:"1,keyasint"`
}

func (m *msgDataTransportClose) validate() error {
	if m.CloseCode > MaxWebTransportCloseCode {
		return fmt.Errorf("%w: close-code %d exceeds 2^32-1", ErrInvalidMessage, m.CloseCode)
	}
	if len(m.Reason) > MaxWebTransportReasonLen {
		return fmt.Errorf("%w: reason is %d bytes, at most %d allowed", ErrInvalidMessage, len(m.Reason), MaxWebTransportReasonLen)
	}
	return nil
}

// data-transport-drain (type key 1206)
type msgDataTransportDrain struct{}
