package ua

import (
	"context"
	"sort"
	"sync"
	"time"

	ospc "github.com/backkem/go-lp2p/openscreen-go/network"
)

// NameStatus says how far a peer's display name can be trusted.
type NameStatus = ospc.DisplayNameStatus

// Dialer opens an unauthenticated OSP connection to a peer, plain or with
// the LP2P session extension. An *ospc.DiscoveredAgent is a Dialer.
type Dialer interface {
	Dial(ctx context.Context, transport ospc.AgentTransport, local *ospc.Agent) (*ospc.UnauthenticatedConnection, error)
	DialSession(ctx context.Context, transport ospc.AgentTransport, local *ospc.Agent, origin string) (*ospc.UnauthenticatedConnection, error)
}

// Peer is a remote device as the user agent knows it.
type Peer struct {
	// ID is stable for the peer's identity (its certificate fingerprint).
	// One device advertises every page that receives, all with its ID, so
	// discovered entries are told apart by Instance too.
	ID string
	// Instance is the mDNS instance name the peer was discovered under, or
	// empty.
	Instance string
	// DisplayName is the name to show the user. Check NameStatus before
	// showing it: truncated names shouldn't be shown and unverified names
	// should be marked as such.
	DisplayName string
	NameStatus  NameStatus
	// FirstSeen and LastSeen are when discovery first and last reported
	// the peer.
	FirstSeen, LastSeen time.Time
	// Dialer connects to the peer. Nil if the peer can't be dialed, e.g.
	// for an incoming connection.
	Dialer Dialer `json:"-"`
}

// Label renders the peer's name for UIs, marking its verification status.
func (p Peer) Label() string {
	switch {
	case p.NameStatus.Verified():
		return p.DisplayName + " (verified)"
	case p.NameStatus.Displayable():
		return p.DisplayName + " (unverified)"
	default:
		return "Unnamed device"
	}
}

// PeerEventKind is the kind of change to a PeerList.
type PeerEventKind int

const (
	PeerAdded PeerEventKind = iota + 1
	PeerUpdated
	PeerRemoved
)

func (k PeerEventKind) String() string {
	switch k {
	case PeerAdded:
		return "added"
	case PeerUpdated:
		return "updated"
	case PeerRemoved:
		return "removed"
	default:
		return "unknown"
	}
}

// DiscoveryEvent is reported by a Discoverer. Kind PeerAdded and PeerUpdated
// are equivalent: the peer was seen, with these properties. PeerRemoved
// only needs Peer.ID.
type DiscoveryEvent struct {
	Kind PeerEventKind
	Peer Peer
}

// Discoverer is a source of discovered peers.
type Discoverer interface {
	// Next blocks until a peer is seen or removed, ctx is done or the
	// discoverer is closed.
	Next(ctx context.Context) (DiscoveryEvent, error)
	Close() error
}

// MDNSDiscoverer discovers OSP agents with mDNS, via ospc.
//
// ospc only reports agents as they are seen, so peers discovered this way
// are never removed from the list.
func MDNSDiscoverer() (Discoverer, error) {
	d, err := ospc.Discover()
	if err != nil {
		return nil, err
	}
	return mdnsDiscoverer{d}, nil
}

type mdnsDiscoverer struct{ d *ospc.Discoverer }

func (m mdnsDiscoverer) Next(ctx context.Context) (DiscoveryEvent, error) {
	a, err := m.d.Accept(ctx)
	if err != nil {
		return DiscoveryEvent{}, err
	}
	name, status := a.DisplayName()
	return DiscoveryEvent{
		Kind: PeerAdded,
		Peer: Peer{ID: string(a.PeerID), Instance: a.InstanceName().Name, DisplayName: name, NameStatus: status, Dialer: a},
	}, nil
}

func (m mdnsDiscoverer) Close() error { return m.d.Close() }

// PeerList is the live list of discovered peers.
type PeerList struct {
	mu      sync.Mutex
	peers   map[string]Peer
	changed chan struct{} // closed and replaced on every change
	now     func() time.Time
}

func newPeerList() *PeerList {
	return &PeerList{
		peers:   make(map[string]Peer),
		changed: make(chan struct{}),
		now:     time.Now,
	}
}

// Peers returns a snapshot of the list, sorted by display name and ID.
func (l *PeerList) Peers() []Peer {
	peers, _ := l.snapshot()
	return peers
}

// key tells discovered entries apart: one device advertises every page
// that receives, all with the same ID.
func (p Peer) key() string { return p.ID + "\x00" + p.Instance }

// Get returns a peer with the given ID, any of its instances.
func (l *PeerList) Get(id string) (Peer, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, p := range l.peers {
		if p.ID == id {
			return p, true
		}
	}
	return Peer{}, false
}

// Watch sends a snapshot of the list right away and then after every change,
// until ctx is done; then it closes the channel. Snapshots are coalesced: a
// slow reader gets the latest list, not every intermediate one.
func (l *PeerList) Watch(ctx context.Context) <-chan []Peer {
	out := make(chan []Peer)
	go func() {
		defer close(out)
		for {
			peers, changed := l.snapshot()
			select {
			case out <- peers:
			case <-changed:
				continue // send the newer list instead
			case <-ctx.Done():
				return
			}
			select {
			case <-changed:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out
}

func (l *PeerList) snapshot() ([]Peer, <-chan struct{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	peers := make([]Peer, 0, len(l.peers))
	for _, p := range l.peers {
		peers = append(peers, p)
	}
	sort.Slice(peers, func(i, j int) bool {
		if peers[i].DisplayName != peers[j].DisplayName {
			return peers[i].DisplayName < peers[j].DisplayName
		}
		return peers[i].ID < peers[j].ID
	})
	return peers, l.changed
}

// apply records a discovery event and reports what changed, if anything.
func (l *PeerList) apply(ev DiscoveryEvent) (PeerEventKind, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	id := ev.Peer.key()
	old, exists := l.peers[id]
	var kind PeerEventKind
	switch {
	case ev.Kind == PeerRemoved:
		if !exists {
			return 0, false
		}
		delete(l.peers, id)
		kind = PeerRemoved
	case !exists:
		p := ev.Peer
		p.FirstSeen = l.now()
		p.LastSeen = p.FirstSeen
		l.peers[id] = p
		kind = PeerAdded
	default:
		p := ev.Peer
		p.FirstSeen = old.FirstSeen
		p.LastSeen = l.now()
		if p.Dialer == nil {
			p.Dialer = old.Dialer
		}
		l.peers[id] = p
		if p.DisplayName == old.DisplayName && p.NameStatus == old.NameStatus {
			return 0, false // seen again, nothing to show
		}
		kind = PeerUpdated
	}
	close(l.changed)
	l.changed = make(chan struct{})
	return kind, true
}

// Picker is what the device picker shows: the peers a page may connect to.
type Picker struct {
	// Origin of the page requesting a connection.
	Origin string
	// Nickname the page uses for itself.
	Nickname string

	list *PeerList
}

// NewPicker creates a picker over list, for UIs and tests.
func NewPicker(list *PeerList, origin, nickname string) *Picker {
	return &Picker{Origin: origin, Nickname: nickname, list: list}
}

// Peers returns the peers to show: those with a displayable name.
func (p *Picker) Peers() []Peer {
	return displayable(p.list.Peers())
}

// Watch is PeerList.Watch, limited to the peers to show.
func (p *Picker) Watch(ctx context.Context) <-chan []Peer {
	in := p.list.Watch(ctx)
	out := make(chan []Peer)
	go func() {
		defer close(out)
		for peers := range in {
			select {
			case out <- displayable(peers):
			case <-ctx.Done():
				return
			}
		}
	}()
	return out
}

func displayable(peers []Peer) []Peer {
	out := peers[:0:0]
	for _, p := range peers {
		if p.NameStatus.Displayable() {
			out = append(out, p)
		}
	}
	return out
}
