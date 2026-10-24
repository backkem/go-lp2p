package ospc

import (
	"testing"
)

// escapeTXT escapes a TXT string the way the mDNS library returns it.
func escapeTXT(s string) string {
	return escapeDNSSD(s)
}

func TestDiscoveryTXT(t *testing.T) {
	const fp = "47DEQpj8HBSa+/TImW+5JCeuQeRkm5NMpJWZG3hSuFU="

	for _, mv := range []uint64{0, 1, 63, 64, 300, 1 << 20, 1 << 40} {
		encoded, err := encodeMetadataVersion(mv)
		if err != nil {
			t.Fatal(err)
		}
		text := []string{
			"fp=" + fp,
			escapeTXT("mv=" + encoded),
			"at=ab+/09Zz",
		}
		txt, err := parseDiscoveryTXT(text)
		if err != nil {
			t.Fatal(err)
		}
		a, err := newDiscoveredAgentFromTXT(`Test\ Receiver`, txt)
		if err != nil {
			t.Fatalf("mv=%d: %v", mv, err)
		}
		if a.PeerID != PeerID(fp) {
			t.Fatalf("wrong PeerID %q", a.PeerID)
		}
		got, ok := a.MetadataVersion()
		if !ok || got != mv {
			t.Fatalf("MetadataVersion() = %d, %v; want %d", got, ok, mv)
		}
		at, err := a.AuthToken()
		if err != nil || at != "ab+/09Zz" {
			t.Fatalf("AuthToken() = %q, %v", at, err)
		}
		if a.Nickname() != "Test Receiver" {
			t.Fatalf("wrong nickname %q", a.Nickname())
		}
	}
}

func TestDiscoveryTXTErrors(t *testing.T) {
	testCases := []struct {
		name string
		text []string
	}{
		{"missing fp", []string{`mv=\000`}},
		{"duplicate fp", []string{"fp=a", "fp=b"}},
		{"empty mv", []string{"fp=a", "mv="}},
		{"truncated mv", []string{"fp=a", `mv=\064`}}, // 2 octet varuint, 1 octet given
		{"trailing mv", []string{"fp=a", `mv=\000\000`}},
		{"no key", []string{"fp=a", "garbage"}},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			txt, err := parseDiscoveryTXT(tc.text)
			if err == nil {
				_, err = newDiscoveredAgentFromTXT("x", txt)
			}
			if err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestDiscoveryTXTOptionalKeys(t *testing.T) {
	txt, err := parseDiscoveryTXT([]string{"fp=a"})
	if err != nil {
		t.Fatal(err)
	}
	a, err := newDiscoveredAgentFromTXT("x", txt)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := a.MetadataVersion(); ok {
		t.Fatal("expected no mv")
	}
	if _, err := a.AuthToken(); err == nil {
		t.Fatal("expected missing at error")
	}
}

func TestValidateAuthToken(t *testing.T) {
	for _, at := range []string{"abcXYZ019+/", randomAT(9)} {
		if err := validateAuthToken(at); err != nil {
			t.Errorf("validateAuthToken(%q): %v", at, err)
		}
	}
	for _, at := range []string{"", "abc=", "ab-c", "ab c", "é"} {
		if err := validateAuthToken(at); err == nil {
			t.Errorf("validateAuthToken(%q): expected error", at)
		}
	}
}

func TestMetadataChangedSince(t *testing.T) {
	discovered := func(peer PeerID, mv uint64, hasMV bool) *DiscoveredAgent {
		return &DiscoveredAgent{PeerID: peer, metadataVersion: mv, hasMetadataVersion: hasMV}
	}
	prev := AgentMetadata{PeerID: "p", MetadataVersion: 5, HasMetadataVersion: true}

	testCases := []struct {
		name  string
		agent *DiscoveredAgent
		prev  AgentMetadata
		want  bool
	}{
		{"unchanged", discovered("p", 5, true), prev, false},
		{"increased", discovered("p", 6, true), prev, true},
		{"decreased", discovered("p", 4, true), prev, true},
		{"other peer", discovered("q", 5, true), prev, true},
		{"no mv advertised", discovered("p", 0, false), prev, true},
		{"no mv known", discovered("p", 5, true), AgentMetadata{PeerID: "p"}, true},
		{"nothing known", discovered("p", 5, true), AgentMetadata{}, true},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.agent.MetadataChangedSince(tc.prev); got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestAgentMetadataSnapshot(t *testing.T) {
	remote := &Agent{PeerID: "p"}
	if _, ok := remote.Metadata(); ok {
		t.Fatal("expected no metadata before agent-info")
	}

	remote.setDiscovery(InstanceName{Name: "TV"}, 7, true)
	remote.setInfo(AgentInfo{DisplayName: "TV", StateToken: "abcd1234"})
	snapshot, ok := remote.Metadata()
	if !ok {
		t.Fatal("expected metadata")
	}

	if (&DiscoveredAgent{PeerID: "p", metadataVersion: 7, hasMetadataVersion: true}).MetadataChangedSince(snapshot) {
		t.Fatal("same mv must not report a change")
	}
	if !(&DiscoveredAgent{PeerID: "p", metadataVersion: 8, hasMetadataVersion: true}).MetadataChangedSince(snapshot) {
		t.Fatal("greater mv must report a change")
	}

	if snapshot.StateReset(AgentInfo{StateToken: "abcd1234"}) {
		t.Fatal("same state-token is not a reset")
	}
	if !snapshot.StateReset(AgentInfo{StateToken: "zzzz9999"}) {
		t.Fatal("new state-token is a reset")
	}
}

func TestLocalMetadataVersionAndStateToken(t *testing.T) {
	a := &Agent{}
	if a.MetadataVersion() != 0 {
		t.Fatal("initial mv must be 0")
	}
	if a.bumpMetadataVersion() != 1 || a.MetadataVersion() != 1 {
		t.Fatal("mv must increase")
	}

	token := a.StateToken()
	if len(token) != 8 {
		t.Fatalf("state token %q must have 8 characters", token)
	}
	for _, c := range token {
		if !(c >= '0' && c <= '9' || c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z') {
			t.Fatalf("state token %q has invalid character %q", token, c)
		}
	}
	if a.StateToken() != token {
		t.Fatal("state token must be stable")
	}
}
