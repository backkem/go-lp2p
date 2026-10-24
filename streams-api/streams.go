// Package streams mirrors a subset of the WHATWG Streams API
// (https://streams.spec.whatwg.org/) in idiomatic Go.
//
// Promise-returning methods of the web API are mapped to blocking calls:
// ReadableStreamDefaultReader.read() becomes Read, which blocks until a
// chunk is available and returns a ReadableStreamReadResult ({value, done}).
// A rejected promise becomes a non-nil error.
package streams

import "errors"

var (
	// ErrStreamClosed is returned when writing to or closing a writable
	// stream that is already closed (a TypeError in the web API).
	ErrStreamClosed = errors.New("streams: stream is closed")

	// ErrReaderModeUnsupported is returned by readers obtained with a reader
	// mode the stream does not support (a TypeError in the web API).
	ErrReaderModeUnsupported = errors.New("streams: reader mode not supported")
)

// BidirectionalStream represents a bidirectional stream.
type BidirectionalStream[T any] interface {
	ReadableStream[T]
	WritableStream[T]
}

// ReadableStream mirrors the web ReadableStream.
type ReadableStream[T any] interface {
	// GetReader returns a reader for the stream. Only the default reader
	// mode is supported; requesting another mode returns a reader whose
	// Read fails with ErrReaderModeUnsupported.
	//
	// Unlike the web API, the stream is not locked to the reader: all
	// readers of a stream share its state and reads are serialized.
	GetReader(*ReadableStreamGetReaderOptions) ReadableStreamDefaultReader[T]
}

type ReadableStreamGetReaderOptions struct {
	Mode ReadableStreamReaderMode
}

type ReadableStreamReaderMode string

const (
	ReadableStreamReaderModeDefault ReadableStreamReaderMode = ""
	ReadableStreamReaderModeBYOB    ReadableStreamReaderMode = "byob"

	// Deprecated: misspelling, use ReadableStreamReaderModeBYOB.
	ReadableStreamReaderModeBYOD = ReadableStreamReaderModeBYOB
)

// WritableStream mirrors the web WritableStream.
type WritableStream[T any] interface {
	GetWriter() WritableStreamDefaultWriter[T]
	// Close closes the stream. It returns ErrStreamClosed if the stream
	// was already closed.
	Close() error
}

// WritableStreamDefaultWriter mirrors the web WritableStreamDefaultWriter.
type WritableStreamDefaultWriter[T any] interface {
	// Write blocks until the chunk has been handed to the underlying sink.
	// Writes are processed one at a time, in order. Once a write fails the
	// stream is errored and all further writes return that error. Writing
	// to a closed stream returns ErrStreamClosed.
	Write(T) error
	// Close closes the underlying stream, see WritableStream.Close.
	Close() error
}

// ReadableStreamDefaultReader mirrors the web ReadableStreamDefaultReader.
type ReadableStreamDefaultReader[T any] interface {
	// Read blocks until a chunk is available. At the end of the stream it
	// returns a result with Done set and a zero Val; every following Read
	// does the same. If the stream errors, Read returns that error, now
	// and on every following call.
	Read() (ReadableStreamReadResult[T], error)
}

// ReadableStreamReadResult mirrors the {value, done} result of read().
type ReadableStreamReadResult[T any] struct {
	Val  T
	Done bool
}
