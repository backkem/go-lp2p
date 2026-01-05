//go:build interop

// Package integration contains integration tests for OpenScreen Application Protocol.
//
// This file tests interoperability between Go and Rust implementations.
//
// Build with: go test -tags=interop ./openscreen-go/application/test/integration/...
//
// To run with verbose logging:
//
//	go test -tags=interop -v ./openscreen-go/application/test/integration/...
//
// Environment variables:
//
//	RUST_APP_DIR - Path to Rust openscreen-rs directory (default: ../openscreen-rs)
//
// Prerequisites:
//
//	Rust toolchain (cargo) must be installed for interop tests
package integration

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/backkem/go-lp2p/openscreen-go/application"
	ospc "github.com/backkem/go-lp2p/openscreen-go/network"
)

const (
	testPSK          = "test-psk"
	testReceiverName = "Test Receiver"
	testSenderName   = "Test Sender"
	discoveryTimeout = 10 * time.Second
)

var (
	rustAppDir   string
	cargoAvailable bool
)

// TestGoToGo tests Go sender connecting to Go receiver.
func TestGoToGo(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Start receiver
	receiverReady := make(chan struct{})
	receiverDone := make(chan error, 1)
	var receivedRequest bool

	go func() {
		err := runGoReceiver(ctx, testReceiverName, testPSK, receiverReady, &receivedRequest)
		receiverDone <- err
	}()

	// Wait for receiver to be ready
	select {
	case <-receiverReady:
		t.Log("Receiver is ready")
	case err := <-receiverDone:
		t.Fatalf("Receiver failed to start: %v", err)
	case <-ctx.Done():
		t.Fatal("Timeout waiting for receiver to start")
	}

	// Give mDNS time to propagate
	time.Sleep(2 * time.Second)

	// Run sender
	info, err := runGoSender(ctx, testSenderName, testPSK, testReceiverName)
	if err != nil {
		t.Fatalf("Sender failed: %v", err)
	}

	// Verify response
	if info.DisplayName != testReceiverName {
		t.Errorf("Expected display name %q, got %q", testReceiverName, info.DisplayName)
	}

	t.Logf("Received AgentInfo: DisplayName=%s, ModelName=%s, StateToken=%s",
		info.DisplayName, info.ModelName, info.StateToken)

	// Cancel to stop receiver
	cancel()

	// Wait for receiver to finish
	select {
	case err := <-receiverDone:
		if err != nil && err != context.Canceled {
			t.Logf("Receiver finished with error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Log("Receiver didn't finish in time (may be ok)")
	}

	t.Log("Go-to-Go test passed")
}

// TestRustReceiverGoSender tests Go sender connecting to Rust receiver.
func TestRustReceiverGoSender(t *testing.T) {
	if !cargoAvailable {
		t.Skip("cargo not available, skipping Rust interop test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Start Rust receiver
	receiverReady := make(chan struct{})
	receiverDone := make(chan error, 1)

	go func() {
		err := runRustReceiver(ctx, testReceiverName, testPSK, receiverReady, t)
		receiverDone <- err
	}()

	// Wait for receiver to be ready
	select {
	case <-receiverReady:
		t.Log("Rust receiver is ready")
	case err := <-receiverDone:
		t.Fatalf("Rust receiver failed to start: %v", err)
	case <-ctx.Done():
		t.Fatal("Timeout waiting for Rust receiver to start")
	}

	// Give extra time for mDNS to propagate
	time.Sleep(2 * time.Second)

	// Run Go sender
	info, err := runGoSender(ctx, testSenderName, testPSK, "")
	if err != nil {
		cancel()
		<-receiverDone
		t.Fatalf("Go sender failed: %v", err)
	}

	t.Logf("Received AgentInfo from Rust: DisplayName=%s, ModelName=%s, StateToken=%s",
		info.DisplayName, info.ModelName, info.StateToken)

	// Cancel to stop receiver
	cancel()
	<-receiverDone

	t.Log("Rust receiver + Go sender interop test passed")
}

// TestGoReceiverRustSender tests Rust sender connecting to Go receiver.
// Uses mDNS discovery - the Rust sender should discover the Go receiver.
func TestGoReceiverRustSender(t *testing.T) {
	if !cargoAvailable {
		t.Skip("cargo not available, skipping Rust interop test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	// Start Go receiver
	receiverReady := make(chan struct{})
	receiverDone := make(chan error, 1)
	var receivedRequest bool

	go func() {
		err := runGoReceiver(ctx, testReceiverName, testPSK, receiverReady, &receivedRequest)
		receiverDone <- err
	}()

	// Wait for receiver to be ready
	select {
	case <-receiverReady:
		t.Log("Go receiver is ready and advertising via mDNS")
	case err := <-receiverDone:
		t.Fatalf("Go receiver failed to start: %v", err)
	case <-ctx.Done():
		t.Fatal("Timeout waiting for Go receiver to start")
	}

	// Give mDNS time to propagate
	time.Sleep(3 * time.Second)

	// Run Rust sender (will use mDNS discovery)
	err := runRustSender(ctx, testPSK, t)
	if err != nil {
		cancel()
		<-receiverDone
		t.Fatalf("Rust sender failed: %v", err)
	}

	// Cancel to stop receiver
	cancel()
	<-receiverDone

	if !receivedRequest {
		t.Error("Go receiver did not receive AgentInfo request")
	}

	t.Log("Go receiver + Rust sender interop test passed")
}

// runGoReceiver runs a Go receiver until the context is cancelled.
func runGoReceiver(ctx context.Context, name, psk string, ready chan<- struct{}, receivedRequest *bool) error {
	fmt.Printf("[Go Receiver] Starting with name=%s, psk=%s\n", name, psk)

	// Create agent
	config := ospc.AgentConfig{
		DisplayName: name,
		ModelName:   "Go Test Receiver",
	}
	agent, err := ospc.NewAgent(config)
	if err != nil {
		return fmt.Errorf("failed to create agent: %w", err)
	}

	fp, _ := agent.CertificateFingerPrint()
	fmt.Printf("[Go Receiver] Agent created, fingerprint: %s\n", fp)

	// Start listener
	listener, err := ospc.Listen(ospc.AgentTransportQUIC, agent)
	if err != nil {
		return fmt.Errorf("failed to start listener: %w", err)
	}
	defer listener.Close()

	fmt.Printf("[Go Receiver] Listening and advertising via mDNS\n")

	// Signal ready
	close(ready)

	// Accept one connection
	acceptCtx, acceptCancel := context.WithCancel(ctx)
	defer acceptCancel()

	fmt.Printf("[Go Receiver] Waiting for connection...\n")
	uConn, err := listener.Accept(acceptCtx)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("accept failed: %w", err)
	}
	defer uConn.Close()

	fmt.Printf("[Go Receiver] Got connection from %s\n", uConn.RemoteAgent().PeerID)

	// Authenticate
	fmt.Printf("[Go Receiver] Waiting for authentication...\n")
	role, err := uConn.AcceptAuthenticate(acceptCtx)
	if err != nil {
		return fmt.Errorf("accept authenticate failed: %w", err)
	}
	fmt.Printf("[Go Receiver] Auth role: %s\n", role)

	fmt.Printf("[Go Receiver] Authenticating with PSK...\n")
	conn, err := uConn.AuthenticatePSK(acceptCtx, []byte(psk))
	if err != nil {
		return fmt.Errorf("PSK auth failed: %w", err)
	}
	defer conn.Close()

	fmt.Printf("[Go Receiver] Authentication successful!\n")

	// Handle request
	appConn := application.NewApplicationConnection(conn)
	fmt.Printf("[Go Receiver] Waiting for AgentInfoRequest...\n")
	req, respond, err := appConn.ReceiveAgentInfoRequest(acceptCtx)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("receive request failed: %w", err)
	}

	*receivedRequest = true
	fmt.Printf("[Go Receiver] Got AgentInfoRequest (id=%d)\n", req.RequestID)

	info := &ospc.MsgAgentInfo{
		DisplayName:  name,
		ModelName:    "Go Test Receiver",
		Capabilities: []ospc.AgentCapability{},
		StateToken:   "ready",
		Locales:      []string{},
	}

	fmt.Printf("[Go Receiver] Sending AgentInfoResponse...\n")
	if err := respond(info); err != nil {
		return fmt.Errorf("respond failed: %w", err)
	}
	fmt.Printf("[Go Receiver] Response sent!\n")

	// Wait for context to be cancelled
	<-ctx.Done()
	return ctx.Err()
}

// runGoSender runs a Go sender and returns the received AgentInfo.
func runGoSender(ctx context.Context, name, psk, receiverName string) (*ospc.MsgAgentInfo, error) {
	fmt.Printf("[Go Sender] Starting with name=%s, psk=%s, looking for=%s\n", name, psk, receiverName)

	// Create agent
	config := ospc.AgentConfig{
		DisplayName: name,
	}
	agent, err := ospc.NewAgent(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create agent: %w", err)
	}

	fmt.Printf("[Go Sender] Agent created\n")

	// Discover
	discoverer := ospc.NewDiscoverer()
	if receiverName != "" {
		discoverer.WithNickname(receiverName)
	}
	if err := discoverer.Start(); err != nil {
		return nil, fmt.Errorf("failed to start discovery: %w", err)
	}
	defer discoverer.Close()

	fmt.Printf("[Go Sender] Discovering receivers (timeout=%s)...\n", discoveryTimeout)

	discoverCtx, discoverCancel := context.WithTimeout(ctx, discoveryTimeout)
	defer discoverCancel()

	discovered, err := discoverer.Accept(discoverCtx)
	if err != nil {
		return nil, fmt.Errorf("discovery failed: %w", err)
	}

	fmt.Printf("[Go Sender] Found receiver: %s (PeerID: %s)\n", discovered.Nickname(), discovered.PeerID)

	// Connect
	fmt.Printf("[Go Sender] Connecting...\n")
	uConn, err := discovered.Dial(ctx, ospc.AgentTransportQUIC, agent)
	if err != nil {
		return nil, fmt.Errorf("dial failed: %w", err)
	}
	defer uConn.Close()

	fmt.Printf("[Go Sender] Connected! Authenticating with PSK...\n")

	// Authenticate
	conn, err := uConn.AuthenticatePSK(ctx, []byte(psk))
	if err != nil {
		return nil, fmt.Errorf("PSK auth failed: %w", err)
	}
	defer conn.Close()

	fmt.Printf("[Go Sender] Authentication successful!\n")

	// Send request
	appConn := application.NewApplicationConnection(conn)
	fmt.Printf("[Go Sender] Sending AgentInfoRequest...\n")
	info, err := appConn.SendAgentInfoRequest(ctx)
	if err != nil {
		return nil, fmt.Errorf("agent info request failed: %w", err)
	}

	fmt.Printf("[Go Sender] Got AgentInfoResponse: DisplayName=%s, ModelName=%s\n", info.DisplayName, info.ModelName)

	return info, nil
}

// runRustReceiver runs the Rust app-receiver using cargo run.
func runRustReceiver(ctx context.Context, name, psk string, ready chan<- struct{}, t *testing.T) error {
	cmd := exec.CommandContext(ctx, "cargo", "run", "--bin", "app-receiver", "--", "--name", name, "--psk", psk)
	cmd.Dir = rustAppDir

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start Rust receiver: %w", err)
	}

	// Log output and detect when ready
	var wg sync.WaitGroup
	readyClosed := false
	var readyMu sync.Mutex

	logAndDetectReady := func(r io.Reader, prefix string) {
		defer wg.Done()
		scanner := bufio.NewScanner(r)
		for scanner.Scan() {
			line := scanner.Text()
			t.Logf("[Rust %s] %s", prefix, line)

			// Detect ready state (look for listening or published messages)
			readyMu.Lock()
			if !readyClosed && (strings.Contains(line, "Listening") ||
				strings.Contains(line, "Published") ||
				strings.Contains(line, "listening") ||
				strings.Contains(line, "Fingerprint")) {
				close(ready)
				readyClosed = true
			}
			readyMu.Unlock()
		}
	}

	wg.Add(2)
	go logAndDetectReady(stdout, "stdout")
	go logAndDetectReady(stderr, "stderr")

	err = cmd.Wait()
	wg.Wait()

	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

// runRustSender runs the Rust app-sender using cargo run.
func runRustSender(ctx context.Context, psk string, t *testing.T) error {
	cmd := exec.CommandContext(ctx, "cargo", "run", "--bin", "app-sender", "--", "--psk", psk)
	cmd.Dir = rustAppDir

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start Rust sender: %w", err)
	}

	// Log output
	var wg sync.WaitGroup
	logOutput := func(r io.Reader, prefix string) {
		defer wg.Done()
		scanner := bufio.NewScanner(r)
		for scanner.Scan() {
			t.Logf("[Rust %s] %s", prefix, scanner.Text())
		}
	}

	wg.Add(2)
	go logOutput(stdout, "stdout")
	go logOutput(stderr, "stderr")

	err = cmd.Wait()
	wg.Wait()

	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

// getRustAppDir returns the path to the Rust openscreen-rs directory.
func getRustAppDir() string {
	if dir := os.Getenv("RUST_APP_DIR"); dir != "" {
		return dir
	}
	// Default: sibling directory to go-lp2p (../ from repo root)
	// From test/integration, we need to go up 4 levels to get to go-lp2p parent
	return filepath.Join("..", "..", "..", "..", "..", "openscreen-rs")
}

// checkCargoAvailable checks if cargo is available and the Rust project exists.
func checkCargoAvailable() bool {
	// Check if cargo is in PATH
	if _, err := exec.LookPath("cargo"); err != nil {
		return false
	}

	// Check if Rust project directory exists
	if _, err := os.Stat(filepath.Join(rustAppDir, "Cargo.toml")); err != nil {
		return false
	}

	return true
}

// TestMain sets up the test environment.
func TestMain(m *testing.M) {
	// Initialize rust app directory
	rustAppDir = getRustAppDir()

	// Make path absolute for cargo run
	if absPath, err := filepath.Abs(rustAppDir); err == nil {
		rustAppDir = absPath
	}

	fmt.Printf("Rust app directory: %s\n", rustAppDir)

	// Check if cargo is available
	cargoAvailable = checkCargoAvailable()
	if cargoAvailable {
		fmt.Println("cargo available: yes (Rust interop tests enabled)")
	} else {
		fmt.Println("cargo available: no (Rust interop tests will be skipped)")
		fmt.Println("  To enable: install Rust toolchain and ensure openscreen-rs is at ../openscreen-rs")
	}

	os.Exit(m.Run())
}
