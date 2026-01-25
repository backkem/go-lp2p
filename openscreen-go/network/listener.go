package ospc

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"

	mdns "github.com/grandcat/zeroconf"
)

var ErrListenerClosed = errors.New("listener closed")

// Listen starts an advertising agent and listens for incoming connections.
func Listen(transportType AgentTransport, a *Agent) (*Listener, error) {
	l := NewListener(a, transportType)

	err := l.run()
	if err != nil {
		return nil, err
	}

	return l, nil
}

// Listener acts as an advertising OSP agent and listens for incoming
// connections.
type Listener struct {
	mu sync.Mutex

	agent         *Agent
	transportType AgentTransport

	accept chan *UnauthenticatedConnection

	alpnListeners map[string]*ALPNListener

	close    chan struct{}
	closeErr error
	done     chan struct{}
}

// NewListener creates a new Listener
func NewListener(a *Agent, transportType AgentTransport) *Listener {
	l := &Listener{
		mu:            sync.Mutex{},
		agent:         a,
		transportType: transportType,
		accept:        make(chan *UnauthenticatedConnection),
		close:         make(chan struct{}),
		closeErr:      nil,
		done:          make(chan struct{}),
	}

	return l
}

// ListenApplication allows you to listen for quic connections
// on the same port but with a different ALPN. Only one per ALPN
// is allowed. Needs to be registered before starting the Listener.
func (l *Listener) ListenApplication(alpn string, config *ALPNListenerConfig) *ALPNListener {
	child := newALPNListener(l, config)

	l.registerALPNListener(alpn, child)

	return child
}

func (l *Listener) registerALPNListener(alpn string, child *ALPNListener) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.alpnListeners[alpn] = child
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

	child := l.alpnListeners[alpn]
	return child
}

func (l *Listener) Start() error {
	return l.run()
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

func (l *Listener) run() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	var pendingConns []*baseConnection
	pendingCh := make(chan exchangeInfoResult)

	acceptCh := l.accept
	closeCh := l.close
	doneCh := l.done

	// List all registered ALPN_OSPs.
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
			if len(cs.PeerCertificates) != 1 {
				return errors.New("didn't expect cert chain")
			}

			alpn := cs.NegotiatedProtocol
			if alpn != ALPN_OSP {
				return l.getALPNListener(alpn).doVerifyConnection(cs)
			}

			peerCert := cs.PeerCertificates[0]
			roots := x509.NewCertPool()
			roots.AddCert(peerCert)

			// Extract expected hostname from peer certificate for validation
			expectedCN := peerCert.Subject.CommonName
			
			opts := x509.VerifyOptions{
				DNSName: expectedCN,
				Roots:   roots,
			}
			_, err := peerCert.Verify(opts)

			return err
		},
	}

	t, err := NewNetworkTransport(l.transportType)
	if err != nil {
		return err
	}
	listener, err := t.ListenAddr(":", tlsConfig)
	if err != nil {
		return err
	}

	fp, err := l.agent.CertificateFingerPrint()
	if err != nil {
		return err
	}

	mvBuf := new(bytes.Buffer)
	writeVaruint(0, mvBuf) // TODO: metadata updates
	mv := mvBuf.String()

	at := randomAT(9)

	// Advertise ourselves
	txt := TXTRecordSet{}
	txt.Set("fp", fp)
	txt.Set("mv", mv)
	txt.Set("at", at)
	txt.Set("fn", "TODO:Remove?")
	// Encode certificate serial number as URL-safe base64 (no padding).
	// Spec says RFC4648 base64, but standard base64 contains +/= which are
	// invalid in DNS labels. See: https://github.com/w3c/openscreenprotocol/issues/365
	snBytes := l.agent.Certificate.Leaf.SerialNumber.Bytes()
	snBase64 := base64.RawURLEncoding.EncodeToString(snBytes)
	txt.Set("sn", snBase64) // TODO: openscreenprotocol#293
	port := listener.Addr().(*net.UDPAddr).Port
	advertiser, err := mdns.Register(l.agent.info.DisplayName, MdnsServiceType, MdnsDomain, port, txt.ToSlice(), nil)
	if err != nil {
		return err
	}

	acceptCtx, acceptCancel := context.WithCancel(context.Background())
	netConns := make(chan NetworkConnection)
	go func() {
		for {
			nc, err := listener.Accept(acceptCtx)
			if err != nil {
				fmt.Printf("AcceptListener error: %s\n", err)
				// TODO: Close early here?
				return
			}

			alpn := nc.ConnectionState().NegotiatedProtocol
			if alpn != ALPN_OSP {
				if qnc, ok := nc.(*QuicNetworkConnection); ok {
					l.getALPNListener(alpn).dispatch(qnc.conn)
				} else {
					panic("TODO: generalize ALPNListener")
				}
				continue
			}

			select {
			case netConns <- nc:
			case <-closeCh:
				return
			}
		}
	}()

	// Run loop
	go func() {
		for {
			select {
			case <-closeCh: // Shutdown initiated
				advertiser.Shutdown()
				acceptCancel()

				for _, conn := range pendingConns {
					_ = conn.Close()
				}

				close(doneCh)
				return

			case nc := <-netConns: // Incoming connection
				remoteAgent, err := l.agent.NewRemoteAgent(nc)
				if err != nil {
					fmt.Printf("failed to create remote agent: %v\n", err)
					continue
				}
				bConn := newBaseConnection(
					nc,
					l.agent,
					remoteAgent,
					AgentRoleServer,
				)

				bConn.runNetwork()

				err = bConn.exchangeInfo(context.Background(), pendingCh)
				if err != nil {
					fmt.Printf("failed to exchange metadata: %v\n", err)
					bConn.closeWithError(fmt.Errorf("failed to exchange metadata: %v", err))
				} else {
					pendingConns = append(pendingConns, bConn)
				}

			case res := <-pendingCh: // Connection with metadata available
				bConn, err := res.conn, res.err

				pendingConns = removeConn(pendingConns, bConn)

				if err != nil {
					break
				}
				uConn := &UnauthenticatedConnection{
					base: bConn,
				}

				select {
				case acceptCh <- uConn:
				case <-closeCh:
				}
			}
		}
	}()

	return nil
}

func removeConn(set []*baseConnection, conn *baseConnection) []*baseConnection {
	for i := 0; i < len(set); i++ {
		if set[i] == conn {
			set[i] = set[len(set)-1]
			return set[:len(set)-1]
		}
	}
	return set
}

// Accept returns an a discovered agent. It should be called in a loop.
func (l *Listener) Accept(ctx context.Context) (*UnauthenticatedConnection, error) {
	l.mu.Lock()
	acceptCh := l.accept
	closeCh := l.close
	l.mu.Unlock()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case a := <-acceptCh:
		return a, nil
	case <-closeCh:
		return nil, l.err()
	}
}

// func (l *Listener) Addr() net.Addr {
// }

// Close closes the listener.
// Any blocked Accept operations will be unblocked and return errors.
func (l *Listener) Close() error {
	l.mu.Lock()
	if l.closeErr != nil {
		l.mu.Unlock()
		return l.closeErr
	}

	l.closeErr = ErrListenerClosed

	close(l.close)
	done := l.done
	l.mu.Unlock()

	// Block till runLoop is gone
	<-done
	return nil
}

func (l *Listener) err() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	return l.closeErr
}

// UnauthenticatedConnection represents an OSPC connection that didn't pass
// authentication yet.
type UnauthenticatedConnection struct {
	base *baseConnection
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

// RequestAuthenticatePSK is used to request authentication as an initiating
// collector agent.
func (c *UnauthenticatedConnection) RequestAuthenticatePSK() error {
	return c.base.RequestAuthenticatePSK()
}

// GeneratePSK creates a PSK based on the negotiated config.
func (c *UnauthenticatedConnection) GeneratePSK() ([]byte, error) {
	return c.base.GeneratePSK()
}

// AcceptAuthenticate is used to handle an incoming authentication request.
// It has to be called for every UnauthenticatedConnection.
func (c *UnauthenticatedConnection) AcceptAuthenticate(ctx context.Context) (role AuthenticationRole, err error) {
	return c.base.AcceptAuthenticate(ctx)
}

// Authenticate is used to authenticate. It will block until authentication is complete
// or the context is closed.
func (c *UnauthenticatedConnection) AuthenticatePSK(ctx context.Context, psk []byte) (*Connection, error) {
	base := c.base

	conn, err := base.AuthenticatePSK(ctx, psk)
	if err != nil {
		return nil, err
	}

	// detach the UnauthenticatedConnection
	c.base = nil

	return conn, nil
}

// Close the unauthenticated connection.
// If the connection has progressed to authenticated, it is not closed
// but an error is returned. This allows for defer closing regardless.
func (c *UnauthenticatedConnection) Close() error {
	if c.base == nil {
		return errors.New("already authenticated")
	}
	return c.base.Close()
}
