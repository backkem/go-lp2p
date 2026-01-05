// app-receiver is an OpenScreen Application Protocol receiver example.
//
// It advertises itself via mDNS, accepts connections, authenticates with a PSK,
// and responds to AgentInfo requests. This matches the Rust app-receiver example
// for interoperability testing.
//
// Usage:
//
//	go run ./openscreen-go/application/examples/app-receiver [flags]
//
// Flags:
//
//	-name string    Display name for this receiver (default "Go OpenScreen Receiver")
//	-psk string     Pre-shared key for authentication (default "test-psk")
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/backkem/go-lp2p/openscreen-go/application"
	ospc "github.com/backkem/go-lp2p/openscreen-go/network"
)

func main() {
	name := flag.String("name", "Go OpenScreen Receiver", "Display name for this receiver")
	psk := flag.String("psk", "test-psk", "Pre-shared key for authentication")
	flag.Parse()

	if err := run(*name, *psk); err != nil {
		log.Fatal(err)
	}
}

func run(name, psk string) error {
	// Create local agent
	config := ospc.AgentConfig{
		DisplayName: name,
		ModelName:   "Go OpenScreen Test Receiver",
	}
	agent, err := ospc.NewAgent(config)
	if err != nil {
		return fmt.Errorf("failed to create agent: %w", err)
	}

	// Print agent info
	fp, err := agent.CertificateFingerPrint()
	if err != nil {
		return fmt.Errorf("failed to get fingerprint: %w", err)
	}
	log.Printf("Receiver Name: %s", name)
	log.Printf("Fingerprint: %s", fp)

	// Start listening
	listener, err := ospc.Listen(ospc.AgentTransportQUIC, agent)
	if err != nil {
		return fmt.Errorf("failed to start listener: %w", err)
	}
	defer listener.Close()

	log.Printf("Listening for connections...")
	log.Printf("PSK: %s", psk)

	// Handle shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Println("Shutting down...")
		cancel()
		listener.Close()
	}()

	// Accept connections in a loop
	for {
		err := acceptConnection(ctx, listener, psk)
		if err != nil {
			if ctx.Err() != nil {
				return nil // Clean shutdown
			}
			log.Printf("Connection error: %v", err)
		}
	}
}

func acceptConnection(ctx context.Context, listener *ospc.Listener, psk string) error {
	// Accept unauthenticated connection
	uConn, err := listener.Accept(ctx)
	if err != nil {
		return fmt.Errorf("accept failed: %w", err)
	}
	defer uConn.Close()

	remoteInfo := uConn.RemoteAgent().Info()
	remoteName := "unknown"
	if remoteInfo != nil {
		remoteName = remoteInfo.DisplayName
	}
	log.Printf("Incoming connection from: %s", remoteName)

	// Wait for authentication request
	log.Printf("Awaiting authentication...")
	role, err := uConn.AcceptAuthenticate(ctx)
	if err != nil {
		return fmt.Errorf("accept authenticate failed: %w", err)
	}
	log.Printf("Authentication role: %s", role)

	// Authenticate with PSK
	conn, err := uConn.AuthenticatePSK(ctx, []byte(psk))
	if err != nil {
		return fmt.Errorf("PSK authentication failed: %w", err)
	}
	defer conn.Close()

	log.Printf("Authentication successful!")

	// Wrap in application connection
	appConn := application.NewApplicationConnection(conn)

	// Handle the connection
	return handleConnection(ctx, appConn)
}

func handleConnection(ctx context.Context, conn *application.ApplicationConnection) error {
	localAgent := conn.LocalAgent()
	localInfo := localAgent.Info()

	for {
		// Wait for agent-info-request
		log.Printf("Waiting for agent-info-request...")
		req, respond, err := conn.ReceiveAgentInfoRequest(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("failed to receive request: %w", err)
		}

		log.Printf("Received agent-info-request (id=%d)", req.RequestID)

		// Build agent info response
		info := &ospc.MsgAgentInfo{
			DisplayName:  localInfo.DisplayName,
			ModelName:    localInfo.ModelName,
			Capabilities: []ospc.AgentCapability{},
			StateToken:   "ready",
			Locales:      localInfo.Locales,
		}
		if info.Locales == nil {
			info.Locales = []string{}
		}

		// Send response
		if err := respond(info); err != nil {
			return fmt.Errorf("failed to send response: %w", err)
		}
		log.Printf("Sent agent-info-response")
	}
}
