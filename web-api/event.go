// Package web mirrors small pieces of the web platform needed by the other
// shims, such as event handler attributes.
package web

import (
	"sync"
)

// HIGH_WATERMARK is the maximum number of events an EventHandler buffers
// while no callback is set. When it is exceeded the oldest event is dropped.
const HIGH_WATERMARK = 100

// CallbackSetter mirrors an event handler attribute such as
// `receiver.onconnection = cb`.
type CallbackSetter[T any] func(callback func(e T))

// EventHandler delivers events of type T to a single callback.
//
// Delivery is ordered: the callback sees events in the order OnCallback was
// called, and is never invoked concurrently with itself, similar to a web
// event loop.
//
// Some web APIs inherently register event handlers after the events may
// already have fired. Events raised while no callback is set are therefore
// buffered (up to HIGH_WATERMARK, dropping the oldest) and delivered once a
// callback is set.
type EventHandler[T any] struct {
	mu          sync.Mutex
	queue       []T
	cb          func(e T)
	dispatching bool
}

func NewEventHandler[T any]() *EventHandler[T] {
	return &EventHandler[T]{}
}

// SetCallback sets (or, with nil, clears) the callback. Buffered events are
// delivered to the new callback on a separate goroutine, so SetCallback does
// not block on them; events raised in the meantime are delivered after them.
func (h *EventHandler[T]) SetCallback(callback func(T)) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.cb = callback
	if h.cb != nil && len(h.queue) > 0 && !h.dispatching {
		h.dispatching = true
		go h.dispatch()
	}
}

// OnCallback raises an event. If a callback is set and no other delivery is
// in progress, the callback runs synchronously before OnCallback returns.
// Otherwise the event is queued behind the events being delivered, or
// buffered until a callback is set.
func (h *EventHandler[T]) OnCallback(e T) {
	h.mu.Lock()
	h.queue = append(h.queue, e)
	if h.cb == nil {
		if len(h.queue) > HIGH_WATERMARK {
			var zero T
			h.queue[0] = zero
			h.queue = h.queue[1:]
		}
		h.mu.Unlock()
		return
	}
	if h.dispatching {
		// The goroutine currently dispatching (possibly our own caller,
		// re-entrantly from the callback) will deliver it in order.
		h.mu.Unlock()
		return
	}
	h.dispatching = true
	h.mu.Unlock()

	h.dispatch()
}

// dispatch delivers queued events one at a time until the queue is empty or
// the callback is cleared. The caller must have set h.dispatching.
func (h *EventHandler[T]) dispatch() {
	for {
		h.mu.Lock()
		if h.cb == nil || len(h.queue) == 0 {
			h.dispatching = false
			if len(h.queue) == 0 {
				h.queue = nil
			}
			h.mu.Unlock()
			return
		}
		cb := h.cb
		e := h.queue[0]
		var zero T
		h.queue[0] = zero
		h.queue = h.queue[1:]
		h.mu.Unlock()

		h.deliver(cb, e)
	}
}

// deliver runs the callback. If it panics, dispatching is reset so the
// handler keeps working for later events, and the panic propagates.
func (h *EventHandler[T]) deliver(cb func(T), e T) {
	ok := false
	defer func() {
		if !ok {
			h.mu.Lock()
			h.dispatching = false
			h.mu.Unlock()
		}
	}()
	cb(e)
	ok = true
}
