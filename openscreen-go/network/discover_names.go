package ospc

import (
	"strings"
	"unicode/utf8"
)

// DNS-SD Instance Names and display names.
//
// Per network.bs (DNS-SD): "Advertising agents must use a DNS-SD Instance
// Name that is a prefix of the agent's display name. If the Instance Name is
// not the complete display name, it must be terminated by a null (\000)
// character, so that a listening agent knows it has been truncated."
//
// Per application.bs (#instance-names): "Agents must treat Instance Names as
// unverified information, and should check that the Instance Name is a prefix
// of the display name received through the agent-info message after a
// successful QUIC connection. Once an agent has done this check, it can show
// the name as a verified display name."

// maxInstanceNameOctets is the maximum length of a DNS label (RFC 1035) and
// therefore of a DNS-SD Instance Name (RFC 6763, section 4.1.1).
const maxInstanceNameOctets = 63

// instanceNameTruncationMarker terminates a truncated Instance Name.
const instanceNameTruncationMarker = "\x00"

// DisplayNameStatus describes how far a remote agent's name can be trusted.
// It models the three categories of application.bs #instance-names.
type DisplayNameStatus int

const (
	// DisplayNameTruncated is a truncated and unverified DNS-SD Instance Name.
	// It should not be shown to the user.
	DisplayNameTruncated DisplayNameStatus = iota
	// DisplayNameUnverified is a complete but unverified DNS-SD Instance
	// Name. It can be shown as unverified prior to authentication.
	DisplayNameUnverified
	// DisplayNameVerified is the display name from agent-info, received over
	// an authenticated connection, of which the Instance Name is a prefix.
	DisplayNameVerified
)

func (s DisplayNameStatus) String() string {
	switch s {
	case DisplayNameTruncated:
		return "truncated"
	case DisplayNameUnverified:
		return "unverified"
	case DisplayNameVerified:
		return "verified"
	default:
		return "unknown"
	}
}

// Displayable reports whether a name with this status may be shown to the
// user. Truncated, unverified names should not be shown.
func (s DisplayNameStatus) Displayable() bool {
	return s != DisplayNameTruncated
}

// Verified reports whether the name is a verified display name.
func (s DisplayNameStatus) Verified() bool {
	return s == DisplayNameVerified
}

// InstanceName is a parsed (unescaped) DNS-SD Instance Name.
type InstanceName struct {
	// Name is the unescaped Instance Name without the truncation marker.
	Name string
	// Truncated is set when the advertiser terminated the Instance Name with
	// a null character, signalling it is not the complete display name.
	Truncated bool
}

// parseInstanceName parses an Instance Name in DNS presentation format, as
// returned by the mDNS resolver.
func parseInstanceName(escaped string) InstanceName {
	raw := unescapeDNSSD(escaped)
	if strings.HasSuffix(raw, instanceNameTruncationMarker) {
		return InstanceName{
			Name:      strings.TrimSuffix(raw, instanceNameTruncationMarker),
			Truncated: true,
		}
	}
	return InstanceName{Name: raw}
}

// Status returns the unverified status of the Instance Name.
func (n InstanceName) Status() DisplayNameStatus {
	if n.Truncated {
		return DisplayNameTruncated
	}
	return DisplayNameUnverified
}

// Verify checks the Instance Name against the display name from agent-info.
// The caller must only pass a display name received over an authenticated
// connection. If the Instance Name is not a prefix of the display name, the
// name stays unverified.
func (n InstanceName) Verify(displayName string) DisplayNameStatus {
	if strings.HasPrefix(displayName, n.Name) {
		return DisplayNameVerified
	}
	return n.Status()
}

// MatchesNickname reports whether the Instance Name could belong to an agent
// with the given display name: an exact match for complete names, or a prefix
// match for truncated names. This is a discovery-time filter only, the result
// is unverified.
func (n InstanceName) MatchesNickname(nickname string) bool {
	if n.Truncated {
		return strings.HasPrefix(nickname, n.Name)
	}
	return n.Name == nickname
}

// instanceNameForDisplayName returns the raw (unescaped) DNS-SD Instance Name
// an advertising agent uses for its display name. Display names that don't
// fit in a DNS label are cut at a UTF-8 character boundary and terminated by
// a null character.
func instanceNameForDisplayName(displayName string) string {
	if len(displayName) <= maxInstanceNameOctets {
		return displayName
	}
	cut := maxInstanceNameOctets - len(instanceNameTruncationMarker)
	for cut > 0 && !utf8.RuneStart(displayName[cut]) {
		cut--
	}
	return displayName[:cut] + instanceNameTruncationMarker
}

// hostnameInstanceName returns the instance name part used to compute the
// agent hostname: the Instance Name without the truncation marker.
func hostnameInstanceName(displayName string) string {
	return strings.TrimSuffix(instanceNameForDisplayName(displayName), instanceNameTruncationMarker)
}

// escapeDNSSD escapes a raw DNS label into DNS presentation format, matching
// the escaping the mDNS library (miekg/dns) applies when unpacking names.
func escapeDNSSD(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '.' || c == '(' || c == ')' || c == ';' || c == ' ' ||
			c == '@' || c == '"' || c == '\\':
			b.WriteByte('\\')
			b.WriteByte(c)
		case c < ' ' || c > '~':
			b.WriteByte('\\')
			b.WriteByte('0' + c/100)
			b.WriteByte('0' + c/10%10)
			b.WriteByte('0' + c%10)
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// unescapeDNSSD removes DNS presentation format escaping (RFC 1035 section
// 5.1) from instance names and TXT strings: "\X" is the literal X and "\DDD"
// is the octet with decimal value DDD. The mDNS library returns names and TXT
// strings in this format (e.g. "Test\ Receiver", "Caf\195\169").
func unescapeDNSSD(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c != '\\' || i+1 >= len(s) {
			b.WriteByte(c)
			continue
		}
		if i+3 < len(s) && isDigit(s[i+1]) && isDigit(s[i+2]) && isDigit(s[i+3]) {
			v := int(s[i+1]-'0')*100 + int(s[i+2]-'0')*10 + int(s[i+3]-'0')
			if v <= 0xff {
				b.WriteByte(byte(v))
				i += 3
				continue
			}
		}
		b.WriteByte(s[i+1])
		i++
	}
	return b.String()
}

func isDigit(c byte) bool {
	return c >= '0' && c <= '9'
}

// setDiscovery records how a remote agent was discovered.
func (a *Agent) setDiscovery(name InstanceName, mv uint64, hasMV bool) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.instanceName = &name
	a.metadataVersion = mv
	a.hasMetadataVersion = hasMV
}

// setAuthenticated marks a remote agent as authenticated: its agent-info was
// received over a connection that completed authentication.
func (a *Agent) setAuthenticated() {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.authenticated = true
}

// Authenticated reports whether a connection to this remote agent completed
// authentication.
func (a *Agent) Authenticated() bool {
	a.mu.Lock()
	defer a.mu.Unlock()

	return a.authenticated
}

// DiscoveredInstanceName returns the DNS-SD Instance Name a remote agent was
// discovered with, if it was discovered.
func (a *Agent) DiscoveredInstanceName() (InstanceName, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.instanceName == nil {
		return InstanceName{}, false
	}
	return *a.instanceName, true
}

// DisplayName returns the name to present for a remote agent and its status:
//   - Once authenticated, the display name from agent-info is verified if the
//     Instance Name the agent was discovered with is a prefix of it, or if the
//     agent wasn't discovered (incoming connection).
//   - Otherwise the discovered Instance Name is returned, unverified or
//     truncated (should not be shown).
//   - An agent that wasn't discovered and isn't authenticated yet returns the
//     agent-info display name as unverified, or "" if it has none.
//
// A mismatch between Instance Name and agent-info display name leaves the
// Instance Name unverified; the agent-info is still available through Info.
func (a *Agent) DisplayName() (string, DisplayNameStatus) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.authenticated && a.info != nil {
		if a.instanceName == nil ||
			a.instanceName.Verify(a.info.DisplayName) == DisplayNameVerified {
			return a.info.DisplayName, DisplayNameVerified
		}
	}
	if a.instanceName != nil {
		return a.instanceName.Name, a.instanceName.Status()
	}
	if a.info != nil {
		return a.info.DisplayName, DisplayNameUnverified
	}
	return "", DisplayNameTruncated
}

// Metadata returns a snapshot of a remote agent's metadata, to compare later
// discoveries against with DiscoveredAgent.MetadataChangedSince. It returns
// false if no agent-info was received yet.
func (a *Agent) Metadata() (AgentMetadata, bool) {
	info := a.Info()
	if info == nil {
		return AgentMetadata{}, false
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	return AgentMetadata{
		PeerID:             a.PeerID,
		MetadataVersion:    a.metadataVersion,
		HasMetadataVersion: a.hasMetadataVersion,
		Info:               *info,
	}, true
}

// MetadataVersion returns the mv value advertised by this (local) agent.
func (a *Agent) MetadataVersion() uint64 {
	a.mu.Lock()
	defer a.mu.Unlock()

	return a.metadataVersion
}

// bumpMetadataVersion increases the mv value of this (local) agent.
func (a *Agent) bumpMetadataVersion() uint64 {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.metadataVersion++
	return a.metadataVersion
}
