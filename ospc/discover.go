package ospc

import (
	"context"
	"errors"
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

	accept chan *RemoteAgent

	close    chan struct{}
	closeErr error
	done     chan struct{}
}

// NewDiscoverer creates a new Discoverer
func NewDiscoverer() *Discoverer {
	d := &Discoverer{
		mu:       sync.Mutex{},
		accept:   make(chan *RemoteAgent),
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

	if d.remoteNickname != nil {
		err = resolver.Lookup(browseCtx, *d.remoteNickname, MdnsServiceType, MdnsDomain, entries)
	} else {
		err = resolver.Browse(browseCtx, MdnsServiceType, MdnsDomain, entries)
	}
	if err != nil {
		browseCancel()
		return err
	}

	acceptCh := d.accept
	closeCh := d.close
	doneCh := d.done

	// Run loop
	go func() {
		for {
			select {
			case <-closeCh:
				browseCancel()
				waitCloseMdns(entries)
				close(doneCh)

			case e, ok := <-entries:
				// Ignore firing due to closed channel
				if !ok {
					continue
				}
				agent := &RemoteAgent{
					info: e,
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
drainLoop:
	for {
		select {
		case _, ok := <-entries:
			if !ok {
				break drainLoop
			}
		}
	}
}

// Accept returns an a discovered agent. It should be called in a loop.
func (d *Discoverer) Accept(ctx context.Context) (*RemoteAgent, error) {
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

// RemoteAgent represents a discovered remote agent that has not been contacted yet.
type RemoteAgent struct {
	info *mdns.ServiceEntry
}

// Nickname of the remote agent
func (a *RemoteAgent) Nickname() string {
	return a.info.ServiceRecord.Instance
}
