// Package demo wires up the examples: browsers (mock user agents) whose user
// is simulated, or, with -interactive, asked on the terminal.
//
// Example output goes to stderr; ospc writes protocol debug output to
// stdout, so `go run ./examples/X >/dev/null` shows just the example.
package demo

import (
	"flag"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	lp2p "github.com/backkem/go-lp2p/lp2p-api"
	ua "github.com/backkem/go-lp2p/lp2p-api/useragent"
)

var interactive = flag.Bool("interactive", false, "ask the user on the terminal instead of simulating them")

// Interactive reports whether -interactive was given. Call after flag.Parse.
func Interactive() bool { return *interactive }

var (
	cliOnce sync.Once
	cli     *ua.CLI
)

// Browser is one simulated device running a browser with one page open.
type Browser struct {
	Name   string
	UA     *ua.UserAgent
	Window *lp2p.Window
}

// NewBrowser starts a browser. The simulated user follows script; with
// -interactive, all browsers share the terminal instead.
func NewBrowser(name string, script *ua.Scripted) *Browser {
	var ui ua.UI = script
	if Interactive() {
		cliOnce.Do(func() { cli = ua.NewCLI(os.Stdin, os.Stderr) })
		ui = cli
	} else {
		script.Name = name
		script.Out = os.Stderr
	}
	u, err := ua.New(ua.Config{Name: name, UI: ui, PairingTimeout: 2 * time.Second})
	if err != nil {
		log.Fatal(err)
	}
	return &Browser{Name: name, UA: u, Window: lp2p.NewWindow(u, "https://"+slug(name)+".example")}
}

// Close the browser.
func (b *Browser) Close() { _ = b.UA.Close() }

func slug(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			out = append(out, r)
		case r >= 'A' && r <= 'Z':
			out = append(out, r+'a'-'A')
		default:
			out = append(out, '-')
		}
	}
	return string(out)
}

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
