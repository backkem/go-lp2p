package main

import (
	"context"
	"fmt"
	"log"

	"net/http"
	_ "net/http/pprof"

	"github.com/backkem/go-lp2p/ospc"
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

	done := make(chan struct{})
	go func() {
		err := simPeerA()
		if err != nil {
			log.Fatalf("Peer A error: %v\n", err)
		}
		close(done)
	}()

	err := simPeerB()
	if err != nil {
		log.Fatalf("Peer B error: %v\n", err)
	}

	<-done

	return nil
}

func simPeerA() error {
	l, err := ospc.Listen(ospc.AgentConfig{
		Nickname: "PeerA",
	})
	if err != nil {
		return err
	}

	uConn, err := l.Accept(context.Background())
	if err != nil {
		return err
	}
	defer uConn.Close() // Cleanup of not authenticated

	// TODO
	//role, err := uConn.AcceptAuthenticate(context.Background())
	//if err != nil {
	//	return err
	//}

	role := uConn.GetAuthenticationRole()

	fmt.Printf("peer A role: %s\n", role)

	// psk := []byte("0124") // TODO
	// conn, err := uConn.AuthenticatePSK(context.Background(), psk)
	// if err != nil {
	// 	return err
	// }
	// defer conn.Close()
	//
	// dc, err := conn.AcceptDataChannel(context.Background())
	// if err != nil {
	// 	return err
	// }
	//
	// buf := make([]byte, 10)
	// n, err := dc.Read(buf)
	// if err != nil {
	// 	return err
	// }
	//
	// fmt.Printf("Got message: %s", buf[:n])

	return nil
}

func simPeerB() error {
	d, err := ospc.Discover()
	if err != nil {
		return err
	}

	agent, err := d.Accept(context.Background())
	if err != nil {
		return err
	}

	log.Printf("Found agent: %s\n", agent.Nickname())

	uConn, err := agent.Dial(context.Background(),
		ospc.AgentConfig{
			Nickname: "PeerB",
		})
	if err != nil {
		return err
	}
	defer uConn.Close() // Cleanup of not authenticated

	log.Printf("Dialed agent: %s\n", uConn.RemoteConfig().Nickname)

	role := uConn.GetAuthenticationRole()

	fmt.Printf("peer B role: %s\n", role)

	// psk := []byte("0124")
	// conn, err := uConn.AuthenticatePSK(context.Background(), psk)
	// if err != nil {
	// 	return err
	// }
	// defer conn.Close()
	//
	// dc, err := conn.OpenDataChannel(context.Background())
	// if err != nil {
	// 	return err
	// }
	//
	// _, err = dc.Write([]byte("Hello!"))

	return nil
}
