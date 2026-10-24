package streams

import (
	"errors"
	"io"
)

var (
	// ErrValueReaderDone is returned by a ValueReader at the end of its
	// values. io.EOF is accepted as well.
	ErrValueReaderDone error = errors.New("iter done reading")
)

// ValueReader is the underlying source of a ValueReadableStream.
type ValueReader[T any] interface {
	Read() (T, error)
}

// ValueWriteCloser is the underlying sink of a ValueWritableStream.
type ValueWriteCloser[T any] interface {
	Write(T) error
	Close() error
}

var _ BidirectionalStream[struct{}] = (*ValueBidirectionalStream[struct{}])(nil)

// ValueBidirectionalStream represents a bidirectional stream.
type ValueBidirectionalStream[T any] struct {
	ValueReadableStream[T]
	ValueWritableStream[T]
}

var _ ReadableStream[struct{}] = (*ValueReadableStream[struct{}])(nil)

// ValueReadableStream is a readable stream of values pulled from a
// ValueReader.
type ValueReadableStream[T any] struct {
	core readableCore[T]
}

func NewValueReadableStream[T any](iter ValueReader[T]) *ValueReadableStream[T] {
	s := &ValueReadableStream[T]{}
	s.core.pull = func() (T, error) {
		val, err := iter.Read()
		if err == ErrValueReaderDone || err == io.EOF {
			return val, errDone
		}
		return val, err
	}
	return s
}

func (s *ValueReadableStream[T]) GetReader(opts *ReadableStreamGetReaderOptions) ReadableStreamDefaultReader[T] {
	if !supportedMode(opts) {
		return unsupportedReader[T]{}
	}
	return &ValueReadableStreamDefaultReader[T]{stream: s}
}

var _ ReadableStreamDefaultReader[struct{}] = (*ValueReadableStreamDefaultReader[struct{}])(nil)

type ValueReadableStreamDefaultReader[T any] struct {
	stream *ValueReadableStream[T]
}

// Read returns the next value, see ReadableStreamDefaultReader.
func (dr *ValueReadableStreamDefaultReader[T]) Read() (ReadableStreamReadResult[T], error) {
	return dr.stream.core.read()
}

var _ WritableStream[struct{}] = (*ValueWritableStream[struct{}])(nil)

// ValueWritableStream is a writable stream of values backed by a
// ValueWriteCloser.
type ValueWritableStream[T any] struct {
	core writableCore[T]
}

func NewValueWritableStream[T any](writer ValueWriteCloser[T]) *ValueWritableStream[T] {
	s := &ValueWritableStream[T]{}
	s.core.write = writer.Write
	s.core.close = writer.Close
	return s
}

func (s *ValueWritableStream[T]) GetWriter() WritableStreamDefaultWriter[T] {
	return &ValueWritableStreamDefaultWriter[T]{stream: s}
}

func (s *ValueWritableStream[T]) Close() error {
	return s.core.closeStream()
}

var _ WritableStreamDefaultWriter[struct{}] = (*ValueWritableStreamDefaultWriter[struct{}])(nil)

type ValueWritableStreamDefaultWriter[T any] struct {
	stream *ValueWritableStream[T]
}

func (dw *ValueWritableStreamDefaultWriter[T]) Write(val T) error {
	return dw.stream.core.writeChunk(val)
}

func (dw *ValueWritableStreamDefaultWriter[T]) Close() error {
	return dw.stream.core.closeStream()
}
