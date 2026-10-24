// Package ospdemo runs and drives Chromium Open Screen's osp_demo binary, for
// interop tests and examples. It is a test rig, not part of the protocol.
//
// osp_demo runs as a publisher (an advertising presentation receiver) or as
// a listener (a presentation controller). It writes its log to a FIFO in its
// working directory (_recv_fifo or _cntl_fifo), blocking until a reader
// opens it, and reads commands from stdin. It also stores its agent
// certificate under osp/impl/quic/certificates/ relative to its working
// directory and crashes on startup without it. The package takes care of
// both.
package ospdemo

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Binary returns the path to osp_demo in $OPENSCREEN_OUT_DIR, or an error if
// it is not set or the binary is missing.
func Binary() (string, error) {
	dir := os.Getenv("OPENSCREEN_OUT_DIR")
	if dir == "" {
		return "", errors.New("OPENSCREEN_OUT_DIR is not set")
	}
	bin := filepath.Join(dir, "osp_demo")
	if fi, err := os.Stat(bin); err != nil {
		return "", err
	} else if fi.IsDir() {
		return "", fmt.Errorf("%s is a directory", bin)
	}
	return bin, nil
}

// Options configure an osp_demo process.
type Options struct {
	// Binary is the path to osp_demo.
	Binary string
	// Args are extra flags, e.g. "--spake2_suite=p256", "--psk=123456".
	// "-v" is always passed.
	Args []string
	// Dir is the working directory. If empty, a temporary directory is
	// created and removed by Stop.
	Dir string
	// OnLine, if set, is called with every log and output line.
	OnLine func(line string)
}

// Process is a running osp_demo.
type Process struct {
	opts    Options
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	fifo    *os.File
	tempDir string

	mu      sync.Mutex
	lines   []string
	changed chan struct{} // Closed and replaced when lines change.

	exited   chan struct{}
	stopOnce sync.Once
}

// StartPublisher runs osp_demo as a presentation receiver advertising
// friendlyName. Only one can run per host: it always binds UDP port 6667.
func StartPublisher(opts Options, friendlyName string) (*Process, error) {
	return start(opts, "_recv_fifo", friendlyName)
}

// StartController runs osp_demo as a listening presentation controller.
func StartController(opts Options) (*Process, error) {
	return start(opts, "_cntl_fifo")
}

func start(opts Options, fifo string, args ...string) (*Process, error) {
	p := &Process{
		opts:    opts,
		changed: make(chan struct{}),
		exited:  make(chan struct{}),
	}
	dir := opts.Dir
	if dir == "" {
		var err error
		if dir, err = os.MkdirTemp("", "osp_demo-"); err != nil {
			return nil, err
		}
		p.tempDir = dir
	}
	fail := func(err error) (*Process, error) {
		if p.tempDir != "" {
			_ = os.RemoveAll(p.tempDir)
		}
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(dir, "osp/impl/quic/certificates"), 0o755); err != nil {
		return fail(err)
	}
	fifoPath := filepath.Join(dir, fifo)
	if err := syscall.Mkfifo(fifoPath, 0o600); err != nil && !errors.Is(err, os.ErrExist) {
		return fail(fmt.Errorf("mkfifo: %w", err))
	}
	// Open the FIFO read-write so the open doesn't block on the writer and
	// the read side survives osp_demo reopening it.
	f, err := os.OpenFile(fifoPath, os.O_RDWR, 0)
	if err != nil {
		return fail(fmt.Errorf("open fifo: %w", err))
	}
	p.fifo = f

	p.cmd = exec.Command(opts.Binary, append(append([]string{"-v"}, opts.Args...), args...)...)
	p.cmd.Dir = dir
	// Own process group so Stop can signal everything; Pdeathsig kills
	// osp_demo even if this process dies without stopping it.
	p.cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGKILL}
	if p.stdin, err = p.cmd.StdinPipe(); err != nil {
		_ = f.Close()
		return fail(err)
	}
	outR, outW, err := os.Pipe()
	if err != nil {
		_ = f.Close()
		return fail(err)
	}
	p.cmd.Stdout, p.cmd.Stderr = outW, outW
	if err := p.cmd.Start(); err != nil {
		_ = f.Close()
		_ = outR.Close()
		_ = outW.Close()
		return fail(fmt.Errorf("start osp_demo: %w", err))
	}
	_ = outW.Close()

	go func() {
		p.collect("out", outR)
		_ = outR.Close()
	}()
	go p.collect("log", f)
	go func() {
		err := p.cmd.Wait()
		p.addLine("proc", fmt.Sprintf("exited: %v", err))
		close(p.exited)
	}()
	return p, nil
}

// Args returns the command line osp_demo was started with.
func (p *Process) Args() []string { return p.cmd.Args }

// Pid returns the process id.
func (p *Process) Pid() int { return p.cmd.Process.Pid }

// Dir returns the working directory.
func (p *Process) Dir() string { return p.cmd.Dir }

func (p *Process) collect(prefix string, r io.Reader) {
	s := bufio.NewScanner(r)
	s.Buffer(make([]byte, 64*1024), 1024*1024)
	for s.Scan() {
		p.addLine(prefix, s.Text())
	}
}

func (p *Process) addLine(prefix, line string) {
	line = prefix + ": " + line
	p.mu.Lock()
	p.lines = append(p.lines, line)
	close(p.changed)
	p.changed = make(chan struct{})
	p.mu.Unlock()
	if p.opts.OnLine != nil {
		p.opts.OnLine(line)
	}
}

// Send writes a command line to osp_demo's stdin, e.g. "connect <name>".
func (p *Process) Send(command string) error {
	_, err := io.WriteString(p.stdin, command+"\n")
	return err
}

// WaitFor waits for a line matching pattern, logged after mark (see Mark;
// 0 means from the start), and returns it.
func (p *Process) WaitFor(mark int, pattern string, timeout time.Duration) (string, bool) {
	re := regexp.MustCompile(pattern)
	deadline := time.After(timeout)
	next := mark
	for {
		p.mu.Lock()
		lines := p.lines[next:]
		next = len(p.lines)
		changed := p.changed
		p.mu.Unlock()
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

// Mark returns the current position in the log, for WaitFor and GrepSince.
func (p *Process) Mark() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.lines)
}

// GrepSince returns the lines after mark that match pattern.
func (p *Process) GrepSince(mark int, pattern string) []string {
	re := regexp.MustCompile(pattern)
	p.mu.Lock()
	defer p.mu.Unlock()
	var out []string
	for _, l := range p.lines[mark:] {
		if re.MatchString(l) {
			out = append(out, l)
		}
	}
	return out
}

// Grep returns all lines so far that match pattern.
func (p *Process) Grep(pattern string) []string { return p.GrepSince(0, pattern) }

// Log returns all lines so far.
func (p *Process) Log() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return strings.Join(p.lines, "\n")
}

// Running reports whether the process is still alive.
func (p *Process) Running() bool {
	select {
	case <-p.exited:
		return false
	default:
		return true
	}
}

// Stop ends the process: EOF on stdin makes osp_demo shut down cleanly,
// then SIGINT, then SIGKILL of its process group. It is safe to call more
// than once.
func (p *Process) Stop() {
	p.stopOnce.Do(func() {
		defer func() {
			_ = p.fifo.Close()
			if p.tempDir != "" {
				_ = os.RemoveAll(p.tempDir)
			}
		}()
		_ = p.stdin.Close()
		for _, sig := range []syscall.Signal{0, syscall.SIGINT, syscall.SIGKILL} {
			if sig != 0 {
				_ = syscall.Kill(-p.cmd.Process.Pid, sig)
			}
			select {
			case <-p.exited:
				return
			case <-time.After(3 * time.Second):
			}
		}
		<-p.exited
	})
}
