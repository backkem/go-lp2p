package main

import (
	"log"

	. "github.com/backkem/go-lp2p/lp2p-api" //lint:ignore ST1001 emulate global API
)

// exampleReceive shows how to make yourself discoverable
func exampleReceive() error {
	// Construct a LP2Receiver to receive peer connections.
	receiver, err := NewLP2Receiver(LP2PReceiverConfig{
		Nickname: "Receiver",
	})
	if err != nil {
		return err
	}

	// Handle incoming connections
	receiver.OnConnection(func(e OnConnectionEvent) {
		conn := e.Connection
		log.Printf("Receiver: got connection\n")

		// Handle incoming data channels
		conn.OnDataChannel(func(e OnDataChannelEvent) { // TODO: This is racy since OnDataChannel is set late
			channel := e.Channel

			// Handle incoming messages
			channel.OnMessage(func(e OnMessageEvent) {
				message := string(e.Payload.(PayloadString).Data)

				log.Printf("Receiver: Received message: %s", message)

				// Signals example end
				notifyPresenter()
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
	request, err := NewLP2PRequest(LP2PRequestConfig{
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
	dc.OnOpen(func(e OnOpenEvent) {
		channel := e.Channel
		log.Printf("Requester: got dataChannel\n")

		// Handle incoming messages
		channel.OnMessage(func(e OnMessageEvent) {
			message := string(e.Payload.(PayloadString).Data)

			log.Printf("Requester: Received message: %s\n", message)

			notifyConsumer() // Signal example end
		})

		// Send our new friend a message.
		channel.SendText("Good day to you, receiver!")
	})

	return nil
}

///
// Below is mostly wiring to setup the example.
///

var notifyPresenter func()
var notifyConsumer func()

func main() {
	// mock user interaction
	DefaultUserAgent.IgnoreConsent = true
	DefaultUserAgent.PSKOverride = []byte("1234")
	DefaultUserAgent.Presenter = func(psk []byte) {
		log.Println("The presenting browser (receiver) shows a pin:")
		log.Printf("Pin: %s (presented to user)\n", string(psk))
	}
	DefaultUserAgent.Consumer = func() ([]byte, error) {
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
