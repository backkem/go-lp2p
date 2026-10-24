package streams

import (
	"io"
)

// defaultReaderBufferSize is the maximum chunk size returned by a
// DataReadableStreamDefaultReader.
const defaultReaderBufferSize = 64 * 1024

var _ BidirectionalStream[[]byte] = (*DataBidirectionalStream)(nil)

// DataBidirectionalStream represents a bidirectional byte stream.
type DataBidirectionalStream struct {
	DataReadableStream
	DataWritableStream
}

var _ ReadableStream[[]byte] = (*DataReadableStream)(nil)

// DataReadableStream is a readable byte stream backed by an io.Reader.
// Each chunk holds the bytes returned by one Read of the underlying reader
// (at most 64 KiB) and is owned by the caller.
type DataReadableStream struct {
	core    readableCore[[]byte]
	inner   io.Reader
	buf     []byte
	pending error // terminal error returned alongside the last chunk
}

func NewDataReadableStream(reader io.Reader) *DataReadableStream {
	s := &DataReadableStream{
		inner: reader,
	}
	s.core.pull = s.pull
	return s
}

func (s *DataReadableStream) GetReader(opts *ReadableStreamGetReaderOptions) ReadableStreamDefaultReader[[]byte] {
	if !supportedMode(opts) {
		return unsupportedReader[[]byte]{}
	}
	return &DataReadableStreamDefaultReader{stream: s}
}

// pull is called with s.core.mu held.
func (s *DataReadableStream) pull() ([]byte, error) {
	if err := s.pending; err != nil {
		s.pending = nil
		return nil, mapEOF(err)
	}
	if s.buf == nil {
		s.buf = make([]byte, defaultReaderBufferSize)
	}
	for {
		n, err := s.inner.Read(s.buf)
		if n > 0 {
			// io.Reader may return data together with an error (often
			// io.EOF); deliver the data first, the error on the next read.
			s.pending = err
			chunk := make([]byte, n)
			copy(chunk, s.buf[:n])
			return chunk, nil
		}
		if err != nil {
			return nil, mapEOF(err)
		}
		// n == 0 && err == nil: nothing happened, read again.
	}
}

func mapEOF(err error) error {
	if err == io.EOF {
		return errDone
	}
	return err
}

var _ ReadableStreamDefaultReader[[]byte] = (*DataReadableStreamDefaultReader)(nil)

type DataReadableStreamDefaultReader struct {
	stream *DataReadableStream
}

// Read returns the next chunk of bytes, see ReadableStreamDefaultReader.
func (dr *DataReadableStreamDefaultReader) Read() (ReadableStreamReadResult[[]byte], error) {
	return dr.stream.core.read()
}

var _ WritableStream[[]byte] = (*DataWritableStream)(nil)

// DataWritableStream is a writable byte stream backed by an io.WriteCloser.
type DataWritableStream struct {
	core writableCore[[]byte]
}

func NewDataWritableStream(writer io.WriteCloser) *DataWritableStream {
	s := &DataWritableStream{}
	s.core.write = func(b []byte) error {
		_, err := writer.Write(b)
		return err
	}
	s.core.close = writer.Close
	return s
}

func (s *DataWritableStream) GetWriter() WritableStreamDefaultWriter[[]byte] {
	return &DataWritableStreamDefaultWriter{stream: s}
}

func (s *DataWritableStream) Close() error {
	return s.core.closeStream()
}

var _ WritableStreamDefaultWriter[[]byte] = (*DataWritableStreamDefaultWriter)(nil)

type DataWritableStreamDefaultWriter struct {
	stream *DataWritableStream
}

func (dw *DataWritableStreamDefaultWriter) Write(b []byte) error {
	return dw.stream.core.writeChunk(b)
}

func (dw *DataWritableStreamDefaultWriter) Close() error {
	return dw.stream.core.closeStream()
}
