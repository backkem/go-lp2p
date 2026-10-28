package ospc

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"
	"sync/atomic"

	mdns "github.com/grandcat/zeroconf"
)

var ErrListenerClosed = errors.New("listener closed")

// Listen starts an advertising agent and listens for incoming connections.
func Listen(transportType AgentTransport, a *Agent) (*Listener, error) {
	l := NewListener(a, transportType)

	err := l.Start()
	if err != nil {
		return nil, err
	}

	return l, nil
}

// Listener acts as an advertising OSP agent and listens for incoming
// connections.
type Listener struct {
	agent         *Agent
	transportType AgentTransport
	log           *slog.Logger

	accept chan *UnauthenticatedConnection

	// Lifecycle
	ctx       context.Context
	cancel    context.CancelCauseFunc
	wg        sync.WaitGroup
	closeOnce sync.Once

	mu sync.Mutex // Protects the state below.

	started       bool
	alpnListeners map[string]*ALPNListener
	netListener   NetworkListener

	// mDNS advertisement, set once running.
	advertiser *mdns.Server
	txt        TXTRecordSet
	// authToken is the advertised at value.
	authToken string
}

// NewListener creates a new Listener
func NewListener(a *Agent, transportType AgentTransport) *Listener {
	ctx, cancel := context.WithCancelCause(context.Background())
	return &Listener{
		agent:         a,
		transportType: transportType,
		log:           a.logger(),
		accept:        make(chan *UnauthenticatedConnection),
		ctx:           ctx,
		cancel:        cancel,
		alpnListeners: make(map[string]*ALPNListener),
	}
}

// ListenApplication allows you to listen for quic connections
// on the same port but with a different ALPN. Only one per ALPN
// is allowed. Needs to be registered before starting the Listener.
func (l *Listener) ListenApplication(alpn string, config *ALPNListenerConfig) *ALPNListener {
	child := newALPNListener(l, config)

	l.mu.Lock()
	defer l.mu.Unlock()
	l.alpnListeners[alpn] = child

	return child
}

func (l *Listener) removeALPNListener(child *ALPNListener) {
	l.mu.Lock()
	defer l.mu.Unlock()

	for k, v := range l.alpnListeners {
		if v == child {
			delete(l.alpnListeners, k)
			return
		}
	}
}

func (l *Listener) getALPNListener(alpn string) *ALPNListener {
	l.mu.Lock()
	defer l.mu.Unlock()

	return l.alpnListeners[alpn]
}

// Start listening and advertising.
func (l *Listener) Start() error {
	return l.start(":", true)
}

// StartWithoutAdvertising listens on addr (e.g. "127.0.0.1:0") without
// advertising the agent with mDNS. Peers connect with a DiscoveredAgent from
// DirectAgent, using Addr and TXT.
func (l *Listener) StartWithoutAdvertising(addr string) error {
	return l.start(addr, false)
}

// Addr returns the local address the listener listens on, or nil if it is
// not started.
func (l *Listener) Addr() net.Addr {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.netListener == nil {
		return nil
	}
	return l.netListener.Addr()
}

// TXT returns a copy of the TXT records the listener advertises, or would
// advertise when started without advertising.
func (l *Listener) TXT() TXTRecordSet {
	l.mu.Lock()
	defer l.mu.Unlock()

	txt := TXTRecordSet{}
	for k, v := range l.txt {
		txt[k] = append([]string(nil), v...)
	}
	return txt
}

// AnnounceMetadataChange signals listening agents that the metadata of the
// local agent changed by advertising a greater mv value, so they know to
// reconnect to discover the updated metadata.
func (l *Listener) AnnounceMetadataChange() error {
	mv, err := encodeMetadataVersion(l.agent.bumpMetadataVersion())
	if err != nil {
		return err
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	if l.txt == nil {
		// Not running yet, the new mv is picked up when starting.
		return nil
	}
	l.txt.Set(txtKeyMetadataVersion, mv)
	if l.advertiser != nil {
		l.advertiser.SetText(l.txt.ToSlice())
	}
	return nil
}

type TXTRecordSet map[string][]string

func (r TXTRecordSet) Set(key, value string) {
	r[key] = []string{value}
}

func (r TXTRecordSet) Add(key, value string) {
	r[key] = append(r[key], value)
}

func (r TXTRecordSet) Get(key string) []string {
	return r[key]
}

func (r TXTRecordSet) GetOne(key string) (string, error) {
	record := r.Get(key)
	if len(record) < 1 {
		return "", fmt.Errorf("no value for key %s", key)
	}
	if len(record) > 1 {
		return "", fmt.Errorf("multiple values for key %s", key)
	}
	return record[0], nil
}

func (r TXTRecordSet) FromSlice(in []string) error {
	for _, pair := range in {
		n := strings.IndexRune(pair, '=')
		if n < 0 {
			return errors.New("failed to find record key")
		}
		key := pair[:n]
		value := pair[n+1:]

		values, ok := r[key]
		if !ok {
			values = []string{}
		}
		values = append(values, value)
		r[key] = values
	}

	return nil
}

func (r TXTRecordSet) ToSlice() []string {
	out := []string{}
	for key, values := range r {
		for _, value := range values {
			pair := fmt.Sprintf("%s=%s", key, value)
			out = append(out, pair)
		}
	}
	return out
}

// start listens on addr and, if advertise is set, advertises the agent with
// mDNS.
func (l *Listener) start(addr string, advertise bool) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.started {
		return errors.New("listener already started")
	}
	if l.ctx.Err() != nil {
		return ErrListenerClosed
	}

	// List all registered ALPNs.
	nextProtos := []string{ALPN_OSP}
	for k := range l.alpnListeners {
		nextProtos = append(nextProtos, k)
	}

	// Listen for and handle incoming connections
	tlsConfig := &tls.Config{
		MinVersion:   tls.VersionTLS13, // OpenScreen spec requires TLS 1.3
		MaxVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{*l.agent.Certificate},
		NextProtos:   nextProtos, // Application-Layer Protocol Negotiation
		ClientAuth:   tls.RequireAnyClientCert,
		VerifyConnection: func(cs tls.ConnectionState) error {
			if len(cs.PeerCertificates) == 0 {
				return errors.New("no peer certificate")
			}
			alpn := cs.NegotiatedProtocol
			if alpn != ALPN_OSP {
				child := l.getALPNListener(alpn)
				if child == nil {
					return fmt.Errorf("unsupported ALPN %q", alpn)
				}
				return child.doVerifyConnection(cs)
			}

			// The listening agent's certificate can't be pinned, its fp is
			// unknown to the advertising agent. network.bs requires both
			// agents to present an agent certificate but defines no check of
			// the listening agent's certificate beyond that: the peer is
			// verified by authentication, with SPAKE2 identities bound to the
			// certificate's agent fingerprint. In particular no hostname is
			// verified.
			return nil
		},
	}

	fp, err := l.agent.advertisedFingerPrint()
	if err != nil {
		return err
	}

	mv, err := encodeMetadataVersion(l.agent.MetadataVersion())
	if err != nil {
		return err
	}

	t, err := newAgentTransport(l.transportType, l.agent.keepAlive, l.log)
	if err != nil {
		return err
	}
	netListener, err := t.ListenAddr(addr, tlsConfig)
	if err != nil {
		return err
	}

	at := randomAT(9)

	txt := TXTRecordSet{}
	txt.Set(txtKeyFingerprint, fp)
	txt.Set(txtKeyMetadataVersion, mv)
	txt.Set(txtKeyAuthToken, at)
	txt.Set("fn", "TODO:Remove?")
	// Encode certificate serial number as URL-safe base64 (no padding).
	// Spec says RFC4648 base64, but standard base64 contains +/= which are
	// invalid in DNS labels. See: https://github.com/w3c/openscreenprotocol/issues/365
	snBytes := l.agent.Certificate.Leaf.SerialNumber.Bytes()
	snBase64 := base64.RawURLEncoding.EncodeToString(snBytes)
	txt.Set("sn", snBase64) // TODO: openscreenprotocol#293

	if advertise {
		port := netListener.Addr().(*net.UDPAddr).Port
		// The Instance Name is a prefix of the display name, truncated if
		// needed, in DNS presentation format as expected by the mDNS library.
		instance := escapeDNSSD(instanceNameForDisplayName(l.agent.Info().DisplayName))
		advertiser, err := mdns.Register(instance, MdnsServiceType, MdnsDomain, port, txt.ToSlice(), nil)
		if err != nil {
			_ = netListener.Close()
			return err
		}
		l.advertiser = advertiser
	}

	l.started = true
	l.netListener = netListener
	l.txt = txt
	l.authToken = at

	l.wg.Add(1)
	go func() {
		defer l.wg.Done()
		l.acceptLoop(netListener)
	}()

	return nil
}

// acceptLoop accepts incoming connections until the listener is closed.
func (l *Listener) acceptLoop(netListener NetworkListener) {
	for {
		nc, err := l.acceptNetworkConnection(netListener)
		if err != nil {
			if l.ctx.Err() == nil {
				l.log.Warn("listener: accept failed, no longer accepting connections", "err", err)
			}
			return
		}
		if nc != nil {
			l.handleNetworkConnection(nc)
		}
	}
}

// acceptNetworkConnection accepts the next connection. Connections for other
// ALPNs are dispatched to their ALPNListener, a nil connection is returned
// for those.
func (l *Listener) acceptNetworkConnection(netListener NetworkListener) (NetworkConnection, error) {
	ql, ok := netListener.(*quicNetworkListener)
	if !ok {
		return netListener.Accept(l.ctx)
	}

	conn, err := ql.acceptRaw(l.ctx)
	if err != nil {
		return nil, err
	}
	alpn := conn.ConnectionState().TLS.NegotiatedProtocol
	if alpn == ALPN_OSP {
		return ql.wrap(conn), nil
	}

	child := l.getALPNListener(alpn)
	if child == nil {
		_ = conn.CloseWithError(QuicErrorCodeNoLongerNeeded, "unsupported ALPN")
		return nil, nil
	}
	l.wg.Add(1)
	go func() {
		defer l.wg.Done()
		child.dispatch(l.ctx, conn)
	}()
	return nil, nil
}

// handleNetworkConnection runs the info exchange on an incoming connection and
// hands it to Accept once complete.
func (l *Listener) handleNetworkConnection(nc NetworkConnection) {
	remoteAgent, err := l.agent.NewRemoteAgent(nc)
	if err != nil {
		l.log.Debug("listener: failed to create remote agent", "err", err)
		_ = nc.Close()
		return
	}

	l.mu.Lock()
	at := l.authToken
	l.mu.Unlock()

	bConn := newBaseConnection(nc, l.agent, remoteAgent, AgentRoleServer, at, l.agent.sessionExtension)
	bConn.runNetwork()

	l.wg.Add(1)
	go func() {
		defer l.wg.Done()

		err := bConn.awaitInfo(l.ctx)
		if err != nil {
			l.log.Info("listener: info exchange failed", "err", err)
			_ = bConn.Close()
			return
		}

		select {
		case l.accept <- &UnauthenticatedConnection{base: bConn}:
		case <-l.ctx.Done():
			_ = bConn.Close()
		}
	}()
}

// Accept returns the next incoming connection. It should be called in a loop.
func (l *Listener) Accept(ctx context.Context) (*UnauthenticatedConnection, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case a := <-l.accept:
		return a, nil
	case <-l.ctx.Done():
		return nil, ErrListenerClosed
	}
}

// Close closes the listener and the connections it didn't hand out yet.
// Connections returned by Accept are not closed. Any blocked Accept
// operations are unblocked and return ErrListenerClosed. Close is idempotent.
func (l *Listener) Close() error {
	l.closeOnce.Do(func() {
		l.cancel(ErrListenerClosed)

		l.mu.Lock()
		netListener := l.netListener
		advertiser := l.advertiser
		l.mu.Unlock()

		if advertiser != nil {
			advertiser.Shutdown()
		}
		if netListener != nil {
			_ = netListener.Close()
		}
	})

	l.wg.Wait()
	return nil
}

// UnauthenticatedConnection represents an OSPC connection that didn't pass
// authentication yet.
type UnauthenticatedConnection struct {
	base          *baseConnection
	authenticated atomic.Bool
}

// LocalAgent provides info about local agent
func (c *UnauthenticatedConnection) LocalAgent() *Agent {
	return c.base.localAgent
}

// RemoteAgent provides info about remote agent
func (c *UnauthenticatedConnection) RemoteAgent() *Agent {
	return c.base.RemoteAgent()
}

// GetAuthenticationRole determines if the agent should act as presenter or consumer of the PSK.
func (c *UnauthenticatedConnection) GetAuthenticationRole() AuthenticationRole {
	return c.base.GetAuthenticationRole()
}

// GeneratePSK creates a random PSK for the PSK presenter, with the entropy
// both agents require. It is canonical decimal text (see ParsePSK): show it
// to the user with FormatPSK and pass it to AuthenticatePSK.
func (c *UnauthenticatedConnection) GeneratePSK() ([]byte, error) {
	return c.base.GeneratePSK()
}

// AcceptAuthenticate waits until the authentication role is known, after the
// auth-capabilities exchange. The presenter then shows a PSK to the user
// (GeneratePSK, FormatPSK), the consumer asks the user for it; both pass it
// to AuthenticatePSK.
func (c *UnauthenticatedConnection) AcceptAuthenticate(ctx context.Context) (role AuthenticationRole, err error) {
	return c.base.AcceptAuthenticate(ctx)
}

// StartAuthentication asks the PSK presenter to show the PSK when this
// agent is the consumer, see baseConnection.StartAuthentication. Call it
// before asking the user for the PSK.
func (c *UnauthenticatedConnection) StartAuthentication() error {
	return c.base.StartAuthentication()
}

// AuthenticatePSK authenticates the peer with SPAKE2 (network.bs
// "Authentication with SPAKE2") using psk, a base-10 numeric PSK in any form
// ParsePSK accepts. It blocks until authentication completes or ctx is done.
//
// If the peer already initiated, the local agent answers it. Otherwise it
// initiates: the PSK presenter with psk-status psk-shown, the PSK consumer
// with psk-needs-presentation. A consumer that doesn't know the PSK yet asks
// the user for it after AcceptAuthenticate; the presenter's handshake is
// answered once AuthenticatePSK is called.
//
// The spec's edwards25519 suite is not implemented: without
// InteropQuirks.Spake2SuiteP256 it fails with ErrSpake2SuiteUnsupported and
// closes the connection. A malformed psk returns ErrInvalidPSK and leaves the
// connection open.
//
// On the client of a connection made with DialSession, the returned
// Connection must not be used before CompleteSession returns it: until the
// server accepted the session, streams it opens are held.
func (c *UnauthenticatedConnection) AuthenticatePSK(ctx context.Context, psk []byte) (*Connection, error) {
	conn, err := c.base.AuthenticatePSK(ctx, psk)
	if err != nil {
		return nil, err
	}

	// Detach the UnauthenticatedConnection, Close no longer applies.
	c.authenticated.Store(true)

	return conn, nil
}

// SkipAuthentication hands the connection to the application protocol
// without authenticating the peer. INSECURE, for interop debugging only: it
// returns ErrQuirkDisabled unless InteropQuirks.AllowSkipAuthentication is
// set on the local agent, and violates network.bs, which requires
// authentication before exchanging application messages. The returned
// Connection reports Authenticated() == false.
func (c *UnauthenticatedConnection) SkipAuthentication() (*Connection, error) {
	conn, err := c.base.skipAuthentication()
	if err != nil {
		return nil, err
	}
	c.authenticated.Store(true)
	return conn, nil
}

// Close the unauthenticated connection.
// If the connection has progressed to authenticated, it is not closed
// but an error is returned. This allows for defer closing regardless.
func (c *UnauthenticatedConnection) Close() error {
	if c.authenticated.Load() {
		return errors.New("already authenticated")
	}
	return c.base.Close()
}
