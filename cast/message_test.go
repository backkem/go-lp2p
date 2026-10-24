package cast

import (
	"bytes"
	"encoding/hex"
	"errors"
	"reflect"
	"testing"
)

// Golden encodings were produced independently with google.golang.org/protobuf
// (dynamicpb built from cast_channel.proto, deterministic marshal).
const (
	goldenBasic   = "08001206736f757263651a0b64657374696e6174696f6e22096e616d657370616365280032077061796c6f6164"
	goldenBinary  = "0803120873656e6465722d301a0a72656365697665722d30222875726e3a782d636173743a636f6d2e676f6f676c652e636173742e74702e6465766963656175746828013a030102ff400148ac02"
	goldenChal    = "0a141210000102030405060708090a0b0c0d0e0f1801"
	goldenResp    = "121f0a037369671204636572741a0269311a02693220022a016e30013a0363726c"
	goldenAuthErr = "1a020801"
)

func unhex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func ptr[T any](v T) *T { return &v }

func TestCastMessageGolden(t *testing.T) {
	cases := []struct {
		name   string
		golden string
		msg    *CastMessage
	}{
		{"basic (message_framer_unittest fixture)", goldenBasic,
			NewStringMessage("source", "destination", "namespace", "payload")},
		{"binary with chunking fields", goldenBinary, &CastMessage{
			ProtocolVersion: CastV2_1_3,
			SourceID:        "sender-0",
			DestinationID:   "receiver-0",
			Namespace:       AuthNamespace,
			PayloadType:     PayloadBinary,
			PayloadBinary:   []byte{1, 2, 0xff},
			Continued:       ptr(true),
			RemainingLength: ptr(uint32(300)),
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			want := unhex(t, tc.golden)
			if got := tc.msg.Marshal(); !bytes.Equal(got, want) {
				t.Fatalf("marshal:\n got %x\nwant %x", got, want)
			}
			var back CastMessage
			if err := back.Unmarshal(want); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(&back, tc.msg) {
				t.Fatalf("unmarshal: got %+v want %+v", back, *tc.msg)
			}
		})
	}
}

func TestDeviceAuthMessageGolden(t *testing.T) {
	nonce := make([]byte, 16)
	for i := range nonce {
		nonce[i] = byte(i)
	}
	cases := []struct {
		name   string
		golden string
		msg    *DeviceAuthMessage
	}{
		{"challenge", goldenChal, &DeviceAuthMessage{Challenge: &AuthChallenge{
			SenderNonce: nonce, HashAlgorithm: ptr(HashSHA256)}}},
		{"response", goldenResp, &DeviceAuthMessage{Response: &AuthResponse{
			Signature:               []byte("sig"),
			ClientAuthCertificate:   []byte("cert"),
			IntermediateCertificate: [][]byte{[]byte("i1"), []byte("i2")},
			SignatureAlgorithm:      ptr(SignatureRSASSAPSS),
			SenderNonce:             []byte("n"),
			HashAlgorithm:           ptr(HashSHA256),
			CRL:                     []byte("crl"),
		}}},
		{"error", goldenAuthErr, &DeviceAuthMessage{Error: &AuthError{ErrorType: AuthErrorNoTLS}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			want := unhex(t, tc.golden)
			if got := tc.msg.Marshal(); !bytes.Equal(got, want) {
				t.Fatalf("marshal:\n got %x\nwant %x", got, want)
			}
			var back DeviceAuthMessage
			if err := back.Unmarshal(want); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(&back, tc.msg) {
				t.Fatalf("unmarshal mismatch:\n got %+v\nwant %+v", back, *tc.msg)
			}
		})
	}
}

func TestAuthDefaults(t *testing.T) {
	var c AuthChallenge
	var r AuthResponse
	if c.Hash() != HashSHA1 || r.Hash() != HashSHA1 {
		t.Fatal("proto default hash must be SHA1")
	}
}

func TestCastMessageUnmarshalErrors(t *testing.T) {
	full := NewStringMessage("s", "d", "ns", "p").Marshal()

	// Every required field must be present.
	missingDest := appendVarintField(nil, 1, 0)
	missingDest = appendStringField(missingDest, 2, "s")
	missingDest = appendStringField(missingDest, 4, "ns")
	missingDest = appendVarintField(missingDest, 5, 0)
	var m CastMessage
	if err := m.Unmarshal(missingDest); !errors.Is(err, ErrMissingRequiredField) {
		t.Fatalf("missing destination: got %v", err)
	}

	// Truncation must fail, never panic, except at the boundary before the
	// optional payload_utf8 field (3 bytes: tag, length, "p").
	for i := 1; i < len(full); i++ {
		if i == len(full)-3 {
			continue
		}
		if err := m.Unmarshal(full[:i]); err == nil {
			t.Fatalf("truncated at %d: expected error", i)
		}
	}

	// Unknown fields of every wire type are skipped.
	ext := append([]byte{}, full...)
	ext = appendVarintField(ext, 100, 7)
	ext = appendBytesField(ext, 101, []byte("x"))
	ext = append(appendTag(ext, 102, wireFixed32), 1, 2, 3, 4)
	ext = append(appendTag(ext, 103, wireFixed64), 1, 2, 3, 4, 5, 6, 7, 8)
	if err := m.Unmarshal(ext); err != nil || m.Payload() != "p" {
		t.Fatalf("unknown fields: %v %+v", err, m)
	}

	// Wrong wire type for a known field.
	bad := appendStringField(nil, 1, "x")
	if err := m.Unmarshal(bad); err == nil {
		t.Fatal("expected wire type error")
	}

	// Invalid UTF-8 in payload_utf8.
	inv := appendVarintField(nil, 1, 0)
	inv = appendStringField(inv, 2, "s")
	inv = appendStringField(inv, 3, "d")
	inv = appendStringField(inv, 4, "n")
	inv = appendVarintField(inv, 5, 0)
	inv = appendBytesField(inv, 6, []byte{0xff, 0xfe})
	if err := m.Unmarshal(inv); err == nil {
		t.Fatal("expected UTF-8 error")
	}

	// Varint overflow.
	over := []byte{0x08, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x7f}
	if err := m.Unmarshal(over); !errors.Is(err, errOverflow) {
		t.Fatalf("overflow: got %v", err)
	}
}

func TestDeviceAuthResponseRequiresFields(t *testing.T) {
	inner := appendBytesField(nil, 1, []byte("sig")) // no client_auth_certificate
	var m DeviceAuthMessage
	if err := m.Unmarshal(appendBytesField(nil, 2, inner)); !errors.Is(err, ErrMissingRequiredField) {
		t.Fatalf("got %v", err)
	}
	if err := m.Unmarshal(appendBytesField(nil, 3, nil)); !errors.Is(err, ErrMissingRequiredField) {
		t.Fatalf("auth error without type: got %v", err)
	}
}

func TestNegativeEnumEncoding(t *testing.T) {
	// proto2 enums are int32 and negative values are sign-extended.
	m := NewStringMessage("s", "d", "n", "")
	m.ProtocolVersion = -1
	var back CastMessage
	if err := back.Unmarshal(m.Marshal()); err != nil || back.ProtocolVersion != -1 {
		t.Fatalf("got %v %v", back.ProtocolVersion, err)
	}
}
