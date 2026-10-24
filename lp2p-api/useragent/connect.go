package ua

import (
	"context"
	"errors"
	"fmt"
	"sync"

	ospc "github.com/backkem/go-lp2p/openscreen-go/network"
)

// Advertisement makes a page discoverable and hands it connections the user
// granted. Create one with UserAgent.Advertise.
type Advertisement struct {
	u        *UserAgent
	origin   string
	nickname string
	listener *ospc.Listener
	conns    chan *Connection

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	once   sync.Once
}

// Connection is an authenticated connection the user granted to a page.
type Connection struct {
	*ospc.Connection
	// Peer is the remote peer, with its name verified where possible.
	Peer Peer
}

// Advertise asks the user to let origin advertise under nickname and, if
// granted, starts advertising and accepting connections.
func (u *UserAgent) Advertise(ctx context.Context, origin, nickname string) (*Advertisement, error) {
	ok, err := u.cfg.UI.RequestPermission(ctx, PermissionRequest{
		Permission: PermissionAdvertise,
		Origin:     origin,
		Nickname:   nickname,
	})
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrPermissionDenied
	}

	agent, err := newAgent(nickname)
	if err != nil {
		return nil, err
	}
	l, err := ospc.Listen(ospc.AgentTransportQUIC, agent)
	if err != nil {
		return nil, fmt.Errorf("listen: %w", err)
	}

	actx, cancel := context.WithCancel(u.ctx)
	a := &Advertisement{
		u:        u,
		origin:   origin,
		nickname: nickname,
		listener: l,
		conns:    make(chan *Connection),
		ctx:      actx,
		cancel:   cancel,
	}
	if err := u.track(a); err != nil {
		cancel()
		_ = l.Close()
		return nil, err
	}

	a.wg.Add(1)
	go a.run()
	return a, nil
}

func (a *Advertisement) run() {
	defer a.wg.Done()
	for {
		uc, err := a.listener.Accept(a.ctx)
		if err != nil {
			if a.ctx.Err() == nil {
				a.u.log.Warn("advertisement stopped accepting", "nickname", a.nickname, "err", err)
			}
			return
		}
		// Handshake each connection on its own: a user deciding on one
		// prompt must not hold up the others.
		a.wg.Add(1)
		go func() {
			defer a.wg.Done()
			a.handshake(uc)
		}()
	}
}

func (a *Advertisement) handshake(uc *ospc.UnauthenticatedConnection) {
	ctx, cancel := a.u.handshakeContext(a.ctx)
	defer cancel()
	defer uc.Close() // no-op once authenticated

	peer := remotePeer(uc.RemoteAgent())
	log := a.u.log.With("nickname", a.nickname, "peer", peer.DisplayName)

	ok, err := a.u.cfg.UI.RequestPermission(ctx, PermissionRequest{
		Permission: PermissionAcceptConnection,
		Origin:     a.origin,
		Nickname:   a.nickname,
		Peer:       peer,
	})
	if err != nil || !ok {
		log.Info("incoming connection not granted", "err", err)
		return
	}

	conn, err := a.u.authenticate(ctx, uc, a.origin, a.nickname, peer)
	if err != nil {
		log.Info("incoming connection failed to authenticate", "err", err)
		return
	}

	select {
	case a.conns <- conn:
	case <-a.ctx.Done():
		_ = conn.Close()
	}
}

// Accept returns the next connection the user granted.
func (a *Advertisement) Accept(ctx context.Context) (*Connection, error) {
	select {
	case c := <-a.conns:
		return c, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-a.ctx.Done():
		return nil, ErrClosed
	}
}

// Close stops advertising. Pending handshakes are aborted; connections
// already returned by Accept stay open.
func (a *Advertisement) Close() error {
	var err error
	a.once.Do(func() {
		a.cancel()
		err = a.listener.Close()
		a.wg.Wait()
		a.u.untrack(a)
	})
	return err
}

// Connect shows the device picker for origin and connects to the peer the
// user picks, pairing with a PIN.
func (u *UserAgent) Connect(ctx context.Context, origin, nickname string) (*Connection, error) {
	if err := u.StartDiscovery(); err != nil {
		return nil, err
	}

	pctx, stop := context.WithCancel(ctx)
	defer stop()
	defer context.AfterFunc(u.ctx, stop)()

	peer, err := u.cfg.UI.PickPeer(pctx, NewPicker(u.peers, origin, nickname))
	if err != nil {
		if u.ctx.Err() != nil {
			return nil, ErrClosed
		}
		return nil, err
	}
	if peer.Dialer == nil {
		// Pickers may return a stale copy; prefer the live entry.
		if p, ok := u.peers.Get(peer.ID); ok {
			peer = p
		}
	}
	if peer.Dialer == nil {
		return nil, fmt.Errorf("peer %q can't be dialed", peer.DisplayName)
	}

	hctx, cancel := u.handshakeContext(ctx)
	defer cancel()

	agent, err := newAgent(nickname)
	if err != nil {
		return nil, err
	}
	uc, err := peer.Dialer.Dial(hctx, ospc.AgentTransportQUIC, agent)
	if err != nil {
		return nil, fmt.Errorf("dial %q: %w", peer.DisplayName, err)
	}
	defer uc.Close() // no-op once authenticated

	return u.authenticate(hctx, uc, origin, nickname, peer)
}

// newAgent creates the local OSP agent of a connection.
func newAgent(nickname string) (*ospc.Agent, error) {
	conf := ospc.NewAgentConfig(nickname)
	// Non-spec opt-in: SPAKE2 over P-256 rather than the spec's edwards25519,
	// which isn't implemented (see ospc.InteropQuirks.Spake2SuiteP256).
	conf.InteropQuirks.Spake2SuiteP256 = true
	return ospc.NewAgent(conf)
}

// authenticate pairs with the peer: one side shows a PIN, the other enters
// it. Which side does what is negotiated by OSP.
func (u *UserAgent) authenticate(ctx context.Context, uc *ospc.UnauthenticatedConnection, origin, nickname string, peer Peer) (*Connection, error) {
	var psk []byte
	if uc.GetAuthenticationRole() == ospc.AuthenticationRolePresenter {
		raw, err := uc.GeneratePSK()
		if err != nil {
			return nil, err
		}
		if err := u.cfg.UI.PresentPIN(ctx, PINPresentation{
			PIN: EncodePIN(raw), Origin: origin, Nickname: nickname, Peer: peer,
		}); err != nil {
			return nil, err
		}
		psk = raw
	} else {
		// The presenter initiates when its user is shown the PIN; this side
		// answers once the user entered it, or initiates itself if the
		// presenter hasn't yet.
		pin, err := u.cfg.UI.EnterPIN(ctx, PINPrompt{Origin: origin, Nickname: nickname, Peer: peer})
		if err != nil {
			return nil, err
		}
		if psk, err = ParsePIN(pin); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrAuthenticationFailed, err)
		}
	}

	pctx, cancel := context.WithTimeout(ctx, u.cfg.PairingTimeout)
	defer cancel()
	conn, err := uc.AuthenticatePSK(pctx, psk)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if pctx.Err() != nil {
			err = errors.New("pairing didn't complete, was the PIN right?")
		}
		return nil, fmt.Errorf("%w: %v", ErrAuthenticationFailed, err)
	}
	return &Connection{Connection: conn, Peer: verifiedPeer(conn.RemoteAgent(), peer)}, nil
}

func remotePeer(a *ospc.Agent) Peer {
	name, status := a.DisplayName()
	return Peer{ID: string(a.PeerID), DisplayName: name, NameStatus: status}
}

func verifiedPeer(a *ospc.Agent, p Peer) Peer {
	r := remotePeer(a)
	p.ID, p.DisplayName, p.NameStatus = r.ID, r.DisplayName, r.NameStatus
	return p
}

// EncodePIN renders a numeric PSK, as returned by GeneratePSK, as a PIN
// for display: dash-separated groups of three digits below 9 digits, four
// otherwise (network.bs Appendix B, see ospc.FormatPSK).
func EncodePIN(psk []byte) string {
	pin, err := ospc.FormatPSK(string(psk))
	if err != nil {
		return string(psk)
	}
	return pin
}

// ParsePIN turns a PIN as typed by the user into the PSK used for
// authentication: dashes, spaces and leading zeros are dropped (network.bs
// Appendix B, see ospc.ParsePSK).
func ParsePIN(pin string) ([]byte, error) {
	psk, err := ospc.ParsePSK(pin)
	if err != nil {
		return nil, err
	}
	return []byte(psk), nil
}
