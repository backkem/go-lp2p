//go:build interop

package main

import (
	"context"
	"testing"
	"time"

	"github.com/backkem/go-lp2p/openscreen-go/test/ospdemo"
)

// TestAgainstOspDemo runs both modes against osp_demo. It needs the interop
// build tag and $OPENSCREEN_OUT_DIR/osp_demo:
//
//	OPENSCREEN_OUT_DIR=/work/openscreen/out/debug \
//	  go test -tags interop -p 1 ./examples/presentation-messages/
//
// -p 1: the osp_demo receiver always binds UDP port 6667, so it can't run
// next to the openscreen-go/test/osp-demo-interop tests.
func TestAgainstOspDemo(t *testing.T) {
	bin, err := ospdemo.Binary()
	if err != nil {
		t.Skipf("osp_demo not available: %v", err)
	}
	for _, mode := range []struct {
		name string
		run  func(context.Context, config) error
	}{
		{"GoController", runController},
		{"GoReceiver", runReceiver},
	} {
		t.Run(mode.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			c := config{
				OspDemo:    bin,
				Name:       unique(mode.name),
				URL:        "https://example.com/",
				PSK:        "123456",
				Spake2P256: true,
			}
			if err := mode.run(ctx, c); err != nil {
				t.Fatal(err)
			}
		})
	}
}
