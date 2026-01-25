package ospc

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	mdns "github.com/grandcat/zeroconf"
)

// unescapeDNSSD removes DNS-SD escaping from instance names.
// The zeroconf library returns names with escaped special chars (e.g., "Test\ Receiver").
func unescapeDNSSD(s string) string {
	return strings.ReplaceAll(s, `\ `, " ")
}

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

// WithNickname defines a nickname of agents to discover.
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
				// Filter by nickname if specified (unescape DNS-SD encoding)
				if remoteNickname != nil && unescapeDNSSD(e.ServiceRecord.Instance) != *remoteNickname {
					continue
				}
				agent, err := newDiscoveredAgent(e)
				if err != nil {
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
	TXT    TXTRecordSet
	info   *mdns.ServiceEntry
}

func newDiscoveredAgent(info *mdns.ServiceEntry) (*DiscoveredAgent, error) {
	txt := TXTRecordSet{}
	err := txt.FromSlice(info.Text)
	if err != nil {
		return nil, err
	}

	fp, err := txt.GetOne("fp")
	if err != nil {
		return nil, fmt.Errorf("failed to get fp record: %v", err)
	}

	return &DiscoveredAgent{
		PeerID: PeerID(fp),
		TXT:    txt,
		info:   info,
	}, nil
}

// Nickname of the remote agent
func (a *DiscoveredAgent) Nickname() string {
	return unescapeDNSSD(a.info.ServiceRecord.Instance)
}
