package ospc

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"testing"
	"time"

	"github.com/quic-go/quic-go"
)

// newQuicPair connects two QuicNetworkConnections over loopback.
func newQuicPair(t *testing.T, clientKA, serverKA KeepAliveConfig) (client, server *QuicNetworkConnection) {
	t.Helper()

	cert, err := generateCert("test", 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	serverTLS := &tls.Config{
		Certificates: []tls.Certificate{*cert},
		NextProtos:   []string{ALPN_OSP},
	}
	clientTLS := &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{ALPN_OSP},
	}

	l, err := (&QuicTransport{KeepAlive: serverKA}).ListenAddr("127.0.0.1:0", serverTLS)
	if err != nil {
		t.Fatal(err)
	}
	ql := l.(*QuicNetworkListener)
	t.Cleanup(func() { ql.listener.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	accepted := make(chan NetworkConnection, 1)
	go func() {
		nc, err := l.Accept(ctx)
		if err != nil {
			accepted <- nil
			return
		}
		accepted <- nc
	}()

	nc, err := (&QuicTransport{KeepAlive: clientKA}).DialAddr(ctx, l.Addr().String(), clientTLS)
	if err != nil {
		t.Fatal(err)
	}
	client = nc.(*QuicNetworkConnection)
	t.Cleanup(func() { client.Close() })

	snc := <-accepted
	if snc == nil {
		t.Fatal("accept failed")
	}
	server = snc.(*QuicNetworkConnection)
	t.Cleanup(func() { server.Close() })

	return client, server
}

var noKeepAlive = KeepAliveConfig{Disabled: true}

func readMessageTimeout(t *testing.T, c *QuicNetworkConnection) interface{} {
	t.Helper()

	type result struct {
		msg interface{}
		err error
	}
	ch := make(chan result, 1)
	go func() {
		msg, err := c.ReadMessage()
		ch <- result{msg, err}
	}()
	select {
	case r := <-ch:
		if r.err != nil {
			t.Fatal(r.err)
		}
		return r.msg
	case <-time.After(2 * time.Second):
		t.Fatal("timeout reading message")
		return nil
	}
}

func agentInfoRequest(id uint64) *msgAgentInfoRequest {
	return &msgAgentInfoRequest{msgRequest: msgRequest{RequestId: msgRequestId(id)}}
}

func expectRequestID(t *testing.T, msg interface{}, id uint64) {
	t.Helper()

	req, ok := msg.(*msgAgentInfoRequest)
	if !ok {
		t.Fatalf("unexpected message type %T", msg)
	}
	if uint64(req.RequestId) != id {
		t.Fatalf("got RequestId %d, expected %d", req.RequestId, id)
	}
}

// waitFor polls cond until it holds or the timeout expires.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timeout waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestQuicConsecutiveMessagesInStream(t *testing.T) {
	client, server := newQuicPair(t, noKeepAlive, noKeepAlive)

	buf := new(bytes.Buffer)
	for i := uint64(1); i <= 3; i++ {
		if err := writeMessage(agentInfoRequest(i), buf); err != nil {
			t.Fatal(err)
		}
	}

	s, err := client.conn.OpenUniStreamSync(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Write(buf.Bytes()); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	for i := uint64(1); i <= 3; i++ {
		expectRequestID(t, readMessageTimeout(t, server), i)
	}
}

func TestQuicStalledStreamDoesNotBlock(t *testing.T) {
	client, server := newQuicPair(t, noKeepAlive, noKeepAlive)

	buf := new(bytes.Buffer)
	if err := writeMessage(agentInfoRequest(1), buf); err != nil {
		t.Fatal(err)
	}

	// Stalled stream: only part of a message, never finished.
	stalled, err := client.conn.OpenUniStreamSync(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stalled.Write(buf.Bytes()[:2]); err != nil {
		t.Fatal(err)
	}

	// A message on another stream is still delivered.
	if err := writeMessage(agentInfoRequest(2), client); err != nil {
		t.Fatal(err)
	}
	expectRequestID(t, readMessageTimeout(t, server), 2)

	// Finishing the stalled stream delivers its message.
	if _, err := stalled.Write(buf.Bytes()[2:]); err != nil {
		t.Fatal(err)
	}
	stalled.Close()
	expectRequestID(t, readMessageTimeout(t, server), 1)
}

func TestQuicKeepAlive(t *testing.T) {
	interval := 50 * time.Millisecond
	client, server := newQuicPair(t, KeepAliveConfig{Interval: interval}, noKeepAlive)

	// The idle client sends a request, the server answers it.
	waitFor(t, "keep-alive response", func() bool {
		return client.keepAlive.responsesReceived.Load() >= 1
	})
	if server.keepAlive.requestsReceived.Load() < 1 {
		t.Fatal("server didn't receive agent-status-request")
	}
	if n := server.keepAlive.responsesReceived.Load(); n != 0 {
		t.Fatalf("server with disabled keep-alive got %d responses", n)
	}

	// Keep-alive continues after the hand-off to the application protocol.
	clientApp, err := client.IntoApplicationConnection()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.IntoApplicationConnection(); err != nil {
		t.Fatal(err)
	}
	handedOff := client.keepAlive.responsesReceived.Load()
	waitFor(t, "keep-alive response after hand-off", func() bool {
		return client.keepAlive.responsesReceived.Load() >= handedOff+2
	})

	// Sending application data resets the timer: no requests while busy.
	s, err := clientApp.OpenStreamSync(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.Write([]byte{0}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(interval / 2) // Let an in-flight request settle.
	busy := server.keepAlive.requestsReceived.Load()
	for i := 0; i < 10; i++ {
		if _, err := s.Write([]byte{0}); err != nil {
			t.Fatal(err)
		}
		time.Sleep(interval / 5)
	}
	if n := server.keepAlive.requestsReceived.Load(); n != busy {
		t.Fatalf("got %d keep-alive requests while busy", n-busy)
	}
}

func TestQuicCloseErrorCode(t *testing.T) {
	client, server := newQuicPair(t, noKeepAlive, noKeepAlive)

	if err := client.Close(); err != nil {
		t.Fatal(err)
	}

	select {
	case <-server.conn.Context().Done():
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for connection close")
	}
	var appErr *quic.ApplicationError
	if err := context.Cause(server.conn.Context()); !errors.As(err, &appErr) {
		t.Fatalf("expected application error, got %v", err)
	}
	if !appErr.Remote || appErr.ErrorCode != QuicErrorCodeNoLongerNeeded {
		t.Fatalf("unexpected close error: %v", appErr)
	}

	if _, err := server.ReadMessage(); err == nil {
		t.Fatal("expected ReadMessage to fail on closed connection")
	}
}
