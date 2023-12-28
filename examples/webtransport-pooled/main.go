package main

import (
	"log"

	"net/http"
	_ "net/http/pprof"

	. "github.com/backkem/go-lp2p/lp2p-api"         //lint:ignore ST1001 emulate global API
	. "github.com/backkem/go-lp2p/streams-api"      //lint:ignore ST1001 emulate global API
	. "github.com/backkem/go-lp2p/webtransport-api" //lint:ignore ST1001 emulate global API
)

// exampleReceive shows how to make yourself discoverable
// and handle incoming transports.
func exampleReceive() error {

	// Construct a LP2Receiver to receive peer connections.
	receiver, err := NewLP2Receiver(LP2PReceiverConfig{
		Nickname: "Receiver",
	})
	if err != nil {
		return err
	}

	// Handle transport
	receiver.OnTransport(func(e OnTransportEvent) {
		t := e.Transport
		log.Printf("Receiver: got transport\n")

		incoming := t.IncomingBidirectionalStreams.GetReader(nil).(ReadableStreamDefaultReader[WebTransportBidirectionalStream])
		res, err := incoming.Read()
		if err != nil {
			log.Fatalf("failed to get stream: %v\n", err)
		}

		reader := res.Val.Readable.GetReader(nil).(ReadableStreamDefaultReader[[]byte])
		for {
			data, err := reader.Read()
			if err != nil {
				log.Fatalf("failed to read: %v\n", err)
			}
			if data.Done {
				break
			}

			log.Printf("Requester: Received: %s\n", string(data.Val))
		}

		notifyPresenter() // Signals example end
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
	t, err := request.NewLP2PQuicTransport(
		LP2PWebTransportOptions{
			AllowPooling: true,
		},
	)
	if err != nil {
		return err
	}

	s, err := t.CreateBidirectionalStream()
	if err != nil {
		log.Fatalf("failed to create stream: %v\n", err)
	}

	writer := s.Writable.GetWriter()

	msg := []byte("Good day to you, receiver!")
	err = writer.Write(msg)
	if err != nil {
		log.Fatalf("failed to write: %v\n", err)
	}

	notifyConsumer() // Signal example end

	return nil
}

///
// Below is mostly wiring to setup the example.
///

var notifyPresenter func()
var notifyConsumer func()

func main() {
	go func() {
		log.Println(http.ListenAndServe("localhost:6060", nil))
	}()

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
