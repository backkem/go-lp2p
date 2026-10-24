package ospc

import (
	"context"
	"errors"
	"fmt"
	"sync"

	mdns "github.com/grandcat/zeroconf"
)

var ErrDiscovererClosed = errors.New("discoverer closed")

// Discover agents
func Discover() (*Discoverer, error) {
	d := NewDiscoverer()

	err := d.run()
	if err != nil {
		return nil, err
	}

	return d, nil
}

// Discoverer is used to discover agents.
type Discoverer struct {
	mu sync.Mutex

	remoteNickname *string

	accept chan *DiscoveredAgent

	close    chan struct{}
	closeErr error
	done     chan struct{}
}

// NewDiscoverer creates a new Discoverer
func NewDiscoverer() *Discoverer {
	d := &Discoverer{
		mu:       sync.Mutex{},
		accept:   make(chan *DiscoveredAgent),
		close:    make(chan struct{}),
		closeErr: nil,
		done:     make(chan struct{}),
	}

	return d
}

// WithNickname defines a nickname of agents to discover. Agents match if
// their DNS-SD Instance Name equals the nickname or, if the Instance Name is
// truncated, is a prefix of it. The match is unverified, see
// Agent.DisplayName for the verified display name after authentication.
func (d *Discoverer) WithNickname(nickname string) {
	d.remoteNickname = &nickname
}

// Start discovering agents
func (d *Discoverer) Start() error {
	return d.run()
}

func (d *Discoverer) run() error {
	resolver, err := mdns.NewResolver(nil)
	if err != nil {
		return err
	}

	browseCtx, browseCancel := context.WithCancel(context.Background())

	entries := make(chan *mdns.ServiceEntry)

	// Always use Browse instead of Lookup. Lookup has issues on Windows when
	// server and client run in the same process (common in tests). We filter
	// by nickname client-side instead.
	err = resolver.Browse(browseCtx, MdnsServiceType, MdnsDomain, entries)
	if err != nil {
		browseCancel()
		return err
	}

	acceptCh := d.accept
	closeCh := d.close
	doneCh := d.done
	remoteNickname := d.remoteNickname

	// Run loop
	go func() {
		for {
			select {
			case <-closeCh:
				browseCancel()
				waitCloseMdns(entries)
				close(doneCh)
				return

			case e, ok := <-entries:
				// Ignore firing due to closed channel
				if !ok {
					continue
				}
				agent, err := newDiscoveredAgent(e)
				if err != nil {
					continue
				}
				// Filter by nickname if specified
				if remoteNickname != nil && !agent.instanceName.MatchesNickname(*remoteNickname) {
					continue
				}
				select {
				case acceptCh <- agent:
				case <-closeCh:
				}
			}
		}
	}()

	return nil
}

// waitCloseMdns ensures mdns is fully shutdown.
func waitCloseMdns(entries chan *mdns.ServiceEntry) {
	// Drain channel
	for range entries {
	}
}

// Accept returns an a discovered agent. It should be called in a loop.
func (d *Discoverer) Accept(ctx context.Context) (*DiscoveredAgent, error) {
	d.mu.Lock()
	acceptCh := d.accept
	closeCh := d.close
	d.mu.Unlock()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case a := <-acceptCh:
		return a, nil
	case <-closeCh:
		return nil, d.err()
	}
}

// Close closes the discoverer.
// Any blocked Accept operations will be unblocked and return errors.
func (d *Discoverer) Close() error {
	d.mu.Lock()
	if d.closeErr != nil {
		d.mu.Unlock()
		return d.closeErr
	}

	d.closeErr = ErrDiscovererClosed

	close(d.close)
	done := d.done
	d.mu.Unlock()

	// Block till runLoop is gone
	<-done
	return nil
}

func (d *Discoverer) err() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	return d.closeErr
}

// DiscoveredAgent represents a discovered remote agent that has not been contacted yet.
type DiscoveredAgent struct {
	PeerID PeerID
	// TXT holds the unescaped TXT records.
	TXT  TXTRecordSet
	info *mdns.ServiceEntry

	instanceName       InstanceName
	metadataVersion    uint64
	hasMetadataVersion bool
}

func newDiscoveredAgent(info *mdns.ServiceEntry) (*DiscoveredAgent, error) {
	txt, err := parseDiscoveryTXT(info.Text)
	if err != nil {
		return nil, err
	}

	a, err := newDiscoveredAgentFromTXT(info.ServiceRecord.Instance, txt)
	if err != nil {
		return nil, err
	}
	a.info = info
	return a, nil
}

// parseDiscoveryTXT parses TXT strings as returned by the mDNS resolver, in
// DNS presentation format (e.g. a varuint mv of 0 is "mv=\000").
func parseDiscoveryTXT(text []string) (TXTRecordSet, error) {
	unescaped := make([]string, 0, len(text))
	for _, t := range text {
		unescaped = append(unescaped, unescapeDNSSD(t))
	}
	txt := TXTRecordSet{}
	err := txt.FromSlice(unescaped)
	if err != nil {
		return nil, err
	}
	return txt, nil
}

// newDiscoveredAgentFromTXT builds a DiscoveredAgent from an escaped Instance
// Name and unescaped TXT records.
func newDiscoveredAgentFromTXT(escapedInstance string, txt TXTRecordSet) (*DiscoveredAgent, error) {
	fp, err := txt.GetOne(txtKeyFingerprint)
	if err != nil {
		return nil, fmt.Errorf("failed to get fp record: %v", err)
	}

	a := &DiscoveredAgent{
		PeerID:       PeerID(fp),
		TXT:          txt,
		instanceName: parseInstanceName(escapedInstance),
	}

	// mv is required by the spec, but tolerate its absence: callers then
	// can't tell whether metadata changed and should assume it did.
	if values := txt.Get(txtKeyMetadataVersion); len(values) > 0 {
		mv, err := txt.GetOne(txtKeyMetadataVersion)
		if err != nil {
			return nil, fmt.Errorf("failed to get mv record: %v", err)
		}
		a.metadataVersion, err = decodeMetadataVersion(mv)
		if err != nil {
			return nil, err
		}
		a.hasMetadataVersion = true
	}

	return a, nil
}

// Nickname of the remote agent: its unescaped DNS-SD Instance Name, without
// truncation marker. It is unverified and may be truncated; prefer
// DisplayName for presenting the agent to users.
func (a *DiscoveredAgent) Nickname() string {
	return a.instanceName.Name
}

// InstanceName returns the parsed DNS-SD Instance Name of the agent.
func (a *DiscoveredAgent) InstanceName() InstanceName {
	return a.instanceName
}

// DisplayName returns the name to present for the agent before connecting,
// and its status. The status is DisplayNameTruncated (don't show) or
// DisplayNameUnverified. After authenticating, use Agent.DisplayName on the
// connection's remote agent to get a verified display name.
func (a *DiscoveredAgent) DisplayName() (string, DisplayNameStatus) {
	return a.instanceName.Name, a.instanceName.Status()
}

// MetadataVersion returns the advertised mv value, if present.
func (a *DiscoveredAgent) MetadataVersion() (uint64, bool) {
	return a.metadataVersion, a.hasMetadataVersion
}

// AuthToken returns the advertised at value: the auth-initiation-token to use
// in the first authentication message to this agent.
func (a *DiscoveredAgent) AuthToken() (string, error) {
	at, err := a.TXT.GetOne(txtKeyAuthToken)
	if err != nil {
		return "", fmt.Errorf("failed to get at record: %v", err)
	}
	if err := validateAuthToken(at); err != nil {
		return "", err
	}
	return at, nil
}

// MetadataChangedSince reports whether the agent's metadata may have changed
// since prev was retrieved, i.e. whether connecting to the agent to discover
// updated metadata is warranted. It is conservative: it returns true unless
// both prev and the advertisement carry the same mv for the same PeerID.
//
// The spec requires mv to only increase; any different value is treated as
// changed since a lower value likely indicates the agent lost its state.
func (a *DiscoveredAgent) MetadataChangedSince(prev AgentMetadata) bool {
	if prev.PeerID != a.PeerID {
		return true
	}
	if !a.hasMetadataVersion || !prev.HasMetadataVersion {
		return true
	}
	return a.metadataVersion != prev.MetadataVersion
}
