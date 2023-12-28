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
