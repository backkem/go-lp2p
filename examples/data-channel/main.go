package main

import (
	"log"

	"github.com/backkem/go-lp2p"
)

// exampleReceive shows how to make yourself discoverable
// and handle incoming connections.
func exampleReceive() error {

	// Construct a LP2Receiver to receive peer connections.
	receiver, err := lp2p.NewLP2Receiver(lp2p.LP2PReceiverConfig{
		Nickname: "Receiver",
	})
	if err != nil {
		return err
	}

	// Handle incoming connections
	receiver.OnConnection(func(e lp2p.OnConnectionEvent) {
		conn := e.Connection
		log.Printf("Receiver: got connection\n")

		// Handle incoming data channels
		conn.OnDataChannel(func(e lp2p.OnDataChannelEvent) {
			channel := e.Channel

			// Handle incoming messages
			channel.OnMessage(func(e lp2p.OnMessageEvent) {
				message := string(e.Payload.(lp2p.PayloadString).Data)

				log.Printf("Receiver: Received message: %s", message)

				notifyPresenter() // Signals example end
			})

			// Send our new friend a message.
			channel.SendText("Good day to you, requester!")
		})
	})

	// Now that all event-handlers are set up, start receiving!
	return receiver.Start()
}

// exampleConnect shows how to connect to a peer.
func exampleConnect() error {

	// Construct a LP2PRequest to request a connection.
	request, err := lp2p.NewLP2PRequest(lp2p.LP2PRequestConfig{
		Nickname: "Requester",
	})
	if err != nil {
		return err
	}

	// Start the request
	conn, err := request.Start()
	if err != nil {
		return err
	}

	log.Printf("Requester: got connection\n")

	// Create a data channel to send data
	dc, err := conn.CreateDataChannel("My Channel", nil)
	if err != nil {
		return err
	}

	// Wait for the channel to be established
	dc.OnOpen(func(e lp2p.OnOpenEvent) {
		channel := e.Channel
		log.Printf("Requester: got dataChannel\n")

		// Handle incoming messages
		channel.OnMessage(func(e lp2p.OnMessageEvent) {
			message := string(e.Payload.(lp2p.PayloadString).Data)

			log.Printf("Requester: Received message: %s\n", message)

			notifyConsumer() // Signal example end
		})

		// Send our new friend a message.
		channel.SendText("Good day to you, receiver!")
	})

	return nil
}

// Below is mostly wiring to setup the example.
var notifyPresenter func()
var notifyConsumer func()

func main() {
	// mock user interaction
	lp2p.DefaultUserAgent.IgnoreConsent = true
	lp2p.DefaultUserAgent.PSKOverride = []byte("1234")
	lp2p.DefaultUserAgent.Presenter = func(psk []byte) {
		log.Println("The presenting browser (receiver) shows a pin:")
		log.Printf("Pin: %s (presented to user)\n", string(psk))
	}
	lp2p.DefaultUserAgent.Consumer = func() ([]byte, error) {
		log.Println("The consuming browser (requester) asks the user to enter the pin:")
		psk := []byte("1234")
		log.Printf("Pin: %s (entered by user)\n", string(psk))
		return psk, nil
	}

	// Track example end
	donePresenter := make(chan struct{})
	notifyPresenter = func() {
		close(donePresenter)
	}
	doneConsumer := make(chan struct{})
	notifyConsumer = func() {
		close(doneConsumer)
	}

	// Spawn exampleReceive
	go func() {
		err := exampleReceive()
		if err != nil {
			log.Fatalf("Receiver error: %v\n", err)
		}
	}()

	// Spawn exampleReceive
	go func() {
		err := exampleConnect()
		if err != nil {
			log.Fatalf("Connect error: %v\n", err)
		}
	}()

	// Wait for completion
	<-donePresenter
	<-doneConsumer
}
