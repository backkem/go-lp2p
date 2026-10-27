// Package demo wires up the examples: browsers (mock user agents) whose user
// is simulated, or, with -interactive, asked on the terminal.
//
// Example output goes to stderr; ospc writes protocol debug output to
// stdout, so `go run ./examples/X >/dev/null` shows just the example.
package demo

import (
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	lp2p "github.com/backkem/go-lp2p/lp2p-api"
	ua "github.com/backkem/go-lp2p/lp2p-api/useragent"
)

var interactive = flag.Bool("interactive", false, "ask the user on the terminal instead of simulating them")

// Interactive reports whether -interactive was given. Call after flag.Parse.
func Interactive() bool { return *interactive }

// Unique appends a random suffix to a device name, e.g. "TV #3fa9", so the
// example picks its own device even when other Open Screen agents on the
// network (or another run of the example) use the same name.
func Unique(name string) string {
	b := make([]byte, 2)
	_, _ = rand.Read(b)
	return name + " #" + hex.EncodeToString(b)
}

// Browser is one simulated device running a browser with one page open.
type Browser struct {
	Name   string
	UA     *ua.UserAgent
	Window *lp2p.Window
}

// NewBrowser starts a browser whose user follows script.
func NewBrowser(name string, script *ua.Scripted) *Browser {
	script.Name = name
	script.Out = os.Stderr
	return newBrowser(name, script)
}

// NewUserBrowser starts the browser of the person running the example: with
// -interactive they are asked on the terminal (device picker, PIN entry),
// otherwise they follow script. Only one browser can own the terminal.
func NewUserBrowser(name string, script *ua.Scripted) *Browser {
	if !Interactive() {
		return NewBrowser(name, script)
	}
	return newBrowser(name, ua.NewCLI(os.Stdin, os.Stderr))
}

// Origin is the origin of the page open in every browser: the same web app
// runs on all devices. By default a receiver only accepts sessions
// requested by its own origin.
const Origin = "https://lp2p.example"

func newBrowser(name string, ui ua.UI) *Browser {
	u, err := ua.New(ua.Config{Name: name, UI: ui, PairingTimeout: 2 * time.Second})
	if err != nil {
		log.Fatal(err)
	}
	return &Browser{Name: name, UA: u, Window: lp2p.NewWindow(u, Origin)}
}

// Close the browser.
func (b *Browser) Close() { _ = b.UA.Close() }

// Pair returns a script for each side of a pairing in which the simulated
// user correctly reads the PIN from one screen and types it on the other.
func Pair() (presenter, consumer *ua.Scripted) {
	relay := ua.NewPINRelay()
	return &ua.Scripted{Present: relay.Present, Enter: relay.Enter},
		&ua.Scripted{Present: relay.Present, Enter: relay.Enter}
}

// Watchdog fails the example if it hasn't finished within d. It is off with
// -interactive, where the user sets the pace.
func Watchdog(d time.Duration) {
	if Interactive() {
		return
	}
	time.AfterFunc(d, func() {
		fmt.Fprintf(os.Stderr, "example didn't finish within %s\n", d)
		os.Exit(1)
	})
}

// Await waits for ch to be closed or receive, failing after a while.
func Await[T any](ch <-chan T, what string) T {
	timeout := 15 * time.Second
	if Interactive() {
		timeout = 10 * time.Minute
	}
	select {
	case v := <-ch:
		return v
	case <-time.After(timeout):
		log.Fatalf("timed out waiting for %s", what)
		panic("unreachable")
	}
}
