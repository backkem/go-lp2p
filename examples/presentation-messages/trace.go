package main

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/backkem/go-lp2p/openscreen-go/application"
	ospc "github.com/backkem/go-lp2p/openscreen-go/network"
)

// out prints the trace. The Go side and osp_demo's log are interleaved, so
// printing is serialized.
var out = struct {
	sync.Mutex
	start time.Time
}{start: time.Now()}

func printf(format string, args ...any) {
	out.Lock()
	defer out.Unlock()
	fmt.Fprintf(os.Stdout, "%6.3fs ", time.Since(out.start).Seconds())
	fmt.Fprintf(os.Stdout, format+"\n", args...)
}

// step prints a step heading.
func step(format string, args ...any) { printf("== "+format, args...) }

// show formats a message struct as {Field:value ...}.
func show(v any) string { return strings.TrimPrefix(fmt.Sprintf("%+v", v), "&") }

// demoLine matches the osp_demo log lines worth showing next to the trace:
// what its demo and presentation code say, and the authentication result.
var demoLine = regexp.MustCompile(`\[INFO:[^\]]*(osp_demo\.cc|presentation_\w+\.cc|SPAKE2: authenticated)[^\]]*\] (.*)$`)

// printDemoLine prints line, an osp_demo log line, if it is worth showing.
func printDemoLine(line string) {
	if m := demoLine.FindStringSubmatch(line); m != nil {
		printf("   osp_demo | %s", m[2])
	}
}

// newMessage returns a value to decode a received message of type k into,
// so the trace shows it with its CDDL field names.
func newMessage(k ospc.TypeKey) any {
	switch k {
	case application.TypeKeyPresentationURLAvailabilityRequest:
		return &application.PresentationURLAvailabilityRequest{}
	case application.TypeKeyPresentationURLAvailabilityResponse:
		return &application.PresentationURLAvailabilityResponse{}
	case application.TypeKeyPresentationURLAvailabilityEvent:
		return &application.PresentationURLAvailabilityEvent{}
	case application.TypeKeyPresentationStartRequest:
		return &application.PresentationStartRequest{}
	case application.TypeKeyPresentationStartResponse:
		return &application.PresentationStartResponse{}
	case application.TypeKeyPresentationTerminationRequest:
		return &application.PresentationTerminationRequest{}
	case application.TypeKeyPresentationTerminationResponse:
		return &application.PresentationTerminationResponse{}
	case application.TypeKeyPresentationTerminationEvent:
		return &application.PresentationTerminationEvent{}
	case application.TypeKeyPresentationConnectionOpenRequest:
		return &application.PresentationConnectionOpenRequest{}
	case application.TypeKeyPresentationConnectionOpenResponse:
		return &application.PresentationConnectionOpenResponse{}
	case application.TypeKeyPresentationConnectionCloseEvent:
		return &application.PresentationConnectionCloseEvent{}
	case application.TypeKeyPresentationChangeEvent:
		return &application.PresentationChangeEvent{}
	case application.TypeKeyPresentationConnectionMessage:
		return &application.PresentationConnectionMessage{}
	case ospc.TypeKeyAgentInfoRequest:
		return &ospc.MsgAgentInfoRequest{}
	}
	var body any
	return &body
}

// received is a message read from the peer, decoded with newMessage.
type received struct {
	TypeKey ospc.TypeKey
	Body    any
}

// peer is the application-protocol side of an authenticated connection. It
// sends each message on its own unidirectional QUIC stream (application.bs
// "Protocol Overview"; done by application.ApplicationConnection) and prints
// every message both ways.
type peer struct {
	app   *application.ApplicationConnection
	inbox chan received
}

// newPeer starts printing and queueing the messages received on conn.
func newPeer(conn *ospc.Connection) *peer {
	p := &peer{
		app:   application.NewApplicationConnection(conn),
		inbox: make(chan received, 64),
	}
	go p.receive()
	return p
}

func (p *peer) Close() error { return p.app.Close() }

// receive prints each message that is not a response to one of our
// requests (those are returned by request), and queues it for next. The
// peer may ask for our agent-info at any time (network.bs "Metadata
// Discovery"); that is answered right away.
func (p *peer) receive() {
	defer close(p.inbox)
	for {
		msg, err := p.app.ReceiveMessage(context.Background())
		if err != nil {
			return
		}
		body := newMessage(msg.TypeKey)
		if err := msg.Decode(body); err != nil {
			printf("<-- %s: undecodable: %v", application.MessageName(msg.TypeKey), err)
			continue
		}
		printf("<-- %s %s", application.MessageName(msg.TypeKey), show(body))
		if msg.TypeKey == ospc.TypeKeyAgentInfoRequest {
			info := p.app.LocalAgent().AgentInfoMessage()
			printf("--> agent-info-response %s", show(info))
			if err := p.app.AnswerAgentInfoRequest(context.Background(), msg); err != nil {
				printf("    error: %v", err)
			}
			continue
		}
		p.inbox <- received{TypeKey: msg.TypeKey, Body: body}
	}
}

// next returns the next received message.
func (p *peer) next(ctx context.Context) (received, error) {
	select {
	case m, ok := <-p.inbox:
		if !ok {
			return received{}, fmt.Errorf("connection closed: %v", p.app.Connection().Err())
		}
		return m, nil
	case <-ctx.Done():
		return received{}, ctx.Err()
	}
}

// expect waits for a message of type k and returns its body. Other messages
// were printed already and are skipped.
func (p *peer) expect(ctx context.Context, k ospc.TypeKey) (any, error) {
	for {
		m, err := p.next(ctx)
		if err != nil {
			return nil, fmt.Errorf("waiting for %s: %w", application.MessageName(k), err)
		}
		if m.TypeKey == k {
			return m.Body, nil
		}
	}
}

// send sends one message.
func (p *peer) send(ctx context.Context, k ospc.TypeKey, msg any) error {
	printf("--> %s %s", application.MessageName(k), show(msg))
	return p.app.SendMessage(ctx, k, msg)
}

// request sends req, which carries id as its request-id, and decodes the
// response of type respType with the same request-id into resp.
func (p *peer) request(ctx context.Context, reqType ospc.TypeKey, id ospc.RequestID, req any, respType ospc.TypeKey, resp any) error {
	printf("--> %s %s", application.MessageName(reqType), show(req))
	if err := p.app.Request(ctx, reqType, id, req, respType, resp); err != nil {
		return fmt.Errorf("%s: %w", application.MessageName(reqType), err)
	}
	printf("<-- %s %s", application.MessageName(respType), show(resp))
	return nil
}
