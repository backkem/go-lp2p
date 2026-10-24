package application

import (
	"fmt"

	ospc "github.com/backkem/go-lp2p/openscreen-go/network"
)

// Presentation protocol messages, application.bs "Presentation Protocol",
// with the CDDL of application_messages.cddl. This is the message set only:
// there is no presentation state machine here. Send them with
// ApplicationConnection.SendMessage / Request and decode received ones with
// Message.Decode.
//
// Struct fields are in ascending key order; the CBOR encoding of ospc sorts
// map keys anyway (RFC 8949 §4.2.1), which Chromium's openscreen requires.

// Type keys of the presentation protocol messages (application.bs appendix).
const (
	TypeKeyPresentationURLAvailabilityRequest  ospc.TypeKey = 14
	TypeKeyPresentationURLAvailabilityResponse ospc.TypeKey = 15
	TypeKeyPresentationConnectionMessage       ospc.TypeKey = 16
	TypeKeyPresentationURLAvailabilityEvent    ospc.TypeKey = 103
	TypeKeyPresentationStartRequest            ospc.TypeKey = 104
	TypeKeyPresentationStartResponse           ospc.TypeKey = 105
	TypeKeyPresentationTerminationRequest      ospc.TypeKey = 106
	TypeKeyPresentationTerminationResponse     ospc.TypeKey = 107
	TypeKeyPresentationTerminationEvent        ospc.TypeKey = 108
	TypeKeyPresentationConnectionOpenRequest   ospc.TypeKey = 109
	TypeKeyPresentationConnectionOpenResponse  ospc.TypeKey = 110
	TypeKeyPresentationConnectionCloseEvent    ospc.TypeKey = 113
	TypeKeyPresentationChangeEvent             ospc.TypeKey = 121
)

// MessageName returns the CDDL name of a presentation protocol or
// agent-info type key, or "type-key <n>" for any other.
func MessageName(k ospc.TypeKey) string {
	switch k {
	case TypeKeyPresentationURLAvailabilityRequest:
		return "presentation-url-availability-request"
	case TypeKeyPresentationURLAvailabilityResponse:
		return "presentation-url-availability-response"
	case TypeKeyPresentationConnectionMessage:
		return "presentation-connection-message"
	case TypeKeyPresentationURLAvailabilityEvent:
		return "presentation-url-availability-event"
	case TypeKeyPresentationStartRequest:
		return "presentation-start-request"
	case TypeKeyPresentationStartResponse:
		return "presentation-start-response"
	case TypeKeyPresentationTerminationRequest:
		return "presentation-termination-request"
	case TypeKeyPresentationTerminationResponse:
		return "presentation-termination-response"
	case TypeKeyPresentationTerminationEvent:
		return "presentation-termination-event"
	case TypeKeyPresentationConnectionOpenRequest:
		return "presentation-connection-open-request"
	case TypeKeyPresentationConnectionOpenResponse:
		return "presentation-connection-open-response"
	case TypeKeyPresentationConnectionCloseEvent:
		return "presentation-connection-close-event"
	case TypeKeyPresentationChangeEvent:
		return "presentation-change-event"
	case ospc.TypeKeyAgentInfoRequest:
		return "agent-info-request"
	case ospc.TypeKeyAgentInfoResponse:
		return "agent-info-response"
	}
	return fmt.Sprintf("type-key %d", k)
}

// URLAvailability is url-availability.
type URLAvailability uint64

const (
	URLAvailable   URLAvailability = 0
	URLUnavailable URLAvailability = 1
	URLInvalid     URLAvailability = 10
)

// Result is result, used by the start, termination and connection-open
// responses.
type Result uint64

const (
	ResultSuccess               Result = 1
	ResultInvalidURL            Result = 10
	ResultInvalidPresentationID Result = 11
	ResultTimeout               Result = 100
	ResultTransientError        Result = 101
	ResultPermanentError        Result = 102
	ResultTerminating           Result = 103
	ResultUnknownError          Result = 199
)

// TerminationSource is presentation-termination-source.
type TerminationSource uint64

const (
	TerminationSourceController TerminationSource = 1
	TerminationSourceReceiver   TerminationSource = 2
	TerminationSourceUnknown    TerminationSource = 255
)

// TerminationReason is presentation-termination-reason. A
// presentation-termination-request only carries application-request or
// user-request.
type TerminationReason uint64

const (
	TerminationReasonApplicationRequest           TerminationReason = 1
	TerminationReasonUserRequest                  TerminationReason = 2
	TerminationReasonReceiverReplacedPresentation TerminationReason = 20
	TerminationReasonReceiverIdleTooLong          TerminationReason = 30
	TerminationReasonReceiverAttemptedToNavigate  TerminationReason = 31
	TerminationReasonReceiverPoweringDown         TerminationReason = 100
	TerminationReasonReceiverError                TerminationReason = 101
	TerminationReasonUnknown                      TerminationReason = 255
)

// ConnectionCloseReason is the reason of a presentation-connection-close-event.
type ConnectionCloseReason uint64

const (
	ConnectionCloseMethodCalled       ConnectionCloseReason = 1
	ConnectionCloseObjectDiscarded    ConnectionCloseReason = 10
	ConnectionCloseUnrecoverableError ConnectionCloseReason = 100
)

// PresentationURLAvailabilityRequest is presentation-url-availability-request
// (type key 14).
type PresentationURLAvailabilityRequest struct {
	RequestID ospc.RequestID `cbor:"0,keyasint"`
	URLs      []string       `cbor:"1,keyasint"` // [1* text]
	// WatchDuration is in microseconds.
	WatchDuration uint64 `cbor:"2,keyasint"`
	WatchID       uint64 `cbor:"3,keyasint"`
}

// PresentationURLAvailabilityResponse is
// presentation-url-availability-response (type key 15). Availabilities
// correspond to the request's URLs by index.
type PresentationURLAvailabilityResponse struct {
	RequestID         ospc.RequestID    `cbor:"0,keyasint"`
	URLAvailabilities []URLAvailability `cbor:"1,keyasint"` // [1* url-availability]
}

// PresentationURLAvailabilityEvent is presentation-url-availability-event
// (type key 103).
type PresentationURLAvailabilityEvent struct {
	WatchID           uint64            `cbor:"0,keyasint"`
	URLAvailabilities []URLAvailability `cbor:"1,keyasint"`
}

// HTTPHeader is http-header, a [key, value] array.
type HTTPHeader struct {
	_     struct{} `cbor:",toarray"`
	Key   string
	Value string
}

// PresentationStartRequest is presentation-start-request (type key 104).
// PresentationID must satisfy ValidatePresentationID.
type PresentationStartRequest struct {
	RequestID      ospc.RequestID `cbor:"0,keyasint"`
	PresentationID string         `cbor:"1,keyasint"`
	URL            string         `cbor:"2,keyasint"`
	Headers        []HTTPHeader   `cbor:"3,keyasint"` // [* http-header], encoded as [] when nil
}

// PresentationStartResponse is presentation-start-response (type key 105).
type PresentationStartResponse struct {
	RequestID        ospc.RequestID `cbor:"0,keyasint"`
	Result           Result         `cbor:"1,keyasint"`
	ConnectionID     uint64         `cbor:"2,keyasint"`
	HTTPResponseCode *uint64        `cbor:"3,keyasint,omitempty"`
}

// PresentationTerminationRequest is presentation-termination-request (type
// key 106).
type PresentationTerminationRequest struct {
	RequestID      ospc.RequestID    `cbor:"0,keyasint"`
	PresentationID string            `cbor:"1,keyasint"`
	Reason         TerminationReason `cbor:"2,keyasint"`
}

// PresentationTerminationResponse is presentation-termination-response (type
// key 107).
type PresentationTerminationResponse struct {
	RequestID ospc.RequestID `cbor:"0,keyasint"`
	Result    Result         `cbor:"1,keyasint"`
}

// PresentationTerminationEvent is presentation-termination-event (type key
// 108).
type PresentationTerminationEvent struct {
	PresentationID string            `cbor:"0,keyasint"`
	Source         TerminationSource `cbor:"1,keyasint"`
	Reason         TerminationReason `cbor:"2,keyasint"`
}

// PresentationConnectionOpenRequest is presentation-connection-open-request
// (type key 109).
type PresentationConnectionOpenRequest struct {
	RequestID      ospc.RequestID `cbor:"0,keyasint"`
	PresentationID string         `cbor:"1,keyasint"`
	URL            string         `cbor:"2,keyasint"`
}

// PresentationConnectionOpenResponse is presentation-connection-open-response
// (type key 110).
type PresentationConnectionOpenResponse struct {
	RequestID       ospc.RequestID `cbor:"0,keyasint"`
	Result          Result         `cbor:"1,keyasint"`
	ConnectionID    uint64         `cbor:"2,keyasint"`
	ConnectionCount uint64         `cbor:"3,keyasint"`
}

// PresentationConnectionCloseEvent is presentation-connection-close-event
// (type key 113).
type PresentationConnectionCloseEvent struct {
	ConnectionID    uint64                `cbor:"0,keyasint"`
	Reason          ConnectionCloseReason `cbor:"1,keyasint"`
	ErrorMessage    *string               `cbor:"2,keyasint,omitempty"`
	ConnectionCount uint64                `cbor:"3,keyasint"`
}

// PresentationChangeEvent is presentation-change-event (type key 121).
type PresentationChangeEvent struct {
	PresentationID  string `cbor:"0,keyasint"`
	ConnectionCount uint64 `cbor:"1,keyasint"`
}

// PresentationConnectionMessage is presentation-connection-message (type key
// 16). Message is "bytes / text": a string is sent as a CBOR text string (a
// Presentation API string message), a []byte as a byte string (binary).
// Decoded messages hold a string or a []byte accordingly.
type PresentationConnectionMessage struct {
	ConnectionID uint64 `cbor:"0,keyasint"`
	Message      any    `cbor:"1,keyasint"`
}
