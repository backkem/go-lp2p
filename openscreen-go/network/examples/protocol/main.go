package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"net/http"
	_ "net/http/pprof"

	"github.com/backkem/go-lp2p/openscreen-go/network"
)

func main() {
	err := mainErr()
	if err != nil {
		log.Fatal(err)
	}
}

func mainErr() error {
	go func() {
		log.Println(http.ListenAndServe("localhost:6060", nil))
	}()

	role := ""
	if len(os.Args) > 1 {
		role = os.Args[1]
	}

	if role == "client" {
		// OSP Client or "Listening" agent
		// queries mDNS
		// Dials quick (the Go API uses this nomenclature to match net.Conn)
		return runClient()
	} else {
		fmt.Println("Awaiting connection... Run the client side: `protocol client`")
		// OSP server or "Advertising" agent
		// Advertizes mDNS
		// Listens for quic (the Go API uses this nomenclature to match net.Conn)
		return runServer()
	}
}

func runServer() error {
	c := ospc.AgentConfig{
		DisplayName: "PeerA",
	}
	a, err := ospc.NewAgent(c)
	if err != nil {
		return err
	}

	l, err := ospc.Listen(ospc.AgentTransportQUIC, a)
	if err != nil {
		return err
	}

	for {
		err := serverAccept(l)
		if err != nil {
			log.Println("Error accepting: ", err)
		}
	}
}

func serverAccept(l *ospc.Listener) error {
	uConn, err := l.Accept(context.Background())
	if err != nil {
		return err
	}
	defer uConn.Close() // Cleanup if not authenticated

	log.Printf("Peer A: awaiting authentication\n")
	role, err := uConn.AcceptAuthenticate(context.Background())
	if err != nil {
		return err
	}

	log.Printf("Peer A: Auth role: %s\n", role)

	psk := []byte("0124") // TODO
	conn, err := uConn.AuthenticatePSK(context.Background(), psk)
	if err != nil {
		return err
	}
	defer conn.Close()

	log.Printf("Peer A: Authentication successful!\n")

	dc, err := conn.AcceptDataChannel(context.Background())
	if err != nil {
		return err
	}

	msg, err := dc.ReceiveMessage()
	if err != nil {
		return err
	}

	log.Printf("Peer A: Got message: %s", msg)

	return nil
}

func runClient() error {
	d, err := ospc.Discover()
	if err != nil {
		return err
	}

	discovered, err := d.Accept(context.Background())
	if err != nil {
		return err
	}

	log.Printf("Peer B: Found agent: %s\n", discovered.Nickname())
	c := ospc.AgentConfig{
		DisplayName: "PeerB",
	}
	a, err := ospc.NewAgent(c)
	if err != nil {
		return err
	}
	uConn, err := discovered.Dial(context.Background(), ospc.AgentTransportQUIC, a)
	if err != nil {
		return err
	}
	defer uConn.Close() // Cleanup if not authenticated

	log.Printf("Peer B: connected to %s\n", uConn.RemoteAgent().Info().DisplayName)

	role := uConn.GetAuthenticationRole()

	log.Printf("Peer B: Auth role: %s\n", role)

	psk := []byte("0124")
	conn, err := uConn.AuthenticatePSK(context.Background(), psk)
	if err != nil {
		return err
	}
	defer conn.Close()

	log.Printf("Peer B: Authentication successful!\n")

	dc, err := conn.OpenDataChannel(context.Background(),
		ospc.DataChannelParameters{
			Label: "My Channel",
		})
	if err != nil {
		return err
	}

	msg := "Hello!"
	err = dc.SendMessage([]byte(msg))
	if err != nil {
		return err
	}

	log.Printf("Peer B: sent message: %s\n", msg)

	return nil
}
