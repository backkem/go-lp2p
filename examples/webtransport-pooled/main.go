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

	listener, err := NewLP2PQuicTransportListener(receiver, LP2PQuicTransportListenerInit{})
	if err != nil {
		return err
	}

	transportReader := listener.IncomingTransports.GetReader(nil).(ReadableStreamDefaultReader[*LP2PQuicTransport])
	tRes, err := transportReader.Read() // Should be called in a loop
	if err != nil || tRes.Done {
		log.Fatalf("failed to get transport: %v\n", err)
	}
	transport := tRes.Val
	log.Printf("Receiver: got transport\n")

	incoming := transport.IncomingBidirectionalStreams.GetReader(nil).(ReadableStreamDefaultReader[WebTransportBidirectionalStream])
	sRes, err := incoming.Read() // Should be called in a loop
	if err != nil || sRes.Done {
		log.Fatalf("failed to get stream: %v\n", err)
	}
	stream := sRes.Val

	reader := stream.Readable.GetReader(nil).(ReadableStreamDefaultReader[[]byte])
	data, err := reader.Read() // Should be called in a loop
	if err != nil || data.Done {
		log.Fatalf("failed to read: %v\n", err)
	}

	log.Printf("Requester: Received: %s\n", string(data.Val))

	// Signals example end
	notifyPresenter()
	return nil
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

	// Signals example end
	notifyConsumer()
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
