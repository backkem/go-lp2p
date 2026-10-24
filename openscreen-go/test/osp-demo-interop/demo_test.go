//go:build interop

package ospdemointerop

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
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

// ospDemo is a running osp_demo process.
//
// osp_demo writes its log to a FIFO in its working directory (_recv_fifo for
// the publisher, _cntl_fifo for the listener) and reads commands from stdin.
// It also needs osp/impl/quic/certificates/ in its working directory to store
// its agent certificate, otherwise it crashes on startup.
type ospDemo struct {
	t     *testing.T
	name  string
	cmd   *exec.Cmd
	stdin io.WriteCloser
	fifo  *os.File

	mu      sync.Mutex
	lines   []string
	changed chan struct{} // Closed and replaced when lines change.

	exited chan struct{}
	waitMu sync.Mutex
}

// startPublisher runs osp_demo as an advertising receiver ("publisher").
func startPublisher(t *testing.T, friendlyName string) *ospDemo {
	return startDemo(t, "recv", "_recv_fifo", friendlyName)
}

// startController runs osp_demo as a listening controller.
func startController(t *testing.T) *ospDemo {
	return startDemo(t, "cntl", "_cntl_fifo")
}

func startDemo(t *testing.T, name, fifo string, args ...string) *ospDemo {
	t.Helper()
	bin := demoBinary(t)

	cwd := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cwd, "osp/impl/quic/certificates"), 0o755); err != nil {
		t.Fatal(err)
	}
	fifoPath := filepath.Join(cwd, fifo)
	if err := syscall.Mkfifo(fifoPath, 0o600); err != nil {
		t.Fatalf("mkfifo: %v", err)
	}

	d := &ospDemo{
		t:       t,
		name:    name,
		changed: make(chan struct{}),
		exited:  make(chan struct{}),
	}

	d.cmd = exec.Command(bin, append([]string{"-v"}, args...)...)
	d.cmd.Dir = cwd
	// Own process group so stop can signal everything; Pdeathsig kills
	// osp_demo even if the test binary dies without running cleanups.
	d.cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGKILL}
	stdin, err := d.cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	d.stdin = stdin
	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	d.cmd.Stdout = outW
	d.cmd.Stderr = outW

	if err := d.cmd.Start(); err != nil {
		t.Fatalf("start osp_demo: %v", err)
	}
	_ = outW.Close()
	t.Logf("[%s] started %s %v (pid %d, cwd %s)", name, bin, d.cmd.Args[1:], d.cmd.Process.Pid, cwd)

	go func() {
		d.collect("out", outR)
		_ = outR.Close()
	}()
	// Open the FIFO read-write so the open doesn't block on the writer and the
	// read side survives osp_demo reopening it. Closed in stop.
	fifoFile, err := os.OpenFile(fifoPath, os.O_RDWR, 0)
	if err != nil {
		t.Fatalf("open fifo: %v", err)
	}
	d.fifo = fifoFile
	go d.collect("log", fifoFile)
	go func() {
		err := d.cmd.Wait()
		d.addLine("proc", fmt.Sprintf("exited: %v", err))
		close(d.exited)
	}()

	t.Cleanup(d.stop)
	return d
}

func (d *ospDemo) collect(prefix string, r io.Reader) {
	s := bufio.NewScanner(r)
	s.Buffer(make([]byte, 64*1024), 1024*1024)
	for s.Scan() {
		d.addLine(prefix, s.Text())
	}
}

func (d *ospDemo) addLine(prefix, line string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.lines = append(d.lines, prefix+": "+line)
	close(d.changed)
	d.changed = make(chan struct{})
}

// Send writes a command to osp_demo's stdin.
func (d *ospDemo) Send(command string) {
	d.t.Helper()
	d.t.Logf("[%s] <stdin> %s", d.name, command)
	if _, err := io.WriteString(d.stdin, command+"\n"); err != nil {
		d.t.Errorf("[%s] write stdin: %v", d.name, err)
	}
}

// WaitFor waits for a log line matching pattern and returns it.
func (d *ospDemo) WaitFor(pattern string, timeout time.Duration) (string, bool) {
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

// Grep returns all log lines so far that match pattern.
func (d *ospDemo) Grep(pattern string) []string {
	re := regexp.MustCompile(pattern)
	d.mu.Lock()
	defer d.mu.Unlock()
	var out []string
	for _, l := range d.lines {
		if re.MatchString(l) {
			out = append(out, l)
		}
	}
	return out
}

// Running reports whether the process is still alive.
func (d *ospDemo) Running() bool {
	select {
	case <-d.exited:
		return false
	default:
		return true
	}
}

// DumpLog writes the full collected log to the test log, or to the file named
// by OSP_DEMO_LOG_DIR/<test>-<name>.log when that is set.
func (d *ospDemo) DumpLog() {
	d.mu.Lock()
	text := strings.Join(d.lines, "\n")
	d.mu.Unlock()
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

// stop ends the process: EOF on stdin makes osp_demo shut down cleanly, then
// SIGINT, then SIGKILL of its process group.
func (d *ospDemo) stop() {
	d.waitMu.Lock()
	defer d.waitMu.Unlock()
	defer d.fifo.Close()
	if !d.Running() {
		return
	}
	_ = d.stdin.Close()
	select {
	case <-d.exited:
		return
	case <-time.After(3 * time.Second):
	}
	_ = syscall.Kill(-d.cmd.Process.Pid, syscall.SIGINT)
	select {
	case <-d.exited:
		return
	case <-time.After(3 * time.Second):
	}
	_ = syscall.Kill(-d.cmd.Process.Pid, syscall.SIGKILL)
	<-d.exited
}

// withTimeout returns a context bounded by the test deadline and timeout.
func withTimeout(t *testing.T, timeout time.Duration) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	t.Cleanup(cancel)
	return ctx
}
