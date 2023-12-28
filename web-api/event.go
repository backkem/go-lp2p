package web

import (
	"fmt"
	"sync"
)

type CallbackSetter[T any] func(callback func(e T))

type EventHandler[T any] struct {
	mu sync.Mutex
	cb func(e T)
}

func NewEventHandler[T any]() *EventHandler[T] {
	return &EventHandler[T]{
		mu: sync.Mutex{},
	}
}

func (h *EventHandler[T]) SetCallback(callback func(T)) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.cb = callback
}

func (h *EventHandler[T]) OnCallback(e T) {
	h.mu.Lock()
	cb := h.cb
	h.mu.Unlock()

	if cb == nil {
		fmt.Printf("no handler set for %T\n", e)
		return
	}
	cb(e)
}
