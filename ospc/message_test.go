package ospc

import (
	"bytes"
	"testing"
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
