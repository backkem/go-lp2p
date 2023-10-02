package ospc

import (
	"sync"

	quic "github.com/quic-go/quic-go"
)

type msgHandler func(msg interface{}, stream *baseStream) error

// baseStream is used to handoff a stream to a DataChannel
type baseStream struct {
	stream quic.Stream

	mu      sync.Mutex
	handler msgHandler
}

func newBaseStream(stream quic.Stream, handler msgHandler) *baseStream {
	return &baseStream{
		stream:  stream,
		mu:      sync.Mutex{},
		handler: handler,
	}
}

func (s *baseStream) Handler() msgHandler {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.handler
}

func (s *baseStream) SetHandler(h msgHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.handler = h
}
