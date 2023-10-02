package ospc

import (
	"fmt"
	"io"

	"github.com/ugorji/go/codec"
)

type TypeKey uint64

// type Message interface {
// 	TypeKey() TypeKey
// }

const (
	// Metadata
	typeKeyAgentInfoRequest    TypeKey = 10
	typeKeyAgentInfoResponse   TypeKey = 11
	typeKeyAgentStatusRequest  TypeKey = 12
	typeKeyAgentStatusResponse TypeKey = 13

	// typeKeyPresentationUrlAvailabilityRequest TypeKey = 14
	// typeKeyPresentationUrlAvailabilityResponse TypeKey = 15
	// typeKeyPresentationConnectionMessage TypeKey = 16

	// typeKeyRemotePlaybackAvailabilityRequest TypeKey = 17
	// typeKeyRemotePlaybackAvailabilityResponse TypeKey = 18
	// typeKeyRemotePlaybackModifyRequest TypeKey = 19
	// typeKeyRemotePlaybackModifyResponse TypeKey = 20
	// typeKeyRemotePlaybackStateEvent TypeKey = 21

	// typeKeyAudioFrame TypeKey = 22

	// typeKeyVideoFrame TypeKey = 23

	typeKeyDataFrame TypeKey = 24

	// typeKeyPresentationUrlAvailabilityEvent TypeKey = 103
	// typeKeyPresentationStartRequest TypeKey = 104
	// typeKeyPresentationStartResponse TypeKey = 105
	// typeKeyPresentationTerminationRequest TypeKey = 106
	// typeKeyPresentationTerminationResponse TypeKey = 107
	// typeKeyPresentationTerminationEvent TypeKey = 108
	// typeKeyPresentationConnectionOpenRequest TypeKey = 109
	// typeKeyPresentationConnectionOpenResponse TypeKey = 110
	// typeKeyPresentationConnectionCloseEvent TypeKey = 113

	// Remote playback
	// typeKeyRemotePlaybackAvailabilityEvent TypeKey = 114
	// typeKeyRemotePlaybackStartRequest TypeKey = 115
	// typeKeyRemotePlaybackStartResponse TypeKey = 116
	// typeKeyRemotePlaybackTerminationRequest TypeKey = 117
	// typeKeyRemotePlaybackTerminationResponse TypeKey = 118
	// typeKeyRemotePlaybackTerminationEvent TypeKey = 119

	// typeKeyPresentationChangeEvent TypeKey = 121

	// Agent info event
	typeKeyAgentInfoEvent TypeKey = 122

	// StreamingCapabilities
	// typeKeyStreamingCapabilitiesRequest TypeKey = 122
	// typeKeyStreamingCapabilitiesResponse TypeKey = 123

	// StreamingSession
	// typeKeyStreamingSessionStartRequest TypeKey = 124
	// typeKeyStreamingSessionStartResponse TypeKey = 125
	// typeKeyStreamingSessionModifyRequest TypeKey = 126
	// typeKeyStreamingSessionModifyResponse TypeKey = 127
	// typeKeyStreamingSessionTerminateRequest TypeKey = 128
	// typeKeyStreamingSessionTerminateResponse TypeKey = 129
	// typeKeyStreamingSessionTerminateEvent TypeKey = 130
	// typeKeyStreamingSessionSenderStatsEvent TypeKey = 131
	// typeKeyStreamingSessionReceiverStatsEvent TypeKey = 132

	// Auth
	typeKeyAuthCapabilities       TypeKey = 1001
	typeKeyAuthSpake2NeedPsk      TypeKey = 1002
	typeKeyAuthSpake2Handshake    TypeKey = 1003
	typeKeyAuthSpake2Confirmation TypeKey = 1004
	typeKeyAuthStatus             TypeKey = 1005

	// DataExchange
	typeKeyDataExchangeStartRequest  TypeKey = 1101
	typeKeyDataExchangeStartResponse TypeKey = 1102
)

func readTypeKey(r io.Reader) (TypeKey, error) {
	i, err := readVaruint(r)
	return TypeKey(i), err
}

func writeTypeKey(v TypeKey, w io.Writer) error {
	return writeVaruint(uint64(v), w)
}

func readMessage(r io.Reader) (interface{}, error) {
	// r = newDebugReadWriter(r)
	typeKey, err := readTypeKey(r)
	if err != nil {
		return nil, err
	}

	// fmt.Println("Got typeKey", typeKey)

	msg, err := newMessageByType(typeKey)
	if err != nil {
		return nil, err
	}

	h := &codec.CborHandle{}
	dec := codec.NewDecoder(r, h)
	err = dec.Decode(&msg)
	if err != nil {
		return nil, err
	}

	// fmt.Printf("<-- Read %T\n", msg)
	return msg, nil

}

func newMessageByType(key TypeKey) (interface{}, error) {
	switch key {
	case typeKeyAgentInfoRequest:
		return &msgAgentInfoRequest{}, nil

	case typeKeyAgentInfoResponse:
		return &msgAgentInfoResponse{}, nil

	case typeKeyDataFrame:
		return &msgDataFrame{}, nil

	case typeKeyAuthCapabilities:
		return &msgAuthCapabilities{}, nil

	case typeKeyAuthSpake2NeedPsk:
		return &msgAuthSpake2NeedPsk{}, nil

	case typeKeyAuthSpake2Handshake:
		return &msgAuthSpake2Handshake{}, nil

	case typeKeyAuthSpake2Confirmation:
		return &msgAuthSpake2Confirmation{}, nil

	case typeKeyAuthStatus:
		return &msgAuthStatus{}, nil

	case typeKeyDataExchangeStartRequest:
		return &msgDataExchangeStartRequest{}, nil

	case typeKeyDataExchangeStartResponse:
		return &msgDataExchangeStartResponse{}, nil

	default:
		return nil, fmt.Errorf("unknown type key: %d", key)
	}
}

// type debugReadWriter struct {
// 	child interface{}
// }
//
// func newDebugReadWriter(child interface{}) debugReadWriter {
// 	return debugReadWriter{
// 		child: child,
// 	}
// }
//
// func (d debugReadWriter) print(fn string, n int, err error, b []byte) {
// 	fmt.Printf("--> %s %d bytes: %#x\n", fn, n, b)
// }
//
// func (d debugReadWriter) Read(p []byte) (int, error) {
// 	// fmt.Println("   -start-read-")
// 	r, ok := d.child.(io.Reader)
// 	if !ok {
// 		return 0, errors.New("not a reader")
// 	}
// 	n, err := r.Read(p)
// 	d.print("Read", n, err, p)
// 	return n, err
// }
//
// func (d debugReadWriter) Write(p []byte) (int, error) {
// 	// fmt.Println("   -start-write-")
// 	r, ok := d.child.(io.Writer)
// 	if !ok {
// 		return 0, errors.New("not a writer")
// 	}
// 	n, err := r.Write(p)
// 	d.print("Write", n, err, p)
// 	return n, err
// }

// Write with type/size prefix
func writeMessage(msg interface{}, w io.Writer) error {
	// fmt.Printf("  --> Writing %T\n", msg)
	// defer fmt.Printf("  --> Done writing %T\n", msg)

	// w = newDebugReadWriter(w)
	tKey, err := typeKeyByMessage(msg)
	if err != nil {
		return err
	}

	err = writeTypeKey(tKey, w)
	if err != nil {
		return err
	}
	h := &codec.CborHandle{}
	enc := codec.NewEncoder(w, h)
	return enc.Encode(msg)
}

func typeKeyByMessage(msg interface{}) (TypeKey, error) {
	switch msg.(type) {
	case *msgAgentInfoRequest:
		return typeKeyAgentInfoRequest, nil

	case *msgAgentInfoResponse:
		return typeKeyAgentInfoResponse, nil

	case *msgDataFrame:
		return typeKeyDataFrame, nil

	case *msgAuthCapabilities:
		return typeKeyAuthCapabilities, nil

	case *msgAuthSpake2NeedPsk:
		return typeKeyAuthSpake2NeedPsk, nil

	case *msgAuthSpake2Handshake:
		return typeKeyAuthSpake2Handshake, nil

	case *msgAuthSpake2Confirmation:
		return typeKeyAuthSpake2Confirmation, nil

	case *msgAuthStatus:
		return typeKeyAuthStatus, nil

	case *msgDataExchangeStartRequest:
		return typeKeyDataExchangeStartRequest, nil

	case *msgDataExchangeStartResponse:
		return typeKeyDataExchangeStartResponse, nil

	default:
		return 0, fmt.Errorf("unknown message type: %T", msg)
	}
}

// agent-info-request
type msgAgentInfoRequest struct {
	RequestID uint64 `codec:"0"`
}

// agent-info-response
type msgAgentInfoResponse struct {
	RequestID uint64           `codec:"0"`
	AgentInfo msgPartAgentInfo `codec:"1"`
}

// agent-info
type msgPartAgentInfo struct {
	DisplayName  string            `codec:"0"`
	ModelName    string            `codec:"1"`
	Capabilities []agentCapability `codec:"2"`
	StateToken   string            `codec:"3"`
	Locales      []string          `codec:"4"`
}

type agentCapability = uint64

const (
	// agentCapabilityReceiveAudio          = 1
	// agentCapabilityReceiveVideo          = 2
	// agentCapabilityReceivePresentation   = 3
	// agentCapabilityControlPresentation   = 4
	// agentCapabilityReceiveRemotePlayback = 5
	// agentCapabilityControlRemotePlayback = 6
	// agentCapabilityReceiveStreaming      = 7
	// agentCapabilitySendStreaming         = 8

	agentCapabilityExchangeData = 10 // TODO: register
)

// data-frame
type msgDataFrame struct {
	EncodingId DataEncoding `codec:"0"`
	// SequenceNumber *uint64 `codec:"1"`
	// StartTime *uint64 `codec:"2"`
	// Duration *uint64 `codec:"3"`
	Payload []byte `codec:"4"` // TODO: any?
	// SyncTime *MediaTime `codec:"5"`
}

type msgPskInputMethod uint64

const (
	msgPskInputMethodNumeric msgPskInputMethod = 0
	msgPskInputMethodQrCode  msgPskInputMethod = 1
)

// auth-capabilities
type msgAuthCapabilities struct {
	PskEaseOfInput      uint64              `codec:"0"`
	PskInputMethods     []msgPskInputMethod `codec:"1"`
	PskMinBitsOfEntropy uint64              `codec:"2"`
}

// auth-spake2-need-psk
type msgAuthSpake2NeedPsk struct {
	AuthInitiationToken string `codec:"0"`
}

// auth-spake2-handshake
type msgAuthSpake2Handshake struct {
	AuthInitiationToken string `codec:"0"`
	Payload             []byte `codec:"1"`
}

// auth-spake2-confirmation
type msgAuthSpake2Confirmation struct {
	Payload []byte `codec:"1"`
}

// auth-status
type msgAuthStatus struct {
	Result msgResult `codec:"1"`
}

type msgResult uint64

const (
	msgResultSuccess               msgResult = 1
	msgResultInvalidUrl            msgResult = 10
	msgResultInvalidPresentationId msgResult = 11
	msgResultTimeout               msgResult = 100
	msgResultTransientError        msgResult = 101
	msgResultPermanentError        msgResult = 102
	msgResultTerminating           msgResult = 103
	msgResultUnknownError          msgResult = 199
)

///
// Below is a non-standard (yet) message for initiating exchange-data.
///

// DataEncoding represents pre-agreed EncodingIds used in exchange-data.
type DataEncoding uint64

const (
	DataEncodingBinary = iota
	DataEncodingString
	DataEncodingArrayBuffer
)

// data-exchange-start-request
type msgDataExchangeStartRequest struct {
	RequestID  uint64 `codec:"0"`
	ExchangeId uint64 `codec:"1"`
	Label      string `codec:"2"`
	Protocol   string `codec:"3"`
}

// data-exchange-start-response
type msgDataExchangeStartResponse struct {
	RequestID uint64    `codec:"0"`
	Result    msgResult `codec:"1"`
}
