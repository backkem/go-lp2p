package application

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	ospc "github.com/backkem/go-lp2p/openscreen-go/network"
)

const testTimeout = 10 * time.Second

func newAgent(t *testing.T, name string) *ospc.Agent {
	t.Helper()
	a, err := ospc.NewAgent(ospc.AgentConfig{
		DisplayName: name,
		KeepAlive:   ospc.KeepAliveConfig{Disabled: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	return a
}

// connectPair authenticates two agents over loopback, without mDNS.
func connectPair(t *testing.T) (server, client *ApplicationConnection) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	l := ospc.NewListener(newAgent(t, "App Server"), ospc.AgentTransportQUIC)
	if err := l.StartWithoutAdvertising("127.0.0.1:0"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	ra, err := ospc.DirectAgent("App Server", l.Addr().(*net.UDPAddr), l.TXT())
	if err != nil {
		t.Fatal(err)
	}

	psk := []byte("4321")
	type result struct {
		conn *ospc.Connection
		err  error
	}
	serverCh := make(chan result, 1)
	go func() {
		uc, err := l.Accept(ctx)
		if err != nil {
			serverCh <- result{err: err}
			return
		}
		defer uc.Close()
		if _, err := uc.AcceptAuthenticate(ctx); err != nil {
			serverCh <- result{err: err}
			return
		}
		conn, err := uc.AuthenticatePSK(ctx, psk)
		serverCh <- result{conn, err}
	}()

	uc, err := ra.Dial(ctx, ospc.AgentTransportQUIC, newAgent(t, "App Client"))
	if err != nil {
		t.Fatal(err)
	}
	defer uc.Close()
	cConn, err := uc.AuthenticatePSK(ctx, psk)
	if err != nil {
		t.Fatal(err)
	}
	s := <-serverCh
	if s.err != nil {
		t.Fatal(s.err)
	}

	server, client = NewApplicationConnection(s.conn), NewApplicationConnection(cConn)
	t.Cleanup(func() {
		client.Close()
		server.Close()
	})
	return server, client
}

func TestAgentInfoRequestResponse(t *testing.T) {
	server, client := connectPair(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		_, respond, err := server.ReceiveAgentInfoRequest(ctx)
		if err != nil {
			errCh <- err
			return
		}
		errCh <- respond(&ospc.MsgAgentInfo{DisplayName: "App Server", StateToken: "abcdefgh"})
	}()

	info, err := client.SendAgentInfoRequest(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if info.DisplayName != "App Server" {
		t.Fatalf("got %q", info.DisplayName)
	}
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}
}

// TestRequestCorrelation answers concurrent requests in reverse order, with
// an unrelated message in between: each response reaches its request by
// request-id, the other message goes to ReceiveMessage.
func TestRequestCorrelation(t *testing.T) {
	server, client := connectPair(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	const n = 3
	type answer struct {
		id   ospc.RequestID
		name string
		err  error
	}
	answers := make(chan answer, n)
	for i := 0; i < n; i++ {
		go func() {
			id := client.NextRequestID()
			var resp ospc.MsgAgentInfoResponse
			err := client.Request(ctx, ospc.TypeKeyAgentInfoRequest, id, &ospc.MsgAgentInfoRequest{RequestID: id},
				ospc.TypeKeyAgentInfoResponse, &resp)
			answers <- answer{id, resp.AgentInfo.DisplayName, err}
		}()
	}

	var reqs []ospc.MsgAgentInfoRequest
	for len(reqs) < n {
		msg, err := server.ReceiveMessage(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if msg.TypeKey != ospc.TypeKeyAgentInfoRequest {
			t.Fatalf("unexpected type %d", msg.TypeKey)
		}
		var req ospc.MsgAgentInfoRequest
		if err := msg.Decode(&req); err != nil {
			t.Fatal(err)
		}
		reqs = append(reqs, req)
	}

	// An unsolicited message, not a response to any pending request.
	event := &ospc.MsgAgentInfoResponse{RequestID: 9999, AgentInfo: ospc.MsgAgentInfo{DisplayName: "event"}}
	if err := server.SendMessage(ctx, ospc.TypeKeyAgentInfoEvent, event); err != nil {
		t.Fatal(err)
	}
	for i := n - 1; i >= 0; i-- {
		err := server.SendMessage(ctx, ospc.TypeKeyAgentInfoResponse, &ospc.MsgAgentInfoResponse{
			RequestID: reqs[i].RequestID,
			AgentInfo: ospc.MsgAgentInfo{DisplayName: name(reqs[i].RequestID)},
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	for i := 0; i < n; i++ {
		a := <-answers
		if a.err != nil {
			t.Fatal(a.err)
		}
		if a.name != name(a.id) {
			t.Fatalf("request %d got response %q", a.id, a.name)
		}
	}

	msg, err := client.ReceiveMessage(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if msg.TypeKey != ospc.TypeKeyAgentInfoEvent {
		t.Fatalf("got type %d, want agent-info-event", msg.TypeKey)
	}
}

func name(id ospc.RequestID) string {
	return "resp-" + string(rune('a'+int(id%26)))
}

func TestRequestAfterClose(t *testing.T) {
	_, client := connectPair(t)
	client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	if _, err := client.SendAgentInfoRequest(ctx); err == nil {
		t.Fatal("expected error after close")
	}
	if _, err := client.ReceiveMessage(ctx); !errors.Is(err, ErrClosed) {
		t.Fatalf("expected ErrClosed, got %v", err)
	}
}
