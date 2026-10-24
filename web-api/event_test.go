package web

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// collector records delivered events and fails on concurrent delivery.
type collector struct {
	t      *testing.T
	mu     sync.Mutex
	got    []int
	active atomic.Int32
	notify chan struct{}
}

func newCollector(t *testing.T) *collector {
	return &collector{t: t, notify: make(chan struct{}, 1000)}
}

func (c *collector) cb(e int) {
	if c.active.Add(1) != 1 {
		c.t.Errorf("callback invoked concurrently")
	}
	defer c.active.Add(-1)
	c.mu.Lock()
	c.got = append(c.got, e)
	c.mu.Unlock()
	c.notify <- struct{}{}
}

func (c *collector) wait(n int) []int {
	c.t.Helper()
	timeout := time.After(5 * time.Second)
	for {
		c.mu.Lock()
		if len(c.got) >= n {
			got := append([]int(nil), c.got...)
			c.mu.Unlock()
			return got
		}
		c.mu.Unlock()
		select {
		case <-c.notify:
		case <-timeout:
			c.t.Fatalf("timed out waiting for %d events, got %v", n, c.got)
		}
	}
}

func assertSequence(t *testing.T, got []int, from, to int) {
	t.Helper()
	if len(got) != to-from {
		t.Fatalf("got %d events %v, want %d..%d", len(got), got, from, to-1)
	}
	for i, e := range got {
		if e != from+i {
			t.Fatalf("event %d is %d, want %d (got %v)", i, e, from+i, got)
		}
	}
}

func TestLiveEventsAreSynchronous(t *testing.T) {
	h := NewEventHandler[int]()
	c := newCollector(t)
	h.SetCallback(c.cb)

	for i := 0; i < 10; i++ {
		h.OnCallback(i)
		c.mu.Lock()
		n := len(c.got)
		c.mu.Unlock()
		if n != i+1 {
			t.Fatalf("event %d not delivered before OnCallback returned", i)
		}
	}
	assertSequence(t, c.wait(10), 0, 10)
}

func TestBufferedThenLiveOrdering(t *testing.T) {
	for run := 0; run < 50; run++ {
		h := NewEventHandler[int]()
		c := newCollector(t)

		for i := 0; i < 50; i++ {
			h.OnCallback(i)
		}
		h.SetCallback(c.cb)
		for i := 50; i < 100; i++ {
			h.OnCallback(i)
		}
		assertSequence(t, c.wait(100), 0, 100)
	}
}

func TestConcurrentProducersNoConcurrentDelivery(t *testing.T) {
	h := NewEventHandler[int]()
	c := newCollector(t)
	for i := 0; i < 20; i++ {
		h.OnCallback(-1)
	}
	h.SetCallback(c.cb)

	var wg sync.WaitGroup
	for p := 0; p < 8; p++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				h.OnCallback(i)
			}
		}()
	}
	wg.Wait()
	got := c.wait(20 + 800)
	for i := 0; i < 20; i++ {
		if got[i] != -1 {
			t.Fatalf("buffered events not delivered first: %v", got[:25])
		}
	}
}

func TestSetCallbackDoesNotBlockOnBufferedEvents(t *testing.T) {
	h := NewEventHandler[int]()
	h.OnCallback(1)

	release := make(chan struct{})
	delivered := make(chan int, 1)
	h.SetCallback(func(e int) {
		<-release
		delivered <- e
	})
	close(release) // SetCallback returned while the callback was blocked.
	if e := <-delivered; e != 1 {
		t.Fatalf("got %d", e)
	}
}

func TestBufferOverflowDropsOldest(t *testing.T) {
	h := NewEventHandler[int]()
	total := HIGH_WATERMARK + 25
	for i := 0; i < total; i++ {
		h.OnCallback(i)
	}
	c := newCollector(t)
	h.SetCallback(c.cb)
	got := c.wait(HIGH_WATERMARK)
	assertSequence(t, got, total-HIGH_WATERMARK, total)
}

func TestReentrantEventIsQueued(t *testing.T) {
	h := NewEventHandler[int]()
	var got []int
	h.SetCallback(func(e int) {
		got = append(got, e)
		if e == 0 {
			// Raising an event from the callback must not recurse or
			// deadlock; it is delivered after this callback returns.
			h.OnCallback(1)
			if len(got) != 1 {
				t.Errorf("re-entrant event delivered recursively")
			}
		}
	})
	h.OnCallback(0)
	assertSequence(t, got, 0, 2)
}

func TestClearCallbackBuffersAgain(t *testing.T) {
	h := NewEventHandler[int]()
	c := newCollector(t)
	h.SetCallback(c.cb)
	h.OnCallback(0)
	h.SetCallback(nil)
	h.OnCallback(1)
	h.OnCallback(2)
	h.SetCallback(c.cb)
	assertSequence(t, c.wait(3), 0, 3)
}

func TestPanickingCallbackDoesNotWedgeHandler(t *testing.T) {
	h := NewEventHandler[int]()
	var got []int
	h.SetCallback(func(e int) {
		if e == 0 {
			panic("boom")
		}
		got = append(got, e)
	})
	func() {
		defer func() { _ = recover() }()
		h.OnCallback(0)
	}()
	h.OnCallback(1)
	if len(got) != 1 || got[0] != 1 {
		t.Fatalf("handler wedged after panic: %v", got)
	}
}
