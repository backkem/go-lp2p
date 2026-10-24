package ospc

import (
	"context"
	"errors"
	"io"
	"net"
	"testing"
	"time"
)

func authenticatedPair(t *testing.T) (server, client *Connection) {
	t.Helper()
	psk := []byte("1234")
	s, c := authenticatePair(t, psk, psk)
	if s.err != nil || c.err != nil {
		t.Fatalf("authentication failed: server: %v, client: %v", s.err, c.err)
	}
	return s.conn, c.conn
}

// TestDataChannelOpenResponse checks the acceptor's side on the wire: it
// answers a data-channel-open-request on the request's stream.
func TestDataChannelOpenResponse(t *testing.T) {
	checkNoLeaks(t)
	server, client := authenticatedPair(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	open := func(id uint64) (ApplicationStream, *msgDataChannelOpenResponse) {
		t.Helper()
		req := &msgDataChannelOpenRequest{msgRequest: msgRequest{RequestId: 42}, ChannelId: id, Label: "l", Protocol: "p"}
		s, err := client.base.openStream(ctx, req)
		if err != nil {
			t.Fatal(err)
		}
		msg, err := readMessage(s)
		if err != nil {
			t.Fatal(err)
		}
		resp, ok := msg.(*msgDataChannelOpenResponse)
		if !ok {
			t.Fatalf("got %T, want data-channel-open-response", msg)
		}
		if resp.RequestId != 42 {
			t.Errorf("response to request %d, want 42", resp.RequestId)
		}
		return s, resp
	}

	s, resp := open(5)
	defer s.Close()
	if resp.Result != ResultSuccess {
		t.Fatalf("result %d, want success", resp.Result)
	}
	dc, err := server.AcceptDataChannel(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer dc.Close()
	if dc.DataChannelParameters != (DataChannelParameters{Label: "l", Protocol: "p", ID: 5}) {
		t.Errorf("accepted %+v", dc.DataChannelParameters)
	}

	// IDs above 65,534 are rejected and the stream ends.
	s2, resp := open(MaxDataChannelID + 1)
	defer s2.Close()
	if resp.Result != ResultPermanentError {
		t.Errorf("result %d, want permanent-error", resp.Result)
	}
	if _, err := readMessage(s2); !errors.Is(err, io.EOF) {
		t.Errorf("after rejection: %v, want EOF", err)
	}
}

func TestDataChannelIDAllocation(t *testing.T) {
	checkNoLeaks(t)
	server, client := authenticatedPair(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	accepted := make(chan *DataChannel, 4)
	for _, c := range []*Connection{server, client} {
		go func() {
			for {
				dc, err := c.AcceptDataChannel(ctx)
				if err != nil {
					return
				}
				accepted <- dc
			}
		}()
	}

	openID := func(c *Connection, id *uint64, want uint64) *DataChannel {
		t.Helper()
		dc, err := c.OpenDataChannel(ctx, DataChannelInit{Label: "x", ID: id})
		if err != nil {
			t.Fatal(err)
		}
		if dc.ID != want {
			t.Errorf("opened ID %d, want %d", dc.ID, want)
		}
		select {
		case rx := <-accepted:
			if rx.ID != want {
				t.Errorf("accepted ID %d, want %d", rx.ID, want)
			}
			t.Cleanup(func() { rx.Close() })
		case <-ctx.Done():
			t.Fatal("not accepted")
		}
		t.Cleanup(func() { dc.Close() })
		return dc
	}

	// The client allocates even IDs, the server odd ones.
	c0 := openID(client, nil, 0)
	openID(server, nil, 1)
	openID(client, nil, 2)
	explicit := uint64(5)
	openID(server, &explicit, 5)
	openID(server, nil, 3)
	openID(server, nil, 7)
	openID(client, nil, 4)

	used := uint64(2)
	if _, err := client.OpenDataChannel(ctx, DataChannelInit{ID: &used}); !errors.Is(err, ErrDataChannelIDInUse) {
		t.Errorf("explicit ID in use: %v", err)
	}
	odd := uint64(9)
	if _, err := client.OpenDataChannel(ctx, DataChannelInit{ID: &odd}); !errors.Is(err, ErrDataChannelIDParity) {
		t.Errorf("explicit ID of the peer's parity: %v", err)
	}

	// The peer's parity is only enforced locally: a peer that opens a channel
	// with an ID of ours (here, one in use) is accepted.
	stray, err := client.base.openDataChannel(ctx, 1, DataChannelInit{Label: "x"})
	if err != nil {
		t.Fatalf("channel with the receiver's parity: %v", err)
	}
	t.Cleanup(func() { stray.Close() })
	select {
	case rx := <-accepted:
		if rx.ID != 1 {
			t.Errorf("accepted ID %d, want 1", rx.ID)
		}
		t.Cleanup(func() { rx.Close() })
	case <-ctx.Done():
		t.Fatal("not accepted")
	}
	big := uint64(MaxDataChannelID + 1)
	if _, err := client.OpenDataChannel(ctx, DataChannelInit{ID: &big}); !errors.Is(err, ErrDataChannelIDOutOfRange) {
		t.Errorf("explicit ID out of range: %v", err)
	}

	// A closed channel's ID is free again.
	if err := c0.Close(); err != nil {
		t.Fatal(err)
	}
	openID(client, nil, 0)
}

// TestAwaitDataChannelOpenResponse checks the opener's side: the open fails
// unless the peer answers with success.
func TestAwaitDataChannelOpenResponse(t *testing.T) {
	cases := []struct {
		name    string
		send    interface{} // nil: send nothing
		wantErr error       // nil: any error
	}{
		{"success", &msgDataChannelOpenResponse{msgResponse: msgResponse{RequestId: 7}, Result: ResultSuccess}, nil},
		{"rejected", &msgDataChannelOpenResponse{msgResponse: msgResponse{RequestId: 7}, Result: ResultPermanentError}, ErrDataChannelRejected},
		{"other request", &msgDataChannelOpenResponse{msgResponse: msgResponse{RequestId: 8}, Result: ResultSuccess}, nil},
		{"data first", &msgDataFrame{Payload: []byte("hi")}, nil},
		{"timeout", nil, context.DeadlineExceeded},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			local, remote := net.Pipe()
			defer remote.Close()
			go func() {
				if tc.send != nil {
					_ = writeMessage(tc.send, remote)
				}
			}()
			ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
			defer cancel()
			err := awaitDataChannelOpenResponse(ctx, local, 7)
			_ = local.Close()
			if tc.name == "success" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil {
				t.Fatal("open succeeded")
			}
			if tc.wantErr != nil && !errors.Is(err, tc.wantErr) {
				t.Fatalf("error %v, want %v", err, tc.wantErr)
			}
		})
	}
}
