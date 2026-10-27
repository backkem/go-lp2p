package ospc

import (
	"bytes"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
)

// Wire vectors derived by hand from Appendix A: the type key as a QUIC
// varint followed by the CBOR map with integer keys in ascending order.
func TestLP2PExtensionMessageVectors(t *testing.T) {
	binding := bytes.Repeat([]byte{0xab}, 32)
	for _, tc := range []struct {
		name string
		msg  interface{}
		hex  string
	}{{
		name: "lp2p-session-request",
		msg:  &msgLp2pSessionRequest{msgRequest{7}, "https://a.example"},
		// 1110 = 0x0456 -> varint 0x4456; {0: 7, 1: "https://a.example"}
		hex: "4456" + "a2" + "0007" + "01" + "71" + hex.EncodeToString([]byte("https://a.example")),
	}, {
		name: "lp2p-session-response success",
		msg:  &msgLp2pSessionResponse{msgResponse{7}, ResultSuccess},
		hex:  "4457" + "a2" + "0007" + "0101",
	}, {
		name: "lp2p-session-response permanent-error",
		msg:  &msgLp2pSessionResponse{msgResponse{7}, ResultPermanentError},
		hex:  "4457" + "a2" + "0007" + "01" + "1866", // 102
	}, {
		name: "data-transport-start-request",
		msg:  &msgDataTransportStartRequest{msgRequest: msgRequest{1}, ExchangeId: 2},
		hex:  "44b1" + "a2" + "0001" + "0102",
	}, {
		name: "data-transport-start-request max exchange-id",
		msg:  &msgDataTransportStartRequest{msgRequest: msgRequest{1}, ExchangeId: MaxExchangeID},
		hex:  "44b1" + "a2" + "0001" + "01" + "1b3fffffffffffffff",
	}, {
		name: "data-transport-start-request with session-binding",
		msg:  &msgDataTransportStartRequest{msgRequest{1}, 3, binding},
		hex:  "44b1" + "a3" + "0001" + "0103" + "02" + "5820" + hex.EncodeToString(binding),
	}, {
		name: "data-transport-start-response",
		msg:  &msgDataTransportStartResponse{msgResponse{1}, ResultSuccess},
		hex:  "44b2" + "a2" + "0001" + "0101",
	}, {
		name: "data-transport-stream",
		msg:  &msgDataTransportStream{ExchangeId: 1000},
		hex:  "44b3" + "a1" + "01" + "1903e8",
	}, {
		name: "data-transport-close empty",
		msg:  &msgDataTransportClose{},
		hex:  "44b5" + "a2" + "0000" + "0160",
	}, {
		name: "data-transport-close max code",
		msg:  &msgDataTransportClose{CloseCode: MaxWebTransportCloseCode, Reason: "bye"},
		hex:  "44b5" + "a2" + "00" + "1affffffff" + "01" + "63" + "627965",
	}, {
		name: "data-transport-drain",
		msg:  &msgDataTransportDrain{},
		hex:  "44b6" + "a0",
	}} {
		t.Run(tc.name, func(t *testing.T) {
			want, err := hex.DecodeString(tc.hex)
			if err != nil {
				t.Fatal(err)
			}
			var buf bytes.Buffer
			if err := writeMessage(tc.msg, &buf); err != nil {
				t.Fatalf("encode: %v", err)
			}
			if !bytes.Equal(buf.Bytes(), want) {
				t.Fatalf("encoded %x, want %x", buf.Bytes(), want)
			}
			got, err := readMessage(bytes.NewReader(want))
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			var again bytes.Buffer
			if err := writeMessage(got, &again); err != nil {
				t.Fatalf("re-encode: %v", err)
			}
			if !bytes.Equal(again.Bytes(), want) {
				t.Fatalf("decoded %#v re-encodes to %x, want %x", got, again.Bytes(), want)
			}
		})
	}
}

func TestLP2PExtensionMessageLimits(t *testing.T) {
	longReason := strings.Repeat("r", MaxWebTransportReasonLen+1)
	for _, tc := range []struct {
		name string
		hex  string      // Rejected on decode.
		msg  interface{} // Rejected on encode.
	}{{
		name: "exchange-id 2^62 in start-request",
		hex:  "44b1" + "a2" + "0001" + "01" + "1b4000000000000000",
		msg:  &msgDataTransportStartRequest{msgRequest: msgRequest{1}, ExchangeId: MaxExchangeID + 1},
	}, {
		name: "exchange-id 2^62 in stream",
		hex:  "44b3" + "a1" + "01" + "1b4000000000000000",
		msg:  &msgDataTransportStream{ExchangeId: MaxExchangeID + 1},
	}, {
		name: "session-binding of 31 bytes",
		hex:  "44b1" + "a3" + "0001" + "0103" + "02" + "581f" + strings.Repeat("ab", 31),
		msg:  &msgDataTransportStartRequest{msgRequest{1}, 3, bytes.Repeat([]byte{0xab}, 31)},
	}, {
		name: "close-code 2^32",
		hex:  "44b5" + "a2" + "00" + "1b0000000100000000" + "0160",
		msg:  &msgDataTransportClose{CloseCode: MaxWebTransportCloseCode + 1},
	}, {
		name: "reason of 1025 bytes",
		hex:  "44b5" + "a2" + "0000" + "01" + "790401" + hex.EncodeToString([]byte(longReason)),
		msg:  &msgDataTransportClose{Reason: longReason},
	}} {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := hex.DecodeString(tc.hex)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := readMessage(bytes.NewReader(raw)); !errors.Is(err, ErrInvalidMessage) {
				t.Errorf("decode: got %v, want ErrInvalidMessage", err)
			}
			if err := writeMessage(tc.msg, new(bytes.Buffer)); !errors.Is(err, ErrInvalidMessage) {
				t.Errorf("encode: got %v, want ErrInvalidMessage", err)
			}
		})
	}
}
