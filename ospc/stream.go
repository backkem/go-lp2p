package ospc

import (
	"sync"
)

type msgHandler func(msg interface{}, stream *baseStream) error

// baseStream is used to handoff a stream to a DataChannel
type baseStream struct {
	stream ApplicationStream

	mu      sync.Mutex
	handler msgHandler
}

func newBaseStream(stream ApplicationStream, handler msgHandler) *baseStream {
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
