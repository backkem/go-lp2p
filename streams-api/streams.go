package streams

// BidirectionalStream represents a bidirectional stream.
type BidirectionalStream[T any] interface {
	ReadableStream[T]
	WritableStream[T]
}

type ReadableStream[T any] interface {
	GetReader(*ReadableStreamGetReaderOptions) interface{}
}

type ReadableStreamGetReaderOptions struct {
	Mode ReadableStreamReaderMode
}

type ReadableStreamReaderMode string

const (
	ReadableStreamReaderModeDefault ReadableStreamReaderMode = ""
	ReadableStreamReaderModeBYOD    ReadableStreamReaderMode = "byob"
)

type WritableStream[T any] interface {
	GetWriter() WritableStreamDefaultWriter[T]
	Close() error
}

type WritableStreamDefaultWriter[T any] interface {
	// Ready() error
	Write(T) error
	Close() error
}

type ReadableStreamDefaultReader[T any] interface {
	Read() (ReadableStreamReadResult[T], error)
	// Closed() error
	// Cancel(reason string) error
}

// type ReadableStreamBYOBReader[T any] interface {
// 	Read(*T, *ReadableStreamBYOBReaderReadOptions) (ReadableStreamReadResult[T], error)
// 	// Closed() error
// 	// Cancel(reason string) error
// }
//
// type ReadableStreamBYOBReaderReadOptions struct {
// 	Min int
// }

type ReadableStreamReadResult[T any] struct {
	Val  T
	Done bool
}
