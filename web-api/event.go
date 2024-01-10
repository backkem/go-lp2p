package web

import (
	"fmt"
	"sync"
)

const HIGH_WATERMARK = 100

type CallbackSetter[T any] func(callback func(e T))

type EventHandler[T any] struct {
	mu sync.Mutex
	b  []T
	cb func(e T)
}

func NewEventHandler[T any]() *EventHandler[T] {
	return &EventHandler[T]{
		mu: sync.Mutex{},
		b:  make([]T, 0),
	}
}

func (h *EventHandler[T]) SetCallback(callback func(T)) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.cb = callback

	// Check if we have any events in the buffer
	h.handle_buffer()
}

// Caller should hold the lock
func (h *EventHandler[T]) handle_buffer() {
	cb := h.cb
	if cb == nil {
		return
	}

	buffer := h.b
	if len(buffer) < 1 {
		return
	}
	h.b = make([]T, 0)

	for i := 0; i < len(buffer); i++ {
		event := buffer[i]
		go cb(event)
	}
}

func (h *EventHandler[T]) OnCallback(e T) {
	h.mu.Lock()
	cb := h.cb
	h.mu.Unlock()

	if cb == nil {
		h.buffer(e)
		return
	}
	cb(e)
}

// Some Web APIs inherently register EventHandlers after they may already fire.
// Therefore we buffer events until the callback is set.
func (h *EventHandler[T]) buffer(e T) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.b = append(h.b, e)

	if len(h.b) > HIGH_WATERMARK {
		fmt.Printf("Event buffer overflow: %d\n", len(h.b))
		h.b = h.b[1:]
	}
}
