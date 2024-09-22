package lp2p

import (
	"context"
	"errors"
	"fmt"

	"github.com/backkem/go-lp2p/ospc"
	"github.com/backkem/go-lp2p/streams-api"
	"github.com/backkem/go-lp2p/webtransport-api"
)

type LP2PQuicTransportListener struct {
	Ready              bool
	IncomingTransports streams.ReadableStream[*LP2PQuicTransport]

	accept chan *LP2PQuicTransport
}

// NewLP2PQuicTransportListener accepts a LP2PReceiver or LP2PRequest as a
// source to get transports from.
func NewLP2PQuicTransportListener(source interface{}, options LP2PQuicTransportListenerInit) (*LP2PQuicTransportListener, error) {
	tSource, ok := source.(transportSource)
	if !ok {
		return nil, errors.New("source does not implement transportSource")
	}

	listener := &LP2PQuicTransportListener{
		Ready:  true,
		accept: make(chan *LP2PQuicTransport),
	}
	listener.IncomingTransports = streams.NewValueReadableStream(&transportReader{listener})

	err := tSource.registerTransportListener(listener)
	if err != nil {
		return nil, err
	}
	return listener, nil
}

type LP2PQuicTransportListenerInit struct {
	// TODO:
}

type incomingTransport struct {
	Transport   *ospc.PooledWebTransport
	IsDedicated bool
}

// AcceptTransport new Transports
func (l *LP2PQuicTransportListener) handleTransport(t incomingTransport) {
	if l == nil {
		return
	}
	s := webtransport.NewSessionAdaptor(t.Transport)

	tp, err := createLP2PQuicTransport(s,
		LP2PWebTransportOptions{
			AllowPooling: !t.IsDedicated,
		})
	if err != nil {
		fmt.Println("error handling transport", err)
		return
	}

	accept := l.accept

	accept <- tp
	// TODO: teardown
	// select {
	// case <-close:
	// case accept <- tp:
	// }
}

// AcceptTransport new Transports
func (l *LP2PQuicTransportListener) acceptTransport(ctx context.Context) (*LP2PQuicTransport, error) {
	acceptCh := l.accept

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case a := <-acceptCh:
		return a, nil
		// case <-closeCh: // TODO: teardown
		// 	return nil, errors.New("closed")
	}
}

// transportSource is an interface that allows getting transports form the
// LP2PReceiver, LP2PRequest or LP2PConnection.
type transportSource interface {
	registerTransportListener(listener *LP2PQuicTransportListener) error
}

var _ transportSource = (*LP2PReceiver)(nil)
var _ transportSource = (*LP2PRequest)(nil)

// var _ transportSource = (*LP2PConnection)(nil)

// transportReader is helper to create streams.ReadableStream[*LP2PQuicTransport]
type transportReader struct {
	inner *LP2PQuicTransportListener
}

var _ streams.ValueReader[*LP2PQuicTransport] = (*transportReader)(nil)

func (r *transportReader) Read() (*LP2PQuicTransport, error) {
	t, err := r.inner.acceptTransport(context.Background())
	if err != nil {
		return nil, err
	}

	return t, nil
}
