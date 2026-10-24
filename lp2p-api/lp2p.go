// Package lp2p is a Go rendition of the Local Peer-to-Peer API.
// https://wicg.github.io/local-peer-to-peer/
//
// The package mirrors the WebIDL as closely as Go allows: interface names
// are kept (LP2PReceiver, LP2PRequest, LP2PConnection, LP2PDataChannel,
// LP2PQuicTransport, LP2PQuicTransportListener), constructors become New*
// functions, promises become blocking calls, event handler attributes become
// On* methods taking a callback, and rejections are *DOMException errors.
//
// The API relies on the user agent for permission prompts, the device
// picker and pairing. That part is mocked by the useragent package (ua).
// Which user agent an object talks to is decided by the Window it is
// created in; the package-level constructors use DefaultWindow.
//
// Go-only additions, needed to release resources, are marked as such.
package lp2p

import (
	"os"
	"sync"

	ua "github.com/backkem/go-lp2p/lp2p-api/useragent"
)

// Window is a Go stand-in for a browsing context: a page (origin) hosted by
// a user agent. In JavaScript the LP2P constructors are globals of the page;
// in Go they are methods on a Window, so one process can host several
// browsers, as the examples do. Go-only.
type Window struct {
	// UserAgent hosting the page.
	UserAgent *ua.UserAgent
	// Origin of the page, shown in permission prompts and the picker.
	Origin string
}

// NewWindow creates a page with the given origin in user agent u.
func NewWindow(u *ua.UserAgent, origin string) *Window {
	return &Window{UserAgent: u, Origin: origin}
}

var (
	defaultMu     sync.Mutex
	defaultWindow *Window
)

// DefaultWindow is the window used by the package-level constructors. Unless
// set with SetDefaultWindow, it is created on first use, with an interactive
// user agent on stdin and stdout. Go-only.
func DefaultWindow() *Window {
	defaultMu.Lock()
	defer defaultMu.Unlock()
	if defaultWindow == nil {
		u, err := ua.New(ua.Config{
			Name: "default",
			UI:   ua.NewCLI(os.Stdin, os.Stdout),
		})
		if err != nil {
			panic(err) // only fails without a UI
		}
		defaultWindow = NewWindow(u, "https://example.com")
	}
	return defaultWindow
}

// SetDefaultWindow replaces the window used by the package-level
// constructors. Go-only.
func SetDefaultWindow(w *Window) {
	defaultMu.Lock()
	defer defaultMu.Unlock()
	defaultWindow = w
}

// NewLP2PReceiver creates an LP2PReceiver in the default window.
func NewLP2PReceiver(options LP2PReceiverOptions) (*LP2PReceiver, error) {
	return DefaultWindow().NewLP2PReceiver(options)
}

// NewLP2PRequest creates an LP2PRequest in the default window.
func NewLP2PRequest(options LP2PRequestOptions) (*LP2PRequest, error) {
	return DefaultWindow().NewLP2PRequest(options)
}
