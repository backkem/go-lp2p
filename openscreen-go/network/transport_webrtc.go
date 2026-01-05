package ospc

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/pion/dtls/v3"
	"github.com/pion/logging"
	"github.com/pion/sctp"
)

const MTU = 8192

var _ NetworkTransport = &DTLSTransport{}

type DTLSTransport struct{}

func NewDTLSTransport() *DTLSTransport {
	return &DTLSTransport{}
}

func toConnectionState(dtlsState *dtls.State) tls.ConnectionState {
	peerCertificates := []*x509.Certificate{}

	for i, raw := range dtlsState.PeerCertificates {
		leaf, err := x509.ParseCertificate(raw)
		if err != nil {
			fmt.Printf("warning: failed to parse peer certificate %d: %v", i, err)
			continue
		}
		peerCertificates = append(peerCertificates, leaf)
	}

	// TODO: more complete mapping.
	// The current implementation uses PeerCertificates & NegotiatedProtocol.
	// We make a fake tls.ConnectionState with that info.
	return tls.ConnectionState{
		PeerCertificates:   peerCertificates,
		NegotiatedProtocol: dtlsState.NegotiatedProtocol,
	}
}

func toClientAuthType(typ tls.ClientAuthType) dtls.ClientAuthType {
	switch typ {
	case tls.NoClientCert:
		return dtls.NoClientCert
	case tls.RequestClientCert:
		return dtls.RequestClientCert
	case tls.RequireAnyClientCert:
		return dtls.RequireAnyClientCert
	case tls.VerifyClientCertIfGiven:
		return dtls.VerifyClientCertIfGiven
	case tls.RequireAndVerifyClientCert:
		return dtls.RequireAndVerifyClientCert
	default:
		panic(fmt.Sprintf("unknown tls.ClientAuthType: %v", typ))
	}
}

func toDtlsConfig(tlsConf *tls.Config) *dtls.Config {
	dtlsConfig := &dtls.Config{
		InsecureSkipVerify: tlsConf.InsecureSkipVerify,
		VerifyConnection: func(s *dtls.State) error {
			return tlsConf.VerifyConnection(toConnectionState(s))
		},
		SupportedProtocols:    tlsConf.NextProtos,
		ServerName:            tlsConf.ServerName,
		Certificates:          tlsConf.Certificates,
		ClientAuth:            toClientAuthType(tlsConf.ClientAuth),
		VerifyPeerCertificate: tlsConf.VerifyPeerCertificate,
	}

	return dtlsConfig
}

func (t *DTLSTransport) DialAddr(ctx context.Context, addr string, tlsConf *tls.Config) (NetworkConnection, error) {
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, err
	}

	dtlsConfig := toDtlsConfig(tlsConf)
	dtlsConn, err := dtls.Dial("udp", udpAddr, dtlsConfig)
	if err != nil {
		return nil, err
	}

	if err := dtlsConn.HandshakeContext(ctx); err != nil {
		return nil, fmt.Errorf("dial failed to handshake: %v", err)
	}

	return NewDTLSNetworkConnection(dtlsConn), nil
}

func (t *DTLSTransport) ListenAddr(addr string, tlsConf *tls.Config) (NetworkListener, error) {
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, err
	}

	dtlsConfig := toDtlsConfig(tlsConf)
	inner, err := dtls.Listen("udp", udpAddr, dtlsConfig)
	if err != nil {
		return nil, err
	}

	return &DTLSNetworkListener{
		listener: inner,
	}, nil
}

var _ NetworkListener = &DTLSNetworkListener{}

type DTLSNetworkListener struct {
	listener net.Listener
}

func (l *DTLSNetworkListener) Accept(ctx context.Context) (NetworkConnection, error) {
	_ = ctx
	conn, err := l.listener.Accept()
	if err != nil {
		return nil, err
	}
	dtlsConn := conn.(*dtls.Conn)
	if err := dtlsConn.HandshakeContext(ctx); err != nil {
		return nil, fmt.Errorf("accept failed to handshake: %v", err)
	}
	return NewDTLSNetworkConnection(dtlsConn), nil
}

func (l *DTLSNetworkListener) Addr() net.Addr {
	return l.listener.Addr()
}

// DTLS connection wrapper that can be handed off between NetworkConnection and
// ApplicationConnection.
type dtlsConnectionBase struct {
	conn *dtls.Conn

	bufCh chan []byte
	lenCh chan int

	rdDoneCh   chan struct{}
	shutdownCh chan struct{}
	doneCh     chan struct{}

	mu       sync.Mutex // Protects state
	closeErr error
}

func newDtlsConnectionBase(conn *dtls.Conn) *dtlsConnectionBase {
	c := &dtlsConnectionBase{
		conn:       conn,
		bufCh:      make(chan []byte),
		lenCh:      make(chan int),
		rdDoneCh:   make(chan struct{}),
		shutdownCh: make(chan struct{}),
		doneCh:     make(chan struct{}),
	}

	go c.run()

	return c
}

// Creates a new dtlsConnectionBase, unblocking all read/writes on the original
// dtlsConnectionBase but keeping the original read loop.
func (c *dtlsConnectionBase) Handoff() error {
	c.setCloseError(ErrTransportHandedOff)
	close(c.rdDoneCh)

	return nil
}

func (c *dtlsConnectionBase) ReadInterruptible(p []byte) (int, error) {
	select {
	case c.bufCh <- p:
		nr := <-c.lenCh
		return nr, nil
	case <-c.rdDoneCh:
		return 0, c.closeError()
	case <-c.doneCh:
		return 0, c.closeError()
	}
}

func (c *dtlsConnectionBase) Read(p []byte) (int, error) {
	select {
	case c.bufCh <- p:
		nr := <-c.lenCh
		return nr, nil
	case <-c.doneCh:
		return 0, c.closeError()
	}
}

func (c *dtlsConnectionBase) Write(p []byte) (int, error) {
	return c.conn.Write(p)
}

func (c *dtlsConnectionBase) run() {
	// Buf should be large enough to hold the largest message we expect to
	// receive in the network protocol.
	buf := make([]byte, 8192)
readLoop:
	for {
		select {
		case <-c.shutdownCh:
			close(c.doneCh)
			return

		default:
			nr, err := c.conn.Read(buf)
			if err != nil {
				close(c.doneCh)
				return
			}

			b := buf[:nr]
			for once := true; once || len(b) > 0; once = false {
				select {
				case p := <-c.bufCh:
					n := copy(p, b)
					c.lenCh <- n
					b = b[n:]

				case <-c.shutdownCh:
					// Continue to proper shutdown
					continue readLoop
				}
			}
		}
	}
}

func (c *dtlsConnectionBase) closeError() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.closeErr
}

func (c *dtlsConnectionBase) setCloseError(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.closeErr = err
}

func (c *dtlsConnectionBase) LocalAddr() net.Addr {
	return c.conn.LocalAddr()
}

func (c *dtlsConnectionBase) RemoteAddr() net.Addr {
	return c.conn.RemoteAddr()
}

func (c *dtlsConnectionBase) SetDeadline(t time.Time) error {
	return c.conn.SetDeadline(t)
}

func (c *dtlsConnectionBase) SetReadDeadline(t time.Time) error {
	return c.conn.SetReadDeadline(t)
}

func (c *dtlsConnectionBase) SetWriteDeadline(t time.Time) error {
	return c.conn.SetWriteDeadline(t)
}

func (c *dtlsConnectionBase) Close() error {
	c.setCloseError(ErrTransportClosed)

	close(c.shutdownCh)
	err := c.conn.Close()

	// Wait for read loop to exit.
	<-c.doneCh

	return err
}

var _ NetworkConnection = &QuicNetworkConnection{}

type DTLSNetworkConnection struct {
	base *dtlsConnectionBase

	mu       sync.Mutex // Protects state
	closeErr error
}

func NewDTLSNetworkConnection(conn *dtls.Conn) *DTLSNetworkConnection {
	base := newDtlsConnectionBase(conn)
	return &DTLSNetworkConnection{
		base: base,
	}
}

func (c *DTLSNetworkConnection) Read(p []byte) (int, error) {
	err := c.closeError()
	if err != nil {
		return 0, err
	}
	return c.base.ReadInterruptible(p)
}

func (c *DTLSNetworkConnection) Write(p []byte) (int, error) {
	err := c.closeError()
	if err != nil {
		return 0, err
	}
	return c.base.Write(p)
}

func (c *DTLSNetworkConnection) IsReliable() bool {
	return false
}

func (c *DTLSNetworkConnection) ConnectionState() tls.ConnectionState {
	dtlsState, ok := c.base.conn.ConnectionState()
	if !ok {
		fmt.Println("warning: ConnectionState called before ConnectionState was set")
		return tls.ConnectionState{}
	}

	return toConnectionState(&dtlsState)
}

func (c *DTLSNetworkConnection) closeError() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.closeErr
}

func (c *DTLSNetworkConnection) setCloseError(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.closeErr = err
}

func (c *DTLSNetworkConnection) shutdown(err error) {
	c.setCloseError(err)
}

func (c *DTLSNetworkConnection) IntoApplicationConnection() (ApplicationConnection, error) {
	c.setCloseError(ErrTransportHandedOff)

	inner := c.base
	err := inner.Handoff()
	if err != nil {
		return nil, err
	}

	sctpAssociation, err := sctp.Client(sctp.Config{
		NetConn:            inner,
		EnableZeroChecksum: false,
		LoggerFactory:      logging.NewDefaultLoggerFactory(),
	})
	if err != nil {
		return nil, err
	}

	return &SCTPApplicationConnection{sctpAssociation: sctpAssociation}, nil
}

func (c *DTLSNetworkConnection) Close() error {
	c.shutdown(ErrTransportClosed)
	return c.base.Close()
}

var _ ApplicationConnection = &QuicApplicationConnection{}

type SCTPApplicationConnection struct {
	sctpAssociation *sctp.Association

	mu            sync.Mutex
	streamCounter uint16
}

func (c *SCTPApplicationConnection) AcceptStream(ctx context.Context) (ApplicationStream, error) {
	_ = ctx // TODO: can we wire this up?
	s, err := c.sctpAssociation.AcceptStream()
	if err != nil {
		return nil, err
	}
	return NewSCTPApplicationStream(s), nil
}

func (c *SCTPApplicationConnection) nextStreamIdentifier() uint16 {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.streamCounter++

	return c.streamCounter
}

func (c *SCTPApplicationConnection) OpenStreamSync(ctx context.Context) (ApplicationStream, error) {
	streamId := c.nextStreamIdentifier()
	s, err := c.sctpAssociation.OpenStream(streamId, sctp.PayloadTypeWebRTCBinary)
	if err != nil {
		return nil, err
	}
	return NewSCTPApplicationStream(s), nil
}

func (c *SCTPApplicationConnection) Close() error {
	return c.sctpAssociation.Close()
}

var _ ApplicationStream = &SCTPApplicationStream{}

type SCTPApplicationStream struct {
	stream *sctp.Stream

	mu             sync.Mutex
	rdBuf          []byte
	rdBufRemaining []byte
}

func NewSCTPApplicationStream(stream *sctp.Stream) *SCTPApplicationStream {
	return &SCTPApplicationStream{
		stream: stream,
		rdBuf:  make([]byte, MTU),
	}
}

func (s *SCTPApplicationStream) Read(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.rdBufRemaining) > 0 {
		return s.readRemaining(p)
	}

	n, err := s.stream.Read(s.rdBuf)
	if err != nil {
		return n, err
	}
	s.rdBufRemaining = s.rdBuf[:n]

	return s.readRemaining(p)
}

// Caller must hold the lock.
func (s *SCTPApplicationStream) readRemaining(p []byte) (int, error) {
	n := copy(p, s.rdBufRemaining)
	s.rdBufRemaining = s.rdBufRemaining[n:]
	return n, nil
}

func (s *SCTPApplicationStream) Write(p []byte) (int, error) {
	return writeChunked(s.stream, p)
}

func (s *SCTPApplicationStream) Close() error {
	return s.stream.Close()
}

func writeChunked(dst io.Writer, p []byte) (int, error) {
	b := p
	nr := 0
	for len(b) > 0 {
		chunk := b
		if len(chunk) > MTU {
			chunk = chunk[:MTU]
		}
		n, err := dst.Write(chunk)
		if err != nil {
			return nr, err
		}
		nr += n
		b = b[n:]
	}
	return nr, nil
}
