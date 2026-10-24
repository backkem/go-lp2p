package ospc

import (
	"bytes"
	"io"
	"reflect"
	"testing"
	"testing/iotest"
)

func TestAgentInfoRequest(t *testing.T) {
	expected := &msgAgentInfoRequest{
		msgRequest: msgRequest{
			RequestId: 100,
		},
	}

	buf := new(bytes.Buffer)

	err := writeMessage(expected, buf)
	if err != nil {
		t.Fatal(err)
	}

	msg, err := readMessage(buf)
	if err != nil {
		t.Fatal(err)
	}

	actual, ok := msg.(*msgAgentInfoRequest)
	if !ok {
		t.Fatalf("different message type")
	}

	if expected.RequestId != actual.RequestId {
		t.Fatalf("different RequestId")
	}
}

func TestAgentInfoResponse(t *testing.T) {
	expected := &msgAgentInfoResponse{
		msgResponse: msgResponse{
			RequestId: 100,
		},
		AgentInfo: msgAgentInfo{
			DisplayName: "Agent007",
			ModelName:   "Bond",
			Capabilities: []msgAgentCapability{
				AgentCapabilityDataChannels,
				AgentCapabilityQuickTransport,
			},
			StateToken: "01234567",
			Locales:    []string{"EN"},
		},
	}

	buf := new(bytes.Buffer)

	err := writeMessage(expected, buf)
	if err != nil {
		t.Fatal(err)
	}

	msg, err := readMessage(buf)
	if err != nil {
		t.Fatal(err)
	}

	actual, ok := msg.(*msgAgentInfoResponse)
	if !ok {
		t.Fatalf("different message type")
	}

	if expected.RequestId != actual.RequestId {
		t.Fatalf("different RequestId")
	}

	if expected.AgentInfo.DisplayName != actual.AgentInfo.DisplayName {
		t.Fatalf("different DisplayName")
	}

	if expected.AgentInfo.Capabilities[0] != actual.AgentInfo.Capabilities[0] {
		t.Fatalf("different capability")
	}
}

func TestConsecutiveMessage(t *testing.T) {
	buf := new(bytes.Buffer)

	oneExpected := &msgAgentInfoRequest{
		msgRequest: msgRequest{
			RequestId: 1,
		},
	}

	err := writeMessage(oneExpected, buf)
	if err != nil {
		t.Fatal(err)
	}

	twoExpected := &msgAgentInfoRequest{
		msgRequest: msgRequest{
			RequestId: 2,
		},
	}

	err = writeMessage(twoExpected, buf)
	if err != nil {
		t.Fatal(err)
	}

	msgOne, err := readMessage(buf)
	if err != nil {
		t.Fatal(err)
	}

	oneActual, ok := msgOne.(*msgAgentInfoRequest)
	if !ok {
		t.Fatalf("one: different message type")
	}

	if oneExpected.RequestId != oneActual.RequestId {
		t.Fatalf("one: different RequestId")
	}

	msgTwo, err := readMessage(buf)
	if err != nil {
		t.Fatal(err)
	}

	twoActual, ok := msgTwo.(*msgAgentInfoRequest)
	if !ok {
		t.Fatalf("two: different message type")
	}

	if twoExpected.RequestId != twoActual.RequestId {
		t.Fatalf("two: different RequestId")
	}

}

func TestConsecutiveMessageExactReads(t *testing.T) {
	buf := new(bytes.Buffer)
	expected := []interface{}{
		&msgAgentInfoResponse{
			msgResponse: msgResponse{RequestId: 1},
			AgentInfo: msgAgentInfo{
				DisplayName:  "Agent007",
				Capabilities: []msgAgentCapability{AgentCapabilityDataChannels},
				Locales:      []string{"EN", "NL"},
			},
		},
		&msgDataFrame{EncodingId: 1, Payload: bytes.Repeat([]byte{0xff}, 300)},
		&msgAgentStatusRequest{msgRequest: msgRequest{RequestId: 2}},
	}
	for _, msg := range expected {
		if err := writeMessage(msg, buf); err != nil {
			t.Fatal(err)
		}
	}
	// Indefinite length CBOR: ["a", "b"] as a data-frame-like body is not a
	// valid message, so check the raw item reader directly.
	indefinite := []byte{0x9f, 0x61, 'a', 0x7f, 0x61, 'b', 0xff, 0xff}

	// Readers that return one byte at a time and data together with io.EOF.
	r := iotest.DataErrReader(iotest.OneByteReader(buf))
	for i, want := range expected {
		msg, err := readMessage(r)
		if err != nil {
			t.Fatalf("message %d: %v", i, err)
		}
		if !reflect.DeepEqual(msg, want) {
			t.Fatalf("message %d: got %#v, expected %#v", i, msg, want)
		}
	}
	if _, err := readMessage(r); err != io.EOF {
		t.Fatalf("expected io.EOF, got %v", err)
	}

	ir := bytes.NewReader(append(indefinite, 0x01))
	item, err := readCBORItem(ir)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(item, indefinite) || ir.Len() != 1 {
		t.Fatalf("indefinite item read incorrectly: %x, %d left", item, ir.Len())
	}

	if _, err := readCBORItem(bytes.NewReader([]byte{0x82, 0x01})); err != io.ErrUnexpectedEOF {
		t.Fatalf("expected io.ErrUnexpectedEOF, got %v", err)
	}
}
