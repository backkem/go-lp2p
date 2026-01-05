package ospc

import (
	"context"
	"crypto/tls"
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
	conn    quic.Connection
	wStream quic.Stream

	bufCh chan []byte
	lenCh chan int

	acceptCancel context.CancelFunc
	shutdownCh   chan struct{}
	doneCh       chan struct{}

	mu       sync.Mutex // Protects state
	closeErr error
}

func NewQuicNetworkConnection(conn quic.Connection) *QuicNetworkConnection {
	ctx, acceptCancelFunc := context.WithCancel(context.Background())

	q := &QuicNetworkConnection{
		conn:         conn,
		bufCh:        make(chan []byte),
		lenCh:        make(chan int),
		acceptCancel: acceptCancelFunc,
		shutdownCh:   make(chan struct{}),
		doneCh:       make(chan struct{}),
	}
	go q.run(ctx)
	return q
}

func (q *QuicNetworkConnection) run(ctx context.Context) {
	rStreams := make([]quic.ReceiveStream, 0)
	for {
		select {
		case <-q.shutdownCh:
			// Stop receiving
			for _, s := range rStreams {
				s.CancelRead(0)
			}
			close(q.doneCh)

			return
		default:
			s, err := q.conn.AcceptStream(ctx)
			if err != nil {
				// Continue to proper shutdown
				continue
			}
			rStreams = append(rStreams, s)
			go q.handleIncoming(s)
		}
	}
}

func (q *QuicNetworkConnection) Read(p []byte) (int, error) {
	select {
	case q.bufCh <- p:
		nr := <-q.lenCh
		return nr, nil
	case <-q.doneCh:
		return 0, q.closeError()
	}
}

func (q *QuicNetworkConnection) Write(p []byte) (int, error) {
	if q.wStream == nil {
		s, err := q.conn.OpenStreamSync(context.Background())
		if err != nil {
			return 0, err
		}
		q.wStream = s
	}
	return q.wStream.Write(p)
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

func (q *QuicNetworkConnection) handleIncoming(s quic.ReceiveStream) {
	for {
		select {
		case p := <-q.bufCh:
			nr, err := s.Read(p)
			q.lenCh <- nr
			if err != nil {
				return
			}

		case <-q.doneCh:
			return
		}
	}
}

func (q *QuicNetworkConnection) shutdown(err error) {
	q.setCloseError(err)
	// Stop accepting new streams
	q.acceptCancel()
	// Signal read loop to shutdown
	close(q.shutdownCh)
	// Ensure read loop is gone
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
