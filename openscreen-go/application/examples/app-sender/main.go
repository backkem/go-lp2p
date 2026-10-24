// app-sender is an OpenScreen Application Protocol sender example.
//
// It discovers receivers via mDNS, connects, authenticates with a PSK,
// sends an AgentInfo request, and displays the response. This matches the
// Rust app-sender example for interoperability testing.
//
// Usage:
//
//	go run ./openscreen-go/application/examples/app-sender [flags]
//
// Flags:
//
//	-name string     Display name for this sender (default "Go OpenScreen Sender")
//	-psk string      Pre-shared key for authentication (default "4321")
//	-receiver string Optional: name of receiver to connect to (discovers any if not set)
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/backkem/go-lp2p/openscreen-go/application"
	ospc "github.com/backkem/go-lp2p/openscreen-go/network"
)

func main() {
	name := flag.String("name", "Go OpenScreen Sender", "Display name for this sender")
	psk := flag.String("psk", "4321", "Pre-shared key for authentication")
	receiver := flag.String("receiver", "", "Name of receiver to connect to (optional)")
	timeout := flag.Duration("timeout", 10*time.Second, "Discovery timeout")
	flag.Parse()

	if err := run(*name, *psk, *receiver, *timeout); err != nil {
		log.Fatal(err)
	}
}

func run(name, psk, receiverName string, timeout time.Duration) error {
	// Create local agent
	config := ospc.AgentConfig{
		DisplayName: name,
		// P-256 SPAKE2 suite: the spec's edwards25519 is not implemented (non-spec opt-in).
		InteropQuirks: ospc.InteropQuirks{Spake2SuiteP256: true},
	}
	agent, err := ospc.NewAgent(config)
	if err != nil {
		return fmt.Errorf("failed to create agent: %w", err)
	}

	log.Printf("Sender Name: %s", name)
	log.Printf("Discovering receivers...")

	// Start discovery
	discoverer := ospc.NewDiscoverer()
	if receiverName != "" {
		discoverer.WithNickname(receiverName)
		log.Printf("Looking for receiver: %s", receiverName)
	}

	if err := discoverer.Start(); err != nil {
		return fmt.Errorf("failed to start discovery: %w", err)
	}
	defer discoverer.Close()

	// Wait for a receiver with timeout
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	discovered, err := discoverer.Accept(ctx)
	if err != nil {
		return fmt.Errorf("discovery failed: %w", err)
	}

	// Instance Names are unverified, truncated ones should not be shown.
	if name, status := discovered.DisplayName(); status.Displayable() {
		log.Printf("Found receiver: %s (%s)", name, status)
	} else {
		log.Printf("Found receiver: %s... (%s)", name, status)
	}

	// Dial the receiver
	log.Printf("Connecting...")
	uConn, err := discovered.Dial(ctx, ospc.AgentTransportQUIC, agent)
	if err != nil {
		return fmt.Errorf("dial failed: %w", err)
	}
	defer uConn.Close()

	remoteInfo := uConn.RemoteAgent().Info()
	if remoteInfo != nil {
		log.Printf("Connected to: %s", remoteInfo.DisplayName)
	}

	// Get authentication role
	role := uConn.GetAuthenticationRole()
	log.Printf("Authentication role: %s", role)

	// Authenticate with PSK
	log.Printf("Authenticating with PSK...")
	conn, err := uConn.AuthenticatePSK(ctx, []byte(psk))
	if err != nil {
		return fmt.Errorf("PSK authentication failed: %w", err)
	}
	defer conn.Close()

	log.Printf("Authentication successful!")
	if name, status := conn.RemoteAgent().DisplayName(); status.Verified() {
		log.Printf("Verified receiver name: %s", name)
	} else {
		log.Printf("Receiver name not verified: %s (%s)", name, status)
	}

	// Wrap in application connection
	appConn := application.NewApplicationConnection(conn)

	// Send agent-info-request
	log.Printf("Sending agent-info-request...")
	info, err := appConn.SendAgentInfoRequest(ctx)
	if err != nil {
		return fmt.Errorf("agent-info request failed: %w", err)
	}

	// Display the response
	log.Printf("Received agent-info-response:")
	fmt.Printf("\n")
	fmt.Printf("%-20s %s\n", "Display Name:", info.DisplayName)
	fmt.Printf("%-20s %s\n", "Model Name:", info.ModelName)
	fmt.Printf("%-20s %s\n", "State Token:", info.StateToken)
	if len(info.Capabilities) > 0 {
		fmt.Printf("%-20s %v\n", "Capabilities:", info.Capabilities)
	}
	if len(info.Locales) > 0 {
		fmt.Printf("%-20s %v\n", "Locales:", info.Locales)
	}
	fmt.Printf("\n")

	log.Printf("Done!")
	return nil
}
