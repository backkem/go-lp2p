package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/backkem/go-lp2p/openscreen-go/application"
	ospc "github.com/backkem/go-lp2p/openscreen-go/network"
	"github.com/backkem/go-lp2p/openscreen-go/test/ospdemo"
)

// runReceiver is the receiving user agent: it advertises itself, and answers
// whatever an osp_demo controller sends until the presentation is
// terminated.
func runReceiver(ctx context.Context, c config) error {
	agent, err := newAgent(c, ospc.CapabilityReceivePresentation)
	if err != nil {
		return err
	}
	step("Advertise as %q (network.bs \"Discovery with mDNS\")", c.Name)
	l, err := ospc.Listen(ospc.AgentTransportQUIC, agent)
	if err != nil {
		return err
	}
	defer l.Close()

	// osp_demo, with its commands typed for you or by you.
	script := make(chan error, 1)
	if c.OspDemo != "" {
		step("Start osp_demo as controller")
		demo, err := startDemo(c, ospdemo.StartController)
		if err != nil {
			return err
		}
		defer demo.Stop()
		go func() { script <- driveDemoController(demo, c) }()
	} else {
		printf("    start osp_demo:  osp_demo %s", strings.Join(demoArgs(c), " "))
		printf("    and type:        connect %s / avail %s / start %s %s / msg hi / close / reconnect / msg hi / term", c.Name, c.URL, c.URL, c.Name)
		close(script)
	}

	uc, err := l.Accept(ctx)
	if err != nil {
		return fmt.Errorf("accept: %w", err)
	}
	step("Accepted %q, authenticate (network.bs SPAKE2)", uc.RemoteAgent().Info().DisplayName)
	conn, err := authenticate(ctx, c, uc)
	if err != nil {
		return err
	}
	p := newPeer(conn)
	defer p.Close()

	if err := serve(ctx, p); err != nil {
		return err
	}
	if err := <-script; err != nil {
		return fmt.Errorf("osp_demo: %w", err)
	}
	return nil
}

// serve answers the controller's messages until the presentation is
// terminated. The receiver keeps the presentation's state: its id and URL,
// and its open connections.
func serve(ctx context.Context, p *peer) error {
	var (
		presentationID, url string
		connections                = map[uint64]bool{} // open connection-ids
		nextConnectionID    uint64 = 1
	)
	for {
		m, err := p.next(ctx)
		if err != nil {
			return err
		}
		switch req := m.Body.(type) {
		case *application.PresentationURLAvailabilityRequest:
			// One url-availability per URL, by index. A real receiver would
			// also send presentation-url-availability-events with the
			// watch-id while the watch-duration lasts.
			resp := &application.PresentationURLAvailabilityResponse{RequestID: req.RequestID}
			for range req.URLs {
				resp.URLAvailabilities = append(resp.URLAvailabilities, application.URLAvailable)
			}
			err = p.send(ctx, application.TypeKeyPresentationURLAvailabilityResponse, resp)

		case *application.PresentationStartRequest:
			// The presentation identifier must have at least 16 ASCII
			// characters, otherwise invalid-presentation-id. A real receiver
			// loads the URL first and answers once it is loaded or failed;
			// the connection-id is the receiver's choice.
			resp := &application.PresentationStartResponse{RequestID: req.RequestID, Result: application.ResultSuccess}
			if application.ValidatePresentationID(req.PresentationID) != nil {
				resp.Result = application.ResultInvalidPresentationID
			} else {
				presentationID, url = req.PresentationID, req.URL
				resp.ConnectionID = nextConnectionID
				connections[nextConnectionID] = true
				nextConnectionID++
			}
			err = p.send(ctx, application.TypeKeyPresentationStartResponse, resp)

		case *application.PresentationConnectionMessage:
			// Answer every message on the same connection.
			if connections[req.ConnectionID] {
				err = p.send(ctx, application.TypeKeyPresentationConnectionMessage, &application.PresentationConnectionMessage{
					ConnectionID: req.ConnectionID, Message: fmt.Sprintf("Go receiver got %q", req.Message),
				})
			}

		case *application.PresentationConnectionCloseEvent:
			// The controller closed a connection; the presentation goes on.
			// A receiver sends presentation-change-event to the other
			// controllers connected to the presentation; there are none.
			delete(connections, req.ConnectionID)

		case *application.PresentationConnectionOpenRequest:
			// A controller reconnects (Presentation API 6.7.1 "Monitoring
			// incoming presentation connections").
			resp := &application.PresentationConnectionOpenResponse{RequestID: req.RequestID, Result: application.ResultSuccess}
			if req.PresentationID != presentationID {
				resp.Result = application.ResultInvalidPresentationID
			} else {
				resp.ConnectionID = nextConnectionID
				connections[nextConnectionID] = true
				nextConnectionID++
			}
			resp.ConnectionCount = uint64(len(connections))
			err = p.send(ctx, application.TypeKeyPresentationConnectionOpenResponse, resp)

		case *application.PresentationTerminationRequest:
			// A receiver also sends presentation-termination-event to the
			// other controllers of the presentation; there are none.
			resp := &application.PresentationTerminationResponse{RequestID: req.RequestID, Result: application.ResultSuccess}
			if req.PresentationID != presentationID {
				resp.Result = application.ResultInvalidPresentationID
			}
			if err := p.send(ctx, application.TypeKeyPresentationTerminationResponse, resp); err != nil {
				return err
			}
			if resp.Result == application.ResultSuccess {
				printf("    presentation %s (%s) terminated", presentationID, url)
				return nil
			}
		}
		if err != nil {
			return err
		}
	}
}

// driveDemoController types osp_demo's controller commands, each once
// osp_demo logged that the previous one took effect.
func driveDemoController(demo *ospdemo.Process, c config) error {
	steps := []struct{ command, wait string }{
		{"", `found! ` + c.Name},
		{"connect " + c.Name, `SPAKE2: authenticated`},
		{"avail " + c.URL, `available! ` + c.Name},
		{"start " + c.URL + " " + c.Name, `presentation connection connected`},
		{"msg hello from osp_demo", `got message: `},
		{"close", ``},
		{"reconnect", `presentation connection connected`},
		{"msg hello again from osp_demo", `got message: `},
		{"term", `(?i)terminat`},
	}
	for _, s := range steps {
		mark := demo.Mark()
		if s.command != "" {
			printf("   osp_demo $ %s", s.command)
			if err := demo.Send(s.command); err != nil {
				return err
			}
		}
		if s.wait == "" {
			time.Sleep(200 * time.Millisecond)
			continue
		}
		if _, ok := demo.WaitFor(mark, s.wait, 20*time.Second); !ok {
			return fmt.Errorf("no %q after %q", s.wait, s.command)
		}
	}
	return nil
}
