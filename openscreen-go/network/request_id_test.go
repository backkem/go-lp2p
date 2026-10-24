package ospc

import (
	"sort"
	"sync"
	"testing"
)

func TestRequestIDAllocatorPerPeer(t *testing.T) {
	a := NewRequestIDAllocator()

	for want := uint64(1); want <= 3; want++ {
		if got := a.Next("peer-a"); got != want {
			t.Fatalf("peer-a: got %d, want %d", got, want)
		}
	}
	// Other peers have independent counters.
	if got := a.Next("peer-b"); got != 1 {
		t.Fatalf("peer-b: got %d, want 1", got)
	}
	if got := a.Next("peer-a"); got != 4 {
		t.Fatalf("peer-a: got %d, want 4", got)
	}

	a.Reset()
	if got := a.Next("peer-a"); got != 1 {
		t.Fatalf("peer-a after reset: got %d, want 1", got)
	}
	if got := a.Next("peer-b"); got != 1 {
		t.Fatalf("peer-b after reset: got %d, want 1", got)
	}
}

func TestRequestIDAllocatorZeroValue(t *testing.T) {
	var a RequestIDAllocator
	if got := a.Next("peer-a"); got != 1 {
		t.Fatalf("got %d, want 1", got)
	}
}

func TestRequestIDAllocatorConcurrent(t *testing.T) {
	const (
		workers = 16
		perWork = 200
	)
	a := NewRequestIDAllocator()
	peers := []PeerID{"peer-a", "peer-b"}

	var mu sync.Mutex
	got := map[PeerID][]uint64{}

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		peer := peers[w%len(peers)]
		wg.Add(1)
		go func() {
			defer wg.Done()
			ids := make([]uint64, 0, perWork)
			for i := 0; i < perWork; i++ {
				ids = append(ids, a.Next(peer))
			}
			mu.Lock()
			got[peer] = append(got[peer], ids...)
			mu.Unlock()
		}()
	}
	wg.Wait()

	// Each peer must have received exactly 1..n without gaps or duplicates.
	for _, peer := range peers {
		ids := got[peer]
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		for i, id := range ids {
			if id != uint64(i+1) {
				t.Fatalf("%s: id[%d] = %d, want %d", peer, i, id, i+1)
			}
		}
		if want := workers / len(peers) * perWork; len(ids) != want {
			t.Fatalf("%s: got %d ids, want %d", peer, len(ids), want)
		}
	}
}

func newTestLocalAgent(t *testing.T) *Agent {
	t.Helper()
	a, err := NewAgent(NewAgentConfig("local"))
	if err != nil {
		t.Fatalf("NewAgent: %v", err)
	}
	return a
}

// TestRequestIDsSurviveReconnect checks that request IDs are not tied to a
// connection: a new connection to the same peer (same fingerprint, new
// remote Agent instance as created by NewRemoteAgent) continues the counter.
func TestRequestIDsSurviveReconnect(t *testing.T) {
	local := newTestLocalAgent(t)

	conn1 := newBaseConnection(nil, local, &Agent{PeerID: "peer-a"}, AgentRoleClient, "")
	for want := uint64(1); want <= 3; want++ {
		if got := conn1.nextRequestID(); got != want {
			t.Fatalf("conn1: got %d, want %d", got, want)
		}
	}

	// Reconnect to the same peer.
	conn2 := newBaseConnection(nil, local, &Agent{PeerID: "peer-a"}, AgentRoleClient, "")
	if got := conn2.nextRequestID(); got != 4 {
		t.Fatalf("conn2 after reconnect: got %d, want 4", got)
	}

	// A different peer has its own counter.
	conn3 := newBaseConnection(nil, local, &Agent{PeerID: "peer-b"}, AgentRoleServer, "")
	if got := conn3.nextRequestID(); got != 1 {
		t.Fatalf("conn3 (other peer): got %d, want 1", got)
	}

	// The application layer allocates from the same counter.
	if got := local.NextRequestID("peer-a"); got != 5 {
		t.Fatalf("agent: got %d, want 5", got)
	}

	// A different local agent has its own counters.
	other := newTestLocalAgent(t)
	if got := other.NextRequestID("peer-a"); got != 1 {
		t.Fatalf("other local agent: got %d, want 1", got)
	}
}

// TestResetState checks that changing the state token resets the counters.
func TestResetState(t *testing.T) {
	local := newTestLocalAgent(t)

	token := local.StateToken()
	if len(token) != 8 {
		t.Fatalf("state token %q: want 8 characters", token)
	}
	if again := local.StateToken(); again != token {
		t.Fatalf("state token changed without reset: %q -> %q", token, again)
	}

	local.NextRequestID("peer-a")
	local.NextRequestID("peer-a")
	local.NextRequestID("peer-b")

	local.ResetState()
	if local.StateToken() == token {
		t.Fatalf("state token not changed by ResetState")
	}
	if got := local.NextRequestID("peer-a"); got != 1 {
		t.Fatalf("peer-a after reset: got %d, want 1", got)
	}
	if got := local.NextRequestID("peer-b"); got != 1 {
		t.Fatalf("peer-b after reset: got %d, want 1", got)
	}
}

// TestRequestIDsConcurrentConnections allocates from several connections to
// the same peer concurrently; IDs must be unique.
func TestRequestIDsConcurrentConnections(t *testing.T) {
	const (
		conns   = 8
		perConn = 100
	)
	local := newTestLocalAgent(t)

	var mu sync.Mutex
	seen := map[uint64]bool{}

	var wg sync.WaitGroup
	for c := 0; c < conns; c++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			conn := newBaseConnection(nil, local, &Agent{PeerID: "peer-a"}, AgentRoleClient, "")
			for i := 0; i < perConn; i++ {
				id := conn.nextRequestID()
				mu.Lock()
				if seen[id] {
					t.Errorf("duplicate request ID %d", id)
				}
				seen[id] = true
				mu.Unlock()
			}
		}()
		// Interleave with state reads on the same agent.
		go func() { _ = local.StateToken() }()
	}
	wg.Wait()

	if len(seen) != conns*perConn {
		t.Fatalf("got %d unique IDs, want %d", len(seen), conns*perConn)
	}
}

func TestKeepAliveUsesPeerRequestIDs(t *testing.T) {
	alloc := NewRequestIDAllocator()
	alloc.Next("peer") // Taken by an earlier request to the same peer.

	sent := make(chan msgRequestId, 1)
	k := newKeepAlive(KeepAliveConfig{}, func(msg interface{}) error {
		sent <- msg.(*msgAgentStatusRequest).RequestId
		return nil
	}, discardLogger)
	k.setRequestIDSource(func() uint64 { return alloc.Next("peer") })
	k.fire()

	if got := <-sent; got != 2 {
		t.Fatalf("keep-alive request ID: got %d, want 2", got)
	}
	if got := alloc.Next("peer"); got != 3 {
		t.Fatalf("next request ID after keep-alive: got %d, want 3", got)
	}
}
