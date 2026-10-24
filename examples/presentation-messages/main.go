// Command presentation-messages sends the Open Screen Presentation Protocol
// messages (application.bs "Presentation Protocol") by hand, against
// Chromium Open Screen's osp_demo.
//
// It is not a Presentation API implementation: each mode is a fixed
// sequence of messages, the way a Presentation API implementation would
// send them for one presentation, with the application.bs and Presentation
// API sections that call for each one.
//
//	-mode controller: Go controller, osp_demo receiver ("publisher").
//	-mode receiver:   osp_demo controller, Go receiver.
//
// With -osp-demo the example runs osp_demo itself (and, in receiver mode,
// types its commands). Without it, it waits for an osp_demo you started.
package main

import (
	"context"
	"crypto/rand"
	"errors"
	"flag"
	"fmt"
	"math/big"
	"os"
	"time"

	ospc "github.com/backkem/go-lp2p/openscreen-go/network"
	"github.com/backkem/go-lp2p/openscreen-go/test/ospdemo"
)

// config is what the two modes need.
type config struct {
	// OspDemo is the path of osp_demo to run, or "" when it is started by
	// hand.
	OspDemo string
	// Peer is the osp_demo receiver to connect to in controller mode
	// (its friendly name). Ignored with OspDemo, which picks one.
	Peer string
	// Name is the Go agent's name.
	Name string
	// URL is the presentation request URL.
	URL string
	// PSK is the pre-shared key both sides authenticate with.
	PSK string
	// Spake2P256 enables the SPAKE2 P-256 suite, see main.
	Spake2P256 bool
}

func main() {
	mode := flag.String("mode", "", "controller (Go controls an osp_demo receiver) or receiver (an osp_demo controller controls Go)")
	var c config
	flag.StringVar(&c.OspDemo, "osp-demo", "", "path of osp_demo to run, e.g. $OPENSCREEN_OUT_DIR/osp_demo; empty: start it yourself")
	flag.StringVar(&c.Peer, "peer", "", "controller mode without -osp-demo: friendly name of the osp_demo receiver")
	flag.StringVar(&c.URL, "url", "https://example.com/", "presentation request URL")
	flag.StringVar(&c.PSK, "psk", "", "PSK, decimal digits (default: random 6 digits)")
	// The one deviation from the spec. network.bs requires SPAKE2 over
	// edwards25519, which neither ospc nor Chromium's openscreen implements;
	// both implement the RFC 9382 P-256 suite as an opt-in (osp_demo:
	// --spake2_suite=p256). Without it authentication fails with
	// ospc.ErrSpake2SuiteUnsupported.
	flag.BoolVar(&c.Spake2P256, "spake2-p256", false, "authenticate with SPAKE2 over P-256 instead of edwards25519 (non-spec, needed for osp_demo)")
	flag.Parse()

	if c.PSK == "" {
		n, _ := rand.Int(rand.Reader, big.NewInt(900_000))
		c.PSK = fmt.Sprint(100_000 + n.Int64())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	var err error
	switch *mode {
	case "controller":
		c.Name = unique("GoController")
		err = runController(ctx, c)
	case "receiver":
		c.Name = unique("GoReceiver")
		err = runReceiver(ctx, c)
	default:
		flag.Usage()
		os.Exit(2)
	}
	if errors.Is(err, ospc.ErrSpake2SuiteUnsupported) {
		err = fmt.Errorf("%w (osp_demo needs -spake2-p256)", err)
	}
	if err != nil {
		printf("error: %v", err)
		os.Exit(1)
	}
	printf("done")
}

// newAgent creates the Go OSP agent. caps are the agent capabilities
// application.bs "Presentation API" requires: control-presentation for a
// controller, receive-presentation for a receiver.
func newAgent(c config, caps ...ospc.AgentCapability) (*ospc.Agent, error) {
	conf := ospc.NewAgentConfig(c.Name)
	conf.Capabilities = caps
	conf.InteropQuirks.Spake2SuiteP256 = c.Spake2P256
	return ospc.NewAgent(conf)
}

// authenticate runs SPAKE2 with the PSK (network.bs "Authentication"). Who
// presents the PSK and who enters it follows from both sides' psk-ease-of-input;
// here both sides already know it.
func authenticate(ctx context.Context, c config, uc *ospc.UnauthenticatedConnection) (*ospc.Connection, error) {
	printf("    SPAKE2 as PSK %v, PSK %s", uc.GetAuthenticationRole(), c.PSK)
	conn, err := uc.AuthenticatePSK(ctx, []byte(c.PSK))
	if err != nil {
		_ = uc.Close()
		return nil, fmt.Errorf("authenticate: %w", err)
	}
	printf("    authenticated")
	return conn, nil
}

// demoArgs are osp_demo's flags matching config.
func demoArgs(c config) []string {
	args := []string{"--psk=" + c.PSK}
	if c.Spake2P256 {
		args = append(args, "--spake2_suite=p256")
	}
	return args
}

// startDemo runs osp_demo with start, printing its interesting log lines.
func startDemo(c config, start func(ospdemo.Options) (*ospdemo.Process, error)) (*ospdemo.Process, error) {
	p, err := start(ospdemo.Options{Binary: c.OspDemo, Args: demoArgs(c), OnLine: printDemoLine})
	if err != nil {
		return nil, err
	}
	printf("    started %v", p.Args())
	return p, nil
}

// unique appends a random suffix to name, so the example finds its own
// osp_demo (and osp_demo its own Go agent) when other Open Screen agents
// run on the network.
func unique(name string) string {
	n, _ := rand.Int(rand.Reader, big.NewInt(1<<16))
	return fmt.Sprintf("%s%04x", name, n)
}
