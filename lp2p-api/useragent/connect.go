package ua

import (
	"context"
	"errors"
	"fmt"
	"sync"

	ospc "github.com/backkem/go-lp2p/openscreen-go/network"
)

// AdvertiseOptions configures an Advertisement.
type AdvertiseOptions struct {
	// Origin of the page advertising, serialized (see OriginOf).
	Origin string
	// Nickname the page advertises under.
	Nickname string
	// AllowedOrigins are serialized origins that, in addition to Origin,
	// may request a session with this advertisement. Opaque origins never
	// match. Only checked with the session extension; see
	// Config.DisableSessionExtension.
	AllowedOrigins []string
}

// Advertisement makes a page discoverable and hands it connections the user
// granted. Create one with UserAgent.Advertise.
type Advertisement struct {
	u        *UserAgent
	origin   string
	nickname string
	allowed  []string
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
	// RequestingOrigin is the origin on whose behalf the connection was
	// requested, serialized. Empty without the session extension.
	RequestingOrigin string
}

// Advertise asks the user to let the page advertise under the nickname and,
// if granted, starts advertising and accepting connections.
func (u *UserAgent) Advertise(ctx context.Context, opts AdvertiseOptions) (*Advertisement, error) {
	ok, err := u.cfg.UI.RequestPermission(ctx, PermissionRequest{
		Permission: PermissionAdvertise,
		Origin:     opts.Origin,
		Nickname:   opts.Nickname,
	})
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrPermissionDenied
	}

	agent, err := u.newAgent(opts.Nickname)
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
		origin:   opts.Origin,
		nickname: opts.Nickname,
		allowed:  append([]string(nil), opts.AllowedOrigins...),
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

// allows reports whether the advertisement allows a requesting origin hint,
// as received on the wire: the page's own origin and its allowed origins
// do; an opaque or unparsable one never does.
func (a *Advertisement) allows(hint string) bool {
	origin, err := OriginOf(hint)
	if err != nil {
		return false
	}
	if SameOrigin(origin, a.origin) {
		return true
	}
	for _, allowed := range a.allowed {
		if SameOrigin(origin, allowed) {
			return true
		}
	}
	return false
}

func (a *Advertisement) handshake(uc *ospc.UnauthenticatedConnection) {
	ctx, cancel := a.u.handshakeContext(a.ctx)
	defer cancel()
	defer uc.Close() // no-op once authenticated

	peer := remotePeer(uc.RemoteAgent())
	log := a.u.log.With("nickname", a.nickname, "peer", peer.DisplayName)

	var conn *Connection
	var err error
	if a.u.sessions() {
		// Handed to the receiving advertisement's Accept by acceptSession.
		if _, err := a.sessionHandshake(ctx, uc, peer, log); err != nil {
			log.Info("incoming connection not accepted", "err", err)
		}
		return
	}
	conn, err = a.legacyHandshake(ctx, uc, peer, log)
	if err != nil {
		log.Info("incoming connection not accepted", "err", err)
		return
	}

	select {
	case a.conns <- conn:
	case <-a.ctx.Done():
		_ = conn.Close()
	}
}

// sessionHandshake runs index.bs "Incoming sessions" over the session
// extension: it answers the client's ceremony if it starts one, asks for
// one when the client requests a session without being verified, and once
// the client is verified and the request is in, delivers the session to the
// receiver that allows its origin: this advertisement, the one the client
// picked, if it does, else another one of the user agent.
func (a *Advertisement) sessionHandshake(ctx context.Context, uc *ospc.UnauthenticatedConnection, peer Peer, log logger) (*Connection, error) {
	u := a.u
	for {
		cue, err := uc.AwaitSession(ctx)
		if err != nil {
			return nil, err
		}
		switch cue {
		case ospc.SessionCueAuthenticate:
			if _, err := u.pair(ctx, uc, a.origin, a.nickname, peer); err != nil {
				return nil, err
			}
			u.trust.addAnchor(peer.ID)
		case ospc.SessionCueRequested:
			if !uc.RemoteAgent().Authenticated() {
				if u.trust.trusted(peer.ID) {
					if err := uc.TrustPeer(); err != nil {
						return nil, err
					}
				} else if err := uc.RequireAuthentication(); err != nil {
					return nil, err
				}
			}
		}
		if !uc.RemoteAgent().Authenticated() {
			continue
		}
		origin, ok := uc.RequestingOrigin()
		if !ok {
			continue // Verified; the request is still to come.
		}
		peer = verifiedPeer(uc.RemoteAgent(), peer)
		target := a
		if !a.allows(origin) {
			target = u.advertisementAllowing(origin)
		}
		if target == nil {
			log.Info("refusing session: requesting origin not allowed", "origin", origin)
			return nil, errors.Join(ErrRefused, uc.RefuseSession())
		}
		if target != a {
			log.Info("delivering session to another receiver that allows the origin", "origin", origin, "nickname", target.nickname)
		}
		return target.acceptSession(ctx, uc, peer, origin)
	}
}

// acceptSession prompts for the grant if needed, accepts the session of a
// verified client for this advertisement, and hands the connection to
// Accept.
func (a *Advertisement) acceptSession(ctx context.Context, uc *ospc.UnauthenticatedConnection, peer Peer, origin string) (*Connection, error) {
	u := a.u
	u.trust.addKnown(peer)

	if !u.trust.granted(a.origin, peer.ID) {
		ok, err := u.cfg.UI.RequestPermission(ctx, PermissionRequest{
			Permission:       PermissionAcceptConnection,
			Origin:           a.origin,
			Nickname:         a.nickname,
			Peer:             peer,
			RequestingOrigin: origin,
		})
		if err != nil {
			_ = uc.RefuseSession()
			return nil, err
		}
		if !ok {
			return nil, errors.Join(ErrPermissionDenied, uc.RefuseSession())
		}
		u.trust.grant(a.origin, peer.ID)
	}

	conn, err := uc.CompleteSession(ctx)
	if err != nil {
		return nil, err
	}
	c := &Connection{Connection: conn, Peer: peer, RequestingOrigin: origin}
	select {
	case a.conns <- c:
		return nil, nil
	case <-a.ctx.Done():
		_ = conn.Close()
		return nil, ErrClosed
	}
}

// legacyHandshake is the flow without the session extension: ask the user,
// then pair with a PIN on every connection.
func (a *Advertisement) legacyHandshake(ctx context.Context, uc *ospc.UnauthenticatedConnection, peer Peer, log logger) (*Connection, error) {
	u := a.u
	ok, err := u.cfg.UI.RequestPermission(ctx, PermissionRequest{
		Permission: PermissionAcceptConnection,
		Origin:     a.origin,
		Nickname:   a.nickname,
		Peer:       peer,
	})
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrPermissionDenied
	}

	conn, err := u.pair(ctx, uc, a.origin, a.nickname, peer)
	if err != nil {
		return nil, err
	}
	peer = verifiedPeer(conn.RemoteAgent(), peer)
	u.trust.addAnchor(peer.ID)
	u.trust.addKnown(peer)
	u.trust.grant(a.origin, peer.ID)
	return &Connection{Connection: conn, Peer: peer}, nil
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
// user picks, pairing with a PIN unless both sides trust each other.
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

	agent, err := u.newAgent(nickname)
	if err != nil {
		return nil, err
	}
	if !u.sessions() {
		uc, err := peer.Dialer.Dial(hctx, ospc.AgentTransportQUIC, agent)
		if err != nil {
			return nil, fmt.Errorf("dial %q: %w", peer.DisplayName, err)
		}
		defer uc.Close() // no-op once authenticated
		conn, err := u.pair(hctx, uc, origin, nickname, peer)
		if err != nil {
			return nil, err
		}
		peer = verifiedPeer(conn.RemoteAgent(), peer)
		u.trust.addAnchor(peer.ID)
		u.trust.addKnown(peer)
		u.trust.grant(origin, peer.ID)
		return &Connection{Connection: conn, Peer: peer}, nil
	}

	uc, err := peer.Dialer.DialSession(hctx, ospc.AgentTransportQUIC, agent, origin)
	if err != nil {
		return nil, fmt.Errorf("dial %q: %w", peer.DisplayName, err)
	}
	defer uc.Close() // no-op once authenticated

	// Verify the server before telling it the origin: a ceremony unless
	// it is trusted. Then request the session and follow the server's
	// cues: it asks for a ceremony if it doesn't trust this agent.
	id := string(uc.RemoteAgent().PeerID)
	if u.trust.trusted(id) {
		if err := uc.TrustPeer(); err != nil {
			return nil, err
		}
	} else {
		if _, err := u.pair(hctx, uc, origin, nickname, peer); err != nil {
			return nil, err
		}
		u.trust.addAnchor(id)
	}
	if err := uc.RequestSession(hctx); err != nil {
		return nil, err
	}
	for {
		cue, err := uc.AwaitSession(hctx)
		if err != nil {
			return nil, sessionError(err)
		}
		if cue != ospc.SessionCueAuthenticate {
			break
		}
		if _, err := u.pair(hctx, uc, origin, nickname, peer); err != nil {
			return nil, err
		}
	}
	conn, err := uc.CompleteSession(hctx)
	if err != nil {
		return nil, sessionError(err)
	}
	peer = verifiedPeer(conn.RemoteAgent(), peer)
	u.trust.addKnown(peer)
	u.trust.grant(origin, peer.ID)
	return &Connection{Connection: conn, Peer: peer, RequestingOrigin: origin}, nil
}

// sessionError maps a refusal by the peer onto ErrRefused.
func sessionError(err error) error {
	if errors.Is(err, ospc.ErrSessionRefused) {
		return fmt.Errorf("%w: %v", ErrRefused, err)
	}
	return err
}

// sessions reports whether the LP2P session extension is in use.
func (u *UserAgent) sessions() bool { return !u.cfg.DisableSessionExtension }

// newAgent creates the local OSP agent of a connection. All agents of the
// user agent share its certificate, so its peer ID is stable; the display
// name carries the page's nickname, as OSP has no field for it yet.
func (u *UserAgent) newAgent(nickname string) (*ospc.Agent, error) {
	conf := ospc.NewAgentConfig(nickname)
	conf.Certificate = u.identity.Certificate
	conf.PeerID = u.identity.PeerID
	conf.SessionExtension = u.sessions()
	// Non-spec opt-in: SPAKE2 over P-256 rather than the spec's edwards25519,
	// which isn't implemented (see ospc.InteropQuirks.Spake2SuiteP256).
	conf.InteropQuirks.Spake2SuiteP256 = true
	return ospc.NewAgent(conf)
}

// pair runs the trust ceremony: one side shows a PIN, the other enters it.
// Which side does what is negotiated by OSP. With the session extension
// only the client starts SPAKE2.
func (u *UserAgent) pair(ctx context.Context, uc *ospc.UnauthenticatedConnection, origin, nickname string, peer Peer) (*ospc.Connection, error) {
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
		// The consumer asks the presenter to show the PIN, whose user is
		// then shown it, and answers once its own user entered it.
		if err := uc.StartAuthentication(); err != nil {
			return nil, err
		}
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
	return conn, nil
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
