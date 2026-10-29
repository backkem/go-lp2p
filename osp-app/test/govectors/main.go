// Prints the CBOR encodings of openscreen-go's application messages, the
// interop vectors in ../vectors.ts: go run ./osp-app/test/govectors

package main

import (
	"encoding/hex"
	"fmt"

	app "github.com/backkem/go-lp2p/openscreen-go/application"
	ospc "github.com/backkem/go-lp2p/openscreen-go/network"
)

func p(name string, v interface{}) {
	b, err := ospc.EncodeCBOR(v)
	if err != nil {
		panic(err)
	}
	fmt.Printf("%q: %q,\n", name, hex.EncodeToString(b))
}

func main() {
	code := uint64(200)
	em := "bye"
	p("agent-info-response", &ospc.MsgAgentInfoResponse{RequestID: 1, AgentInfo: ospc.MsgAgentInfo{DisplayName: "TV", ModelName: "M1", Capabilities: []ospc.AgentCapability{3, 5}, StateToken: "abcd1234", Locales: []string{"en-US"}}})
	p("agent-info-response/no-model", &ospc.MsgAgentInfoResponse{RequestID: 1, AgentInfo: ospc.MsgAgentInfo{DisplayName: "TV", Capabilities: []ospc.AgentCapability{3}, StateToken: "abcd1234"}})
	p("presentation-url-availability-request", &app.PresentationURLAvailabilityRequest{RequestID: 2, URLs: []string{"https://example.com/"}, WatchDuration: 60000000, WatchID: 1})
	p("presentation-url-availability-response", &app.PresentationURLAvailabilityResponse{RequestID: 2, URLAvailabilities: []app.URLAvailability{0, 1, 10}})
	p("presentation-url-availability-event", &app.PresentationURLAvailabilityEvent{WatchID: 1, URLAvailabilities: []app.URLAvailability{1}})
	p("presentation-start-request", &app.PresentationStartRequest{RequestID: 3, PresentationID: "SD3I7BVM34KZ4M2CT4YG7KENWJ", URL: "https://example.com/", Headers: []app.HTTPHeader{{Key: "Accept-Language", Value: "en"}}})
	p("presentation-start-request/no-headers", &app.PresentationStartRequest{RequestID: 3, PresentationID: "SD3I7BVM34KZ4M2CT4YG7KENWJ", URL: "https://example.com/"})
	p("presentation-start-response", &app.PresentationStartResponse{RequestID: 3, Result: 1, ConnectionID: 7, HTTPResponseCode: &code})
	p("presentation-start-response/no-code", &app.PresentationStartResponse{RequestID: 3, Result: 10})
	p("presentation-termination-request", &app.PresentationTerminationRequest{RequestID: 4, PresentationID: "SD3I7BVM34KZ4M2CT4YG7KENWJ", Reason: 1})
	p("presentation-termination-response", &app.PresentationTerminationResponse{RequestID: 4, Result: 1})
	p("presentation-termination-event", &app.PresentationTerminationEvent{PresentationID: "SD3I7BVM34KZ4M2CT4YG7KENWJ", Source: 2, Reason: 100})
	p("presentation-connection-open-request", &app.PresentationConnectionOpenRequest{RequestID: 5, PresentationID: "SD3I7BVM34KZ4M2CT4YG7KENWJ", URL: "https://example.com/"})
	p("presentation-connection-open-response", &app.PresentationConnectionOpenResponse{RequestID: 5, Result: 1, ConnectionID: 8, ConnectionCount: 2})
	p("presentation-connection-close-event", &app.PresentationConnectionCloseEvent{ConnectionID: 8, Reason: 1, ErrorMessage: &em, ConnectionCount: 1})
	p("presentation-change-event", &app.PresentationChangeEvent{PresentationID: "SD3I7BVM34KZ4M2CT4YG7KENWJ", ConnectionCount: 1})
	p("presentation-connection-message/text", &app.PresentationConnectionMessage{ConnectionID: 7, Message: "hello"})
	p("presentation-connection-message/bytes", &app.PresentationConnectionMessage{ConnectionID: 7, Message: []byte{1, 2, 3}})
}
