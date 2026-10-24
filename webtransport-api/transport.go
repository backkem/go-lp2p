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

	return t, nil
}

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
	t.mu.Unlock()

	t.cancel()
	return t.session.CloseWithError(closeInfo.CloseCode, truncateReason(closeInfo.Reason))
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
