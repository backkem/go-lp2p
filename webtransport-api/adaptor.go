package webtransport

import (
	"context"
)

type ConcreteListener[S Session] interface {
	Accept(context.Context) (S, error)
	Close() error
}

// ListenerAdaptor is a shim to convert concrete implementation
// to a generic one.
type ListenerAdaptor[S Session] struct {
	inner ConcreteListener[S]
}

func NewListenerAdaptor[S Session](l ConcreteListener[S]) *ListenerAdaptor[S] {
	return &ListenerAdaptor[S]{
		inner: l,
	}
}

func (a *ListenerAdaptor[S]) Accept(ctx context.Context) (Session, error) {
	// Avoid returning a non-nil interface holding a nil pointer.
	s, err := a.inner.Accept(ctx)
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (a *ListenerAdaptor[S]) Close() error {
	return a.inner.Close()
}

type ConcreteSession[S Stream] interface {
	AcceptStream(context.Context) (S, error)
	OpenStreamSync(context.Context) (S, error)
	CloseWithError(uint64, string) error
}

// SessionAdaptor is a shim to convert concrete implementation
// to a generic one.
type SessionAdaptor[S Stream] struct {
	inner ConcreteSession[S]
}

func NewSessionAdaptor[S Stream](s ConcreteSession[S]) *SessionAdaptor[S] {
	return &SessionAdaptor[S]{
		inner: s,
	}
}

func (a *SessionAdaptor[S]) AcceptStream(ctx context.Context) (Stream, error) {
	// Avoid returning a non-nil interface holding a nil pointer.
	s, err := a.inner.AcceptStream(ctx)
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (a *SessionAdaptor[S]) OpenStreamSync(ctx context.Context) (Stream, error) {
	// Avoid returning a non-nil interface holding a nil pointer.
	s, err := a.inner.OpenStreamSync(ctx)
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (a *SessionAdaptor[S]) CloseWithError(i uint64, msg string) error {
	return a.inner.CloseWithError(i, msg)
}
