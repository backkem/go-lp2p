// Command lp2p-data-channel chats over an LP2PDataChannel: several messages
// each way, then a clean close.
package main

import (
	"flag"
	"log"
	"time"

	"github.com/backkem/go-lp2p/examples/internal/demo"
	. "github.com/backkem/go-lp2p/lp2p-api" //lint:ignore ST1001 emulate the global web API
	ua "github.com/backkem/go-lp2p/lp2p-api/useragent"
)

var replies = []string{
	"Hi requester, receiver here.",
	"Doing fine, thanks. You?",
	"Bye!",
}

// receiverPage runs on the receiving device: it answers each message with
// the next reply.
func receiverPage(w *Window, done chan<- struct{}) error {
	receiver, err := w.NewLP2PReceiver(LP2PReceiverOptions{Nickname: "Chat receiver"})
	if err != nil {
		return err
	}

	receiver.OnConnection(func(e LP2PConnectionEvent) {
		log.Println("Receiver: got a connection")

		e.Connection.OnDataChannel(func(e LP2PDataChannelEvent) {
			channel := e.Channel
			log.Printf("Receiver: got data channel %q", channel.Label())

			n := 0
			channel.OnMessage(func(e OnMessageEvent) {
				log.Printf("Receiver: got %q", e.Payload.(PayloadString))
				if n < len(replies) {
					channel.SendText(replies[n])
					n++
				}
			})
			channel.OnClose(func(OnCloseEvent) {
				log.Println("Receiver: channel closed")
				close(done)
			})
		})
	})

	// Blocks until the user allowed the page to be discoverable.
	return receiver.Start()
}

// requesterPage runs on the other device: it connects to a peer the user
// picks and starts chatting.
func requesterPage(w *Window, done chan<- struct{}) error {
	request, err := w.NewLP2PRequest(LP2PRequestOptions{Nickname: "Chat requester"})
	if err != nil {
		return err
	}

	// Blocks while the user picks a device and pairs with it.
	conn, err := request.Start()
	if err != nil {
		return err
	}
	log.Println("Requester: got a connection")

	channel, err := conn.CreateDataChannel("chat", &LP2PDataChannelInit{Protocol: "text"})
	if err != nil {
		return err
	}

	messages := []string{"Hello receiver!", "How are you?", "Gotta go."}
	got := 0
	channel.OnOpen(func(OnOpenEvent) {
		log.Println("Requester: channel open")
		for _, m := range messages {
			channel.SendText(m)
		}
	})
	channel.OnMessage(func(e OnMessageEvent) {
		log.Printf("Requester: got %q", e.Payload.(PayloadString))
		if got++; got == len(replies) {
			// Closing is a handshake: onclose fires once the peer closed too.
			channel.Close()
		}
	})
	channel.OnClose(func(OnCloseEvent) {
		log.Println("Requester: channel closed")
		conn.Close()
		close(done)
	})
	return nil
}

func main() {
	flag.Parse()
	demo.Watchdog(20 * time.Second)

	// Two devices, each running a browser; the PIN shown on one is typed
	// on the other.
	presenter, consumer := demo.Pair()
	consumer.Pick = ua.PickByName("Chat receiver") // the user picks it in the device picker
	receiverDevice := demo.NewBrowser("Receiver device", presenter)
	requesterDevice := demo.NewUserBrowser("Requester device", consumer)
	defer receiverDevice.Close()
	defer requesterDevice.Close()

	receiverDone := make(chan struct{})
	requesterDone := make(chan struct{})
	if err := receiverPage(receiverDevice.Window, receiverDone); err != nil {
		log.Fatalf("Receiver: %v", err)
	}
	if err := requesterPage(requesterDevice.Window, requesterDone); err != nil {
		log.Fatalf("Requester: %v", err)
	}
	demo.Await(receiverDone, "the receiver to finish")
	demo.Await(requesterDone, "the requester to finish")
	log.Println("done")
}
