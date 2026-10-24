// Package ua is a mock user agent (browser) for the lp2p package.
//
// The Local Peer-to-Peer API leaves a lot to the user agent: asking the user
// for permission, showing a device picker, pairing devices with a PIN. This
// package implements just enough of that to illustrate the API. It is not a
// product: it exists so the lp2p package can be exercised end to end.
//
// All user interaction goes through the UI interface. Scripted answers
// programmatically, for examples and tests; CLI asks on a terminal.
//
// A UserAgent continuously discovers peers (mDNS via ospc by default, or any
// Discoverer) into a PeerList, which the device picker shows live.
package ua

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"
)

// Errors returned by the user agent. The lp2p package maps them onto
// DOMExceptions; use errors.Is to tell them apart.
var (
	// ErrPermissionDenied means the user declined a permission prompt.
	ErrPermissionDenied = errors.New("permission denied by user")
	// ErrCanceled means the user dismissed a prompt or the device picker.
	ErrCanceled = errors.New("canceled by user")
	// ErrAuthenticationFailed means pairing with the peer failed, for
	// example because the wrong PIN was entered.
	ErrAuthenticationFailed = errors.New("authentication failed")
	// ErrClosed means the user agent or advertisement was closed.
	ErrClosed = errors.New("user agent closed")
)

const (
	// DefaultHandshakeTimeout bounds consent and authentication of a
	// connection.
	DefaultHandshakeTimeout = 60 * time.Second
	// DefaultPairingTimeout bounds the PIN exchange once the PIN is in.
	DefaultPairingTimeout = 5 * time.Second
)

// Config configures a UserAgent.
type Config struct {
	// Name identifies this user agent in logs, e.g. "Browser A".
	Name string
	// UI is the browser chrome that talks to the user. Required.
	UI UI
	// Discover creates the source of discovered peers. Nil uses mDNS, see
	// MDNSDiscoverer.
	Discover func() (Discoverer, error)
	// HandshakeTimeout bounds the consent prompt plus authentication of a
	// single connection. Zero means DefaultHandshakeTimeout.
	HandshakeTimeout time.Duration
	// PairingTimeout bounds the SPAKE2 exchange once the PIN has been shown
	// or entered. A pairing that doesn't finish in time is reported as
	// ErrAuthenticationFailed: ospc doesn't report a PIN mismatch to either
	// side, so a stalled exchange is how a wrong PIN shows. Zero means
	// DefaultPairingTimeout.
	PairingTimeout time.Duration
	// Logger receives diagnostics. Nil discards them.
	Logger *slog.Logger
}

// UserAgent is a mock browser hosting LP2P pages. It is safe for concurrent
// use. Close it to stop discovery and all advertisements.
type UserAgent struct {
	cfg    Config
	log    *slog.Logger
	peers  *PeerList
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	mu      sync.Mutex
	closed  bool
	disc    Discoverer
	adverts map[*Advertisement]struct{}
}

// New creates a user agent.
func New(cfg Config) (*UserAgent, error) {
	if cfg.UI == nil {
		return nil, errors.New("ua: Config.UI is required")
	}
	if cfg.Discover == nil {
		cfg.Discover = MDNSDiscoverer
	}
	if cfg.HandshakeTimeout <= 0 {
		cfg.HandshakeTimeout = DefaultHandshakeTimeout
	}
	if cfg.PairingTimeout <= 0 {
		cfg.PairingTimeout = DefaultPairingTimeout
	}
	log := cfg.Logger
	if log == nil {
		log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	if cfg.Name != "" {
		log = log.With("ua", cfg.Name)
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &UserAgent{
		cfg:     cfg,
		log:     log,
		peers:   newPeerList(),
		ctx:     ctx,
		cancel:  cancel,
		adverts: make(map[*Advertisement]struct{}),
	}, nil
}

// Name of the user agent, as configured.
func (u *UserAgent) Name() string { return u.cfg.Name }

// UI returns the user agent's UI.
func (u *UserAgent) UI() UI { return u.cfg.UI }

// Peers is the live list of discovered peers. It stays empty until
// discovery is started, see StartDiscovery.
func (u *UserAgent) Peers() *PeerList { return u.peers }

// StartDiscovery starts discovering peers in the background, if it isn't
// running yet. Discovery runs until the user agent is closed.
func (u *UserAgent) StartDiscovery() error {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.closed {
		return ErrClosed
	}
	if u.disc != nil {
		return nil
	}
	d, err := u.cfg.Discover()
	if err != nil {
		return fmt.Errorf("start discovery: %w", err)
	}
	u.disc = d

	u.wg.Add(1)
	go func() {
		defer u.wg.Done()
		for {
			ev, err := d.Next(u.ctx)
			if err != nil {
				if u.ctx.Err() == nil {
					u.log.Warn("discovery stopped", "err", err)
				}
				return
			}
			u.peers.apply(ev)
		}
	}()
	return nil
}

// Close stops discovery and all advertisements. Connections already handed
// to pages stay open; their owners close them.
func (u *UserAgent) Close() error {
	u.mu.Lock()
	if u.closed {
		u.mu.Unlock()
		return nil
	}
	u.closed = true
	disc := u.disc
	adverts := make([]*Advertisement, 0, len(u.adverts))
	for a := range u.adverts {
		adverts = append(adverts, a)
	}
	u.mu.Unlock()

	u.cancel()
	var errs []error
	for _, a := range adverts {
		errs = append(errs, a.Close())
	}
	if disc != nil {
		errs = append(errs, disc.Close())
	}
	u.wg.Wait()
	return errors.Join(errs...)
}

func (u *UserAgent) track(a *Advertisement) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.closed {
		return ErrClosed
	}
	u.adverts[a] = struct{}{}
	return nil
}

func (u *UserAgent) untrack(a *Advertisement) {
	u.mu.Lock()
	delete(u.adverts, a)
	u.mu.Unlock()
}

// handshakeContext derives a context bounded by the handshake timeout that
// is also canceled when the user agent closes.
func (u *UserAgent) handshakeContext(ctx context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(ctx, u.cfg.HandshakeTimeout)
	stop := context.AfterFunc(u.ctx, cancel)
	return ctx, func() {
		stop()
		cancel()
	}
}
