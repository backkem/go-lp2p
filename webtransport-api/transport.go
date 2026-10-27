package webtransport

import (
	"context"
	"errors"
	"sync"
	"unicode/utf8"

	"github.com/backkem/go-lp2p/streams-api"
)

// ErrTransportClosed is returned when using a Transport after Close
// (an InvalidStateError in the web API).
var ErrTransportClosed = errors.New("webtransport: transport is closed")

// ErrNotSupported is returned by operations the underlying session doesn't
// provide, such as datagrams or keying material exporters.
var ErrNotSupported = errors.New("webtransport: not supported by the session")

// maxReasonLength is the maximum length of a close reason in bytes.
const maxReasonLength = 1024

// Transport is the base for implementing a WebTransport. Most of the
// functionality of a Transport is in the base class to allow
// implementing variants that share the same interface.
type Transport struct {
	mu      sync.Mutex
	session Session
	closed  bool

	// ctx is cancelled on Close, unblocking pending accepts and opens.
	ctx    context.Context
	cancel context.CancelFunc

	// IncomingBidirectionalStreams yields the streams opened by the peer.
	// It is done once the transport is closed locally; it errors if the
	// underlying session fails.
	IncomingBidirectionalStreams streams.ReadableStream[WebTransportBidirectionalStream]
	// IncomingUnidirectionalStreams streams.IterReadableStream[streams.DataReadableStream]

	// Datagrams sends and receives datagrams, if the session supports
	// them (SessionDatagrams); nil otherwise.
	Datagrams *WebTransportDatagramDuplexStream

	closeInfo WebTransportCloseInfo // Set by Close.
}

func NewTransport(s Session) (*Transport, error) {
	if s == nil {
		return nil, errors.New("webtransport: nil session")
	}

	ctx, cancel := context.WithCancel(context.Background())
	t := &Transport{
		session: s,
		ctx:     ctx,
		cancel:  cancel,
	}
	t.IncomingBidirectionalStreams = streams.NewValueReadableStream(&streamReader{t: t})
	if d, ok := s.(SessionDatagrams); ok {
		t.Datagrams = &WebTransportDatagramDuplexStream{
			Readable: streams.NewValueReadableStream[[]byte](datagramReader{t, d}),
			Writable: streams.NewValueWritableStream[[]byte](datagramWriter{t, d}),
		}
	}
	return t, nil
}

// Closed blocks until the transport is closed, as the closed promise. It
// returns the close info of a clean close, by Close or by the peer, or the
// error of an abrupt one. Without a SessionCloser it only reports local
// closes.
func (t *Transport) Closed(ctx context.Context) (WebTransportCloseInfo, error) {
	if c, ok := t.session.(SessionCloser); ok {
		return c.Closed(ctx)
	}
	select {
	case <-ctx.Done():
		return WebTransportCloseInfo{}, ctx.Err()
	case <-t.ctx.Done():
		t.mu.Lock()
		defer t.mu.Unlock()
		return t.closeInfo, nil
	}
}

// Draining blocks until the peer asks to finish using the transport, as the
// draining promise. It fails with ErrTransportClosed if the transport is
// closed first, and with ErrNotSupported without a SessionDrainer.
func (t *Transport) Draining(ctx context.Context) error {
	d, ok := t.session.(SessionDrainer)
	if !ok {
		return ErrNotSupported
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.ctx.Done():
		return ErrTransportClosed
	case <-d.Draining():
		return nil
	}
}

// ExportKeyingMaterial derives keying material bound to the session, as
// exportKeyingMaterial(). It fails with ErrNotSupported without a
// SessionExporter.
func (t *Transport) ExportKeyingMaterial(label string, context []byte, length int) ([]byte, error) {
	e, ok := t.session.(SessionExporter)
	if !ok {
		return nil, ErrNotSupported
	}
	return e.ExportKeyingMaterial(label, context, length)
}

// WebTransportDatagramDuplexStream sends and receives datagrams.
type WebTransportDatagramDuplexStream struct {
	Readable streams.ReadableStream[[]byte]
	Writable streams.WritableStream[[]byte]
}

type datagramReader struct {
	t *Transport
	d SessionDatagrams
}

func (r datagramReader) Read() ([]byte, error) {
	p, err := r.d.ReceiveDatagram(r.t.ctx)
	if err != nil && r.t.ctx.Err() != nil {
		return nil, streams.ErrValueReaderDone
	}
	return p, err
}

type datagramWriter struct {
	t *Transport
	d SessionDatagrams
}

func (w datagramWriter) Write(p []byte) error {
	w.t.mu.Lock()
	closed := w.t.closed
	w.t.mu.Unlock()
	if closed {
		return ErrTransportClosed
	}
	return w.d.SendDatagram(p)
}

func (w datagramWriter) Close() error { return nil }

// CreateBidirectionalStream opens a new bidirectional stream. It returns
// ErrTransportClosed once the transport is closed, including when Close is
// called while the stream is being opened.
func (t *Transport) CreateBidirectionalStream() (WebTransportBidirectionalStream, error) {
	t.mu.Lock()
	closed := t.closed
	t.mu.Unlock()
	if closed {
		return WebTransportBidirectionalStream{}, ErrTransportClosed
	}

	s, err := t.session.OpenStreamSync(t.ctx)
	if err != nil {
		if t.ctx.Err() != nil {
			return WebTransportBidirectionalStream{}, ErrTransportClosed
		}
		return WebTransportBidirectionalStream{}, err
	}

	return NewWebTransportBidirectionalStream(s), nil
}

// func (b *TransportBase) CreateUnidirectionalStream() (*WritableStream, error) {
// }

type streamReader struct {
	t *Transport
}

var _ streams.ValueReader[WebTransportBidirectionalStream] = (*streamReader)(nil)

func (r *streamReader) Read() (WebTransportBidirectionalStream, error) {
	s, err := r.t.session.AcceptStream(r.t.ctx)
	if err != nil {
		if r.t.ctx.Err() != nil {
			// Closed locally: the incoming stream ends gracefully.
			return WebTransportBidirectionalStream{}, streams.ErrValueReaderDone
		}
		return WebTransportBidirectionalStream{}, err
	}

	return NewWebTransportBidirectionalStream(s), nil
}

// Close closes the transport with the given close info. Pending and future
// stream operations fail or end. Closing an already closed transport is a
// no-op. Reasons longer than 1024 bytes are truncated, as in the web API.
func (t *Transport) Close(closeInfo WebTransportCloseInfo) error {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return nil
	}
	t.closed = true
	closeInfo.Reason = truncateReason(closeInfo.Reason)
	t.closeInfo = closeInfo
	t.mu.Unlock()
	t.cancel()
	return t.session.CloseWithError(closeInfo.CloseCode, closeInfo.Reason)
}

// truncateReason truncates s to maxReasonLength bytes without splitting a
// UTF-8 sequence.
func truncateReason(s string) string {
	if len(s) <= maxReasonLength {
		return s
	}
	n := maxReasonLength
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

type WebTransportCloseInfo struct {
	CloseCode uint64
	Reason    string
}
