package cast_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"log"
	"math/big"
	"time"

	"github.com/backkem/go-lp2p/cast"
)

// selfSignedTLS returns a short-lived self-signed certificate like the ones
// Cast receivers present.
func selfSignedTLS() *tls.Config {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		log.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "receiver"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(47 * time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, key.Public(), key)
	if err != nil {
		log.Fatal(err)
	}
	return &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}}
}

// A sender and a receiver talking over loopback: the sender opens a virtual
// connection to receiver-0 and exchanges messages on an app namespace.
func Example() {
	const ns = "urn:x-cast:com.example.echo"
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Receiver: listen, accept one sender, echo everything on ns.
	ln, err := cast.Listen("127.0.0.1:0", selfSignedTLS())
	if err != nil {
		log.Fatal(err)
	}
	defer ln.Close()
	go func() {
		sock, err := ln.Accept(ctx)
		if err != nil {
			return
		}
		ch := cast.NewChannel(sock, nil)
		ch.Handle(ns, func(ch *cast.Channel, m *cast.CastMessage) {
			_ = ch.Send(cast.NewStringMessage(m.DestinationID, m.SourceID, ns, "echo: "+m.Payload()))
		})
		ch.Start()
		<-ctx.Done()
		ch.Close()
	}()

	// Sender: dial, connect, send, wait for the echo.
	sock, err := cast.Dial(ctx, ln.Addr().String(), nil)
	if err != nil {
		log.Fatal(err)
	}
	ch := cast.NewChannel(sock, &cast.Config{Heartbeat: cast.DefaultHeartbeat})
	replies := make(chan string, 1)
	ch.Handle(ns, func(_ *cast.Channel, m *cast.CastMessage) { replies <- m.Payload() })
	ch.Start()
	defer ch.Close()

	vc := cast.VirtualConnection{LocalID: cast.PlatformSenderID, PeerID: cast.PlatformReceiverID}
	if err := ch.Connect(ctx, vc); err != nil {
		log.Fatal(err)
	}
	if err := ch.SendString(vc, ns, "hello"); err != nil {
		log.Fatal(err)
	}
	fmt.Println(<-replies)
	// Output: echo: hello
}
