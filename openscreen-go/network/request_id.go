package ospc

import "sync"

// RequestIDAllocator allocates request and watch IDs per remote agent.
//
// Per the OpenScreen Application Protocol, request-id and watch-id values come
// from a counter that starts at 1 and increments by 1 for each ID. IDs are not
// tied to a particular connection: closing and reopening a connection to the
// same peer must not reset the counter. Counters are therefore keyed by the
// remote agent's PeerID (its certificate fingerprint), which identifies the
// peer across reconnects.
//
// The counters are only reset when the local agent changes its state token
// (see Agent.ResetState).
//
// A RequestIDAllocator is safe for concurrent use.
type RequestIDAllocator struct {
	mu   sync.Mutex
	next map[PeerID]uint64
}

// NewRequestIDAllocator returns an empty allocator.
func NewRequestIDAllocator() *RequestIDAllocator {
	return &RequestIDAllocator{
		next: make(map[PeerID]uint64),
	}
}

// Next returns the next request ID for the given peer. The first ID for a
// peer is 1.
func (a *RequestIDAllocator) Next(peer PeerID) uint64 {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.next == nil {
		a.next = make(map[PeerID]uint64)
	}
	id, ok := a.next[peer]
	if !ok {
		id = 1
	}
	a.next[peer] = id + 1
	return id
}

// Reset resets the counters of all peers to 1.
func (a *RequestIDAllocator) Reset() {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.next = make(map[PeerID]uint64)
}

// initStateLocked lazily initializes the local agent state. a.mu must be held.
func (a *Agent) initStateLocked() {
	if a.requestIDs == nil {
		a.requestIDs = NewRequestIDAllocator()
	}
	if a.stateToken == "" {
		a.stateToken = randomAlphaNum(8)
	}
}

// RequestIDs returns the allocator used for request and watch IDs sent by
// this (local) agent. It is shared by all connections of the agent, so IDs
// continue to increase when reconnecting to the same peer.
func (a *Agent) RequestIDs() *RequestIDAllocator {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.initStateLocked()
	return a.requestIDs
}

// NextRequestID returns the next request (or watch) ID to use for a message
// sent by this (local) agent to the given peer.
func (a *Agent) NextRequestID(peer PeerID) uint64 {
	return a.RequestIDs().Next(peer)
}

// StateToken returns the state token of this (local) agent: 8 characters in
// the range [0-9A-Za-z].
func (a *Agent) StateToken() string {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.initStateLocked()
	return a.stateToken
}

// ResetState signals that this (local) agent lost its state: it picks a new
// state token and resets the request ID counters of all peers to 1.
func (a *Agent) ResetState() {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.initStateLocked()
	a.stateToken = randomAlphaNum(8)
	a.requestIDs.Reset()
}
