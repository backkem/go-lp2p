package cast

import "time"

// Clock abstracts timers so heartbeat behaviour can be tested deterministically.
type Clock interface {
	AfterFunc(d time.Duration, f func()) Timer
}

// Timer is the subset of *time.Timer used by this package.
type Timer interface {
	Stop() bool
	Reset(d time.Duration) bool
}

// SystemClock uses the time package.
type SystemClock struct{}

// AfterFunc implements Clock.
func (SystemClock) AfterFunc(d time.Duration, f func()) Timer { return time.AfterFunc(d, f) }
