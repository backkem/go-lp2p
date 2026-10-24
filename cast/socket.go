package cast

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"sync"
)

// DefaultPort is the TCP port Cast receivers listen on.
const DefaultPort = 8009

// Socket is a framed CastMessage stream over a (normally TLS) connection,
// the Go counterpart of openscreen's CastSocket. Send is safe for concurrent
// use; Receive must be called from a single goroutine (Channel does this).
type Socket struct {
	conn net.Conn
	r    *bufio.Reader

	wmu sync.Mutex

	localCert []byte // DER of the certificate we presented, if known
}

// NewSocket wraps an established connection.
func NewSocket(conn net.Conn) *Socket {
	return &Socket{conn: conn, r: bufio.NewReaderSize(conn, 16*1024)}
}

// Send frames and writes msg.
func (s *Socket) Send(msg *CastMessage) error {
	frame, err := EncodeFrame(msg)
	if err != nil {
		return err
	}
	s.wmu.Lock()
	defer s.wmu.Unlock()
	_, err = s.conn.Write(frame)
	return err
}

// Receive blocks until the next message arrives.
func (s *Socket) Receive() (*CastMessage, error) { return ReadMessage(s.r) }

// Close closes the underlying connection.
func (s *Socket) Close() error { return s.conn.Close() }

// Conn returns the underlying connection.
func (s *Socket) Conn() net.Conn { return s.conn }

// RemoteAddr returns the peer address.
func (s *Socket) RemoteAddr() net.Addr { return s.conn.RemoteAddr() }

// ConnectionState returns the TLS state, ok=false if the socket is not TLS.
func (s *Socket) ConnectionState() (tls.ConnectionState, bool) {
	tc, ok := s.conn.(*tls.Conn)
	if !ok {
		return tls.ConnectionState{}, false
	}
	return tc.ConnectionState(), true
}

// PeerCertificate returns the peer's TLS leaf certificate, if any.
func (s *Socket) PeerCertificate() *x509.Certificate {
	st, ok := s.ConnectionState()
	if !ok || len(st.PeerCertificates) == 0 {
		return nil
	}
	return st.PeerCertificates[0]
}

// LocalCertificate returns the DER of the TLS certificate this side
// presented, when it is known (sockets from Listener, or set explicitly).
func (s *Socket) LocalCertificate() []byte { return s.localCert }

// SetLocalCertificate records the DER of the certificate this side presented.
// Needed by DeviceAuthResponder when the socket was not created by Listener.
func (s *Socket) SetLocalCertificate(der []byte) { s.localCert = der }

// SenderTLSConfig returns the TLS client configuration Cast senders use.
// Cast receivers present short-lived self-signed TLS certificates, so the
// TLS layer does not verify them; trust comes from device authentication,
// which binds the TLS certificate to a device certificate chain (see
// Authenticate). Do not use a socket for anything sensitive before that.
func SenderTLSConfig() *tls.Config {
	return &tls.Config{
		InsecureSkipVerify: true, //nolint:gosec // verified by device auth
		MinVersion:         tls.VersionTLS12,
	}
}

// Dial connects to a Cast receiver at addr (host:port) over TLS. A nil
// tlsConf uses SenderTLSConfig.
func Dial(ctx context.Context, addr string, tlsConf *tls.Config) (*Socket, error) {
	if tlsConf == nil {
		tlsConf = SenderTLSConfig()
	}
	d := tls.Dialer{Config: tlsConf}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}
	s := NewSocket(conn)
	if len(tlsConf.Certificates) > 0 && len(tlsConf.Certificates[0].Certificate) > 0 {
		s.localCert = tlsConf.Certificates[0].Certificate[0]
	}
	return s, nil
}

// Listener accepts Cast sockets over TLS.
type Listener struct {
	ln        net.Listener
	tlsConf   *tls.Config
	localCert []byte
}

// Listen starts a TLS listener for Cast senders. tlsConf must carry the
// receiver's TLS certificate in Certificates[0]; it is also the certificate
// signed during device authentication.
func Listen(addr string, tlsConf *tls.Config) (*Listener, error) {
	if tlsConf == nil || len(tlsConf.Certificates) == 0 || len(tlsConf.Certificates[0].Certificate) == 0 {
		return nil, errors.New("cast: Listen requires a TLS certificate")
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	return &Listener{ln: ln, tlsConf: tlsConf, localCert: tlsConf.Certificates[0].Certificate[0]}, nil
}

// Accept waits for the next sender and completes the TLS handshake.
func (l *Listener) Accept(ctx context.Context) (*Socket, error) {
	type result struct {
		c   net.Conn
		err error
	}
	ch := make(chan result, 1)
	go func() {
		c, err := l.ln.Accept()
		ch <- result{c, err}
	}()
	var raw net.Conn
	select {
	case <-ctx.Done():
		// The pending Accept goroutine finishes when the listener closes or
		// the next connection arrives; close that connection in that case.
		go func() {
			if r := <-ch; r.c != nil {
				_ = r.c.Close()
			}
		}()
		return nil, ctx.Err()
	case r := <-ch:
		if r.err != nil {
			return nil, r.err
		}
		raw = r.c
	}
	tc := tls.Server(raw, l.tlsConf)
	if err := tc.HandshakeContext(ctx); err != nil {
		_ = raw.Close()
		return nil, err
	}
	s := NewSocket(tc)
	s.localCert = l.localCert
	return s, nil
}

// Addr returns the listening address.
func (l *Listener) Addr() net.Addr { return l.ln.Addr() }

// Close stops listening.
func (l *Listener) Close() error { return l.ln.Close() }
