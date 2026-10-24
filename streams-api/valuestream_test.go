package streams

import (
	"errors"
	"io"
	"testing"
)

type sliceReader[T any] struct {
	vals    []T
	doneErr error
}

func (s *sliceReader[T]) Read() (T, error) {
	var zero T
	if len(s.vals) == 0 {
		return zero, s.doneErr
	}
	v := s.vals[0]
	s.vals = s.vals[1:]
	return v, nil
}

func TestValueReader(t *testing.T) {
	for _, doneErr := range []error{ErrValueReaderDone, io.EOF} {
		r := NewValueReadableStream[int](&sliceReader[int]{vals: []int{1, 2, 3}, doneErr: doneErr}).GetReader(nil)
		var got []int
		for {
			res, err := r.Read()
			if err != nil {
				t.Fatalf("Read: %v", err)
			}
			if res.Done {
				break
			}
			got = append(got, res.Val)
		}
		if len(got) != 3 || got[0] != 1 || got[2] != 3 {
			t.Fatalf("got %v", got)
		}
		if res, err := r.Read(); err != nil || !res.Done {
			t.Fatalf("done not sticky: (%v, %v)", res, err)
		}
	}
}

func TestValueReaderError(t *testing.T) {
	boom := errors.New("boom")
	r := NewValueReadableStream[int](&sliceReader[int]{doneErr: boom}).GetReader(nil)
	for i := 0; i < 2; i++ {
		if _, err := r.Read(); !errors.Is(err, boom) {
			t.Fatalf("read %d: got %v, want %v", i, err, boom)
		}
	}
}

type sliceWriter[T any] struct {
	vals   []T
	closes int
}

func (s *sliceWriter[T]) Write(v T) error { s.vals = append(s.vals, v); return nil }
func (s *sliceWriter[T]) Close() error    { s.closes++; return nil }

func TestValueWriter(t *testing.T) {
	sink := &sliceWriter[string]{}
	s := NewValueWritableStream[string](sink)
	w := s.GetWriter()
	_ = w.Write("a")
	_ = w.Write("b")
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := w.Write("c"); !errors.Is(err, ErrStreamClosed) {
		t.Fatalf("write after close: %v", err)
	}
	if err := w.Close(); !errors.Is(err, ErrStreamClosed) {
		t.Fatalf("second close: %v", err)
	}
	if len(sink.vals) != 2 || sink.closes != 1 {
		t.Fatalf("sink got %v, closed %d times", sink.vals, sink.closes)
	}
}
