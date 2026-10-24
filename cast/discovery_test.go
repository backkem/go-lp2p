package cast

import (
	"context"
	"net"
	"os"
	"testing"
	"time"

	"github.com/grandcat/zeroconf"
)

func testReceiverInfo() *ReceiverInfo {
	return &ReceiverInfo{
		UniqueID:        "4ef522244a5a877f35ddead7d98702e6",
		ProtocolVersion: 5,
		Capabilities:    CapVideoOutput | CapAudioOutput,
		Status:          ReceiverIdle,
		ModelName:       "Chromecast Ultra",
		FriendlyName:    "Living Room TV",
		Port:            DefaultPort,
	}
}

func TestReceiverInfoTXTRoundTrip(t *testing.T) {
	in := testReceiverInfo()
	var out ReceiverInfo
	if err := out.ParseTXT(append(in.TXT(), "rs=Netflix", "bs=FA8FCA")); err != nil {
		t.Fatal(err)
	}
	if out.UniqueID != in.UniqueID || out.ProtocolVersion != 5 || out.Capabilities != 5 ||
		out.Status != ReceiverIdle || out.ModelName != in.ModelName || out.FriendlyName != in.FriendlyName {
		t.Fatalf("got %+v", out)
	}
	if out.TXTExtras["rs"] != "Netflix" || out.TXTExtras["bs"] != "FA8FCA" {
		t.Fatalf("extras %v", out.TXTExtras)
	}
}

func TestReceiverInfoParseTXTValidation(t *testing.T) {
	base := map[string]string{"id": "abc", "ve": "2", "ca": "1", "st": "1", "fn": "TV"}
	mutate := []struct {
		key, val string
		drop     bool
	}{
		{key: "id", drop: true},
		{key: "ve", drop: true},
		{key: "ve", val: "1"},
		{key: "ve", val: "100"},
		{key: "ve", val: "x"},
		{key: "ca", drop: true},
		{key: "ca", val: "-1"},
		{key: "st", val: "2"},
		{key: "st", drop: true},
		{key: "fn", drop: true},
	}
	var ok ReceiverInfo
	if err := ok.ParseTXT(toTXT(base)); err != nil {
		t.Fatalf("base record rejected: %v", err)
	}
	for _, m := range mutate {
		kv := map[string]string{}
		for k, v := range base {
			kv[k] = v
		}
		if m.drop {
			delete(kv, m.key)
		} else {
			kv[m.key] = m.val
		}
		var r ReceiverInfo
		if err := r.ParseTXT(toTXT(kv)); err == nil {
			t.Errorf("%+v accepted", m)
		}
	}
}

func toTXT(kv map[string]string) []string {
	var out []string
	for k, v := range kv {
		out = append(out, k+"="+v)
	}
	return out
}

func TestInstanceID(t *testing.T) {
	r := &ReceiverInfo{ModelName: "A Very Long Model Name Indeed", UniqueID: "ab-cd-ef"}
	if got, want := r.InstanceID(), "A-Very-Long-Model-Na-abcdef"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	r = &ReceiverInfo{UniqueID: "0123456789012345678901234567890123456789012345678901234567890123456789"}
	if len(r.InstanceID()) != 63 {
		t.Fatalf("not truncated: %q", r.InstanceID())
	}
}

func TestReceiverFromEntry(t *testing.T) {
	e := zeroconf.NewServiceEntry("x", ServiceType, Domain)
	e.Port = 8009
	e.AddrIPv4 = []net.IP{net.IPv4(192, 168, 1, 7)}
	e.Text = testReceiverInfo().TXT()
	r, err := receiverFromEntry(e)
	if err != nil {
		t.Fatal(err)
	}
	if r.Addr() != "192.168.1.7:8009" {
		t.Fatalf("addr %s", r.Addr())
	}
	e.AddrIPv4 = nil
	if _, err := receiverFromEntry(e); err == nil {
		t.Fatal("entry without address accepted")
	}
}

// TestMDNSLoopback uses real multicast; it is opt-in because sandboxes and
// CI runners often block it. Run with CAST_MDNS_TEST=1.
func TestMDNSLoopback(t *testing.T) {
	if os.Getenv("CAST_MDNS_TEST") == "" {
		t.Skip("set CAST_MDNS_TEST=1 to run the multicast test")
	}
	info := testReceiverInfo()
	info.UniqueID = "mdns-test-" + time.Now().Format("150405.000")
	adv, err := Advertise(info)
	if err != nil {
		t.Fatal(err)
	}
	defer adv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	found, err := Browse(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for r := range found {
		if r.UniqueID == info.UniqueID {
			if r.FriendlyName != info.FriendlyName || r.Port != DefaultPort {
				t.Fatalf("got %+v", r)
			}
			return
		}
	}
	t.Fatal("receiver not discovered")
}
