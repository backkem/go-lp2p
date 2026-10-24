package ua

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
)

// CLI is an interactive UI on a terminal. Prompts are shown one at a time.
//
// Input is read by a background goroutine started on the first prompt. A
// blocking read on a terminal can't be interrupted, so that goroutine lives
// as long as the input does.
type CLI struct {
	in  io.Reader
	out io.Writer

	prompt   sync.Mutex // one prompt at a time
	startIn  sync.Once
	lines    chan string
	inputErr error // set before lines is closed
}

var _ UI = (*CLI)(nil)

// NewCLI creates a CLI reading answers from in and writing prompts to out.
func NewCLI(in io.Reader, out io.Writer) *CLI {
	return &CLI{in: in, out: out}
}

func (c *CLI) readLine(ctx context.Context) (string, error) {
	c.startIn.Do(func() {
		c.lines = make(chan string)
		go func() {
			sc := bufio.NewScanner(c.in)
			for sc.Scan() {
				c.lines <- strings.TrimSpace(sc.Text())
			}
			c.inputErr = sc.Err()
			if c.inputErr == nil {
				c.inputErr = io.EOF
			}
			close(c.lines)
		}()
	})
	select {
	case l, ok := <-c.lines:
		if !ok {
			return "", fmt.Errorf("read input: %w", c.inputErr)
		}
		return l, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

// RequestPermission implements UI.
func (c *CLI) RequestPermission(ctx context.Context, req PermissionRequest) (bool, error) {
	c.prompt.Lock()
	defer c.prompt.Unlock()
	switch req.Permission {
	case PermissionAdvertise:
		fmt.Fprintf(c.out, "%s wants to make this device discoverable as %q. Allow? [y/N] ", req.Origin, req.Nickname)
	case PermissionAcceptConnection:
		fmt.Fprintf(c.out, "%s (%q) wants to accept a connection from %s. Allow? [y/N] ", req.Origin, req.Nickname, req.Peer.Label())
	default:
		fmt.Fprintf(c.out, "%s asks for %s. Allow? [y/N] ", req.Origin, req.Permission)
	}
	l, err := c.readLine(ctx)
	if err != nil {
		return false, err
	}
	return strings.EqualFold(l, "y") || strings.EqualFold(l, "yes"), nil
}

// PickPeer implements UI. The list is redrawn whenever it changes; the user
// types a number to pick or q to cancel.
func (c *CLI) PickPeer(ctx context.Context, p *Picker) (Peer, error) {
	c.prompt.Lock()
	defer c.prompt.Unlock()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	type answer struct {
		line string
		err  error
	}
	answers := make(chan answer)
	ask := func() {
		go func() {
			l, err := c.readLine(ctx)
			select {
			case answers <- answer{l, err}:
			case <-ctx.Done():
			}
		}()
	}

	fmt.Fprintf(c.out, "%s (%q) wants to connect to a device.\n", p.Origin, p.Nickname)
	var shown []Peer
	updates := p.Watch(ctx)
	ask()
	for {
		select {
		case peers, ok := <-updates:
			if !ok {
				return Peer{}, ctx.Err()
			}
			shown = peers
			if len(peers) == 0 {
				fmt.Fprintln(c.out, "  Searching for devices... (q to cancel)")
				continue
			}
			fmt.Fprintln(c.out, "  Devices:")
			for i, peer := range peers {
				fmt.Fprintf(c.out, "  %d) %s\n", i+1, peer.Label())
			}
			fmt.Fprint(c.out, "  Pick a device (number, q to cancel): ")
		case a := <-answers:
			if a.err != nil {
				return Peer{}, a.err
			}
			if strings.EqualFold(a.line, "q") {
				return Peer{}, ErrCanceled
			}
			i, err := strconv.Atoi(a.line)
			if err != nil || i < 1 || i > len(shown) {
				fmt.Fprintf(c.out, "  No device %q. Pick a device (number, q to cancel): ", a.line)
				ask()
				continue
			}
			return shown[i-1], nil
		}
	}
}

// PresentPIN implements UI.
func (c *CLI) PresentPIN(ctx context.Context, p PINPresentation) error {
	fmt.Fprintf(c.out, "To pair with %s, enter this PIN there: %s\n", p.Peer.Label(), p.PIN)
	return nil
}

// EnterPIN implements UI. An empty line cancels.
func (c *CLI) EnterPIN(ctx context.Context, p PINPrompt) (string, error) {
	c.prompt.Lock()
	defer c.prompt.Unlock()
	fmt.Fprintf(c.out, "Enter the PIN shown on %s: ", p.Peer.Label())
	l, err := c.readLine(ctx)
	if err != nil {
		return "", err
	}
	if l == "" {
		return "", ErrCanceled
	}
	return l, nil
}
