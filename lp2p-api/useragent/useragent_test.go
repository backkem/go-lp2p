package ua

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	ospc "github.com/backkem/go-lp2p/openscreen-go/network"
)

// fakeDiscoverer is a Discoverer driven by the test.
type fakeDiscoverer struct {
	events chan DiscoveryEvent
	closed chan struct{}
}

func newFakeDiscoverer() *fakeDiscoverer {
	return &fakeDiscoverer{events: make(chan DiscoveryEvent), closed: make(chan struct{})}
}

func (f *fakeDiscoverer) Next(ctx context.Context) (DiscoveryEvent, error) {
	select {
	case ev := <-f.events:
		return ev, nil
	case <-ctx.Done():
		return DiscoveryEvent{}, ctx.Err()
	case <-f.closed:
		return DiscoveryEvent{}, ErrClosed
	}
}

func (f *fakeDiscoverer) Close() error {
	close(f.closed)
	return nil
}

func (f *fakeDiscoverer) emit(t *testing.T, kind PeerEventKind, id, name string, status NameStatus) {
	t.Helper()
	select {
	case f.events <- DiscoveryEvent{Kind: kind, Peer: Peer{ID: id, DisplayName: name, NameStatus: status}}:
	case <-time.After(5 * time.Second):
		t.Fatal("discovery loop not consuming events")
	}
}

func newTestUA(t *testing.T, ui UI) (*UserAgent, *fakeDiscoverer) {
	t.Helper()
	fd := newFakeDiscoverer()
	u, err := New(Config{Name: "test", UI: ui, Discover: func() (Discoverer, error) { return fd, nil }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = u.Close() })
	return u, fd
}

// waitFor reads snapshots from w until ok accepts one.
func waitFor(t *testing.T, w <-chan []Peer, what string, ok func([]Peer) bool) []Peer {
	t.Helper()
	timeout := time.After(5 * time.Second)
	for {
		select {
		case peers, open := <-w:
			if !open {
				t.Fatalf("watch closed waiting for %s", what)
			}
			if ok(peers) {
				return peers
			}
		case <-timeout:
			t.Fatalf("timed out waiting for %s", what)
		}
	}
}

func names(peers []Peer) string {
	var s []string
	for _, p := range peers {
		s = append(s, p.DisplayName)
	}
	return strings.Join(s, ",")
}

func TestNewRequiresUI(t *testing.T) {
	if _, err := New(Config{}); err == nil {
		t.Fatal("New without a UI succeeded")
	}
}

func TestPeerListUpdates(t *testing.T) {
	u, fd := newTestUA(t, &Scripted{})
	if err := u.StartDiscovery(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := u.Peers().Watch(ctx)
	waitFor(t, w, "initial empty list", func(p []Peer) bool { return len(p) == 0 })

	fd.emit(t, PeerAdded, "b", "Bedroom", ospc.DisplayNameUnverified)
	fd.emit(t, PeerAdded, "a", "Attic", ospc.DisplayNameUnverified)
	waitFor(t, w, "two peers, sorted", func(p []Peer) bool { return names(p) == "Attic,Bedroom" })

	// Seen again unchanged: no update. Renamed: update, FirstSeen kept.
	first, _ := u.Peers().Get("a")
	fd.emit(t, PeerUpdated, "a", "Attic", ospc.DisplayNameUnverified)
	fd.emit(t, PeerUpdated, "a", "Loft", ospc.DisplayNameUnverified)
	waitFor(t, w, "rename", func(p []Peer) bool { return names(p) == "Bedroom,Loft" })
	if p, _ := u.Peers().Get("a"); !p.FirstSeen.Equal(first.FirstSeen) || p.LastSeen.Before(first.LastSeen) {
		t.Errorf("seen times not maintained: first %v -> %v, last %v -> %v", first.FirstSeen, p.FirstSeen, first.LastSeen, p.LastSeen)
	}

	fd.emit(t, PeerRemoved, "b", "", 0)
	waitFor(t, w, "removal", func(p []Peer) bool { return names(p) == "Loft" })

	cancel()
	for range w { // closes once ctx is done
	}
}

func TestPeerListApply(t *testing.T) {
	l := newPeerList()
	ev := func(kind PeerEventKind, id, name string) DiscoveryEvent {
		return DiscoveryEvent{Kind: kind, Peer: Peer{ID: id, DisplayName: name, NameStatus: ospc.DisplayNameUnverified}}
	}
	steps := []struct {
		ev      DiscoveryEvent
		kind    PeerEventKind
		changed bool
	}{
		{ev(PeerAdded, "x", "X"), PeerAdded, true},
		{ev(PeerAdded, "x", "X"), 0, false},
		{ev(PeerUpdated, "x", "Y"), PeerUpdated, true},
		{ev(PeerRemoved, "x", ""), PeerRemoved, true},
		{ev(PeerRemoved, "x", ""), 0, false},
		{ev(PeerUpdated, "z", "Z"), PeerAdded, true}, // an update for an unknown peer adds it
	}
	for i, s := range steps {
		kind, changed := l.apply(s.ev)
		if kind != s.kind || changed != s.changed {
			t.Errorf("step %d: got (%v, %v), want (%v, %v)", i, kind, changed, s.kind, s.changed)
		}
	}
}

func TestPickerHidesTruncatedNames(t *testing.T) {
	l := newPeerList()
	l.apply(DiscoveryEvent{Peer: Peer{ID: "1", DisplayName: "Shown", NameStatus: ospc.DisplayNameUnverified}})
	l.apply(DiscoveryEvent{Peer: Peer{ID: "2", DisplayName: "Trunc", NameStatus: ospc.DisplayNameTruncated}})
	p := NewPicker(l, "https://o.test", "me")
	if got := names(p.Peers()); got != "Shown" {
		t.Errorf("picker shows %q, want only Shown", got)
	}
	if got := len(l.Peers()); got != 2 {
		t.Errorf("list has %d peers, want 2", got)
	}
}

func TestPickByNameWaitsForPeer(t *testing.T) {
	u, fd := newTestUA(t, &Scripted{})
	if err := u.StartDiscovery(); err != nil {
		t.Fatal(err)
	}
	picked := make(chan Peer, 1)
	go func() {
		p, err := PickByName("Office")(context.Background(), NewPicker(u.Peers(), "o", "me"))
		if err != nil {
			t.Error(err)
		}
		picked <- p
	}()
	fd.emit(t, PeerAdded, "1", "Kitchen", ospc.DisplayNameUnverified)
	fd.emit(t, PeerAdded, "2", "Office", ospc.DisplayNameUnverified)
	select {
	case p := <-picked:
		if p.ID != "2" {
			t.Errorf("picked %+v, want Office", p)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("PickByName didn't pick the peer once it appeared")
	}
}

func TestPickByNameCanceled(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := PickByName("nobody")(ctx, NewPicker(newPeerList(), "o", "me"))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("got %v, want deadline exceeded", err)
	}
}

func TestConnectPickerDismissed(t *testing.T) {
	u, _ := newTestUA(t, &Scripted{}) // nil Pick dismisses the picker
	_, err := u.Connect(context.Background(), "o", "me")
	if !errors.Is(err, ErrCanceled) {
		t.Errorf("got %v, want ErrCanceled", err)
	}
}

func TestConnectUndialablePeer(t *testing.T) {
	u, fd := newTestUA(t, &Scripted{Pick: PickByName("Fake")})
	go func() {
		fd.events <- DiscoveryEvent{Peer: Peer{ID: "1", DisplayName: "Fake", NameStatus: ospc.DisplayNameUnverified}}
	}()
	_, err := u.Connect(context.Background(), "o", "me")
	if err == nil || !strings.Contains(err.Error(), "can't be dialed") {
		t.Errorf("got %v, want an undialable peer error", err)
	}
}

func TestCloseUnblocksPicker(t *testing.T) {
	u, _ := newTestUA(t, &Scripted{Pick: PickByName("never")})
	done := make(chan error, 1)
	go func() {
		_, err := u.Connect(context.Background(), "o", "me")
		done <- err
	}()
	time.Sleep(50 * time.Millisecond)
	if err := u.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, ErrClosed) {
			t.Errorf("got %v, want ErrClosed", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Close didn't unblock the picker")
	}
	if err := u.StartDiscovery(); !errors.Is(err, ErrClosed) {
		t.Errorf("StartDiscovery after Close: %v", err)
	}
	if err := u.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
}

func TestAdvertiseDenied(t *testing.T) {
	var out bytes.Buffer
	u, _ := newTestUA(t, &Scripted{Name: "B", Out: &out, Permission: Deny(PermissionAdvertise)})
	_, err := u.Advertise(context.Background(), "https://o.test", "Me")
	if !errors.Is(err, ErrPermissionDenied) {
		t.Errorf("got %v, want ErrPermissionDenied", err)
	}
	if want := `[B] https://o.test wants to be discoverable as "Me": denied`; !strings.Contains(out.String(), want) {
		t.Errorf("narration %q lacks %q", out.String(), want)
	}
}

func TestPINEncoding(t *testing.T) {
	for _, tc := range []struct {
		raw  []byte
		want string
	}{
		{[]byte{0, 0, 0}, "0000-0000"},
		{[]byte{0, 0x30, 0x39}, "0001-2345"},
		{[]byte{0xff, 0xff, 0xff}, "1677-7215"},
		{[]byte{0xff, 0xff}, "6-5535"},
	} {
		got := EncodePIN(tc.raw)
		if got != tc.want {
			t.Errorf("EncodePIN(%x) = %q, want %q", tc.raw, got, tc.want)
		}
		psk, err := ParsePIN(" " + strings.ReplaceAll(got, "-", " - ") + " ")
		if err != nil || !bytes.Equal(psk, pinPSK(got)) {
			t.Errorf("ParsePIN(%q) = %q, %v; want %q", got, psk, err, pinPSK(got))
		}
	}
	for _, bad := range []string{"", " - ", "12a4"} {
		if _, err := ParsePIN(bad); err == nil {
			t.Errorf("ParsePIN(%q) succeeded", bad)
		}
	}
}

func TestPINRelay(t *testing.T) {
	r := NewPINRelay()
	got := make(chan string, 1)
	go func() {
		pin, _ := r.Enter(context.Background(), PINPrompt{Nickname: "B", Peer: Peer{DisplayName: "A"}})
		got <- pin
	}()
	// A PIN for another pairing isn't taken.
	r.Present(PINPresentation{PIN: "999", Nickname: "C", Peer: Peer{DisplayName: "B"}})
	r.Present(PINPresentation{PIN: "123", Nickname: "A", Peer: Peer{DisplayName: "B"}})
	select {
	case pin := <-got:
		if pin != "123" {
			t.Errorf("entered %q, want 123", pin)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Enter didn't get the PIN")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := r.Enter(ctx, PINPrompt{Nickname: "X"}); !errors.Is(err, context.Canceled) {
		t.Errorf("canceled Enter: %v", err)
	}
}

func TestCLI(t *testing.T) {
	in := strings.NewReader("y\nn\n9\n2\n1234-5678\n")
	var out bytes.Buffer
	c := NewCLI(in, &out)
	ctx := context.Background()

	if ok, err := c.RequestPermission(ctx, PermissionRequest{Permission: PermissionAdvertise, Origin: "o", Nickname: "n"}); !ok || err != nil {
		t.Errorf("first prompt: %v %v, want granted", ok, err)
	}
	if ok, _ := c.RequestPermission(ctx, PermissionRequest{Permission: PermissionAdvertise}); ok {
		t.Error("second prompt granted, want denied")
	}

	l := newPeerList()
	l.apply(DiscoveryEvent{Peer: Peer{ID: "1", DisplayName: "Alpha", NameStatus: ospc.DisplayNameUnverified}})
	l.apply(DiscoveryEvent{Peer: Peer{ID: "2", DisplayName: "Beta", NameStatus: ospc.DisplayNameUnverified}})
	p, err := c.PickPeer(ctx, NewPicker(l, "o", "n")) // "9" is out of range, then "2"
	if err != nil || p.ID != "2" {
		t.Errorf("picked %+v, %v; want Beta", p, err)
	}
	if pin, err := c.EnterPIN(ctx, PINPrompt{}); pin != "1234-5678" || err != nil {
		t.Errorf("EnterPIN: %q %v", pin, err)
	}
	if _, err := c.EnterPIN(ctx, PINPrompt{}); err == nil {
		t.Error("EnterPIN at end of input succeeded")
	}
	for _, want := range []string{"1) Alpha (unverified)", "2) Beta (unverified)", `No device "9"`} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output lacks %q:\n%s", want, out.String())
		}
	}
}
