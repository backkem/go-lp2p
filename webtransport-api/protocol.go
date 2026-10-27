package webtransport

import (
	"context"
	"io"
)

// Abstractions to allow different underlying protocol implementations.
// These are largely a high-level subset of quic-go's interfaces.

// A Listener
type Listener interface {
	Accept(context.Context) (Session, error)
	Close() error
}

// A Session
type Session interface {
	AcceptStream(context.Context) (Stream, error)
	// AcceptUniStream(context.Context) (ReceiveStream, error)
	OpenStreamSync(context.Context) (Stream, error)
	// OpenUniStreamSync(context.Context) (SendStream, error)
	CloseWithError(uint64, string) error
}

// Optional capabilities of a Session, checked at run time by Transport.

// SessionCloser reports how a session was closed.
type SessionCloser interface {
	// Closed blocks until the session is closed and returns the close info
	// of a clean close, or the error of an abrupt one.
	Closed(context.Context) (WebTransportCloseInfo, error)
}

// SessionDrainer reports the peer's request to drain the session.
type SessionDrainer interface {
	// Draining is closed once the peer asked to finish using the session.
	Draining() <-chan struct{}
}

// SessionExporter derives keying material bound to the session.
type SessionExporter interface {
	ExportKeyingMaterial(label string, context []byte, length int) ([]byte, error)
}

// SessionDatagrams sends and receives datagrams on the session.
type SessionDatagrams interface {
	SendDatagram([]byte) error
	ReceiveDatagram(context.Context) ([]byte, error)
}

// Stream
type Stream interface {
	ReceiveStream
	SendStream
}

// A ReceiveStream is a unidirectional Receive Stream.
type ReceiveStream interface {
	StreamID() int64
	io.Reader
}

// A SendStream is a unidirectional Send Stream.
type SendStream interface {
	StreamID() int64
	io.Writer
	io.Closer
}
