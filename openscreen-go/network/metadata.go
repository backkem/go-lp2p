package ospc

import (
	"bytes"
	"errors"
	"fmt"
)

// Metadata change detection.
//
// Per network.bs, the mDNS TXT key "mv" is "An unsigned integer value that
// indicates that metadata has changed. The advertising agent must update it
// to a greater value. This signals to the listening agent that it should
// connect to the advertising agent to discover updated metadata. The value
// should be encoded as a variable-length integer."
//
// The TXT key "at" is unrelated: it is the auth-initiation-token, "An
// alphanumeric, unguessable token consisting of characters from the set
// [A-Za-z0-9+/]".
//
// The state-token is only carried in agent-info (application.bs): "A random
// alphanumeric value consisting of 8 characters in the range [0-9A-Za-z]. This
// value is set before the agent makes its first connection and must be set to
// a new value when the agent is reset or otherwise lost all of its state
// related to this protocol."

const (
	txtKeyFingerprint     = "fp"
	txtKeyMetadataVersion = "mv"
	txtKeyAuthToken       = "at"
)

// encodeMetadataVersion encodes mv as a variable-length integer.
func encodeMetadataVersion(mv uint64) (string, error) {
	buf := new(bytes.Buffer)
	if err := writeVaruint(mv, buf); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// decodeMetadataVersion decodes an unescaped mv TXT value.
func decodeMetadataVersion(value string) (uint64, error) {
	r := bytes.NewReader([]byte(value))
	mv, err := readVaruint(r)
	if err != nil {
		return 0, fmt.Errorf("invalid mv record: %v", err)
	}
	if r.Len() != 0 {
		return 0, errors.New("invalid mv record: trailing data")
	}
	return mv, nil
}

// validateAuthToken checks the at TXT value is from the set [A-Za-z0-9+/].
func validateAuthToken(at string) error {
	if at == "" {
		return errors.New("empty at record")
	}
	for i := 0; i < len(at); i++ {
		c := at[i]
		if !(c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || isDigit(c) || c == '+' || c == '/') {
			return fmt.Errorf("invalid character %q in at record", c)
		}
	}
	return nil
}

// AgentMetadata is a snapshot of what is known about a remote agent after
// connecting to it. Callers can keep it (e.g. keyed by PeerID) and compare a
// later discovery against it to avoid needless connections; see
// DiscoveredAgent.MetadataChangedSince.
type AgentMetadata struct {
	PeerID PeerID
	// MetadataVersion is the mv value seen at discovery, before Info was
	// retrieved. Only meaningful if HasMetadataVersion is set.
	MetadataVersion    uint64
	HasMetadataVersion bool
	// Info is the agent-info received from the agent.
	Info AgentInfo
}

// StateReset reports whether the agent advertised a different state-token in
// info than in the snapshot, meaning it lost its state. Per application.bs,
// the agent "should discard any requests, responses, watches and events for
// that agent".
func (m AgentMetadata) StateReset(info AgentInfo) bool {
	return m.Info.StateToken != "" && info.StateToken != m.Info.StateToken
}
