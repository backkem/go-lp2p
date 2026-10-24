package main

import (
	"context"
	"fmt"

	"github.com/backkem/go-lp2p/openscreen-go/application"
	ospc "github.com/backkem/go-lp2p/openscreen-go/network"
	"github.com/backkem/go-lp2p/openscreen-go/test/ospdemo"
)

// runController is the controlling user agent: it starts a presentation on
// an osp_demo receiver, exchanges messages with it, reconnects and
// terminates it.
func runController(ctx context.Context, c config) error {
	if c.OspDemo != "" {
		c.Peer = unique("OspDemoReceiver")
		step("Start osp_demo as receiver %q", c.Peer)
		demo, err := startDemo(c, func(o ospdemo.Options) (*ospdemo.Process, error) {
			return ospdemo.StartPublisher(o, c.Peer)
		})
		if err != nil {
			return err
		}
		defer demo.Stop()
	} else if c.Peer == "" {
		return fmt.Errorf("controller mode needs -osp-demo, or -peer with the name of a running osp_demo receiver")
	}

	// application.bs "Presentation API": the controller may use mDNS, QUIC
	// and agent-info to discover receivers (Presentation API 6.4.2).
	step("Discover %q (network.bs \"Discovery with mDNS\")", c.Peer)
	d := ospc.NewDiscoverer()
	d.WithNickname(c.Peer)
	if err := d.Start(); err != nil {
		return err
	}
	found, err := d.Accept(ctx)
	_ = d.Close()
	if err != nil {
		return fmt.Errorf("discover %q: %w", c.Peer, err)
	}
	printf("    found %q", found.Nickname())

	step("Connect and authenticate (network.bs: QUIC, TLS 1.3, agent-info, SPAKE2)")
	agent, err := newAgent(c, ospc.CapabilityControlPresentation)
	if err != nil {
		return err
	}
	uc, err := found.Dial(ctx, ospc.AgentTransportQUIC, agent)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	printf("    connected to %q", uc.RemoteAgent().Info().DisplayName)
	conn, err := authenticate(ctx, c, uc)
	if err != nil {
		return err
	}
	p := newPeer(conn)
	defer p.Close()

	// Presentation API 6.4.2: is the URL available on this display?
	// application.bs: presentation-url-availability-request; the response
	// has one url-availability per URL. watch-duration asks for
	// presentation-url-availability-events for the next minute.
	step("URL availability (Presentation API 6.4.2)")
	id := p.app.NextRequestID()
	var avail application.PresentationURLAvailabilityResponse
	if err := p.request(ctx, application.TypeKeyPresentationURLAvailabilityRequest, id,
		&application.PresentationURLAvailabilityRequest{RequestID: id, URLs: []string{c.URL}, WatchDuration: 60_000_000, WatchID: 1},
		application.TypeKeyPresentationURLAvailabilityResponse, &avail); err != nil {
		return err
	}
	if len(avail.URLAvailabilities) != 1 || avail.URLAvailabilities[0] != application.URLAvailable {
		return fmt.Errorf("%s is not available: %v", c.URL, avail.URLAvailabilities)
	}

	// Presentation API 6.3.4 "Starting a presentation connection":
	// presentation-start-request with a new presentation identifier, at
	// least 16 ASCII characters. The receiver picks the connection-id.
	step("Start the presentation (Presentation API 6.3.4)")
	presentationID := application.NewPresentationID()
	id = p.app.NextRequestID()
	var start application.PresentationStartResponse
	if err := p.request(ctx, application.TypeKeyPresentationStartRequest, id,
		&application.PresentationStartRequest{RequestID: id, PresentationID: presentationID, URL: c.URL},
		application.TypeKeyPresentationStartResponse, &start); err != nil {
		return err
	}
	if start.Result != application.ResultSuccess {
		return fmt.Errorf("start failed: result %d", start.Result)
	}

	// Presentation API 6.5.2 "Sending a message": a string message is a
	// CBOR text string in presentation-connection-message. osp_demo
	// echoes it.
	step("Send a message (Presentation API 6.5.2)")
	if err := exchange(ctx, p, start.ConnectionID, "hello from the Go controller"); err != nil {
		return err
	}

	// Presentation API 6.5.5 "Closing a PresentationConnection": the
	// controller sends presentation-connection-close-event; the
	// presentation keeps running.
	step("Close the connection (Presentation API 6.5.5)")
	if err := p.send(ctx, application.TypeKeyPresentationConnectionCloseEvent, &application.PresentationConnectionCloseEvent{
		ConnectionID: start.ConnectionID,
		Reason:       application.ConnectionCloseMethodCalled,
		// Required by the CDDL, but only meaningful from the receiver
		// (application.bs lists it for the receiver's close event only).
		ConnectionCount: 0,
	}); err != nil {
		return err
	}

	// Presentation API 6.3.5 "Reconnecting to a presentation":
	// presentation-connection-open-request with the presentation's URL and
	// identifier. The receiver answers with a new connection-id and the
	// number of connections to the presentation.
	step("Reconnect to the presentation (Presentation API 6.3.5)")
	id = p.app.NextRequestID()
	var open application.PresentationConnectionOpenResponse
	if err := p.request(ctx, application.TypeKeyPresentationConnectionOpenRequest, id,
		&application.PresentationConnectionOpenRequest{RequestID: id, PresentationID: presentationID, URL: c.URL},
		application.TypeKeyPresentationConnectionOpenResponse, &open); err != nil {
		return err
	}
	if open.Result != application.ResultSuccess {
		return fmt.Errorf("reconnect failed: result %d", open.Result)
	}
	if err := exchange(ctx, p, open.ConnectionID, "hello again"); err != nil {
		return err
	}

	// Presentation API 6.5.6 "Terminating a presentation in a controlling
	// browsing context": presentation-termination-request with reason
	// application-request.
	step("Terminate the presentation (Presentation API 6.5.6)")
	id = p.app.NextRequestID()
	var term application.PresentationTerminationResponse
	if err := p.request(ctx, application.TypeKeyPresentationTerminationRequest, id,
		&application.PresentationTerminationRequest{RequestID: id, PresentationID: presentationID, Reason: application.TerminationReasonApplicationRequest},
		application.TypeKeyPresentationTerminationResponse, &term); err != nil {
		return err
	}
	if term.Result != application.ResultSuccess {
		return fmt.Errorf("terminate failed: result %d", term.Result)
	}
	return nil
}

// exchange sends text on the connection and waits for the receiver's reply
// on it.
func exchange(ctx context.Context, p *peer, connectionID uint64, text string) error {
	if err := p.send(ctx, application.TypeKeyPresentationConnectionMessage,
		&application.PresentationConnectionMessage{ConnectionID: connectionID, Message: text}); err != nil {
		return err
	}
	for {
		body, err := p.expect(ctx, application.TypeKeyPresentationConnectionMessage)
		if err != nil {
			return err
		}
		if m := body.(*application.PresentationConnectionMessage); m.ConnectionID == connectionID {
			return nil
		}
	}
}
