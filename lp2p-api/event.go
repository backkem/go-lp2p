package lp2p

import "sync"

// maxBufferedEvents bounds events held while no handler is set.
const maxBufferedEvents = 4096

// eventSlot is one event handler attribute (onmessage, onclose, ...).
//
// Events fired before a handler is set are buffered and delivered, in order,
// when it is set. In a browser that rarely matters, because events wait for
// the script's current task to finish; here callbacks run on Go routines, so
// without buffering a handler set right after construction could miss
// events. Delivery is serialized: handlers see events in the order fired.
type eventSlot[T any] struct {
	deliver sync.Mutex // held while calling the handler

	mu  sync.Mutex
	cb  func(T)
	buf []T
}

func (s *eventSlot[T]) set(cb func(T)) {
	s.mu.Lock()
	s.cb = cb
	pending := cb != nil && len(s.buf) > 0
	s.mu.Unlock()
	if !pending {
		return
	}
	if s.deliver.TryLock() {
		defer s.deliver.Unlock()
		s.flush()
		return
	}
	// A delivery is in progress, possibly the caller's own handler; flush
	// once it is done rather than deadlock.
	go func() {
		s.deliver.Lock()
		defer s.deliver.Unlock()
		s.flush()
	}()
}

func (s *eventSlot[T]) fire(e T) {
	s.deliver.Lock()
	defer s.deliver.Unlock()
	s.mu.Lock()
	if len(s.buf) >= maxBufferedEvents {
		s.buf = s.buf[1:]
	}
	s.buf = append(s.buf, e)
	s.mu.Unlock()
	s.flush()
}

// flush delivers buffered events; the caller holds deliver.
func (s *eventSlot[T]) flush() {
	for {
		s.mu.Lock()
		cb := s.cb
		if cb == nil || len(s.buf) == 0 {
			s.mu.Unlock()
			return
		}
		e := s.buf[0]
		s.buf = s.buf[1:]
		s.mu.Unlock()
		cb(e)
	}
}
