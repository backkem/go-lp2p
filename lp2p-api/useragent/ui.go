package ua

import (
	"context"
	"fmt"
)

// UI is the browser chrome: everything the user agent shows to, or asks
// of, the user. Implementations must be safe for concurrent use; the user
// agent may prompt for several connections at once.
//
// Every method should return promptly once ctx is done.
type UI interface {
	// RequestPermission asks the user to allow what req describes.
	RequestPermission(ctx context.Context, req PermissionRequest) (bool, error)

	// PickPeer shows the device picker and returns the peer the user chose,
	// or ErrCanceled if they dismissed it. The picker's list is live: peers
	// come, go and change while it is shown, see Picker.Watch.
	PickPeer(ctx context.Context, p *Picker) (Peer, error)

	// PresentPIN shows the PIN the user must enter on the other device. It
	// should return once the PIN is shown, not when it is entered.
	PresentPIN(ctx context.Context, p PINPresentation) error

	// EnterPIN asks the user for the PIN shown on the other device.
	EnterPIN(ctx context.Context, p PINPrompt) (string, error)
}

// Permission is something a page can ask the user to allow.
type Permission int

const (
	// PermissionAdvertise lets a page make the device discoverable under
	// a nickname (LP2PReceiver.start).
	PermissionAdvertise Permission = iota + 1
	// PermissionAcceptConnection lets a page receive a connection from a
	// specific peer.
	PermissionAcceptConnection
)

func (p Permission) String() string {
	switch p {
	case PermissionAdvertise:
		return "advertise"
	case PermissionAcceptConnection:
		return "accept-connection"
	default:
		return fmt.Sprintf("Permission(%d)", int(p))
	}
}

// PermissionRequest describes a permission prompt.
type PermissionRequest struct {
	Permission Permission
	// Origin of the page asking.
	Origin string
	// Nickname the page uses for itself.
	Nickname string
	// Peer is the remote peer, for PermissionAcceptConnection. With the
	// session extension it is authenticated by then, and its name verified
	// where possible; without, its name is not verified yet.
	Peer Peer
	// RequestingOrigin is the origin on whose behalf the peer requests the
	// session, for PermissionAcceptConnection with the session extension.
	// It is asserted by the peer's user agent, not proven.
	RequestingOrigin string
}

// PINPresentation is a PIN to show to the user.
type PINPresentation struct {
	PIN      string
	Origin   string
	Nickname string
	// Peer that will enter the PIN.
	Peer Peer
}

// PINPrompt asks the user to enter a PIN.
type PINPrompt struct {
	Origin   string
	Nickname string
	// Peer showing the PIN.
	Peer Peer
}
