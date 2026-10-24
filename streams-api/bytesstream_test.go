package streams

import (
	"bytes"
	"errors"
	"io"
	"sync"
	"testing"
	"testing/iotest"
)

func readAll(t *testing.T, r ReadableStreamDefaultReader[[]byte]) [][]byte {
	t.Helper()
	var chunks [][]byte
	for {
		res, err := r.Read()
		if err != nil {
			t.Fatalf("Read: %v", err)
		}
		if res.Done {
			if res.Val != nil {
				t.Fatalf("done result carries value %q", res.Val)
			}
			return chunks
		}
		chunks = append(chunks, res.Val)
	}
}

func TestDataReaderReturnsReadLength(t *testing.T) {
	msg := []byte("Good day to you, receiver!")
	r := NewDataReadableStream(bytes.NewReader(msg)).GetReader(nil)

	chunks := readAll(t, r)
	if len(chunks) != 1 || !bytes.Equal(chunks[0], msg) {
		t.Fatalf("got %q, want one chunk %q", chunks, msg)
	}
}

func TestDataReaderSmallReads(t *testing.T) {
	msg := []byte("hello")
	r := NewDataReadableStream(iotest.OneByteReader(bytes.NewReader(msg))).GetReader(nil)

	chunks := readAll(t, r)
	if len(chunks) != len(msg) {
		t.Fatalf("got %d chunks, want %d", len(chunks), len(msg))
	}
	if got := bytes.Join(chunks, nil); !bytes.Equal(got, msg) {
		t.Fatalf("got %q, want %q", got, msg)
	}
}

func TestDataReaderDataWithEOF(t *testing.T) {
	// DataErrReader returns the final data together with io.EOF.
	msg := []byte("last bytes")
	r := NewDataReadableStream(iotest.DataErrReader(bytes.NewReader(msg))).GetReader(nil)

	if got := bytes.Join(readAll(t, r), nil); !bytes.Equal(got, msg) {
		t.Fatalf("got %q, want %q", got, msg)
	}
}

func TestDataReaderLargeInputIsChunked(t *testing.T) {
	msg := bytes.Repeat([]byte("x"), 3*defaultReaderBufferSize+7)
	r := NewDataReadableStream(bytes.NewReader(msg)).GetReader(nil)

	chunks := readAll(t, r)
	for _, c := range chunks {
		if len(c) > defaultReaderBufferSize {
			t.Fatalf("chunk of %d bytes exceeds %d", len(c), defaultReaderBufferSize)
		}
	}
	if got := bytes.Join(chunks, nil); !bytes.Equal(got, msg) {
		t.Fatalf("reassembled %d bytes, want %d", len(got), len(msg))
	}
}

func TestDataReaderChunksAreNotReused(t *testing.T) {
	r := NewDataReadableStream(&limitedReader{r: bytes.NewReader([]byte("aabb")), max: 2}).GetReader(nil)
	first, _ := r.Read()
	second, _ := r.Read()
	if string(first.Val) != "aa" || string(second.Val) != "bb" {
		t.Fatalf("got %q, %q; want \"aa\", \"bb\"", first.Val, second.Val)
	}
}

func TestDataReaderDoneIsSticky(t *testing.T) {
	r := NewDataReadableStream(bytes.NewReader(nil)).GetReader(nil)
	for i := 0; i < 3; i++ {
		res, err := r.Read()
		if err != nil || !res.Done {
			t.Fatalf("read %d: got (%v, %v), want done", i, res, err)
		}
	}
}

func TestDataReaderErrorIsSticky(t *testing.T) {
	boom := errors.New("boom")
	src := io.MultiReader(bytes.NewReader([]byte("ok")), iotest.ErrReader(boom))
	r := NewDataReadableStream(src).GetReader(nil)

	res, err := r.Read()
	if err != nil || string(res.Val) != "ok" {
		t.Fatalf("first read: got (%q, %v)", res.Val, err)
	}
	for i := 0; i < 2; i++ {
		if _, err := r.Read(); !errors.Is(err, boom) {
			t.Fatalf("read %d: got %v, want %v", i, err, boom)
		}
	}
}

func TestDataReadersShareState(t *testing.T) {
	s := NewDataReadableStream(&limitedReader{r: bytes.NewReader([]byte("aabb")), max: 2})
	a, b := s.GetReader(nil), s.GetReader(nil)
	ra, _ := a.Read()
	rb, _ := b.Read()
	if string(ra.Val) != "aa" || string(rb.Val) != "bb" {
		t.Fatalf("got %q, %q", ra.Val, rb.Val)
	}
	if res, _ := a.Read(); !res.Done {
		t.Fatalf("want done")
	}
}

func TestDataReaderConcurrentReads(t *testing.T) {
	msg := bytes.Repeat([]byte("0123456789"), 1000)
	s := NewDataReadableStream(iotest.OneByteReader(bytes.NewReader(msg)))

	var (
		mu    sync.Mutex
		total int
		wg    sync.WaitGroup
	)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := s.GetReader(nil)
			for {
				res, err := r.Read()
				if err != nil || res.Done {
					return
				}
				mu.Lock()
				total += len(res.Val)
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if total != len(msg) {
		t.Fatalf("read %d bytes, want %d", total, len(msg))
	}
}

func TestBYOBReaderUnsupported(t *testing.T) {
	r := NewDataReadableStream(bytes.NewReader(nil)).GetReader(&ReadableStreamGetReaderOptions{
		Mode: ReadableStreamReaderModeBYOB,
	})
	if _, err := r.Read(); !errors.Is(err, ErrReaderModeUnsupported) {
		t.Fatalf("got %v, want %v", err, ErrReaderModeUnsupported)
	}
}

type recordingWriter struct {
	bytes.Buffer
	closes   int
	writeErr error
}

func (w *recordingWriter) Write(p []byte) (int, error) {
	if w.writeErr != nil {
		return 0, w.writeErr
	}
	return w.Buffer.Write(p)
}

func (w *recordingWriter) Close() error {
	w.closes++
	return nil
}

func TestDataWriter(t *testing.T) {
	sink := &recordingWriter{}
	s := NewDataWritableStream(sink)
	w := s.GetWriter()

	for _, part := range []string{"hello", ", ", "world"} {
		if err := w.Write([]byte(part)); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if got := sink.String(); got != "hello, world" {
		t.Fatalf("sink got %q", got)
	}
	if err := w.Write([]byte("late")); !errors.Is(err, ErrStreamClosed) {
		t.Fatalf("write after close: got %v, want %v", err, ErrStreamClosed)
	}
	if err := s.Close(); !errors.Is(err, ErrStreamClosed) {
		t.Fatalf("second close: got %v, want %v", err, ErrStreamClosed)
	}
	if sink.closes != 1 {
		t.Fatalf("sink closed %d times, want 1", sink.closes)
	}
}

func TestDataWriterErrorIsSticky(t *testing.T) {
	boom := errors.New("boom")
	sink := &recordingWriter{writeErr: boom}
	s := NewDataWritableStream(sink)
	w := s.GetWriter()

	if err := w.Write([]byte("a")); !errors.Is(err, boom) {
		t.Fatalf("got %v, want %v", err, boom)
	}
	sink.writeErr = nil
	if err := w.Write([]byte("b")); !errors.Is(err, boom) {
		t.Fatalf("errored stream accepted write: %v", err)
	}
	if err := s.Close(); !errors.Is(err, boom) {
		t.Fatalf("close of errored stream: got %v, want %v", err, boom)
	}
	if sink.closes != 1 {
		t.Fatalf("errored sink not released on close")
	}
}

func TestDataBidirectionalRoundTrip(t *testing.T) {
	pr, pw := io.Pipe()
	w := NewDataWritableStream(pw).GetWriter()
	r := NewDataReadableStream(pr).GetReader(nil)

	go func() {
		_ = w.Write([]byte("ping"))
		_ = w.Close()
	}()

	if got := bytes.Join(readAll(t, r), nil); string(got) != "ping" {
		t.Fatalf("got %q", got)
	}
}

// limitedReader returns at most max bytes per Read.
type limitedReader struct {
	r   io.Reader
	max int
}

func (l *limitedReader) Read(p []byte) (int, error) {
	if len(p) > l.max {
		p = p[:l.max]
	}
	return l.r.Read(p)
}
