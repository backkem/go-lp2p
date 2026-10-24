//go:build interop

package lp2pdemointerop

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	ospc "github.com/backkem/go-lp2p/openscreen-go/network"
)

const (
	discoveryTimeout = 20 * time.Second
	stepTimeout      = 10 * time.Second
)

func randInt(max int64) int64 {
	n, err := rand.Int(rand.Reader, big.NewInt(max))
	if err != nil {
		panic(err)
	}
	return n.Int64()
}

// uniqueName returns an mDNS instance name that no other run uses.
func uniqueName(prefix string) string {
	return fmt.Sprintf("%s%05x", prefix, randInt(1<<20))
}

// harnessPSK is the PSK both sides use, so either can be the PSK presenter.
var harnessPSK = fmt.Sprint(100_000 + randInt(900_000))

func demoBinary(t *testing.T) string {
	t.Helper()
	dir := os.Getenv("OPENSCREEN_OUT_DIR")
	if dir == "" {
		dir = "/work/openscreen/out/debug"
	}
	bin := filepath.Join(dir, "lp2p_demo")
	if fi, err := os.Stat(bin); err != nil || fi.IsDir() {
		t.Skipf("lp2p_demo not found at %s (set OPENSCREEN_OUT_DIR): %v", bin, err)
	}
	return bin
}

// lp2pDemo is a running lp2p_demo process. It prints "lp2p_demo: ..." lines
// to stdout, logs to stderr, and sends the lines written to its stdin.
type lp2pDemo struct {
	t     *testing.T
	cmd   *exec.Cmd
	stdin io.WriteCloser

	mu      sync.Mutex
	lines   []string
	changed chan struct{}
	exited  chan struct{}
}

func startDemo(t *testing.T, args ...string) *lp2pDemo {
	t.Helper()
	bin := demoBinary(t)
	cwd := t.TempDir()
	// The agent certificate is stored here.
	if err := os.MkdirAll(filepath.Join(cwd, "osp/impl/quic/certificates"), 0o755); err != nil {
		t.Fatal(err)
	}
	args = append([]string{"--spake2_suite=p256", "--psk=" + harnessPSK}, args...)
	d := &lp2pDemo{t: t, changed: make(chan struct{}), exited: make(chan struct{})}
	d.cmd = exec.Command(bin, args...)
	d.cmd.Dir = cwd
	d.cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGKILL}
	stdin, err := d.cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	d.stdin = stdin
	stdout, err := d.cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	stderr, err := d.cmd.StderrPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := d.cmd.Start(); err != nil {
		t.Fatalf("start lp2p_demo: %v", err)
	}
	t.Logf("started lp2p_demo %s (pid %d)", strings.Join(args, " "), d.cmd.Process.Pid)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); d.collect("out", stdout) }()
	go func() { defer wg.Done(); d.collect("err", stderr) }()
	go func() {
		wg.Wait()
		err := d.cmd.Wait()
		d.addLine("proc", fmt.Sprintf("exited: %v", err))
		close(d.exited)
	}()
	t.Cleanup(d.stop)
	return d
}

func (d *lp2pDemo) collect(prefix string, r io.Reader) {
	s := bufio.NewScanner(r)
	s.Buffer(make([]byte, 64*1024), 1024*1024)
	for s.Scan() {
		d.addLine(prefix, s.Text())
	}
}

func (d *lp2pDemo) addLine(prefix, line string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.lines = append(d.lines, prefix+": "+line)
	close(d.changed)
	d.changed = make(chan struct{})
}

// WaitFor waits for an output line matching pattern.
func (d *lp2pDemo) WaitFor(pattern string, timeout time.Duration) (string, bool) {
	re := regexp.MustCompile(pattern)
	deadline := time.After(timeout)
	next := 0
	for {
		d.mu.Lock()
		lines := d.lines[next:]
		next = len(d.lines)
		changed := d.changed
		d.mu.Unlock()
		for _, l := range lines {
			if re.MatchString(l) {
				return l, true
			}
		}
		select {
		case <-changed:
		case <-deadline:
			return "", false
		}
	}
}

// MustSee fails t unless an output line matches pattern in time.
func (d *lp2pDemo) MustSee(pattern string, timeout time.Duration) {
	d.t.Helper()
	line, ok := d.WaitFor(pattern, timeout)
	if !ok {
		d.t.Fatalf("lp2p_demo did not print %q", pattern)
	}
	d.t.Logf("lp2p_demo: %s", line)
}

// Send writes a line to lp2p_demo's stdin, which it sends as a text message.
func (d *lp2pDemo) Send(line string) {
	d.t.Helper()
	if _, err := io.WriteString(d.stdin, line+"\n"); err != nil {
		d.t.Fatalf("write lp2p_demo stdin: %v", err)
	}
}

func (d *lp2pDemo) log() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return strings.Join(d.lines, "\n")
}

// stop closes stdin, which ends lp2p_demo, then kills it if needed.
func (d *lp2pDemo) stop() {
	_ = d.stdin.Close()
	select {
	case <-d.exited:
	case <-time.After(5 * time.Second):
		_ = syscall.Kill(-d.cmd.Process.Pid, syscall.SIGKILL)
		<-d.exited
	}
	if d.t.Failed() || testing.Verbose() {
		d.t.Logf("lp2p_demo output:\n%s", d.log())
	}
}

// newAgent returns an ospc agent with the one other non-spec option the
// harness uses: SPAKE2 over P-256, matching lp2p_demo --spake2_suite=p256.
func newAgent(t *testing.T, name string) *ospc.Agent {
	t.Helper()
	conf := ospc.NewAgentConfig(name)
	conf.InteropQuirks.Spake2SuiteP256 = true
	var logs bytes.Buffer
	var mu sync.Mutex
	conf.WithLogger(slog.New(slog.NewTextHandler(lockedWriter{&mu, &logs}, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() {
		if t.Failed() {
			mu.Lock()
			defer mu.Unlock()
			t.Logf("ospc log:\n%s", logs.String())
		}
	})
	a, err := ospc.NewAgent(conf)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

type lockedWriter struct {
	mu *sync.Mutex
	w  io.Writer
}

func (w lockedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.w.Write(p)
}

func withTimeout(t *testing.T, timeout time.Duration) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	t.Cleanup(cancel)
	return ctx
}

// dialDemo discovers the lp2p_demo listener name, connects and authenticates.
func dialDemo(t *testing.T, name string) *ospc.Connection {
	t.Helper()
	d := ospc.NewDiscoverer()
	d.WithNickname(name)
	if err := d.Start(); err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	discovered, err := d.Accept(withTimeout(t, discoveryTimeout))
	if err != nil {
		t.Fatalf("did not discover %q: %v", name, err)
	}
	uc, err := discovered.Dial(withTimeout(t, stepTimeout), ospc.AgentTransportQUIC, newAgent(t, uniqueName("GoLp2pReq")))
	if err != nil {
		t.Fatalf("dial %q: %v", name, err)
	}
	conn, err := uc.AuthenticatePSK(withTimeout(t, stepTimeout), []byte(harnessPSK))
	if err != nil {
		t.Fatalf("authenticate with %q: %v", name, err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

type message struct {
	payload  []byte
	encoding ospc.DataEncoding
	err      error
}

// receive reads one message from dc, or gives up after timeout.
func receive(dc *ospc.DataChannel, timeout time.Duration) message {
	ch := make(chan message, 1)
	go func() {
		payload, encoding, err := dc.ReceiveMessageWithEncoding()
		ch <- message{payload, encoding, err}
	}()
	select {
	case m := <-ch:
		return m
	case <-time.After(timeout):
		return message{err: fmt.Errorf("no message within %v", timeout)}
	}
}

func expectMessage(t *testing.T, dc *ospc.DataChannel, payload []byte, encoding ospc.DataEncoding) {
	t.Helper()
	m := receive(dc, stepTimeout)
	if m.err != nil {
		t.Fatalf("receive: %v", m.err)
	}
	if !bytes.Equal(m.payload, payload) || m.encoding != encoding {
		t.Fatalf("received %q (encoding %d), want %q (encoding %d)", m.payload, m.encoding, payload, encoding)
	}
	t.Logf("received %q (encoding %d)", m.payload, m.encoding)
}

// TestGoRequestCppReceiver: lp2p_demo advertises as an LP2P receiver; ospc
// discovers it, connects, opens a data channel and exchanges text and binary
// messages, which lp2p_demo echoes.
func TestGoRequestCppReceiver(t *testing.T) {
	name := uniqueName("CppLp2pRecv")
	port := fmt.Sprint(20000 + randInt(20000))
	demo := startDemo(t, "--listen="+name, "--port="+port)
	demo.MustSee(`lp2p_demo: listening as `+name, stepTimeout)

	conn := dialDemo(t, name)
	id := uint64(3)
	dc, err := conn.OpenDataChannel(withTimeout(t, stepTimeout), ospc.DataChannelInit{Label: "chat", Protocol: "test", ID: &id})
	if err != nil {
		t.Fatalf("OpenDataChannel: %v", err)
	}
	defer dc.Close()

	if err := dc.SendMessageWithEncoding([]byte("hello"), ospc.DataEncodingString); err != nil {
		t.Fatal(err)
	}
	demo.MustSee(`lp2p_demo: data channel 3 "chat" open`, stepTimeout)
	demo.MustSee(`lp2p_demo: received on 3 \(string\): hello$`, stepTimeout)
	expectMessage(t, dc, []byte("echo: hello"), ospc.DataEncodingString)

	binary := []byte{0, 1, 2, 0xff}
	if err := dc.SendMessageWithEncoding(binary, ospc.DataEncodingArrayBuffer); err != nil {
		t.Fatal(err)
	}
	expectMessage(t, dc, binary, ospc.DataEncodingArrayBuffer)
	demo.MustSee(`lp2p_demo: received on 3 \(arraybuffer\)`, stepTimeout)
}

// listenGo advertises an ospc agent and returns the connection lp2p_demo
// makes to it once authenticated.
func listenGo(t *testing.T, name string) <-chan *ospc.Connection {
	t.Helper()
	l, err := ospc.Listen(ospc.AgentTransportQUIC, newAgent(t, name))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	conns := make(chan *ospc.Connection, 1)
	go func() {
		uc, err := l.Accept(context.Background())
		if err != nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), stepTimeout)
		defer cancel()
		conn, err := uc.AuthenticatePSK(ctx, []byte(harnessPSK))
		if err != nil {
			t.Errorf("authenticate lp2p_demo: %v", err)
			_ = uc.Close()
			return
		}
		conns <- conn
	}()
	return conns
}

func awaitConnection(t *testing.T, conns <-chan *ospc.Connection) *ospc.Connection {
	t.Helper()
	select {
	case conn := <-conns:
		t.Cleanup(func() { _ = conn.Close() })
		return conn
	case <-time.After(discoveryTimeout):
		t.Fatal("lp2p_demo did not connect")
		return nil
	}
}

// TestCppRequestGoReceiver: ospc advertises as an LP2P receiver; lp2p_demo
// discovers it, connects and opens a data channel, and the sides exchange
// text messages.
func TestCppRequestGoReceiver(t *testing.T) {
	name := uniqueName("GoLp2pRecv")
	conns := listenGo(t, name)
	demo := startDemo(t, "--connect="+name, "--label=chat", "--protocol=test")
	conn := awaitConnection(t, conns)

	dc, err := conn.AcceptDataChannel(withTimeout(t, stepTimeout))
	if err != nil {
		t.Fatalf("AcceptDataChannel: %v", err)
	}
	defer dc.Close()
	if dc.Label != "chat" || dc.Protocol != "test" || dc.ID != 0 {
		t.Errorf("data channel %+v, want label chat, protocol test, ID 0", dc.DataChannelParameters)
	}
	demo.MustSee(`lp2p_demo: data channel 0 "chat" open`, stepTimeout)

	demo.Send("hello")
	expectMessage(t, dc, []byte("hello"), ospc.DataEncodingString)
	if err := dc.SendMessageWithEncoding([]byte("hi from Go"), ospc.DataEncodingString); err != nil {
		t.Fatal(err)
	}
	demo.MustSee(`lp2p_demo: received on 0 \(string\): hi from Go$`, stepTimeout)
}

// TestSpecOpenResponse checks data-channel-open-response both ways: ospc
// waits for lp2p_demo's response before the channel is open, so the first
// message it reads is lp2p_demo's echo; lp2p_demo's channel opens only once
// ospc responded.
func TestSpecOpenResponse(t *testing.T) {
	t.Run("GoRequest", func(t *testing.T) {
		name := uniqueName("CppLp2pRecv")
		port := fmt.Sprint(20000 + randInt(20000))
		demo := startDemo(t, "--listen="+name, "--port="+port)
		demo.MustSee(`lp2p_demo: listening as `+name, stepTimeout)
		conn := dialDemo(t, name)
		dc, err := conn.OpenDataChannel(withTimeout(t, stepTimeout), ospc.DataChannelInit{Label: "chat"})
		if err != nil {
			t.Fatalf("OpenDataChannel: %v", err)
		}
		defer dc.Close()
		// The response arrived before OpenDataChannel returned, so lp2p_demo
		// has the channel already.
		demo.MustSee(`lp2p_demo: data channel 0 "chat" open`, stepTimeout)
		if err := dc.SendMessageWithEncoding([]byte("hello"), ospc.DataEncodingString); err != nil {
			t.Fatal(err)
		}
		expectMessage(t, dc, []byte("echo: hello"), ospc.DataEncodingString)
	})
	t.Run("CppRequest", func(t *testing.T) {
		name := uniqueName("GoLp2pRecv")
		conns := listenGo(t, name)
		demo := startDemo(t, "--connect="+name)
		conn := awaitConnection(t, conns)
		dc, err := conn.AcceptDataChannel(withTimeout(t, stepTimeout))
		if err != nil {
			t.Fatalf("AcceptDataChannel: %v", err)
		}
		defer dc.Close()
		demo.MustSee(`lp2p_demo: data channel 0 "chat" open`, stepTimeout)
	})
}

// TestChannelIDs checks channel-id allocation on a connection with channels
// opened from both sides. Both implementations take the lowest ID no open
// channel of the connection uses: lp2p_demo opens 0, so ospc's next channel
// on that connection gets 1.
func TestChannelIDs(t *testing.T) {
	name := uniqueName("GoLp2pRecv")
	conns := listenGo(t, name)
	demo := startDemo(t, "--connect="+name, "--label=chat")
	conn := awaitConnection(t, conns)
	rx, err := conn.AcceptDataChannel(withTimeout(t, stepTimeout))
	if err != nil {
		t.Fatalf("AcceptDataChannel: %v", err)
	}
	defer rx.Close()
	if rx.ID != 0 {
		t.Errorf("lp2p_demo's channel has ID %d, want 0", rx.ID)
	}
	demo.MustSee(`lp2p_demo: data channel 0 "chat" open`, stepTimeout)

	tx, err := conn.OpenDataChannel(withTimeout(t, stepTimeout), ospc.DataChannelInit{Label: "back"})
	if err != nil {
		t.Fatalf("OpenDataChannel: %v", err)
	}
	defer tx.Close()
	if tx.ID != 1 {
		t.Errorf("ospc's channel has ID %d, want 1", tx.ID)
	}
	demo.MustSee(`lp2p_demo: data channel 1 "back" open`, stepTimeout)
	if err := tx.SendMessageWithEncoding([]byte("on 1"), ospc.DataEncodingString); err != nil {
		t.Fatal(err)
	}
	demo.MustSee(`lp2p_demo: received on 1 \(string\): on 1$`, stepTimeout)
}
