package streams

import (
	"errors"
	"sync"
)

// errDone is returned by a pull function to signal the end of the stream.
var errDone = errors.New("streams: done")

// readableCore holds the state shared by a readable stream and its readers:
// reads are serialized, and the closed (done) and errored states are sticky.
type readableCore[T any] struct {
	mu   sync.Mutex
	done bool
	err  error
	// pull returns the next chunk, errDone at the end of the stream or
	// another error if the stream failed.
	pull func() (T, error)
}

func (c *readableCore[T]) read() (ReadableStreamReadResult[T], error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.err != nil {
		return ReadableStreamReadResult[T]{}, c.err
	}
	if c.done {
		return ReadableStreamReadResult[T]{Done: true}, nil
	}

	val, err := c.pull()
	if err == errDone {
		c.done = true
		return ReadableStreamReadResult[T]{Done: true}, nil
	}
	if err != nil {
		c.err = err
		return ReadableStreamReadResult[T]{}, err
	}
	return ReadableStreamReadResult[T]{Val: val}, nil
}

// writableCore holds the state shared by a writable stream and its writers:
// writes are serialized, closing is final and a failed write errors the
// stream.
type writableCore[T any] struct {
	mu     sync.Mutex
	closed bool
	err    error
	write  func(T) error
	close  func() error
}

func (c *writableCore[T]) writeChunk(val T) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.err != nil {
		return c.err
	}
	if c.closed {
		return ErrStreamClosed
	}
	if err := c.write(val); err != nil {
		c.err = err
		return err
	}
	return nil
}

func (c *writableCore[T]) closeStream() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return ErrStreamClosed
	}
	c.closed = true
	// Close the sink even if the stream is errored, to release it.
	err := c.close()
	if c.err != nil {
		return c.err
	}
	return err
}

// unsupportedReader is returned for unsupported reader modes.
type unsupportedReader[T any] struct{}

func (unsupportedReader[T]) Read() (ReadableStreamReadResult[T], error) {
	return ReadableStreamReadResult[T]{}, ErrReaderModeUnsupported
}

func supportedMode(opts *ReadableStreamGetReaderOptions) bool {
	return opts == nil || opts.Mode == ReadableStreamReaderModeDefault
}
