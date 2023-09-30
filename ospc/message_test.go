package ospc

import (
	"bytes"
	"testing"
)

func TestAgentInfoRequest(t *testing.T) {
	expected := &msgAgentInfoRequest{
		RequestID: 100,
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

	if expected.RequestID != actual.RequestID {
		t.Fatalf("different RequestID")
	}
}

func TestAgentInfoResponse(t *testing.T) {
	expected := &msgAgentInfoResponse{
		RequestID: 100,
		AgentInfo: msgPartAgentInfo{
			DisplayName: "Agent007",
			ModelName:   "Bond",
			Capabilities: []agentCapability{
				agentCapabilityExchangeData,
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

	if expected.RequestID != actual.RequestID {
		t.Fatalf("different RequestID")
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
		RequestID: 1,
	}

	err := writeMessage(oneExpected, buf)
	if err != nil {
		t.Fatal(err)
	}

	twoExpected := &msgAgentInfoRequest{
		RequestID: 2,
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

	if oneExpected.RequestID != oneActual.RequestID {
		t.Fatalf("one: different RequestID")
	}

	msgTwo, err := readMessage(buf)
	if err != nil {
		t.Fatal(err)
	}

	twoActual, ok := msgTwo.(*msgAgentInfoRequest)
	if !ok {
		t.Fatalf("two: different message type")
	}

	if twoExpected.RequestID != twoActual.RequestID {
		t.Fatalf("two: different RequestID")
	}

}
