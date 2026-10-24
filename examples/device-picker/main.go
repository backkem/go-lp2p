// Command device-picker shows the user agent's device picker: three devices
// advertise themselves, the picker lists them as they are discovered, and
// the user picks one. Only the picked device gets a connection.
package main

import (
	"context"
	"flag"
	"log"
	"slices"
	"strings"
	"time"

	"github.com/backkem/go-lp2p/examples/internal/demo"
	. "github.com/backkem/go-lp2p/lp2p-api" //lint:ignore ST1001 emulate the global web API
	ua "github.com/backkem/go-lp2p/lp2p-api/useragent"
)

var devices = []string{"Kitchen speaker", "Living room TV", "Office display"}

const choice = "Living room TV"

// displayPage runs on each device: it greets whoever connects.
func displayPage(w *Window, nickname string) (*LP2PReceiver, error) {
	receiver, err := w.NewLP2PReceiver(LP2PReceiverOptions{Nickname: nickname})
	if err != nil {
		return nil, err
	}
	receiver.OnConnection(func(e LP2PConnectionEvent) {
		log.Printf("%s: got a connection", nickname)
		e.Connection.OnDataChannel(func(e LP2PDataChannelEvent) {
			e.Channel.SendText("Hello from " + nickname)
		})
	})
	return receiver, receiver.Start()
}

// remotePage runs on the phone: it asks for a device and says hi.
func remotePage(w *Window) error {
	request, err := w.NewLP2PRequest(LP2PRequestOptions{Nickname: "Phone"})
	if err != nil {
		return err
	}
	// The page never sees the list of devices: the user agent shows it
	// and only hands over the one the user picks.
	conn, err := request.Start()
	if err != nil {
		return err
	}
	defer conn.Close()

	channel, err := conn.CreateDataChannel("hello", nil)
	if err != nil {
		return err
	}
	greeting := make(chan string, 1)
	channel.OnMessage(func(e OnMessageEvent) { greeting <- e.Payload.(PayloadString).String() })
	log.Printf("Phone: %q", demo.Await(greeting, "a greeting"))
	return nil
}

// pickWhenAllListed simulates a user watching the picker fill up, then
// choosing a device by name. Peers may come and go while it's shown.
func pickWhenAllListed(ctx context.Context, p *ua.Picker) (ua.Peer, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	for peers := range p.Watch(ctx) {
		var labels []string
		for _, peer := range peers {
			labels = append(labels, peer.Label())
		}
		log.Printf("Picker shows: [%s]", strings.Join(labels, ", "))

		listed := 0
		for _, peer := range peers {
			if slices.Contains(devices, peer.DisplayName) {
				listed++
			}
		}
		if listed < len(devices) {
			continue // wait for the others to show up
		}
		for _, peer := range peers {
			if peer.DisplayName == choice {
				return peer, nil
			}
		}
	}
	return ua.Peer{}, ctx.Err()
}

func main() {
	flag.Parse()
	demo.Watchdog(20 * time.Second)

	// One PIN relay per device: the user reads the PIN off the device
	// they picked and types it on the phone.
	relay := ua.NewPINRelay()
	phone := demo.NewBrowser("Phone", &ua.Scripted{Pick: pickWhenAllListed, Enter: relay.Enter, Present: relay.Present})
	defer phone.Close()

	// Discovery runs from the moment the phone's page creates a request,
	// so start the phone first and let the devices appear one by one.
	errc := make(chan error, 1)
	go func() { errc <- remotePage(phone.Window) }()

	for _, name := range devices {
		time.Sleep(150 * time.Millisecond)
		b := demo.NewBrowser(name, &ua.Scripted{Enter: relay.Enter, Present: relay.Present})
		defer b.Close()
		r, err := displayPage(b.Window, name)
		if err != nil {
			log.Fatalf("%s: %v", name, err)
		}
		defer r.Close()
	}

	if err := demo.Await(errc, "the phone"); err != nil {
		log.Fatalf("Phone: %v", err)
	}
}
