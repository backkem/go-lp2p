//go:build interop && unix

// Interop tests against Chromium's Open Screen Library standalone Cast
// binaries (cast_receiver / cast_sender). Run with:
//
//	OPENSCREEN_OUT_DIR=/path/to/openscreen/out/debug go test -tags interop -run Interop -v ./cast/
//
// Tests skip when the binaries are missing. See README.md.
package cast_test

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/backkem/go-lp2p/cast"
)

const (
	// Cast Streaming (mirroring) audio+video app id, as launched by
	// cast_sender (cast/common/public/cast_streaming_app_ids.h).
	mirroringAppID = "0F5096E8"
	webrtcNS       = "urn:x-cast:com.google.cast.webrtc"
	// openscreen's receiver/sender default; only used for messages.
	procTimeout = 60 * time.Second
)

func openscreenBin(t *testing.T, name string) string {
	t.Helper()
	dir := os.Getenv("OPENSCREEN_OUT_DIR")
	if dir == "" {
		dir = "/work/openscreen/out/debug"
	}
	p := filepath.Join(dir, name)
	if st, err := os.Stat(p); err != nil || st.IsDir() || st.Mode()&0o111 == 0 {
		t.Skipf("%s not found (set OPENSCREEN_OUT_DIR to an openscreen build dir with %s built)", p, name)
	}
	return p
}

// proc is a C++ child process whose combined output is captured.
type proc struct {
	name string
	cmd  *exec.Cmd
	mu   sync.Mutex
	out  bytes.Buffer
	done chan struct{}
	// stdin feeds the process (cast_sender's interactive receiver menu).
	stdin io.WriteCloser
}

func (p *proc) Write(b []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.out.Write(b)
}

func (p *proc) Log() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.out.String()
}

func (p *proc) exited() bool {
	select {
	case <-p.done:
		return true
	default:
		return false
	}
}

// waitLog waits until the output contains substr.
func (p *proc) waitLog(substr string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if strings.Contains(p.Log(), substr) {
			return true
		}
		if p.exited() {
			return strings.Contains(p.Log(), substr)
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}

// startProc starts bin in its own process group; the group is terminated
// (SIGTERM, then SIGKILL) at test cleanup and the output logged.
func startProc(t *testing.T, dir, bin string, args ...string) *proc {
	t.Helper()
	p := &proc{name: filepath.Base(bin), done: make(chan struct{})}
	p.cmd = exec.Command(bin, args...)
	p.cmd.Dir = dir
	p.cmd.Stdout = p
	p.cmd.Stderr = p
	p.cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	var err error
	if p.stdin, err = p.cmd.StdinPipe(); err != nil {
		t.Fatal(err)
	}
	if err := p.cmd.Start(); err != nil {
		t.Fatalf("start %s: %v", bin, err)
	}
	t.Logf("started %s %s (pid %d)", p.name, strings.Join(args, " "), p.cmd.Process.Pid)
	go func() {
		_ = p.cmd.Wait()
		close(p.done)
	}()
	hardStop := time.AfterFunc(procTimeout, func() { _ = syscall.Kill(-p.cmd.Process.Pid, syscall.SIGKILL) })
	t.Cleanup(func() {
		hardStop.Stop()
		p.stop()
		t.Logf("---- %s output ----\n%s---- end %s output ----", p.name, p.Log(), p.name)
	})
	return p
}

func (p *proc) stop() {
	if p.exited() {
		return
	}
	_ = syscall.Kill(-p.cmd.Process.Pid, syscall.SIGTERM)
	select {
	case <-p.done:
	case <-time.After(3 * time.Second):
		_ = syscall.Kill(-p.cmd.Process.Pid, syscall.SIGKILL)
		<-p.done
	}
}

// checkWholeFrames fails if the C++ side read a partial CastMessage frame:
// openscreen's CastSocket logs this at ERROR level for every TLS read that
// ends mid-frame (see TestSocketSendsMessageInOneTLSRecord).
func checkWholeFrames(t *testing.T, p *proc) {
	t.Helper()
	if strings.Contains(p.Log(), "InsufficientBuffer") {
		t.Errorf("%s read a partial CastMessage frame (InsufficientBuffer)", p.name)
	}
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// inbox collects JSON messages of one namespace.
type inbox struct {
	t  *testing.T
	ch chan map[string]any
}

func newInbox(t *testing.T, label string) (*inbox, cast.Handler) {
	in := &inbox{t: t, ch: make(chan map[string]any, 64)}
	return in, func(_ *cast.Channel, m *cast.CastMessage) {
		t.Logf("%s <- %s->%s %s: %s", label, m.SourceID, m.DestinationID, m.Namespace, m.Payload())
		var v map[string]any
		if json.Unmarshal([]byte(m.Payload()), &v) != nil {
			return
		}
		v["_source"] = m.SourceID
		select {
		case in.ch <- v:
		default:
		}
	}
}

// wait returns the first message whose "type" (or "responseType", used by
// GET_APP_AVAILABILITY replies) is one of types.
func (in *inbox) wait(timeout time.Duration, types ...string) (map[string]any, error) {
	tm := time.NewTimer(timeout)
	defer tm.Stop()
	for {
		select {
		case v := <-in.ch:
			for _, typ := range types {
				if v["type"] == typ || v["responseType"] == typ {
					return v, nil
				}
			}
		case <-tm.C:
			return nil, fmt.Errorf("timed out waiting for %v", types)
		}
	}
}

// ---------------------------------------------------------------------------
// Go sender -> C++ cast_receiver

type cppReceiver struct {
	proc  *proc
	addr  string
	port  int
	roots *x509.CertPool
}

// startCppReceiver generates developer credentials with `cast_receiver -g`
// (a self-signed "Cast Root CA"; the receiver derives an intermediate,
// device and TLS certificate from it at startup) and runs the receiver on
// the loopback interface.
func startCppReceiver(t *testing.T, discovery bool) *cppReceiver {
	t.Helper()
	bin := openscreenBin(t, "cast_receiver")
	dir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	gen := exec.CommandContext(ctx, bin, "-g")
	gen.Dir = dir
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("cast_receiver -g: %v\n%s", err, out)
	}
	keyPath := filepath.Join(dir, "generated_root_cast_receiver.key")
	crtPath := filepath.Join(dir, "generated_root_cast_receiver.crt")
	pemBytes, err := os.ReadFile(crtPath)
	if err != nil {
		t.Fatalf("read generated root: %v", err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(pemBytes) {
		t.Fatalf("generated root certificate does not parse")
	}

	port := freePort(t)
	args := []string{"-v", "-p", keyPath, "-d", crtPath, "-r", strconv.Itoa(port)}
	if !discovery {
		args = append(args, "-x")
	}
	args = append(args, "lo")
	p := startProc(t, dir, bin, args...)
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	// Wait for the startup line rather than probing the port: a bare TCP
	// probe makes the receiver log a failed SSL_accept.
	if !p.waitLog("CastService is running", 15*time.Second) {
		t.Fatalf("cast_receiver did not start")
	}
	return &cppReceiver{proc: p, addr: addr, port: port, roots: roots}
}

// rawHeartbeat exchanges PING/PONG directly on the socket, before a Channel
// owns the read side.
func rawHeartbeat(sock *cast.Socket) error {
	ping := cast.NewStringMessage(cast.PlatformSenderID, cast.PlatformReceiverID, cast.HeartbeatNamespace, `{"type":"PING"}`)
	if err := sock.Send(ping); err != nil {
		return err
	}
	_ = sock.Conn().SetReadDeadline(time.Now().Add(5 * time.Second))
	defer sock.Conn().SetReadDeadline(time.Time{})
	for {
		m, err := sock.Receive()
		if err != nil {
			return err
		}
		if m.Namespace == cast.HeartbeatNamespace && strings.Contains(m.Payload(), `"PONG"`) {
			return nil
		}
	}
}

func TestInteropGoSenderToCppReceiver(t *testing.T) {
	rcv := startCppReceiver(t, false)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 1. TLS (retrying briefly while the receiver binds its port).
	var sock *cast.Socket
	var err error
	for i := 0; i < 50; i++ {
		if sock, err = cast.Dial(ctx, rcv.addr, nil); err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("step tls: %v", err)
	}
	st, _ := sock.ConnectionState()
	peer := sock.PeerCertificate()
	t.Logf("step tls: ok, %s %s, peer %q valid %s .. %s (%T)", tls.VersionName(st.Version),
		tls.CipherSuiteName(st.CipherSuite), peer.Subject, peer.NotBefore, peer.NotAfter, peer.PublicKey)

	// 2. Heartbeat on the raw socket.
	if err := rawHeartbeat(sock); err != nil {
		t.Fatalf("step heartbeat: %v", err)
	}
	t.Logf("step heartbeat: PING answered with PONG")

	// 3. Channel + device auth with the generated developer root as anchor.
	ch := cast.NewChannel(sock, &cast.Config{
		// Aggressive keep-alive so the liveness check below is meaningful.
		Heartbeat: cast.HeartbeatConfig{Interval: 300 * time.Millisecond, Timeout: 2 * time.Second},
	})
	recvInbox, recvHandler := newInbox(t, "receiver")
	ch.Handle(cast.ReceiverNamespace, recvHandler)
	rtcInbox, rtcHandler := newInbox(t, "webrtc")
	ch.Handle(webrtcNS, rtcHandler)
	ch.Start()
	defer ch.Close()

	if err := cast.Authenticate(ctx, ch, &cast.CertVerifier{Roots: rcv.roots, RequireSHA256: true}); err != nil {
		t.Fatalf("step auth: %v", err)
	}
	t.Logf("step auth: device certificate chain and signature verified against generated root")

	// 4. Virtual connection to the platform receiver.
	platform := cast.VirtualConnection{LocalID: cast.PlatformSenderID, PeerID: cast.PlatformReceiverID}
	if err := ch.Connect(ctx, platform); err != nil {
		t.Fatalf("step connect: %v", err)
	}
	t.Logf("step connect: CONNECTED, protocol version %d", ch.Connections()[platform].ProtocolVersion)

	// 5. Receiver protocol.
	send := func(vc cast.VirtualConnection, ns string, v any) {
		t.Helper()
		b, _ := json.Marshal(v)
		if err := ch.SendString(vc, ns, string(b)); err != nil {
			t.Fatalf("send %s: %v", ns, err)
		}
	}
	send(platform, cast.ReceiverNamespace, map[string]any{"type": "GET_STATUS", "requestId": 1})
	if _, err := recvInbox.wait(5*time.Second, "RECEIVER_STATUS"); err != nil {
		t.Fatalf("step GET_STATUS: %v", err)
	}
	t.Logf("step GET_STATUS: RECEIVER_STATUS received")

	send(platform, cast.ReceiverNamespace, map[string]any{"type": "GET_APP_AVAILABILITY", "requestId": 2, "appId": []string{mirroringAppID, "00000000"}})
	avail, err := recvInbox.wait(5*time.Second, "GET_APP_AVAILABILITY")
	if err != nil {
		t.Fatalf("step GET_APP_AVAILABILITY: %v", err)
	}
	if a, _ := avail["availability"].(map[string]any); a[mirroringAppID] != "APP_AVAILABLE" {
		t.Fatalf("step GET_APP_AVAILABILITY: mirroring app not available: %v", avail)
	}
	t.Logf("step GET_APP_AVAILABILITY: %v", avail["availability"])

	send(platform, cast.ReceiverNamespace, map[string]any{"type": "LAUNCH", "requestId": 3, "appId": mirroringAppID, "language": "en-US", "supportedAppTypes": []string{"WEB"}})
	if _, err := recvInbox.wait(5*time.Second, "LAUNCH_STATUS", "LAUNCH_ERROR"); err != nil {
		t.Fatalf("step LAUNCH: %v", err)
	}
	var transportID, sessionID string
	for transportID == "" {
		status, err := recvInbox.wait(5*time.Second, "RECEIVER_STATUS")
		if err != nil {
			t.Fatalf("step LAUNCH: no RECEIVER_STATUS with the launched app: %v", err)
		}
		s, _ := status["status"].(map[string]any)
		apps, _ := s["applications"].([]any)
		if len(apps) == 0 {
			continue
		}
		app, _ := apps[0].(map[string]any)
		if app["appId"] != mirroringAppID {
			continue
		}
		transportID, _ = app["transportId"].(string)
		sessionID, _ = app["sessionId"].(string)
	}
	t.Logf("step LAUNCH: mirroring app running, sessionId=%s transportId=%s", sessionID, transportID)

	// 6. Connect to the app and negotiate a streaming session (control only).
	appVC := cast.VirtualConnection{LocalID: "sender-interop", PeerID: transportID}
	if err := ch.Connect(ctx, appVC); err != nil {
		t.Fatalf("step app connect: %v", err)
	}
	t.Logf("step app connect: CONNECTED to %s", transportID)

	send(appVC, webrtcNS, map[string]any{"type": "OFFER", "seqNum": 1, "offer": testOffer()})
	answer, err := rtcInbox.wait(5*time.Second, "ANSWER")
	if err != nil {
		t.Fatalf("step OFFER/ANSWER: %v", err)
	}
	if answer["result"] != "ok" {
		t.Fatalf("step OFFER/ANSWER: receiver rejected offer: %v", answer)
	}
	ans, _ := answer["answer"].(map[string]any)
	t.Logf("step OFFER/ANSWER: ANSWER ok, udpPort=%v sendIndexes=%v ssrcs=%v", ans["udpPort"], ans["sendIndexes"], ans["ssrcs"])

	// 7. Stop the app.
	send(platform, cast.ReceiverNamespace, map[string]any{"type": "STOP", "requestId": 4, "sessionId": sessionID})
	if _, err := recvInbox.wait(5*time.Second, "RECEIVER_STATUS"); err != nil {
		t.Fatalf("step STOP: %v", err)
	}
	t.Logf("step STOP: RECEIVER_STATUS received")

	// 8. Keep-alive: our PINGs every 300ms must be answered so the 2s
	// liveness timeout never fires.
	select {
	case <-ch.Done():
		t.Fatalf("step keep-alive: channel closed: %v", ch.Err())
	case <-time.After(3 * time.Second):
	}
	t.Logf("step keep-alive: channel alive after 3s with 2s liveness timeout")
	checkWholeFrames(t, rcv.proc)
}

func testOffer() map[string]any {
	return map[string]any{
		"castMode": "mirroring",
		"supportedStreams": []any{
			map[string]any{
				"index": 0, "type": "video_source", "codecName": "vp8", "rtpProfile": "cast",
				"rtpPayloadType": 100, "ssrc": 19088744, "maxFrameRate": "30000/1001",
				"targetDelay": 400, "timeBase": "1/90000", "maxBitRate": 5000000,
				"aesKey":      "bbf109bf84513b456b13a184453b66ce",
				"aesIvMask":   "edaf9e4536e2b66191f560d9c04b2a69",
				"resolutions": []any{map[string]any{"width": 1280, "height": 720}},
			},
			map[string]any{
				"index": 1, "type": "audio_source", "codecName": "opus", "rtpProfile": "cast",
				"rtpPayloadType": 96, "ssrc": 19088745, "targetDelay": 400, "bitRate": 124000,
				"timeBase": "1/48000", "channels": 2,
				"aesKey":    "51027e4e2347cbcb49d57ef10177aebc",
				"aesIvMask": "7f12a19be62a36c04ae4116caaeff6d1",
			},
		},
	}
}

func TestInteropDiscoveryCppReceiver(t *testing.T) {
	rcv := startCppReceiver(t, true)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	found, err := cast.Browse(ctx)
	if err != nil {
		t.Fatalf("browse: %v", err)
	}
	for r := range found {
		t.Logf("discovered %q id=%s port=%d v4=%v txt=%v", r.FriendlyName, r.UniqueID, r.Port, r.IPv4, r.TXTExtras)
		if r.Port == rcv.port {
			t.Logf("found cast_receiver: id=%s fn=%q md=%q ve=%d ca=%#x st=%d", r.UniqueID, r.FriendlyName, r.ModelName, r.ProtocolVersion, r.Capabilities, r.Status)
			return
		}
	}
	t.Fatalf("cast_receiver on port %d not discovered within 10s", rcv.port)
}

// ---------------------------------------------------------------------------
// C++ cast_sender -> Go receiver

// devicePKI is a developer Cast PKI: root -> intermediate -> device, all
// RSA, shaped like the one cast_receiver generates.
type devicePKI struct {
	rootPEM []byte
	chain   [][]byte
	key     crypto.Signer
}

func newDevicePKI(t *testing.T) *devicePKI {
	t.Helper()
	mk := func(cn string, pub crypto.PublicKey, parent *x509.Certificate, parentKey crypto.Signer, isCA bool) *x509.Certificate {
		sn, _ := rand.Int(rand.Reader, big.NewInt(1<<62))
		tmpl := &x509.Certificate{
			SerialNumber:          sn,
			Subject:               pkix.Name{CommonName: cn},
			NotBefore:             time.Now().Add(-time.Hour),
			NotAfter:              time.Now().Add(72 * time.Hour),
			BasicConstraintsValid: true,
			IsCA:                  isCA,
			KeyUsage:              x509.KeyUsageDigitalSignature,
		}
		if isCA {
			tmpl.KeyUsage |= x509.KeyUsageCertSign
		}
		if parent == nil {
			parent = tmpl
		}
		der, err := x509.CreateCertificate(rand.Reader, tmpl, parent, pub, parentKey)
		if err != nil {
			t.Fatal(err)
		}
		c, err := x509.ParseCertificate(der)
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	keys := make([]*rsa.PrivateKey, 3)
	for i := range keys {
		k, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatal(err)
		}
		keys[i] = k
	}
	root := mk("Cast Root CA", keys[0].Public(), nil, keys[0], true)
	inter := mk("Cast Intermediate", keys[1].Public(), root, keys[0], true)
	dev := mk("go-cast-interop-device", keys[2].Public(), inter, keys[1], false)
	return &devicePKI{
		rootPEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: root.Raw}),
		chain:   [][]byte{dev.Raw, inter.Raw},
		key:     keys[2],
	}
}

// receiverTLS returns a self-signed TLS certificate valid for 2 days (the
// sender rejects receiver TLS certificates living longer than 4 days).
func receiverTLS(t *testing.T) *tls.Config {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: "go-cast-interop-tls"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(47 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, key.Public(), key)
	if err != nil {
		t.Fatal(err)
	}
	return &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}}
}

// cppSenderFiles writes the developer root the sender trusts (-d) and a
// placeholder media file (cast_sender only opens it after negotiation).
func cppSenderFiles(t *testing.T, pki *devicePKI) (dir, rootPath, media string) {
	t.Helper()
	dir = t.TempDir()
	rootPath = filepath.Join(dir, "root.pem")
	if err := os.WriteFile(rootPath, pki.rootPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	media = filepath.Join(dir, "media.webm")
	if err := os.WriteFile(media, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	return dir, rootPath, media
}

func TestInteropCppSenderToGoReceiver(t *testing.T) {
	bin := openscreenBin(t, "cast_sender")
	pki := newDevicePKI(t)
	dir, rootPath, media := cppSenderFiles(t, pki)

	ln, err := cast.Listen("127.0.0.1:0", receiverTLS(t))
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	sender := startProc(t, dir, bin, "-v", "-d", rootPath, net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), media)
	serveCppSender(ctx, t, ln, pki, sender)
}

// serveCppSender accepts one cast_sender connection on ln and plays a
// minimal Cast receiver: device auth, platform connection, LAUNCH of the
// mirroring app, app connection, and the streaming OFFER (declined).
func serveCppSender(ctx context.Context, t *testing.T, ln *cast.Listener, pki *devicePKI, sender *proc) {
	t.Helper()
	sock, err := ln.Accept(ctx)
	if err != nil {
		t.Fatalf("step tls: %v", err)
	}
	st, _ := sock.ConnectionState()
	t.Logf("step tls: accepted %s, %s %s", sock.RemoteAddr(), tls.VersionName(st.Version), tls.CipherSuiteName(st.CipherSuite))

	const (
		transportID = "go-mirroring-app"
		sessionID   = "go-session-1"
	)
	var (
		mu       sync.Mutex
		launched bool
		events   = make(chan string, 64)
	)
	event := func(format string, args ...any) {
		s := fmt.Sprintf(format, args...)
		t.Log(s)
		select {
		case events <- s:
		default:
		}
	}

	ch := cast.NewChannel(sock, &cast.Config{
		OnConnectionOpened: func(vc cast.VirtualConnection, info cast.ConnectionInfo) {
			event("connected %s->%s (version %d, ua %q)", vc.PeerID, vc.LocalID, info.ProtocolVersion, info.UserAgent)
		},
		OnConnectionClosed: func(vc cast.VirtualConnection, reason cast.CloseReason) {
			event("closed %s->%s: %v", vc.PeerID, vc.LocalID, reason)
		},
	})
	auth := (&cast.DeviceAuthResponder{Chain: pki.chain, Key: pki.key}).Handler()
	ch.Handle(cast.AuthNamespace, func(c *cast.Channel, m *cast.CastMessage) {
		event("auth challenge from %s (%d bytes)", m.SourceID, len(m.PayloadBinary))
		auth(c, m)
	})

	receiverStatus := func(requestID any) string {
		status := map[string]any{
			"volume":        map[string]any{"level": 1.0, "muted": false},
			"userEq":        map[string]any{},
			"isActiveInput": true,
		}
		mu.Lock()
		if launched {
			status["applications"] = []any{map[string]any{
				"appId": mirroringAppID, "universalAppId": mirroringAppID, "sessionId": sessionID,
				"transportId": transportID, "displayName": "Go Mirroring", "isIdleScreen": false,
				"namespaces": []any{map[string]any{"name": webrtcNS}},
			}}
		}
		mu.Unlock()
		b, _ := json.Marshal(map[string]any{"type": "RECEIVER_STATUS", "requestId": requestID, "status": status})
		return string(b)
	}
	ch.Handle(cast.ReceiverNamespace, func(c *cast.Channel, m *cast.CastMessage) {
		var req map[string]any
		if json.Unmarshal([]byte(m.Payload()), &req) != nil {
			return
		}
		event("receiver ns: %s", m.Payload())
		reply := func(s string) {
			if err := c.Send(cast.NewStringMessage(m.DestinationID, m.SourceID, cast.ReceiverNamespace, s)); err != nil {
				t.Logf("reply: %v", err)
			}
		}
		switch req["type"] {
		case "GET_STATUS":
			reply(receiverStatus(req["requestId"]))
		case "LAUNCH":
			mu.Lock()
			launched = req["appId"] == mirroringAppID
			mu.Unlock()
			b, _ := json.Marshal(map[string]any{"type": "LAUNCH_STATUS", "launchRequestId": req["requestId"], "status": "USER_ALLOWED"})
			reply(string(b))
			reply(receiverStatus(req["requestId"]))
		case "STOP":
			mu.Lock()
			launched = false
			mu.Unlock()
			reply(receiverStatus(req["requestId"]))
		}
	})
	offers := make(chan string, 4)
	ch.Handle(webrtcNS, func(c *cast.Channel, m *cast.CastMessage) {
		event("webrtc ns from %s: %.300s", m.SourceID, m.Payload())
		var v map[string]any
		if json.Unmarshal([]byte(m.Payload()), &v) == nil && v["type"] == "OFFER" {
			offers <- m.Payload()
		}
	})
	ch.Start()
	defer ch.Close()

	select {
	case offer := <-offers:
		var v struct {
			SeqNum int `json:"seqNum"`
			Offer  struct {
				CastMode string `json:"castMode"`
				Streams  []struct {
					Type      string `json:"type"`
					CodecName string `json:"codecName"`
				} `json:"supportedStreams"`
			} `json:"offer"`
		}
		if err := json.Unmarshal([]byte(offer), &v); err != nil {
			t.Fatalf("step OFFER: bad JSON: %v", err)
		}
		t.Logf("step OFFER: seqNum=%d castMode=%s streams=%+v", v.SeqNum, v.Offer.CastMode, v.Offer.Streams)
		if len(v.Offer.Streams) == 0 {
			t.Fatalf("step OFFER: no streams")
		}
		// Heartbeat probe: receivers may PING senders. openscreen's
		// standalone sender has no tp.heartbeat handler, so this documents
		// whether a Go receiver can rely on PONGs from it.
		if err := ch.Send(cast.NewStringMessage(cast.PlatformReceiverID, cast.PlatformSenderID, cast.HeartbeatNamespace, `{"type":"PING"}`)); err != nil {
			t.Fatalf("step heartbeat: %v", err)
		}
		if sender.waitLog(`"PONG"`, 2*time.Second) {
			t.Logf("step heartbeat: cast_sender answered PING")
		} else {
			t.Logf("step heartbeat: cast_sender did not answer PING within 2s (no heartbeat handler in openscreen's sender; Go receivers must not enable a liveness timeout against it)")
		}
		// Cast Streaming (RTP) is out of scope: decline the offer so the
		// sender stops cleanly.
		b, _ := json.Marshal(map[string]any{"type": "ANSWER", "seqNum": v.SeqNum, "result": "error",
			"error": map[string]any{"code": 1, "description": "go-lp2p interop test: streaming not implemented"}})
		if err := ch.SendString(cast.VirtualConnection{LocalID: transportID, PeerID: senderOf(ch, transportID)}, webrtcNS, string(b)); err != nil {
			t.Logf("send ANSWER: %v", err)
		}
	case <-ch.Done():
		t.Fatalf("channel closed before OFFER: %v", ch.Err())
	case <-sender.done:
		t.Fatalf("cast_sender exited before OFFER")
	case <-ctx.Done():
		t.Fatalf("no OFFER within timeout")
	}
	sender.waitLog("ANSWER", 2*time.Second)
	checkWholeFrames(t, sender)
}

// senderOf returns the peer id connected to local id local.
func senderOf(ch *cast.Channel, local string) string {
	for vc := range ch.Connections() {
		if vc.LocalID == local {
			return vc.PeerID
		}
	}
	return ""
}

// TestInteropCppSenderDiscoversGoReceiver advertises a Go receiver over
// mDNS and runs cast_sender in discovery mode on the loopback interface,
// picking our receiver from its interactive menu.
func TestInteropCppSenderDiscoversGoReceiver(t *testing.T) {
	bin := openscreenBin(t, "cast_sender")
	pki := newDevicePKI(t)
	dir, rootPath, media := cppSenderFiles(t, pki)

	// Advertise publishes every interface address (the sender picked the
	// eth0 address even though it browses on lo), so listen on all.
	ln, err := cast.Listen(":0", receiverTLS(t))
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	uid := make([]byte, 6)
	_, _ = rand.Read(uid)
	info := &cast.ReceiverInfo{
		UniqueID:        fmt.Sprintf("%x", uid),
		ProtocolVersion: 2,
		Capabilities:    cast.CapVideoOutput | cast.CapAudioOutput,
		Status:          cast.ReceiverIdle,
		ModelName:       "go-lp2p",
		FriendlyName:    fmt.Sprintf("Go Interop %x", uid[:2]),
		Port:            ln.Addr().(*net.TCPAddr).Port,
	}
	adv, err := cast.Advertise(info)
	if err != nil {
		t.Fatalf("advertise: %v", err)
	}
	defer adv.Close()
	t.Logf("advertising %q (%s) on port %d", info.FriendlyName, info.InstanceID(), info.Port)

	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	sender := startProc(t, dir, bin, "-v", "-d", rootPath, "lo", media)
	if !sender.waitLog("Discovered: "+info.FriendlyName, 30*time.Second) {
		t.Fatalf("step discovery: cast_sender did not discover %q", info.FriendlyName)
	}
	t.Logf("step discovery: cast_sender discovered %q", info.FriendlyName)
	if !sender.waitLog("Enter choice", 10*time.Second) {
		t.Fatalf("step discovery: no receiver menu")
	}
	choice := -1
	for _, line := range strings.Split(sender.Log(), "\n") {
		var i int
		if n, _ := fmt.Sscanf(line, "[%d]:", &i); n == 1 && strings.Contains(line, info.FriendlyName+" @ ") {
			choice = i
			t.Logf("step discovery: menu entry %q", strings.TrimSpace(line))
		}
	}
	if choice < 0 {
		t.Fatalf("step discovery: %q not in menu", info.FriendlyName)
	}
	if _, err := fmt.Fprintf(sender.stdin, "%d\n", choice); err != nil {
		t.Fatal(err)
	}
	serveCppSender(ctx, t, ln, pki, sender)
}
