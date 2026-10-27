package ua

import (
	"sort"
	"sync"
)

// Local state of the local peer-to-peer manager (index.bs "Local state"),
// kept in memory for the lifetime of the UserAgent:
//
//   - the local trust store: the fingerprints of peers the user paired with
//     (trust anchors scoped to that one peer, as the OSP binding has);
//   - the known peers map: peers discovered or connected;
//   - the peer grants: per origin, the peers it may use.
//
// They are kept consistent: removing a trust anchor removes the grants of
// that peer, removing a known peer removes it from all grants.
type trustState struct {
	mu      sync.Mutex
	anchors map[string]struct{}            // Peer ID -> trusted.
	known   map[string]Peer                // Peer ID -> peer.
	grants  map[string]map[string]struct{} // Origin -> peer IDs.
}

func newTrustState() *trustState {
	return &trustState{
		anchors: make(map[string]struct{}),
		known:   make(map[string]Peer),
		grants:  make(map[string]map[string]struct{}),
	}
}

func (t *trustState) trusted(id string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	_, ok := t.anchors[id]
	return ok
}

func (t *trustState) addAnchor(id string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.anchors[id] = struct{}{}
}

func (t *trustState) addKnown(p Peer) {
	t.mu.Lock()
	defer t.mu.Unlock()
	p.Dialer = nil
	t.known[p.ID] = p
}

func (t *trustState) granted(origin, id string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	_, ok := t.grants[origin][id]
	return ok
}

func (t *trustState) grant(origin, id string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.grants[origin] == nil {
		t.grants[origin] = make(map[string]struct{})
	}
	t.grants[origin][id] = struct{}{}
}

func (t *trustState) ungrantAll(id string) {
	for _, ids := range t.grants {
		delete(ids, id)
	}
}

// TrustAnchors returns the IDs (certificate fingerprints) of the peers the
// user agent trusts: those paired with a PIN, sorted.
func (u *UserAgent) TrustAnchors() []string {
	t := u.trust
	t.mu.Lock()
	defer t.mu.Unlock()
	ids := make([]string, 0, len(t.anchors))
	for id := range t.anchors {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// Trusted reports whether the peer with the given ID is a trust anchor.
func (u *UserAgent) Trusted(id string) bool { return u.trust.trusted(id) }

// KnownPeers returns the peers the user agent had a session with, sorted by
// ID. Whether a peer is trusted follows from TrustAnchors.
func (u *UserAgent) KnownPeers() []Peer {
	t := u.trust
	t.mu.Lock()
	defer t.mu.Unlock()
	peers := make([]Peer, 0, len(t.known))
	for _, p := range t.known {
		peers = append(peers, p)
	}
	sort.Slice(peers, func(i, j int) bool { return peers[i].ID < peers[j].ID })
	return peers
}

// PeerGrants returns the IDs of the peers origin may use, sorted.
func (u *UserAgent) PeerGrants(origin string) []string {
	t := u.trust
	t.mu.Lock()
	defer t.mu.Unlock()
	ids := make([]string, 0, len(t.grants[origin]))
	for id := range t.grants[origin] {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// RemoveTrustAnchor stops trusting a peer. Its grants are removed too, as
// the peer is no longer authenticated. The next session with it needs a
// PIN again.
func (u *UserAgent) RemoveTrustAnchor(id string) {
	t := u.trust
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.anchors, id)
	t.ungrantAll(id)
}

// ForgetPeer removes a known peer and its grants. It stays trusted.
func (u *UserAgent) ForgetPeer(id string) {
	t := u.trust
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.known, id)
	t.ungrantAll(id)
}

// ClearSiteData removes the grants of an origin, as clearing its site data
// or permissions would.
func (u *UserAgent) ClearSiteData(origin string) {
	t := u.trust
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.grants, origin)
}
