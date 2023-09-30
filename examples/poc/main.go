package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/backkem/go-lp2p"
)

func main() {
	err := mainErr()
	if err != nil {
		log.Fatal(err)
	}
}

func mainErr() error {
	go func() {
		fmt.Println(http.ListenAndServe("localhost:6060", nil))
	}()

	go func() {
		err := simPeerA()
		if err != nil {
			log.Fatalf("Peer A error: %v\n", err)
		}
	}()

	err := simPeerB()
	if err != nil {
		log.Fatalf("Peer B error: %v\n", err)
	}

	return nil
}

func simPeerA() error {
	receiver, err := lp2p.NewLP2Receiver(lp2p.LP2PReceiverConfig{
		Nickname: "Peer A",
	})
	if err != nil {
		return fmt.Errorf("failed to create connection receiver: %v", err)
	}

	receiver.OnConnection(func(e lp2p.OnConnectionEvent) {
		conn := e.Connection
		conn.OnDataChannel(func(e lp2p.OnDataChannelEvent) {
			channel := e.Channel

			channel.SendText("Good day to you, Peer B!")

			channel.OnMessage(func(e lp2p.OnMessageEvent) {
				payload := e.Payload.(lp2p.PayloadString)
				message := string(payload.Data)
				fmt.Printf("Peer A: Received message: %s", message)
			})
		})

	})

	err = receiver.Start()
	if err != nil {
		return fmt.Errorf("failed to start connection receiver: %v", err)
	}

	return nil
}

func simPeerB() error {
	request, err := lp2p.NewLP2PRequest(lp2p.LP2PRequestConfig{
		Nickname: "Peer B",
	})
	if err != nil {
		return fmt.Errorf("failed to create connection request: %v", err)
	}

	conn, err := request.Start()
	if err != nil {
		return fmt.Errorf("failed to start connection request: %v", err)
	}

	conn.OnDataChannel(func(e lp2p.OnDataChannelEvent) {
		channel := e.Channel

		channel.SendText("Good day to you, Peer A!")

		channel.OnMessage(func(e lp2p.OnMessageEvent) {
			payload := e.Payload.(lp2p.PayloadString)
			message := string(payload.Data)
			fmt.Printf("Peer B: Received message: %s", message)
		})
	})

	return nil
}
