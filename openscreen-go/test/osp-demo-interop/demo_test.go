//go:build interop

package ospdemointerop

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/backkem/go-lp2p/openscreen-go/test/ospdemo"
)

// demoBinary returns the path to osp_demo, skipping the test if it is missing.
func demoBinary(t *testing.T) string {
	t.Helper()
	dir := os.Getenv("OPENSCREEN_OUT_DIR")
	if dir == "" {
		dir = "/work/openscreen/out/debug"
	}
	bin := filepath.Join(dir, "osp_demo")
	if fi, err := os.Stat(bin); err != nil || fi.IsDir() {
		t.Skipf("osp_demo not found at %s (set OPENSCREEN_OUT_DIR): %v", bin, err)
	}
	return bin
}

// harnessPSK is the PSK both sides use: $OSP_DEMO_PSK, or a random 6-digit
// one per run. osp_demo gets it with --psk, so it works whichever side ends up
// the PSK presenter (network.bs: the lower psk-ease-of-input, the server on a
// tie); the presenter "shows" it, the consumer uses it as the user's input.
var harnessPSK = func() string {
	if psk := os.Getenv("OSP_DEMO_PSK"); psk != "" {
		return psk
	}
	n, err := rand.Int(rand.Reader, big.NewInt(900_000))
	if err != nil {
		panic(err)
	}
	return fmt.Sprint(100_000 + n.Int64())
}()

// demoAuthArgs are the osp_demo authentication flags. --spake2_suite=p256 is
// the only deviation from network.bs, matching ospc's Spake2SuiteP256 quirk;
// SPAKE2 identities, password hash and gating of unauthenticated messages are
// osp_demo's spec defaults. $OSP_DEMO_EASE_OF_INPUT sets
// --psk_ease_of_input (default 50), to make osp_demo present the PSK.
func demoAuthArgs() []string {
	args := []string{"--spake2_suite=p256", "--psk=" + harnessPSK}
	if ease := os.Getenv("OSP_DEMO_EASE_OF_INPUT"); ease != "" {
		args = append(args, "--psk_ease_of_input="+ease)
	}
	return args
}

// ospDemo is a running osp_demo process (see package ospdemo), bound to a
// test: it is stopped when the test ends and its commands are logged.
type ospDemo struct {
	*ospdemo.Process
	t    *testing.T
	name string
}

// startPublisher runs osp_demo as an advertising receiver ("publisher").
func startPublisher(t *testing.T, friendlyName string) *ospDemo {
	t.Helper()
	return startDemo(t, "recv", func(o ospdemo.Options) (*ospdemo.Process, error) {
		return ospdemo.StartPublisher(o, friendlyName)
	})
}

// startController runs osp_demo as a listening controller.
func startController(t *testing.T) *ospDemo {
	t.Helper()
	return startDemo(t, "cntl", ospdemo.StartController)
}

func startDemo(t *testing.T, name string, start func(ospdemo.Options) (*ospdemo.Process, error)) *ospDemo {
	t.Helper()
	p, err := start(ospdemo.Options{Binary: demoBinary(t), Args: demoAuthArgs(), Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("[%s] started %v (pid %d, cwd %s)", name, p.Args(), p.Pid(), p.Dir())
	t.Cleanup(p.Stop)
	return &ospDemo{Process: p, t: t, name: name}
}

// Send writes a command to osp_demo's stdin.
func (d *ospDemo) Send(command string) {
	d.t.Helper()
	d.t.Logf("[%s] <stdin> %s", d.name, command)
	if err := d.Process.Send(command); err != nil {
		d.t.Errorf("[%s] write stdin: %v", d.name, err)
	}
}

// WaitFor waits for a log line matching pattern, from the start of the log,
// and returns it.
func (d *ospDemo) WaitFor(pattern string, timeout time.Duration) (string, bool) {
	return d.Process.WaitFor(0, pattern, timeout)
}

// DumpLog writes the full collected log to the test log, or to the file named
// by OSP_DEMO_LOG_DIR/<test>-<name>.log when that is set.
func (d *ospDemo) DumpLog() {
	text := d.Log()
	if dir := os.Getenv("OSP_DEMO_LOG_DIR"); dir != "" {
		name := strings.NewReplacer("/", "_", " ", "_").Replace(d.t.Name()) + "-" + d.name + ".log"
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(text+"\n"), 0o644); err == nil {
			d.t.Logf("[%s] full log: %s", d.name, path)
			return
		}
	}
	if testing.Verbose() {
		d.t.Logf("[%s] full log:\n%s", d.name, text)
	}
}

// withTimeout returns a context bounded by the test deadline and timeout.
func withTimeout(t *testing.T, timeout time.Duration) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	t.Cleanup(cancel)
	return ctx
}
