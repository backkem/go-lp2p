package ospc

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/miekg/dns"
)

// wireRoundTrip packs an escaped Instance Name the way the mDNS library does
// when advertising and unpacks it the way it does when browsing.
func wireRoundTrip(t *testing.T, escapedInstance string) string {
	t.Helper()
	name := escapedInstance + "." + MdnsServiceType + "." + MdnsDomain + "."
	buf := make([]byte, 512)
	n, err := dns.PackDomainName(name, buf, 0, nil, false)
	if err != nil {
		t.Fatalf("pack %q: %v", name, err)
	}
	unpacked, _, err := dns.UnpackDomainName(buf[:n], 0)
	if err != nil {
		t.Fatalf("unpack: %v", err)
	}
	suffix := "." + MdnsServiceType + "." + MdnsDomain + "."
	if !strings.HasSuffix(unpacked, suffix) {
		t.Fatalf("unexpected unpacked name %q", unpacked)
	}
	return strings.TrimSuffix(unpacked, suffix)
}

func TestInstanceNameForDisplayName(t *testing.T) {
	long := strings.Repeat("a", 100)
	// "é" is 2 octets at offsets 61-62: the cut at 62 would split it.
	splitTwo := strings.Repeat("a", 61) + "é" + "xyz"
	// A 4 octet rune at offsets 60-63.
	splitFour := strings.Repeat("a", 60) + "😀" + "xyz"
	exact := strings.Repeat("b", maxInstanceNameOctets)

	testCases := []struct {
		name      string
		display   string
		want      string
		truncated bool
	}{
		{"short", "Living Room TV", "Living Room TV", false},
		{"exactly 63 octets", exact, exact, false},
		{"ascii truncated", long, strings.Repeat("a", 62), true},
		{"2 octet rune at cut", splitTwo, strings.Repeat("a", 61), true},
		{"4 octet rune at cut", splitFour, strings.Repeat("a", 60), true},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			raw := instanceNameForDisplayName(tc.display)
			if len(raw) > maxInstanceNameOctets {
				t.Fatalf("instance name too long: %d octets", len(raw))
			}
			parsed := parseInstanceName(wireRoundTrip(t, escapeDNSSD(raw)))
			if parsed.Name != tc.want || parsed.Truncated != tc.truncated {
				t.Fatalf("got %q truncated=%v, want %q truncated=%v",
					parsed.Name, parsed.Truncated, tc.want, tc.truncated)
			}
			if !utf8.ValidString(parsed.Name) {
				t.Fatalf("truncated name is not valid UTF-8: %q", parsed.Name)
			}
			if got := parsed.Verify(tc.display); got != DisplayNameVerified {
				t.Fatalf("Verify(%q) = %v, want verified", tc.display, got)
			}
		})
	}
}

func TestParseInstanceName(t *testing.T) {
	testCases := []struct {
		escaped   string
		want      string
		truncated bool
	}{
		{`Test\ Receiver`, "Test Receiver", false},
		{`Caf\195\169`, "Café", false},
		{`a\.b\ \(c\)\\d\"e\@f\;g`, `a.b (c)\d"e@f;g`, false},
		{`Long\ Na\000`, "Long Na", true},
		{`Caf\195\000`, "Caf\xc3", true}, // non-compliant cut mid-rune
		{`plain`, "plain", false},
	}
	for _, tc := range testCases {
		got := parseInstanceName(tc.escaped)
		if got.Name != tc.want || got.Truncated != tc.truncated {
			t.Errorf("parseInstanceName(%q) = %q truncated=%v, want %q truncated=%v",
				tc.escaped, got.Name, got.Truncated, tc.want, tc.truncated)
		}
	}
}

func TestEscapeDNSSDMatchesWire(t *testing.T) {
	for _, raw := range []string{
		"Test Receiver",
		`a.b (c)\d"e@f;g`,
		"Café 😀",
		"tab\there\x00",
	} {
		escaped := escapeDNSSD(raw)
		if got := wireRoundTrip(t, escaped); got != escaped {
			t.Errorf("escapeDNSSD(%q) = %q, wire gives %q", raw, escaped, got)
		}
		if got := unescapeDNSSD(escaped); got != raw {
			t.Errorf("unescapeDNSSD(escapeDNSSD(%q)) = %q", raw, got)
		}
	}
}

func TestInstanceNameVerify(t *testing.T) {
	testCases := []struct {
		name     string
		instance InstanceName
		display  string
		want     DisplayNameStatus
	}{
		{"complete match", InstanceName{Name: "TV"}, "TV", DisplayNameVerified},
		{"complete prefix", InstanceName{Name: "TV"}, "TV Bedroom", DisplayNameVerified},
		{"truncated prefix", InstanceName{Name: "Very Long", Truncated: true}, "Very Long Name", DisplayNameVerified},
		{"complete mismatch", InstanceName{Name: "Bob"}, "Alice", DisplayNameUnverified},
		{"truncated mismatch", InstanceName{Name: "Bob", Truncated: true}, "Alice", DisplayNameTruncated},
		{"display shorter", InstanceName{Name: "TV Bedroom"}, "TV", DisplayNameUnverified},
		{"case sensitive", InstanceName{Name: "tv"}, "TV", DisplayNameUnverified},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.instance.Verify(tc.display); got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}

	if DisplayNameTruncated.Displayable() || !DisplayNameUnverified.Displayable() ||
		!DisplayNameVerified.Displayable() || DisplayNameUnverified.Verified() {
		t.Fatal("unexpected DisplayNameStatus predicates")
	}
}

func TestInstanceNameMatchesNickname(t *testing.T) {
	if !(InstanceName{Name: "TV"}).MatchesNickname("TV") {
		t.Error("exact match failed")
	}
	if (InstanceName{Name: "TV"}).MatchesNickname("TV Bedroom") {
		t.Error("complete name must match exactly")
	}
	if !(InstanceName{Name: "TV Bed", Truncated: true}).MatchesNickname("TV Bedroom") {
		t.Error("truncated prefix match failed")
	}
	if (InstanceName{Name: "TV Bed", Truncated: true}).MatchesNickname("Radio") {
		t.Error("truncated mismatch matched")
	}
}

func TestAgentHostnameUsesInstanceName(t *testing.T) {
	display := strings.Repeat("Long Name ", 10)
	parsed := parseInstanceName(wireRoundTrip(t, escapeDNSSD(instanceNameForDisplayName(display))))

	advertiser := buildAgentHostname("AQ", display, MdnsDomain)
	dialer := buildAgentHostname("AQ", parsed.Name, MdnsDomain)
	if advertiser != dialer {
		t.Fatalf("hostname mismatch: %q != %q", advertiser, dialer)
	}
}

func TestAgentDisplayName(t *testing.T) {
	newRemote := func() *Agent { return &Agent{PeerID: "peer"} }

	t.Run("discovered, before auth", func(t *testing.T) {
		a := newRemote()
		a.setDiscovery(InstanceName{Name: "Kitchen"}, 0, false)
		a.setInfo(AgentInfo{DisplayName: "Kitchen Speaker"})
		assertDisplayName(t, a, "Kitchen", DisplayNameUnverified)
	})
	t.Run("discovered truncated, before auth", func(t *testing.T) {
		a := newRemote()
		a.setDiscovery(InstanceName{Name: "Kitch", Truncated: true}, 0, false)
		assertDisplayName(t, a, "Kitch", DisplayNameTruncated)
	})
	t.Run("discovered truncated, verified", func(t *testing.T) {
		a := newRemote()
		a.setDiscovery(InstanceName{Name: "Kitch", Truncated: true}, 0, false)
		a.setInfo(AgentInfo{DisplayName: "Kitchen Speaker"})
		a.setAuthenticated()
		assertDisplayName(t, a, "Kitchen Speaker", DisplayNameVerified)
	})
	t.Run("discovered, mismatch after auth", func(t *testing.T) {
		a := newRemote()
		a.setDiscovery(InstanceName{Name: "Kitchen"}, 0, false)
		a.setInfo(AgentInfo{DisplayName: "Garage"})
		a.setAuthenticated()
		assertDisplayName(t, a, "Kitchen", DisplayNameUnverified)
	})
	t.Run("incoming, authenticated", func(t *testing.T) {
		a := newRemote()
		a.setInfo(AgentInfo{DisplayName: "Phone"})
		a.setAuthenticated()
		assertDisplayName(t, a, "Phone", DisplayNameVerified)
	})
	t.Run("incoming, unauthenticated", func(t *testing.T) {
		a := newRemote()
		a.setInfo(AgentInfo{DisplayName: "Phone"})
		assertDisplayName(t, a, "Phone", DisplayNameUnverified)
	})
	t.Run("nothing known", func(t *testing.T) {
		assertDisplayName(t, newRemote(), "", DisplayNameTruncated)
	})
}

func assertDisplayName(t *testing.T, a *Agent, wantName string, wantStatus DisplayNameStatus) {
	t.Helper()
	name, status := a.DisplayName()
	if name != wantName || status != wantStatus {
		t.Fatalf("DisplayName() = %q, %v; want %q, %v", name, status, wantName, wantStatus)
	}
}

func ExampleAgent_DisplayName() {
	a := &Agent{}
	a.setDiscovery(parseInstanceName(`Living\ Ro\000`), 0, false)
	name, status := a.DisplayName()
	fmt.Println(name, status, status.Displayable())

	a.setInfo(AgentInfo{DisplayName: "Living Room TV"})
	a.setAuthenticated()
	name, status = a.DisplayName()
	fmt.Println(name, status, status.Displayable())
	// Output:
	// Living Ro truncated false
	// Living Room TV verified true
}
