package ospc

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"sync"

	"github.com/quic-go/quic-go"
)

var _ NetworkTransport = &QuicTransport{}

type QuicTransport struct{}

func NewQuicTransport() *QuicTransport {
	return &QuicTransport{}
}

func (t *QuicTransport) DialAddr(ctx context.Context, addr string, tlsConf *tls.Config) (NetworkConnection, error) {
	qConn, err := quic.DialAddr(ctx, addr, tlsConf, nil)
	if err != nil {
		return nil, err
	}
	return NewQuicNetworkConnection(qConn), nil
}

type ospConnectionIDGenerator struct {
}

func (g *ospConnectionIDGenerator) GenerateConnectionID() (quic.ConnectionID, error) {
	return quic.ConnectionID{}, nil
}

func (g *ospConnectionIDGenerator) ConnectionIDLen() int {
	return 0
}

func (t *QuicTransport) ListenAddr(addr string, tlsConf *tls.Config) (NetworkListener, error) {
	// ListenAddr is a version of quic.ListenAddr that overwrites the
	// ConnectionID behavior to match the OSP zero-length requirement.
	conn, err := listenUDP(addr)
	if err != nil {
		return nil, err
	}
	qConn, err := (&quic.Transport{
		Conn:                  conn,
		ConnectionIDGenerator: &ospConnectionIDGenerator{},
	}).Listen(tlsConf, nil)
	if err != nil {
		return nil, err
	}

	return &QuicNetworkListener{
		listener: qConn,
	}, nil
}

var _ NetworkListener = &QuicNetworkListener{}

type QuicNetworkListener struct {
	listener *quic.Listener
}

func (l *QuicNetworkListener) Accept(ctx context.Context) (NetworkConnection, error) {
	conn, err := l.listener.Accept(ctx)
	if err != nil {
		return nil, err
	}
	return NewQuicNetworkConnection(conn), nil
}

func (l *QuicNetworkListener) Addr() net.Addr {
	return l.listener.Addr()
}

var _ NetworkConnection = &QuicNetworkConnection{}

type QuicNetworkConnection struct {
	conn quic.Connection

	// Read: pipe fed sequentially by run()
	pr *io.PipeReader
	pw *io.PipeWriter

	// Lifecycle
	acceptCancel context.CancelFunc
	doneCh       chan struct{} // closed when run() exits

	mu       sync.Mutex // Protects closeErr
	closeErr error
}

func NewQuicNetworkConnection(conn quic.Connection) *QuicNetworkConnection {
	pr, pw := io.Pipe()
	ctx, cancelFunc := context.WithCancel(context.Background())
	q := &QuicNetworkConnection{
		conn:         conn,
		pr:           pr,
		pw:           pw,
		acceptCancel: cancelFunc,
		doneCh:       make(chan struct{}),
	}
	go q.run(ctx)
	return q
}

func (q *QuicNetworkConnection) run(ctx context.Context) {
	defer close(q.doneCh)
	defer q.pw.CloseWithError(q.closeError())

	streamCh := make(chan quic.ReceiveStream, 16)

	// Accept unidirectional streams (spec-compliant peers)
	go func() {
		for {
			s, err := q.conn.AcceptUniStream(ctx)
			if err != nil {
				return
			}
			streamCh <- s
		}
	}()

	// Accept bidirectional streams (backward compat)
	go func() {
		for {
			s, err := q.conn.AcceptStream(ctx)
			if err != nil {
				return
			}
			streamCh <- s
		}
	}()

	// Process streams one at a time — no interleaving
	for {
		select {
		case s := <-streamCh:
			io.Copy(q.pw, s)
		case <-ctx.Done():
			return
		}
	}
}

func (q *QuicNetworkConnection) Read(p []byte) (int, error) {
	return q.pr.Read(p)
}

// Write opens a new unidirectional stream per call, writes the data,
// and closes the stream (sending FIN). This matches the spec requirement
// of one unidirectional stream per message.
func (q *QuicNetworkConnection) Write(p []byte) (int, error) {
	s, err := q.conn.OpenUniStreamSync(context.Background())
	if err != nil {
		return 0, err
	}
	n, err := s.Write(p)
	if err != nil {
		s.CancelWrite(0)
		return n, err
	}
	if closeErr := s.Close(); closeErr != nil {
		return n, closeErr
	}
	return n, nil
}

func (q *QuicNetworkConnection) IsReliable() bool {
	return true
}

func (q *QuicNetworkConnection) ConnectionState() tls.ConnectionState {
	return q.conn.ConnectionState().TLS
}

func (q *QuicNetworkConnection) closeError() error {
	q.mu.Lock()
	defer q.mu.Unlock()

	return q.closeErr
}

func (q *QuicNetworkConnection) setCloseError(err error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.closeErr = err
}

func (q *QuicNetworkConnection) shutdown(err error) {
	q.setCloseError(err)
	q.acceptCancel()
	q.pw.CloseWithError(err)
	<-q.doneCh
}

func (q *QuicNetworkConnection) IntoApplicationConnection() (ApplicationConnection, error) {
	q.shutdown(ErrTransportHandedOff)
	return &QuicApplicationConnection{conn: q.conn}, nil
}

func (q *QuicNetworkConnection) Close() error {
	q.shutdown(ErrTransportClosed)
	// TODO: refine error?
	return q.conn.CloseWithError(1, "Closed")
}

var _ ApplicationConnection = &QuicApplicationConnection{}

type QuicApplicationConnection struct {
	conn quic.Connection
}

func (q *QuicApplicationConnection) AcceptStream(ctx context.Context) (ApplicationStream, error) {
	s, err := q.conn.AcceptStream(ctx)
	if err != nil {
		return nil, err
	}
	return &QuicApplicationStream{stream: s}, nil
}

func (q *QuicApplicationConnection) OpenStreamSync(ctx context.Context) (ApplicationStream, error) {
	s, err := q.conn.OpenStreamSync(ctx)
	if err != nil {
		return nil, err
	}
	return &QuicApplicationStream{stream: s}, nil
}

func (q *QuicApplicationConnection) Close() error {
	// TODO: refine error?
	return q.conn.CloseWithError(1, "Closed")
}

var _ ApplicationStream = &QuicApplicationStream{}

type QuicApplicationStream struct {
	stream quic.Stream
}

func (s *QuicApplicationStream) Read(p []byte) (int, error) {
	return s.stream.Read(p)
}

func (s *QuicApplicationStream) Write(p []byte) (int, error) {
	return s.stream.Write(p)
}

func (s *QuicApplicationStream) Close() error {
	return s.stream.Close()
}
