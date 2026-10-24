package main

import (
	"context"
	"log"

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

	done := make(chan struct{})
	go func() {
		err := simPeerA()
		if err != nil {
			log.Fatalf("Peer A error: %v\n", err)
		}
		close(done)
	}()

	err := simPeerB(done)
	if err != nil {
		log.Fatalf("Peer B error: %v\n", err)
	}

	<-done

	return nil
}

func simPeerA() error {
	c := ospc.AgentConfig{
		DisplayName: "PeerA",
		// P-256 SPAKE2 suite: the spec's edwards25519 is not implemented (non-spec opt-in).
		InteropQuirks: ospc.InteropQuirks{Spake2SuiteP256: true},
	}
	a, err := ospc.NewAgent(c)
	if err != nil {
		return err
	}

	l, err := ospc.Listen(ospc.AgentTransportQUIC, a)
	if err != nil {
		return err
	}

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

// simPeerB waits for peerADone before closing its connection: closing
// right after sending can discard the message before Peer A reads it.
func simPeerB(peerADone <-chan struct{}) error {
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
		// P-256 SPAKE2 suite: the spec's edwards25519 is not implemented (non-spec opt-in).
		InteropQuirks: ospc.InteropQuirks{Spake2SuiteP256: true},
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

	<-peerADone
	return nil
}
