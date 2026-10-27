package ua

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
)

// Scripted is a non-interactive UI for examples and tests: every prompt is
// answered by a function. Nil functions use sensible defaults, noted per
// field. If Out is set, every interaction is narrated there, as a user would
// see it.
type Scripted struct {
	// Name prefixes narration, e.g. "Browser A".
	Name string
	// Out receives narration. Nil is silent.
	Out io.Writer

	// Permission answers permission prompts. Nil grants everything.
	Permission func(PermissionRequest) bool
	// Pick chooses from the device picker. Nil cancels the picker.
	Pick func(ctx context.Context, p *Picker) (Peer, error)
	// Present is called when a PIN is shown. Nil only narrates.
	Present func(PINPresentation)
	// Enter supplies a PIN. Nil cancels.
	Enter func(ctx context.Context, p PINPrompt) (string, error)

	mu sync.Mutex // serializes narration
}

var _ UI = (*Scripted)(nil)

func (s *Scripted) say(format string, args ...any) {
	if s.Out == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	prefix := ""
	if s.Name != "" {
		prefix = "[" + s.Name + "] "
	}
	fmt.Fprintf(s.Out, prefix+format+"\n", args...)
}

// RequestPermission implements UI.
func (s *Scripted) RequestPermission(ctx context.Context, req PermissionRequest) (bool, error) {
	ok := s.Permission == nil || s.Permission(req)
	answer := "allowed"
	if !ok {
		answer = "denied"
	}
	switch req.Permission {
	case PermissionAdvertise:
		s.say("%s wants to be discoverable as %q: %s", req.Origin, req.Nickname, answer)
	case PermissionAcceptConnection:
		from := req.Peer.Label()
		if req.RequestingOrigin != "" {
			from += " on behalf of " + req.RequestingOrigin
		}
		s.say("%s (%q) wants to accept a connection from %s: %s", req.Origin, req.Nickname, from, answer)
	default:
		s.say("%s asks for %s: %s", req.Origin, req.Permission, answer)
	}
	return ok, nil
}

// PickPeer implements UI.
func (s *Scripted) PickPeer(ctx context.Context, p *Picker) (Peer, error) {
	s.say("%s (%q) shows the device picker", p.Origin, p.Nickname)
	if s.Pick == nil {
		s.say("user dismissed the picker")
		return Peer{}, ErrCanceled
	}
	peer, err := s.Pick(ctx, p)
	if err != nil {
		s.say("picker closed: %v", err)
		return Peer{}, err
	}
	s.say("user picked %s", peer.Label())
	return peer, nil
}

// PresentPIN implements UI.
func (s *Scripted) PresentPIN(ctx context.Context, p PINPresentation) error {
	s.say("shows PIN %s to pair with %s", p.PIN, p.Peer.Label())
	if s.Present != nil {
		s.Present(p)
	}
	return nil
}

// EnterPIN implements UI.
func (s *Scripted) EnterPIN(ctx context.Context, p PINPrompt) (string, error) {
	s.say("asks for the PIN shown on %s", p.Peer.Label())
	if s.Enter == nil {
		s.say("user dismissed the PIN prompt")
		return "", ErrCanceled
	}
	pin, err := s.Enter(ctx, p)
	if err != nil {
		return "", err
	}
	s.say("user enters PIN %s", pin)
	return pin, nil
}

// Deny returns a Scripted.Permission that denies the given permissions and
// grants all others.
func Deny(perms ...Permission) func(PermissionRequest) bool {
	return func(req PermissionRequest) bool {
		for _, p := range perms {
			if req.Permission == p {
				return false
			}
		}
		return true
	}
}

// PickByName returns a Scripted.Pick that waits for a peer whose display
// name is name to show up in the picker and picks it. It gives up when ctx
// is done.
func PickByName(name string) func(context.Context, *Picker) (Peer, error) {
	return func(ctx context.Context, p *Picker) (Peer, error) {
		for peers := range p.Watch(ctx) {
			for _, peer := range peers {
				if peer.DisplayName == name {
					return peer, nil
				}
			}
		}
		return Peer{}, fmt.Errorf("no peer named %q found: %w", name, ctx.Err())
	}
}

// FixedPIN returns a Scripted.Enter that always enters pin.
func FixedPIN(pin string) func(context.Context, PINPrompt) (string, error) {
	return func(context.Context, PINPrompt) (string, error) { return pin, nil }
}

// PINRelay stands in for the user reading a PIN off one screen and typing
// it into another. Wire Present into the presenting UI and Enter into the
// entering one; Enter waits until the matching PIN is shown.
//
// PINs are matched by nickname: a PIN presented by "A" to "B" is entered by
// "B" for "A".
type PINRelay struct {
	mu    sync.Mutex
	shown map[[2]string]chan string
}

// NewPINRelay creates a PINRelay.
func NewPINRelay() *PINRelay {
	return &PINRelay{shown: make(map[[2]string]chan string)}
}

func (r *PINRelay) slot(presenter, consumer string) chan string {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := [2]string{strings.ToLower(presenter), strings.ToLower(consumer)}
	ch, ok := r.shown[k]
	if !ok {
		ch = make(chan string, 1)
		r.shown[k] = ch
	}
	return ch
}

// Present is a Scripted.Present.
func (r *PINRelay) Present(p PINPresentation) {
	ch := r.slot(p.Nickname, p.Peer.DisplayName)
	select {
	case <-ch: // replace a PIN nobody entered
	default:
	}
	ch <- p.PIN
}

// Enter is a Scripted.Enter.
func (r *PINRelay) Enter(ctx context.Context, p PINPrompt) (string, error) {
	select {
	case pin := <-r.slot(p.Peer.DisplayName, p.Nickname):
		return pin, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

// EnterWithin is like Enter, but the simulated user gives up (ErrCanceled)
// if no PIN is shown within d. Use it where the other side may never show
// one, e.g. because its user refused the connection: ospc doesn't tell the
// entering side that the connection was dropped before pairing.
func (r *PINRelay) EnterWithin(d time.Duration) func(context.Context, PINPrompt) (string, error) {
	return func(ctx context.Context, p PINPrompt) (string, error) {
		wctx, cancel := context.WithTimeout(ctx, d)
		defer cancel()
		pin, err := r.Enter(wctx, p)
		if err != nil && ctx.Err() == nil {
			return "", fmt.Errorf("%w: no PIN shown within %s", ErrCanceled, d)
		}
		return pin, err
	}
}
