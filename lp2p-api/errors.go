package lp2p

import (
	"context"
	"errors"

	ua "github.com/backkem/go-lp2p/lp2p-api/useragent"
	ospc "github.com/backkem/go-lp2p/openscreen-go/network"
)

// DOMException names used by this package.
const (
	NotAllowedError   = "NotAllowedError"
	AbortError        = "AbortError"
	InvalidStateError = "InvalidStateError"
	NotSupportedError = "NotSupportedError"
	NetworkError      = "NetworkError"
	TimeoutError      = "TimeoutError"
	OperationError    = "OperationError"
	// TypeError stands for a JavaScript TypeError.
	TypeError = "TypeError"
)

// DOMException is the error a rejected promise or throwing method returns.
// Compare Name against the constants above, or use errors.Is with the
// underlying cause (e.g. ua.ErrPermissionDenied).
type DOMException struct {
	Name    string
	Message string
	Err     error // underlying cause, if any
}

func (e *DOMException) Error() string {
	if e.Message == "" {
		return e.Name
	}
	return e.Name + ": " + e.Message
}

func (e *DOMException) Unwrap() error { return e.Err }

func newDOMException(name, msg string, cause error) *DOMException {
	return &DOMException{Name: name, Message: msg, Err: cause}
}

// toDOMException maps user agent and transport errors onto DOMExceptions.
func toDOMException(err error) error {
	if err == nil {
		return nil
	}
	var de *DOMException
	if errors.As(err, &de) {
		return err
	}
	switch {
	case errors.Is(err, ua.ErrPermissionDenied):
		return newDOMException(NotAllowedError, "the user denied permission", err)
	case errors.Is(err, ua.ErrAuthenticationFailed):
		return newDOMException(NotAllowedError, "pairing with the peer failed", err)
	case errors.Is(err, ua.ErrCanceled), errors.Is(err, context.Canceled):
		return newDOMException(AbortError, "the user canceled the request", err)
	case errors.Is(err, context.DeadlineExceeded):
		return newDOMException(TimeoutError, "the request timed out", err)
	case errors.Is(err, ospc.ErrDataChannelIDOutOfRange):
		return newDOMException(TypeError, err.Error(), err)
	case errors.Is(err, ospc.ErrDataChannelIDInUse), errors.Is(err, ospc.ErrDataChannelIDParity):
		return newDOMException(OperationError, err.Error(), err)
	case errors.Is(err, ospc.ErrDataChannelRejected):
		return newDOMException(OperationError, err.Error(), err)
	case errors.Is(err, ua.ErrClosed):
		return newDOMException(InvalidStateError, "the user agent is closed", err)
	default:
		return newDOMException(NetworkError, err.Error(), err)
	}
}

func errClosed(what string) error {
	return newDOMException(InvalidStateError, what+" is closed", nil)
}
