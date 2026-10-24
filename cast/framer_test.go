package cast

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
	"testing/iotest"
	"time"
)

func testMessage() *CastMessage {
	return NewStringMessage("source", "destination", "namespace", "payload")
}

func TestFramerRoundTrip(t *testing.T) {
	frame, err := EncodeFrame(testMessage())
	if err != nil {
		t.Fatal(err)
	}
	if n := binary.BigEndian.Uint32(frame); int(n) != len(frame)-HeaderSize {
		t.Fatalf("header %d, body %d", n, len(frame)-HeaderSize)
	}
	if !bytes.Equal(frame[HeaderSize:], unhex(t, goldenBasic)) {
		t.Fatal("frame body does not match golden encoding")
	}

	var stream bytes.Buffer
	for i := 0; i < 3; i++ {
		if err := WriteMessage(&stream, testMessage()); err != nil {
			t.Fatal(err)
		}
	}
	// One byte at a time exercises partial reads of header and body.
	r := iotest.OneByteReader(&stream)
	for i := 0; i < 3; i++ {
		m, err := ReadMessage(r)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(m, testMessage()) {
			t.Fatalf("got %v", m)
		}
	}
	if _, err := ReadMessage(r); err != io.EOF {
		t.Fatalf("want EOF, got %v", err)
	}
}

func TestTryDecodeFrameIncremental(t *testing.T) {
	frame, _ := EncodeFrame(testMessage())
	for i := 0; i < len(frame); i++ {
		m, n, err := TryDecodeFrame(frame[:i])
		if m != nil || n != 0 || err != nil {
			t.Fatalf("prefix %d: got %v %d %v", i, m, n, err)
		}
	}
	two := append(append([]byte{}, frame...), frame...)
	m, n, err := TryDecodeFrame(two)
	if err != nil || n != len(frame) || m.Payload() != "payload" {
		t.Fatalf("got %v %d %v", m, n, err)
	}
}

func TestFramerIllegalLargeMessage(t *testing.T) {
	// Fixtures from message_framer_unittest.cc: 0x58585858 and 65537.
	for _, hdr := range [][]byte{{0x58, 0x58, 0x58, 0x58}, {0x00, 0x01, 0x00, 0x01}} {
		frame, _ := EncodeFrame(testMessage())
		copy(frame, hdr)
		if _, err := ReadMessage(bytes.NewReader(frame)); !errors.Is(err, ErrMessageTooLarge) {
			t.Fatalf("ReadMessage %x: got %v", hdr, err)
		}
		if _, _, err := TryDecodeFrame(frame); !errors.Is(err, ErrMessageTooLarge) {
			t.Fatalf("TryDecodeFrame %x: got %v", hdr, err)
		}
	}
}

func TestFramerSizeLimit(t *testing.T) {
	overhead := len(NewStringMessage("s", "d", "n", "").Marshal())
	// The length varint of payload_utf8 grows from 1 to 3 bytes for large
	// payloads, so account for the 2 extra bytes.
	fits := MaxBodySize - overhead - 2
	if _, err := EncodeFrame(NewStringMessage("s", "d", "n", strings.Repeat("x", fits))); err != nil {
		t.Fatalf("max-size message rejected: %v", err)
	}
	if _, err := EncodeFrame(NewStringMessage("s", "d", "n", strings.Repeat("x", fits+1))); !errors.Is(err, ErrMessageTooLarge) {
		t.Fatalf("oversize message accepted: %v", err)
	}
	// A max-size body is accepted on read.
	frame, _ := EncodeFrame(NewStringMessage("s", "d", "n", strings.Repeat("x", fits)))
	if binary.BigEndian.Uint32(frame) != MaxBodySize {
		t.Fatalf("expected body of exactly MaxBodySize, got %d", binary.BigEndian.Uint32(frame))
	}
	if _, err := ReadMessage(bytes.NewReader(frame)); err != nil {
		t.Fatal(err)
	}
}

func TestFramerTruncatedAndGarbage(t *testing.T) {
	frame, _ := EncodeFrame(testMessage())
	if _, err := ReadMessage(bytes.NewReader(frame[:len(frame)-1])); err != io.ErrUnexpectedEOF {
		t.Fatalf("truncated body: got %v", err)
	}
	if _, err := ReadMessage(bytes.NewReader(frame[:2])); err != io.ErrUnexpectedEOF {
		t.Fatalf("truncated header: got %v", err)
	}
	garbage := []byte{0, 0, 0, 3, 0xff, 0xff, 0xff}
	if _, err := ReadMessage(bytes.NewReader(garbage)); err == nil {
		t.Fatal("garbage body accepted")
	}
	if _, err := ReadMessage(iotest.DataErrReader(bytes.NewReader(frame))); err != nil {
		t.Fatalf("data+err reader: %v", err)
	}
}

func FuzzReadMessage(f *testing.F) {
	frame, _ := EncodeFrame(testMessage())
	f.Add(frame)
	f.Add([]byte{0, 0, 0, 4, 0x1a, 0x02, 0x08, 0x01}) // valid proto, not a CastMessage
	f.Add([]byte{0x58, 0x58, 0x58, 0x58})
	f.Fuzz(func(t *testing.T, data []byte) {
		m, err := ReadMessage(bytes.NewReader(data))
		if err != nil {
			return
		}
		// Anything accepted must re-encode and decode to the same message.
		re, err := EncodeFrame(m)
		if err != nil {
			t.Fatalf("re-encode: %v", err)
		}
		m2, err := ReadMessage(bytes.NewReader(re))
		if err != nil || !reflect.DeepEqual(m, m2) {
			t.Fatalf("round trip: %v %v %v", m, m2, err)
		}
	})
}

// TestSocketSendsMessageInOneTLSRecord: openscreen's CastSocket logs an error
// for every TLS read that ends mid-frame, so each CastMessage should travel
// in a single TLS record (found in interop with cast_sender, whose auth
// response read failed once with InsufficientBuffer before Go's dynamic
// record sizing was disabled).
func TestSocketSendsMessageInOneTLSRecord(t *testing.T) {
	snd, rcv := socketPair(t, nil)
	payload := make([]byte, 8*1024) // larger than Go's initial ~1 KiB records
	msg := NewBinaryMessage("a", "b", AuthNamespace, payload)
	want := len(msg.Marshal()) + 4

	for _, dir := range []struct {
		name     string
		from, to *Socket
	}{{"dialer to listener", snd, rcv}, {"listener to dialer", rcv, snd}} {
		if err := dir.from.Send(msg); err != nil {
			t.Fatal(err)
		}
		// tls.Conn.Read returns data from at most one record.
		buf := make([]byte, 64*1024)
		_ = dir.to.Conn().SetReadDeadline(time.Now().Add(5 * time.Second))
		n, err := dir.to.Conn().Read(buf)
		if err != nil {
			t.Fatal(err)
		}
		if n != want {
			t.Errorf("%s: first TLS record carried %d of %d frame bytes", dir.name, n, want)
		}
	}
}
