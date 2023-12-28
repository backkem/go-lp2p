package streams

import (
	"errors"
)

var (
	ErrValueReaderDone error = errors.New("iter done reading")
)

type ValueReader[T any] interface {
	Read() (T, error)
}

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

type ValueReadableStream[T any] struct {
	inner ValueReader[T]
}

func NewValueReadableStream[T any](iter ValueReader[T]) *ValueReadableStream[T] {
	return &ValueReadableStream[T]{
		inner: iter,
	}
}

func (s *ValueReadableStream[T]) GetReader(opts *ReadableStreamGetReaderOptions) interface{} {
	// if opts != nil && opts.Mode == ReadableStreamReaderModeBYOD {
	// 	return &IterReadableStreamBYOBReader[T]{
	// 		inner: s.inner,
	// 	}
	// }
	return &ValueReadableStreamDefaultReader[T]{
		inner: s.inner,
	}
}

type ValueReadableStreamDefaultReader[T any] struct {
	inner ValueReader[T]
}

func (dr *ValueReadableStreamDefaultReader[T]) Read() (ReadableStreamReadResult[T], error) {
	val, err := dr.inner.Read()
	if err != nil {
		if err == ErrValueReaderDone {
			return ReadableStreamReadResult[T]{
				Done: true,
			}, nil
		}
		return ReadableStreamReadResult[T]{}, err
	}
	return ReadableStreamReadResult[T]{
		Val:  val,
		Done: false,
	}, nil
}

var _ WritableStream[struct{}] = (*ValueWritableStream[struct{}])(nil)

// ValueWritableStream[T] represents a basic byte stream
type ValueWritableStream[T any] struct {
	inner ValueWriteCloser[T]
}

func NewValueWritableStream[T any](writer ValueWriteCloser[T]) *ValueWritableStream[T] {
	return &ValueWritableStream[T]{
		inner: writer,
	}
}

func (s *ValueWritableStream[T]) GetWriter() WritableStreamDefaultWriter[T] {
	return &ValueWritableStreamDefaultWriter[T]{
		inner: s.inner,
	}
}

func (dr *ValueWritableStream[T]) Close() error {
	return dr.inner.Close()
}

type ValueWritableStreamDefaultWriter[T any] struct {
	inner ValueWriteCloser[T]
}

func (dr *ValueWritableStreamDefaultWriter[T]) Write(val T) error {
	return dr.inner.Write(val)
}

func (dr *ValueWritableStreamDefaultWriter[T]) Close() error {
	return dr.inner.Close()
}
