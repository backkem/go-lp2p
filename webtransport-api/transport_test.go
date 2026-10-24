package webtransport

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

// pipeStream is one end of an in-memory bidirectional stream.
type pipeStream struct {
	id int64
	r  *io.PipeReader
	w  *io.PipeWriter
}

func (s *pipeStream) StreamID() int64             { return s.id }
func (s *pipeStream) Read(p []byte) (int, error)  { return s.r.Read(p) }
func (s *pipeStream) Write(p []byte) (int, error) { return s.w.Write(p) }
func (s *pipeStream) Close() error                { return s.w.Close() }

func newStreamPair(id int64) (*pipeStream, *pipeStream) {
	ar, bw := io.Pipe()
	br, aw := io.Pipe()
	return &pipeStream{id, ar, aw}, &pipeStream{id, br, bw}
}

// fakeSession connects to a peer fakeSession through channels. Like the
// pooled ospc transport, CloseWithError does not unblock pending calls.
type fakeSession struct {
	incoming chan *pipeStream
	peer     *fakeSession
	failErr  error

	mu        sync.Mutex
	nextID    int64
	closes    []WebTransportCloseInfo
	acceptErr error
}

func newSessionPair() (*fakeSession, *fakeSession) {
	a := &fakeSession{incoming: make(chan *pipeStream)}
	b := &fakeSession{incoming: make(chan *pipeStream)}
	a.peer, b.peer = b, a
	return a, b
}

func (s *fakeSession) AcceptStream(ctx context.Context) (*pipeStream, error) {
	s.mu.Lock()
	err := s.acceptErr
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case st := <-s.incoming:
		return st, nil
	}
}

func (s *fakeSession) OpenStreamSync(ctx context.Context) (*pipeStream, error) {
	s.mu.Lock()
	s.nextID++
	id := s.nextID
	s.mu.Unlock()
	local, remote := newStreamPair(id)
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case s.peer.incoming <- remote:
		return local, nil
	}
}

func (s *fakeSession) CloseWithError(code uint64, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closes = append(s.closes, WebTransportCloseInfo{code, reason})
	return nil
}

func newTransportPair(t *testing.T) (*Transport, *Transport, *fakeSession, *fakeSession) {
	t.Helper()
	sa, sb := newSessionPair()
	a, err := NewTransport(NewSessionAdaptor[*pipeStream](sa))
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewTransport(NewSessionAdaptor[*pipeStream](sb))
	if err != nil {
		t.Fatal(err)
	}
	return a, b, sa, sb
}

func TestBidirectionalStreamRoundTrip(t *testing.T) {
	a, b, _, _ := newTransportPair(t)
	msg := []byte("Good day to you, receiver!")

	errc := make(chan error, 1)
	go func() {
		s, err := a.CreateBidirectionalStream()
		if err != nil {
			errc <- err
			return
		}
		w := s.Writable.GetWriter()
		if err := w.Write(msg); err != nil {
			errc <- err
			return
		}
		errc <- w.Close()
	}()

	res, err := b.IncomingBidirectionalStreams.GetReader(nil).Read()
	if err != nil || res.Done {
		t.Fatalf("accept: (%v, %v)", res.Done, err)
	}
	r := res.Val.Readable.GetReader(nil)
	var got []byte
	for {
		chunk, err := r.Read()
		if err != nil {
			t.Fatalf("Read: %v", err)
		}
		if chunk.Done {
			break
		}
		got = append(got, chunk.Val...)
	}
	if !bytes.Equal(got, msg) {
		t.Fatalf("got %q, want %q", got, msg)
	}
	if err := <-errc; err != nil {
		t.Fatalf("writer: %v", err)
	}
}

func TestCloseEndsIncomingStreams(t *testing.T) {
	_, b, _, _ := newTransportPair(t)
	r := b.IncomingBidirectionalStreams.GetReader(nil)

	type result struct {
		done bool
		err  error
	}
	resc := make(chan result, 1)
	go func() {
		res, err := r.Read()
		resc <- result{res.Done, err}
	}()

	time.Sleep(10 * time.Millisecond) // let the read block
	if err := b.Close(WebTransportCloseInfo{}); err != nil {
		t.Fatalf("Close: %v", err)
	}
	select {
	case res := <-resc:
		if res.err != nil || !res.done {
			t.Fatalf("pending read after close: done=%v err=%v, want done", res.done, res.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("pending read not unblocked by Close")
	}
	if res, err := r.Read(); err != nil || !res.Done {
		t.Fatalf("read after close: (%v, %v), want done", res.Done, err)
	}
}

func TestCloseUnblocksCreateBidirectionalStream(t *testing.T) {
	a, _, _, _ := newTransportPair(t) // nobody accepts on the peer
	errc := make(chan error, 1)
	go func() {
		_, err := a.CreateBidirectionalStream()
		errc <- err
	}()
	time.Sleep(10 * time.Millisecond)
	_ = a.Close(WebTransportCloseInfo{})
	select {
	case err := <-errc:
		if !errors.Is(err, ErrTransportClosed) {
			t.Fatalf("got %v, want %v", err, ErrTransportClosed)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("CreateBidirectionalStream not unblocked by Close")
	}
	if _, err := a.CreateBidirectionalStream(); !errors.Is(err, ErrTransportClosed) {
		t.Fatalf("after close: got %v, want %v", err, ErrTransportClosed)
	}
}

func TestCloseIsIdempotentAndForwardsInfo(t *testing.T) {
	a, _, sa, _ := newTransportPair(t)
	info := WebTransportCloseInfo{CloseCode: 7, Reason: "bye"}
	if err := a.Close(info); err != nil {
		t.Fatal(err)
	}
	if err := a.Close(WebTransportCloseInfo{CloseCode: 9}); err != nil {
		t.Fatal(err)
	}
	if len(sa.closes) != 1 || sa.closes[0] != info {
		t.Fatalf("session closes: %v, want [%v]", sa.closes, info)
	}
}

func TestCloseTruncatesReason(t *testing.T) {
	a, _, sa, _ := newTransportPair(t)
	// 'é' is two bytes; 1023 bytes of 'a' put it across the limit.
	reason := strings.Repeat("a", maxReasonLength-1) + "é"
	_ = a.Close(WebTransportCloseInfo{Reason: reason})
	if got := sa.closes[0].Reason; got != strings.Repeat("a", maxReasonLength-1) {
		t.Fatalf("reason truncated to %d bytes", len(got))
	}
}

func TestSessionErrorErrorsIncomingStreams(t *testing.T) {
	_, b, _, sb := newTransportPair(t)
	boom := errors.New("connection lost")
	sb.mu.Lock()
	sb.acceptErr = boom
	sb.mu.Unlock()

	r := b.IncomingBidirectionalStreams.GetReader(nil)
	for i := 0; i < 2; i++ {
		if _, err := r.Read(); !errors.Is(err, boom) {
			t.Fatalf("read %d: got %v, want %v", i, err, boom)
		}
	}
}

func TestAdaptorDoesNotReturnTypedNil(t *testing.T) {
	sa, _ := newSessionPair()
	sa.acceptErr = errors.New("fail")
	s, err := NewSessionAdaptor[*pipeStream](sa).AcceptStream(context.Background())
	if err == nil || s != nil {
		t.Fatalf("got (%v, %v), want (nil, error)", s, err)
	}
}

func TestNewTransportNilSession(t *testing.T) {
	if _, err := NewTransport(nil); err == nil {
		t.Fatal("want error for nil session")
	}
}
