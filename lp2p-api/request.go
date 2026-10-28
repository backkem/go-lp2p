package lp2p

import (
	"context"
	"errors"
	"sync"
)

// LP2PRequestOptions configures an LP2PRequest.
type LP2PRequestOptions struct {
	// Nickname the page presents itself with to the peer.
	Nickname string
}

// LP2PRequestStartOptions are the options of LP2PRequest.Start.
type LP2PRequestStartOptions struct {
	// Signal aborts Start, as an AbortSignal would: when the context is
	// done, Start fails with the abort reason (context.Cause, as a
	// DOMException) and the picker or the session being set up is taken
	// down. Nil never aborts.
	Signal context.Context
}

// startOp is one run of Start: the [[StartPromise]].
type startOp struct {
	done chan struct{}
	conn *LP2PConnection
	err  error
}

// LP2PRequest requests a connection to a peer the user picks.
type LP2PRequest struct {
	window  *Window
	options LP2PRequestOptions

	mu sync.Mutex
	op *startOp // Pending or succeeded; nil after a failure.
}

// NewLP2PRequest creates an LP2PRequest in this window. The user agent
// starts discovering peers right away, so the picker is populated by the
// time Start is called.
func (w *Window) NewLP2PRequest(options LP2PRequestOptions) (*LP2PRequest, error) {
	if err := w.UserAgent.StartDiscovery(); err != nil {
		return nil, toDOMException(err)
	}
	return &LP2PRequest{window: w, options: options}, nil
}

// Start shows the device picker and connects to the peer the user picks,
// pairing with it if needed. It blocks until the peer accepted the session
// or something failed. Calling Start again while it is pending, or after it
// succeeded, returns the same result and ignores the new options; after a
// failure, Start starts over.
//
// Every failure is a NotAllowedError DOMException, so that a page can't
// tell whether the user dismissed the picker, pairing failed or the peer
// refused the session; the user agent's reason is available with errors.Is
// (ua.ErrCanceled, ua.ErrAuthenticationFailed, ua.ErrRefused, ...). An
// abort through options.Signal fails with the abort reason instead; an
// already aborted signal fails right away. Aborting after Start succeeded
// does nothing: close the connection instead.
func (r *LP2PRequest) Start(options ...LP2PRequestStartOptions) (*LP2PConnection, error) {
	var signal context.Context
	if len(options) > 0 {
		signal = options[0].Signal
	}
	if signal != nil && signal.Err() != nil {
		return nil, abortReason(signal)
	}

	r.mu.Lock()
	if op := r.op; op != nil {
		r.mu.Unlock()
		<-op.done
		return op.conn, op.err
	}
	op := &startOp{done: make(chan struct{})}
	r.op = op
	r.mu.Unlock()

	op.conn, op.err = r.start(signal)
	r.mu.Lock()
	if op.err != nil {
		r.op = nil
	}
	r.mu.Unlock()
	close(op.done)
	return op.conn, op.err
}

func (r *LP2PRequest) start(signal context.Context) (*LP2PConnection, error) {
	ctx := context.Background()
	if signal != nil {
		// Aborting takes down the picker or the session being set up.
		var cancel context.CancelFunc
		ctx, cancel = context.WithCancel(ctx)
		defer cancel()
		defer context.AfterFunc(signal, cancel)()
	}

	u := r.window.UserAgent
	conn, err := u.Connect(ctx, r.window.Origin, r.options.Nickname)
	if signal != nil && signal.Err() != nil {
		if err == nil {
			_ = conn.Close()
		}
		return nil, abortReason(signal)
	}
	if err != nil {
		return nil, newDOMException(NotAllowedError, "connection not allowed: "+err.Error(), err)
	}
	c := newLP2PConnection(conn)
	c.expose()
	return c, nil
}

// abortReason is the error an aborted Start fails with: the signal's cause
// if it is a DOMException, else an AbortError wrapping it.
func abortReason(signal context.Context) error {
	cause := context.Cause(signal)
	var de *DOMException
	if errors.As(cause, &de) {
		return cause
	}
	return newDOMException(AbortError, "the request was aborted", cause)
}
