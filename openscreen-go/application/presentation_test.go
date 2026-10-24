package application

import (
	"bytes"
	"encoding/hex"
	"testing"

	ospc "github.com/backkem/go-lp2p/openscreen-go/network"
)

// TestPresentationMessageEncoding checks the CBOR of the presentation
// messages against application_messages.cddl: integer keys in ascending
// order, http-header as a [key, value] array, nil lists as [], and
// presentation-connection-message carrying text or bytes.
func TestPresentationMessageEncoding(t *testing.T) {
	code := uint64(200)
	for _, tc := range []struct {
		name string
		msg  any
		want string // hex
	}{
		{
			"availability request",
			&PresentationURLAvailabilityRequest{RequestID: 1, URLs: []string{"a"}, WatchDuration: 2, WatchID: 3},
			"a4" + "0001" + "0181" + "6161" + "0202" + "0303",
		},
		{
			"start request, no headers",
			&PresentationStartRequest{RequestID: 1, PresentationID: "p", URL: "u"},
			"a4" + "0001" + "016170" + "026175" + "0380",
		},
		{
			"start request, one header",
			&PresentationStartRequest{RequestID: 1, PresentationID: "p", URL: "u", Headers: []HTTPHeader{{Key: "k", Value: "v"}}},
			"a4" + "0001" + "016170" + "026175" + "0381" + "82616b6176",
		},
		{
			"start response with http-response-code",
			&PresentationStartResponse{RequestID: 1, Result: ResultSuccess, ConnectionID: 7, HTTPResponseCode: &code},
			"a4" + "0001" + "0101" + "0207" + "0318c8",
		},
		{
			"start response without http-response-code",
			&PresentationStartResponse{RequestID: 1, Result: ResultInvalidPresentationID, ConnectionID: 0},
			"a3" + "0001" + "010b" + "0200",
		},
		{
			"text message",
			&PresentationConnectionMessage{ConnectionID: 7, Message: "hi"},
			"a2" + "0007" + "01626869",
		},
		{
			"binary message",
			&PresentationConnectionMessage{ConnectionID: 7, Message: []byte("hi")},
			"a2" + "0007" + "01426869",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, err := ospc.EncodeCBOR(tc.msg)
			if err != nil {
				t.Fatal(err)
			}
			if got := hex.EncodeToString(b); got != tc.want {
				t.Errorf("encoding = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestPresentationConnectionMessageDecode(t *testing.T) {
	for _, tc := range []struct {
		hex  string
		want any
	}{
		{"a2000701626869", "hi"},
		{"a2000701426869", []byte("hi")},
	} {
		b, _ := hex.DecodeString(tc.hex)
		var m PresentationConnectionMessage
		if err := ospc.DecodeCBOR(bytes.NewReader(b), &m); err != nil {
			t.Fatal(err)
		}
		switch want := tc.want.(type) {
		case string:
			if got, ok := m.Message.(string); !ok || got != want {
				t.Errorf("%s: message = %#v, want %q", tc.hex, m.Message, want)
			}
		case []byte:
			if got, ok := m.Message.([]byte); !ok || !bytes.Equal(got, want) {
				t.Errorf("%s: message = %#v, want %q", tc.hex, m.Message, want)
			}
		}
		if m.ConnectionID != 7 {
			t.Errorf("connection-id = %d", m.ConnectionID)
		}
	}
}

func TestHTTPHeaderDecode(t *testing.T) {
	b, _ := hex.DecodeString("a4" + "0001" + "016170" + "026175" + "0381" + "82616b6176")
	var m PresentationStartRequest
	if err := ospc.DecodeCBOR(bytes.NewReader(b), &m); err != nil {
		t.Fatal(err)
	}
	if len(m.Headers) != 1 || m.Headers[0].Key != "k" || m.Headers[0].Value != "v" {
		t.Errorf("headers = %+v", m.Headers)
	}
}
