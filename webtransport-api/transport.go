package webtransport

import (
	"context"
	"sync"

	"github.com/backkem/go-lp2p/streams-api"
)

// Transport is the base for implementing a WebTransport. Most of the
// functionality of a Transport is in the base class to allow
// implementing variants that share the same interface.
type Transport struct {
	lock    sync.RWMutex
	session Session

	IncomingBidirectionalStreams streams.ReadableStream[WebTransportBidirectionalStream]
	// IncomingUnidirectionalStreams streams.IterReadableStream[streams.DataReadableStream]

}

func NewTransport(s Session) (*Transport, error) {
	inBi := streams.NewValueReadableStream(&streamReader{
		inner: s,
	})

	base := &Transport{
		lock:                         sync.RWMutex{},
		session:                      s,
		IncomingBidirectionalStreams: inBi,
	}

	return base, nil
}

// CreateBidirectionalStream creates an QuicBidirectionalStream object.
func (b *Transport) CreateBidirectionalStream() (WebTransportBidirectionalStream, error) {
	s, err := b.session.OpenStreamSync(context.Background())
	if err != nil {
		return WebTransportBidirectionalStream{}, err
	}

	return NewWebTransportBidirectionalStream(s), nil
}

// func (b *TransportBase) CreateUnidirectionalStream() (*WritableStream, error) {
// }

type streamReader struct {
	inner Session
}

var _ streams.ValueReader[WebTransportBidirectionalStream] = (*streamReader)(nil)

func (i *streamReader) Read() (WebTransportBidirectionalStream, error) {
	s, err := i.inner.AcceptStream(context.Background())
	if err != nil {
		return WebTransportBidirectionalStream{}, err
	}

	return NewWebTransportBidirectionalStream(s), nil
}

// Close the TransportBase.
func (b *Transport) Close(closeInfo WebTransportCloseInfo) error {
	b.lock.Lock()
	defer b.lock.Unlock()

	if b.session == nil {
		return nil
	}

	if closeInfo.CloseCode > 0 ||
		len(closeInfo.Reason) > 0 {
		return b.session.CloseWithError(closeInfo.CloseCode, closeInfo.Reason)
	}

	return b.session.CloseWithError(0, "close")
}

type WebTransportCloseInfo struct {
	CloseCode uint64
	Reason    string
}
