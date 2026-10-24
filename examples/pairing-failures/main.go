// Command pairing-failures shows how the API reports what can go wrong
// before a connection exists: the user refuses, dismisses the picker, or
// types the wrong PIN. Pages see a DOMException; the user agent's reason is
// available with errors.Is.
package main

import (
	"errors"
	"flag"
	"log"
	"time"

	"github.com/backkem/go-lp2p/examples/internal/demo"
	. "github.com/backkem/go-lp2p/lp2p-api" //lint:ignore ST1001 emulate the global web API
	ua "github.com/backkem/go-lp2p/lp2p-api/useragent"
)

// report logs how a page would handle the error from start().
func report(scenario string, err error) {
	var de *DOMException
	switch {
	case err == nil:
		log.Fatalf("%s: unexpectedly succeeded", scenario)
	case !errors.As(err, &de):
		log.Fatalf("%s: not a DOMException: %v", scenario, err)
	}
	reason := "other"
	switch {
	case errors.Is(err, ua.ErrPermissionDenied):
		reason = "user said no"
	case errors.Is(err, ua.ErrCanceled):
		reason = "user canceled"
	case errors.Is(err, ua.ErrAuthenticationFailed):
		reason = "pairing failed"
	}
	log.Printf("%s: page gets %s (%s)", scenario, de.Name, reason)
}

func startReceiver(b *demo.Browser, nickname string) *LP2PReceiver {
	r, err := b.Window.NewLP2PReceiver(LP2PReceiverOptions{Nickname: nickname})
	if err != nil {
		log.Fatal(err)
	}
	r.OnConnection(func(e LP2PConnectionEvent) {
		log.Fatalf("%s: unexpectedly got a connection", nickname)
	})
	if err := r.Start(); err != nil {
		log.Fatalf("%s: %v", nickname, err)
	}
	return r
}

func connect(b *demo.Browser) error {
	request, err := b.Window.NewLP2PRequest(LP2PRequestOptions{Nickname: "Requester"})
	if err != nil {
		return err
	}
	_, err = request.Start()
	return err
}

func main() {
	flag.Parse()
	demo.Watchdog(20 * time.Second)

	// 1. The user doesn't want the device to be discoverable.
	{
		b := demo.NewBrowser("Shy device", &ua.Scripted{Permission: ua.Deny(ua.PermissionAdvertise)})
		r, _ := b.Window.NewLP2PReceiver(LP2PReceiverOptions{Nickname: "Shy device"})
		report("1. receiver.start(), advertising denied", r.Start())
		b.Close()
	}

	// 2. The user closes the device picker without picking.
	{
		b := demo.NewBrowser("Indecisive phone", &ua.Scripted{}) // no Pick: dismisses the picker
		report("2. request.start(), picker dismissed", connect(b))
		b.Close()
	}

	// 3. The user types the wrong PIN. ospc doesn't report a PIN mismatch,
	// so the user agent gives up after its pairing timeout (2s here).
	{
		name := demo.Unique("TV")
		tv := demo.NewBrowser(name, &ua.Scripted{Enter: ua.FixedPIN("0000-0000")})
		r := startReceiver(tv, name)
		phone := demo.NewBrowser("Clumsy phone", &ua.Scripted{
			Pick:  ua.PickByName(name),
			Enter: ua.FixedPIN("0000-0000"),
		})
		report("3. request.start(), wrong PIN", connect(phone))
		phone.Close()
		r.Close()
		tv.Close()
	}

	// 4. The receiving user refuses the incoming connection. The phone
	// isn't told (ospc has no close signal before pairing); its user gives
	// up waiting for a PIN.
	{
		relay := ua.NewPINRelay()
		name := demo.Unique("Projector")
		projector := demo.NewBrowser(name, &ua.Scripted{
			Permission: ua.Deny(ua.PermissionAcceptConnection),
			Present:    relay.Present,
		})
		r := startReceiver(projector, name)
		phone := demo.NewBrowser("Unwelcome phone", &ua.Scripted{
			Pick:  ua.PickByName(name),
			Enter: relay.EnterWithin(2 * time.Second),
		})
		report("4. request.start(), connection refused by the receiver", connect(phone))
		phone.Close()
		r.Close()
		projector.Close()
	}
}
