package cast

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/grandcat/zeroconf"
)

// DNS-SD identifiers and TXT keys, from openscreen cast/common/public/
// receiver_info.h.
const (
	ServiceType = "_googlecast._tcp"
	Domain      = "local."

	TXTKeyUniqueID     = "id"
	TXTKeyVersion      = "ve"
	TXTKeyCapabilities = "ca"
	TXTKeyStatus       = "st"
	TXTKeyFriendlyName = "fn"
	TXTKeyModelName    = "md"
)

// Capability bits advertised in the "ca" TXT key.
const (
	CapVideoOutput    uint64 = 1 << 0
	CapVideoInput     uint64 = 1 << 1
	CapAudioOutput    uint64 = 1 << 2
	CapAudioInput     uint64 = 1 << 3
	CapDevModeEnabled uint64 = 1 << 4
)

// ReceiverStatus is the "st" TXT key.
type ReceiverStatus int

const (
	ReceiverIdle ReceiverStatus = 0
	ReceiverBusy ReceiverStatus = 1
)

// ReceiverInfo describes a Cast receiver as advertised over mDNS.
type ReceiverInfo struct {
	UniqueID        string // "id": 128-bit hex id
	ProtocolVersion int    // "ve": Cast protocol version, 2..99
	Capabilities    uint64 // "ca": bitset of Cap* flags
	Status          ReceiverStatus
	ModelName       string // "md", optional
	FriendlyName    string // "fn"

	Port      int
	IPv4      net.IP
	IPv6      net.IP
	HostName  string
	TXTExtras map[string]string // unrecognised TXT keys (e.g. rs, ic, bs)
}

// maxLabelLength is the DNS label limit applied to instance names.
const maxLabelLength = 63

// InstanceID returns the DNS-SD instance name: the model name (max 20 bytes,
// spaces replaced by hyphens), a hyphen, then the unique id without hyphens;
// truncated to 63 bytes (CalculateInstanceId).
func (r *ReceiverInfo) InstanceID() string {
	model := r.ModelName
	if len(model) > 20 {
		model = model[:20]
	}
	name := strings.ReplaceAll(model, " ", "-")
	if name != "" {
		name += "-"
	}
	name += strings.ReplaceAll(r.UniqueID, "-", "")
	if len(name) > maxLabelLength {
		name = name[:maxLabelLength]
	}
	return name
}

// Addr returns host:port, preferring IPv4.
func (r *ReceiverInfo) Addr() string {
	ip := r.IPv4
	if ip == nil {
		ip = r.IPv6
	}
	return net.JoinHostPort(ip.String(), strconv.Itoa(r.Port))
}

// TXT encodes the TXT record as key=value strings.
func (r *ReceiverInfo) TXT() []string {
	return []string{
		TXTKeyUniqueID + "=" + r.UniqueID,
		TXTKeyVersion + "=" + strconv.Itoa(r.ProtocolVersion),
		TXTKeyCapabilities + "=" + strconv.FormatUint(r.Capabilities, 10),
		TXTKeyStatus + "=" + strconv.Itoa(int(r.Status)),
		TXTKeyModelName + "=" + r.ModelName,
		TXTKeyFriendlyName + "=" + r.FriendlyName,
	}
}

// ParseTXT fills the TXT-derived fields of r, applying the validation of
// DnsSdInstanceEndpointToReceiverInfo.
func (r *ReceiverInfo) ParseTXT(txt []string) error {
	kv := map[string]string{}
	for _, e := range txt {
		k, v, _ := strings.Cut(e, "=")
		kv[strings.ToLower(k)] = v
	}
	r.UniqueID = kv[TXTKeyUniqueID]
	if r.UniqueID == "" {
		return errors.New("cast: missing receiver unique ID in record")
	}
	ve, ok := kv[TXTKeyVersion]
	if !ok {
		return errors.New("cast: missing Cast protocol version in record")
	}
	v, err := strconv.Atoi(ve)
	if err != nil || v < 2 || v > 99 {
		return fmt.Errorf("cast: invalid Cast protocol version %q in record", ve)
	}
	r.ProtocolVersion = v
	ca, ok := kv[TXTKeyCapabilities]
	if !ok {
		return errors.New("cast: missing receiver capabilities in record")
	}
	if r.Capabilities, err = strconv.ParseUint(ca, 10, 64); err != nil {
		return fmt.Errorf("cast: invalid receiver capabilities %q in record", ca)
	}
	switch kv[TXTKeyStatus] {
	case "0":
		r.Status = ReceiverIdle
	case "1":
		r.Status = ReceiverBusy
	default:
		return errors.New("cast: missing or invalid receiver status in record")
	}
	r.ModelName = kv[TXTKeyModelName]
	r.FriendlyName = kv[TXTKeyFriendlyName]
	if r.FriendlyName == "" {
		return errors.New("cast: missing receiver friendly name in record")
	}
	r.TXTExtras = nil
	for k, v := range kv {
		switch k {
		case TXTKeyUniqueID, TXTKeyVersion, TXTKeyCapabilities, TXTKeyStatus, TXTKeyModelName, TXTKeyFriendlyName:
		default:
			if r.TXTExtras == nil {
				r.TXTExtras = map[string]string{}
			}
			r.TXTExtras[k] = v
		}
	}
	return nil
}

// receiverFromEntry converts a zeroconf entry to a ReceiverInfo.
func receiverFromEntry(e *zeroconf.ServiceEntry) (*ReceiverInfo, error) {
	r := &ReceiverInfo{Port: e.Port, HostName: e.HostName}
	if len(e.AddrIPv4) > 0 {
		r.IPv4 = e.AddrIPv4[0]
	}
	if len(e.AddrIPv6) > 0 {
		r.IPv6 = e.AddrIPv6[0]
	}
	if r.IPv4 == nil && r.IPv6 == nil {
		return nil, errors.New("cast: no IPv4 nor IPv6 address in record")
	}
	if r.Port == 0 {
		return nil, errors.New("cast: invalid TCP port in record")
	}
	if err := r.ParseTXT(e.Text); err != nil {
		return nil, err
	}
	return r, nil
}

// Browse discovers Cast receivers until ctx is cancelled. Invalid records
// are skipped. The returned channel is closed when browsing stops.
func Browse(ctx context.Context) (<-chan *ReceiverInfo, error) {
	resolver, err := zeroconf.NewResolver(nil)
	if err != nil {
		return nil, err
	}
	entries := make(chan *zeroconf.ServiceEntry)
	if err := resolver.Browse(ctx, ServiceType, Domain, entries); err != nil {
		return nil, err
	}
	out := make(chan *ReceiverInfo)
	go func() {
		defer close(out)
		// zeroconf closes entries once ctx is done.
		for e := range entries {
			r, err := receiverFromEntry(e)
			if err != nil {
				continue
			}
			select {
			case out <- r:
			case <-ctx.Done():
			}
		}
	}()
	return out, nil
}

// Advertisement is a running mDNS advertisement.
type Advertisement struct{ server *zeroconf.Server }

// Close stops advertising.
func (a *Advertisement) Close() { a.server.Shutdown() }

// Advertise publishes r as a Cast receiver on r.Port. r.UniqueID,
// r.FriendlyName, r.Port and r.ProtocolVersion (>= 2) must be set.
func Advertise(r *ReceiverInfo) (*Advertisement, error) {
	if r.UniqueID == "" || r.FriendlyName == "" || r.Port == 0 || r.ProtocolVersion < 2 {
		return nil, errors.New("cast: invalid ReceiverInfo for advertisement")
	}
	if r.Status != ReceiverIdle && r.Status != ReceiverBusy {
		return nil, errors.New("cast: invalid receiver status")
	}
	srv, err := zeroconf.Register(r.InstanceID(), ServiceType, Domain, r.Port, r.TXT(), nil)
	if err != nil {
		return nil, err
	}
	return &Advertisement{server: srv}, nil
}
