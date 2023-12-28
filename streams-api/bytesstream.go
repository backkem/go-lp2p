package streams

import (
	"io"
	"math"
)

const defaultReaderBufferSize = math.MaxUint16

var _ BidirectionalStream[[]byte] = (*DataBidirectionalStream)(nil)

// DataBidirectionalStream represents a bidirectional stream.
type DataBidirectionalStream struct {
	DataReadableStream
	DataWritableStream
}

var _ ReadableStream[[]byte] = (*DataReadableStream)(nil)

// DataReadableStream represents a basic byte stream
type DataReadableStream struct {
	inner io.Reader
}

func NewDataReadableStream(reader io.Reader) *DataReadableStream {
	return &DataReadableStream{
		inner: reader,
	}
}

func (s *DataReadableStream) GetReader(opts *ReadableStreamGetReaderOptions) interface{} {
	return &DataReadableStreamDefaultReader{
		inner: s.inner,
	}
}

type DataReadableStreamDefaultReader struct {
	inner io.Reader
}

func (dr *DataReadableStreamDefaultReader) Read() (ReadableStreamReadResult[[]byte], error) {
	val := make([]byte, defaultReaderBufferSize)
	_, err := dr.inner.Read(val)
	if err != nil {
		if err == io.EOF {
			return ReadableStreamReadResult[[]byte]{
				Done: true,
			}, nil
		}
		return ReadableStreamReadResult[[]byte]{}, err
	}
	return ReadableStreamReadResult[[]byte]{
		Val:  val,
		Done: false,
	}, nil
}

var _ WritableStream[[]byte] = (*DataWritableStream)(nil)

// DataWritableStream represents a basic byte stream
type DataWritableStream struct {
	inner io.WriteCloser
}

func NewDataWritableStream(writer io.WriteCloser) *DataWritableStream {
	return &DataWritableStream{
		inner: writer,
	}
}

func (s *DataWritableStream) GetWriter() WritableStreamDefaultWriter[[]byte] {
	return &DataWritableStreamDefaultWriter{
		inner: s.inner,
	}
}

func (dr *DataWritableStream) Close() error {
	return dr.inner.Close()
}

type DataWritableStreamDefaultWriter struct {
	inner io.WriteCloser
}

func (dr *DataWritableStreamDefaultWriter) Write(b []byte) error {
	_, err := dr.inner.Write(b)
	return err
}

func (dr *DataWritableStreamDefaultWriter) Close() error {
	return dr.inner.Close()
}
