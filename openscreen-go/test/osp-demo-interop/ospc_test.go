//go:build interop

package ospdemointerop

// Interop stages that use the real ospc Dial/Listen paths and the
// application package.
//
// Each direction starts with no interop quirks and enables a quirk only when
// an attempt fails in the way that quirk addresses (see ospc.InteropQuirks).
// The quirks a stage needed are logged as "QUIRKS NEEDED" and summarized at
// the end of the test, so the output shows what still deviates from the spec.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/backkem/go-lp2p/openscreen-go/application"
	ospc "github.com/backkem/go-lp2p/openscreen-go/network"
)

// maxQuirkAttempts bounds the connection attempts per direction: one
// without quirks plus one per quirk that can be enabled on failure.
const maxQuirkAttempts = 4

// quirkNames lists the enabled quirks.
func quirkNames(q ospc.InteropQuirks) []string {
	var names []string
	if q.AcceptCertificateFingerprint {
		names = append(names, "AcceptCertificateFingerprint")
	}
	if q.AdvertiseCertificateFingerprint {
		names = append(names, "AdvertiseCertificateFingerprint")
	}
	if q.TolerateMissingAgentInfo {
		names = append(names, "TolerateMissingAgentInfo")
	}
	if q.AllowSkipAuthentication {
		names = append(names, "AllowSkipAuthentication")
	}
	return names
}

func quirkString(names []string) string {
	if len(names) == 0 {
		return "none"
	}
	return strings.Join(names, ", ")
}

// stageReport collects per-stage results and the quirks each needed.
type stageReport struct {
	mu   sync.Mutex
	rows []string
}

func newStageReport(t *testing.T) *stageReport {
	r := &stageReport{}
	t.Cleanup(func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		t.Logf("STAGE SUMMARY (%s):\n%s", t.Name(), strings.Join(r.rows, "\n"))
	})
	return r
}

// record adds the result of stage t, with the quirks it needed. Call it
// deferred at the start of the stage.
func (r *stageReport) record(t *testing.T, quirks *[]string) {
	result := "PASS"
	switch {
	case t.Skipped():
		result = "SKIP"
	case t.Failed():
		result = "FAIL"
	}
	name := t.Name()[strings.LastIndex(t.Name(), "/")+1:]
	r.mu.Lock()
	r.rows = append(r.rows, fmt.Sprintf("| %s | %s | %s |", name, result, quirkString(*quirks)))
	r.mu.Unlock()
}

// logBuffer collects ospc logs, so the harness can tell why the listener
// dropped a connection. It is dumped when the test ends.
type logBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *logBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *logBuffer) Contains(s string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return strings.Contains(b.buf.String(), s)
}

func newLogBuffer(t *testing.T, name string) *logBuffer {
	b := &logBuffer{}
	t.Cleanup(func() {
		b.mu.Lock()
		text := b.buf.String()
		b.mu.Unlock()
		if dir := os.Getenv("OSP_DEMO_LOG_DIR"); dir != "" {
			path := dir + "/" + strings.NewReplacer("/", "_", " ", "_").Replace(t.Name()) + "-" + name + ".log"
			if os.WriteFile(path, []byte(text), 0o644) == nil {
				t.Logf("[%s] ospc log: %s", name, path)
				return
			}
		}
		if testing.Verbose() {
			t.Logf("[%s] ospc log:\n%s", name, text)
		}
	})
	return b
}

func newQuirkAgent(t *testing.T, name string, q ospc.InteropQuirks, logs *logBuffer) *ospc.Agent {
	t.Helper()
	conf := ospc.NewAgentConfig(name)
	conf.InteropQuirks = q
	conf.WithLogger(slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	a, err := ospc.NewAgent(conf)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

// spake2Blocker is why real SPAKE2 with osp_demo is not attempted by
// default, see INTEROP.md.
const spake2Blocker = "SPAKE2 parameters differ: ospc (spake2-go) hashes the password with SHA-256 where " +
	"network.bs requires SHA-512 (osp_demo uses SHA-512), uses the presenter's fingerprint as identity A where " +
	"network.bs requires the client's, and starts as PSK consumer with the deprecated auth-spake2-need-psk " +
	"(type 99001) instead of auth-spake2-handshake psk-needs-presentation; both sides use P-256, not the " +
	"spec's edwards25519. Set OSP_DEMO_TRY_SPAKE2=1 to attempt it anyway"

// pskFromDemo returns the PSK to authenticate with osp_demo: $OSP_DEMO_PSK,
// or the PSK osp_demo shows as presenter, as canonical digits.
func pskFromDemo(d *ospDemo, timeout time.Duration) []byte {
	if psk := os.Getenv("OSP_DEMO_PSK"); psk != "" {
		return []byte(psk)
	}
	l, ok := d.WaitFor(`show this PSK to the user: [0-9]`, timeout)
	if !ok {
		return nil
	}
	shown := l[strings.Index(l, "show this PSK to the user: ")+len("show this PSK to the user: "):]
	digits := strings.TrimLeft(regexp.MustCompile(`[^0-9]`).ReplaceAllString(shown, ""), "0")
	return []byte(digits)
}

// authenticateOrSkip authenticates uc with osp_demo if it took part in the
// auth-capabilities exchange and a PSK is available. Otherwise it hands the
// connection over unauthenticated with the AllowSkipAuthentication quirk and
// returns the BLOCKER reason, for the caller to skip the stage with once it
// stored the connection. It fails t if neither works.
func authenticateOrSkip(t *testing.T, uc *ospc.UnauthenticatedConnection, d *ospDemo, quirks *[]string) (*ospc.Connection, string) {
	t.Helper()
	remote := uc.RemoteAgent()
	reason := ""
	switch {
	case !remote.HasAuthenticationInfo() || remote.Info() == nil:
		reason = "(Chromium side) osp_demo did not send auth-capabilities / agent-info-response, so SPAKE2 can't start"
	case os.Getenv("OSP_DEMO_TRY_SPAKE2") == "":
		reason = "(Go side) osp_demo took part in the auth-capabilities exchange, but " + spake2Blocker
	default:
		psk := pskFromDemo(d, stepTimeout)
		if psk == nil {
			reason = "(harness) osp_demo sent auth-capabilities but no PSK is known (set OSP_DEMO_PSK or have osp_demo log it)"
			break
		}
		t.Logf("authenticating as %v with PSK %q", uc.GetAuthenticationRole(), psk)
		ctx := withTimeout(t, 3*stepTimeout)
		if uc.GetAuthenticationRole() == ospc.AuthenticationRoleConsumer {
			if err := uc.RequestAuthenticatePSK(); err != nil {
				t.Fatalf("RequestAuthenticatePSK: %v", err)
			}
		}
		conn, err := uc.AuthenticatePSK(ctx, psk)
		if err != nil {
			t.Fatalf("SPAKE2 authentication with osp_demo failed: %v", err)
		}
		t.Logf("authenticated with osp_demo")
		return conn, ""
	}

	conn, err := uc.SkipAuthentication()
	if err != nil {
		t.Fatalf("SkipAuthentication: %v", err)
	}
	*quirks = append(*quirks, "AllowSkipAuthentication")
	t.Logf("QUIRKS NEEDED: AllowSkipAuthentication")
	return conn, fmt.Sprintf("BLOCKER %s; continuing UNAUTHENTICATED via the AllowSkipAuthentication quirk", reason)
}

// skipIfGated skips t if osp_demo held back our application messages because
// the connection is not authenticated, which is what network.bs requires.
func skipIfGated(t *testing.T, app *application.ApplicationConnection, d *ospDemo, err error) {
	t.Helper()
	if app.Connection().Authenticated() {
		return
	}
	if held := d.Grep(`holding back message`); len(held) > 0 || strings.Contains(fmt.Sprint(err), "deadline") {
		t.Skipf("BLOCKER (authentication): osp_demo holds back application messages from the unauthenticated connection, as network.bs requires (%v). osp_demo: %q", err, held)
	}
}

// expectMessage waits for an application message of type typeKey. It answers
// agent-info-request and logs and discards anything else.
func expectMessage(ctx context.Context, t *testing.T, app *application.ApplicationConnection, typeKey ospc.TypeKey) (map[uint64]any, error) {
	for {
		msg, err := app.ReceiveMessage(ctx)
		if err != nil {
			return nil, err
		}
		var body any
		if err := msg.Decode(&body); err != nil {
			return nil, fmt.Errorf("type %d: %w", msg.TypeKey, err)
		}
		m := normalizeMap(body)
		t.Logf("<-- type=%d body=%v", msg.TypeKey, m)
		if msg.TypeKey == typeKey {
			return m, nil
		}
		if msg.TypeKey == ospc.TypeKeyAgentInfoRequest {
			info := app.LocalAgent().Info()
			_ = app.SendMessage(ctx, ospc.TypeKeyAgentInfoResponse, map[uint64]any{0: m[0], 1: map[uint64]any{
				0: info.DisplayName, 1: info.ModelName, 2: []uint64{3}, 3: app.LocalAgent().StateToken(), 4: []string{"en"},
			}})
		}
	}
}

// request sends a request with a fresh request-id at key 0 and waits for
// the correlated response.
func request(ctx context.Context, t *testing.T, app *application.ApplicationConnection, reqType ospc.TypeKey, body map[uint64]any, respType ospc.TypeKey) (map[uint64]any, error) {
	id := app.NextRequestID()
	body[0] = uint64(id)
	t.Logf("--> type=%d body=%v", reqType, body)
	var resp any
	if err := app.Request(ctx, reqType, id, body, respType, &resp); err != nil {
		return nil, err
	}
	m := normalizeMap(resp)
	t.Logf("<-- type=%d body=%v", respType, m)
	return m, nil
}

func send(ctx context.Context, t *testing.T, app *application.ApplicationConnection, typeKey ospc.TypeKey, body map[uint64]any) {
	t.Helper()
	t.Logf("--> type=%d body=%v", typeKey, body)
	if err := app.SendMessage(ctx, typeKey, body); err != nil {
		t.Fatal(err)
	}
}

// TestOSPCDialPublisher: osp_demo advertises as a presentation receiver,
// ospc discovers it, dials it and acts as a controller.
func TestOSPCDialPublisher(t *testing.T) {
	top := t
	report := newStageReport(t)
	logs := newLogBuffer(t, "ospc")
	name := "GoIopRecv" + randSuffix()
	pub := startPublisher(t, name)
	t.Cleanup(pub.DumpLog)
	if _, ok := pub.WaitFor(`Running publisher demo|publisher started`, 5*time.Second); !ok || !pub.Running() {
		t.Fatalf("osp_demo publisher did not start")
	}

	var discovered *ospc.DiscoveredAgent
	t.Run("01-mdns-discover", func(t *testing.T) {
		var quirks []string
		defer report.record(t, &quirks)
		d := ospc.NewDiscoverer()
		d.WithNickname(name)
		if err := d.Start(); err != nil {
			t.Fatal(err)
		}
		defer d.Close()
		a, err := d.Accept(withTimeout(t, discoveryTimeout))
		if err != nil {
			t.Fatalf("ospc did not discover %q: %v", name, err)
		}
		discovered = a
		t.Logf("discovered %q, TXT %v", a.Nickname(), a.TXT)
		if _, ok := a.MetadataVersion(); !ok {
			t.Logf("SPEC DRIFT (Chromium side): TXT \"mv\" missing (required by network.bs; ospc tolerates it)")
		}
	})

	var uc *ospc.UnauthenticatedConnection
	t.Run("02-dial", func(t *testing.T) {
		var quirks []string
		defer report.record(t, &quirks)
		requireStage(t, discovered != nil, "01-mdns-discover")

		q := ospc.InteropQuirks{AllowSkipAuthentication: true} // Only enables the API, recorded when used.
		var errs []string
		for attempt := 0; attempt < maxQuirkAttempts; attempt++ {
			agent := newQuirkAgent(t, "GoIopController"+randSuffix(), q, logs)
			c, err := discovered.Dial(withTimeout(t, 3*stepTimeout), ospc.AgentTransportQUIC, agent)
			if err == nil {
				uc = c
				top.Cleanup(func() { _ = c.Close() })
				break
			}
			t.Logf("attempt %d with quirks [%s]: %v", attempt+1, quirkString(quirkNames(q)), err)
			errs = append(errs, err.Error())
			skipIfDemoCrashed(t, pub)
			switch {
			case strings.Contains(err.Error(), "fingerprint") && !q.AcceptCertificateFingerprint:
				q.AcceptCertificateFingerprint = true
				quirks = append(quirks, "AcceptCertificateFingerprint")
			case errors.Is(err, ospc.ErrNoAgentInfo) && !q.TolerateMissingAgentInfo:
				q.TolerateMissingAgentInfo = true
				quirks = append(quirks, "TolerateMissingAgentInfo")
			default:
				t.Skipf("BLOCKER: ospc Dial failed even with quirks [%s]: %s", quirkString(quirks), strings.Join(errs, "; "))
			}
		}
		if uc == nil {
			t.Skipf("BLOCKER: ospc Dial failed: %s", strings.Join(errs, "; "))
		}
		t.Logf("QUIRKS NEEDED: %s", quirkString(quirks))
		t.Logf("connected; remote agent info: %+v, auth-capabilities received: %v", uc.RemoteAgent().Info(), uc.RemoteAgent().HasAuthenticationInfo())
	})

	var app *application.ApplicationConnection
	t.Run("03-authenticate", func(t *testing.T) {
		var quirks []string
		defer report.record(t, &quirks)
		requireStage(t, uc != nil, "02-dial")
		conn, blocker := authenticateOrSkip(t, uc, pub, &quirks)
		app = application.NewApplicationConnection(conn)
		top.Cleanup(func() { _ = app.Close() })
		if blocker != "" {
			t.Skip(blocker)
		}
	})

	presentationID := "go-iop-" + randToken(8)
	var connectionID uint64
	availOK, startOK := false, false

	t.Run("04-presentation-url-availability", func(t *testing.T) {
		var quirks []string
		defer report.record(t, &quirks)
		requireStage(t, app != nil, "03-authenticate")
		m, err := request(withTimeout(t, stepTimeout), t, app, tkPresentationURLAvailabilityReq, map[uint64]any{
			1: []string{testURL}, 2: uint64(60_000_000), 3: uint64(1),
		}, tkPresentationURLAvailabilityResp)
		if err != nil {
			skipIfDemoCrashed(t, pub)
			skipIfGated(t, app, pub, err)
			t.Fatal(err)
		}
		avail, _ := m[1].([]any)
		if len(avail) != 1 {
			t.Fatalf("url-availabilities = %v, want one entry", m[1])
		}
		if v, _ := asUint(avail[0]); v != 0 {
			t.Fatalf("url-availability = %v, want 0 (available)", avail[0])
		}
		availOK = true
	})

	t.Run("05-presentation-start", func(t *testing.T) {
		var quirks []string
		defer report.record(t, &quirks)
		requireStage(t, availOK, "04-presentation-url-availability")
		m, err := request(withTimeout(t, stepTimeout), t, app, tkPresentationStartRequest, map[uint64]any{
			1: presentationID, 2: testURL, 3: []any{},
		}, tkPresentationStartResponse)
		if err != nil {
			t.Fatal(err)
		}
		if r, _ := asUint(m[1]); r != 1 {
			t.Fatalf("result = %v, want 1 (success)", m[1])
		}
		connectionID, _ = asUint(m[2])
		startOK = true
	})

	t.Run("06-presentation-connection-message", func(t *testing.T) {
		var quirks []string
		defer report.record(t, &quirks)
		requireStage(t, startOK, "05-presentation-start")
		const text = "hello from go"
		ctx := withTimeout(t, stepTimeout)
		send(ctx, t, app, tkPresentationConnectionMessage, map[uint64]any{0: connectionID, 1: text})
		if l, ok := pub.WaitFor(`got message: `+text, stepTimeout); ok {
			t.Logf("osp_demo: %s", l)
		} else {
			t.Errorf("osp_demo did not log the message")
		}
		m, err := expectMessage(ctx, t, app, tkPresentationConnectionMessage)
		if err != nil {
			t.Fatal(err)
		}
		if got := fmt.Sprint(m[1]); got != "--echo-- "+text {
			t.Errorf("echo = %q, want %q", got, "--echo-- "+text)
		}
	})

	t.Run("07-presentation-termination", func(t *testing.T) {
		var quirks []string
		defer report.record(t, &quirks)
		requireStage(t, startOK, "05-presentation-start")
		m, err := request(withTimeout(t, stepTimeout), t, app, tkPresentationTerminationRequest, map[uint64]any{
			1: presentationID, 2: uint64(1),
		}, tkPresentationTerminationResponse)
		if err != nil {
			t.Fatal(err)
		}
		if r, _ := asUint(m[1]); r != 1 {
			t.Errorf("result = %v, want 1 (success)", m[1])
		}
	})
}

// TestOSPCListenController: ospc listens and advertises as a presentation
// receiver, osp_demo's controller discovers and connects to it.
func TestOSPCListenController(t *testing.T) {
	top := t
	report := newStageReport(t)
	logs := newLogBuffer(t, "ospc")
	ctrl := startController(t)
	t.Cleanup(ctrl.DumpLog)

	q := ospc.InteropQuirks{AllowSkipAuthentication: true} // Only enables the API, recorded when used.
	var connectQuirks []string
	var name string
	var uc *ospc.UnauthenticatedConnection
	found := false
	var blocker string

	// attempt advertises a fresh listener with the current quirks and has
	// osp_demo connect to it. It reports why it failed, or "" on success.
	attempt := func(t *testing.T) (failure string) {
		name = "GoIopOspc" + randSuffix()
		agent := newQuirkAgent(t, name, q, logs)
		l, err := ospc.Listen(ospc.AgentTransportQUIC, agent)
		if err != nil {
			t.Fatal(err)
		}
		accepted := make(chan *ospc.UnauthenticatedConnection, 1)
		go func() {
			c, err := l.Accept(context.Background())
			if err == nil {
				accepted <- c
			}
		}()

		l1, ok := ctrl.WaitFor(`found! `+name, discoveryTimeout)
		if !ok {
			_ = l.Close()
			return "discovery"
		}
		found = true
		t.Logf("osp_demo: %s", l1)

		mark := ctrl.Mark()
		ctrl.Send("connect " + name)
		select {
		case c := <-accepted:
			uc = c
			// Closing the listener would close the accepted connection.
			top.Cleanup(func() { _ = c.Close(); _ = l.Close() })
			return ""
		case <-time.After(3 * stepTimeout):
		}
		_ = l.Close()
		for _, line := range ctrl.GrepSince(mark, `(?i)connect|handshake|fingerprint|certificate|verif|fail|error`) {
			t.Logf("osp_demo: %s", line)
		}
		skipIfDemoCrashed(t, ctrl)
		switch {
		case len(ctrl.GrepSince(mark, `(?i)fingerprint|certificate`)) > 0:
			return "fingerprint"
		case logs.Contains(ospc.ErrNoAgentInfo.Error()):
			return "agent-info"
		}
		return "unknown"
	}

	t.Run("01-connect", func(t *testing.T) {
		defer report.record(t, &connectQuirks)
		for i := 0; i < maxQuirkAttempts && uc == nil; i++ {
			failure := attempt(t)
			t.Logf("attempt %d with quirks [%s]: %s", i+1, quirkString(quirkNames(q)), map[string]string{"": "connected"}[failure]+failure)
			switch {
			case failure == "":
			case failure == "fingerprint" && !q.AdvertiseCertificateFingerprint:
				q.AdvertiseCertificateFingerprint = true
				connectQuirks = append(connectQuirks, "AdvertiseCertificateFingerprint")
			case failure == "agent-info" && !q.TolerateMissingAgentInfo:
				q.TolerateMissingAgentInfo = true
				connectQuirks = append(connectQuirks, "TolerateMissingAgentInfo")
			case failure == "discovery":
				t.Fatalf("osp_demo did not discover %q", name)
			default:
				blocker = failure
				t.Skipf("BLOCKER: osp_demo could not connect to the ospc listener (%s) even with quirks [%s]", failure, quirkString(connectQuirks))
			}
		}
		if uc == nil {
			t.Skipf("BLOCKER: no connection after %d attempts (%s)", maxQuirkAttempts, blocker)
		}
		t.Logf("QUIRKS NEEDED: %s", quirkString(connectQuirks))
		t.Logf("ospc accepted osp_demo; remote agent info: %+v, auth-capabilities received: %v", uc.RemoteAgent().Info(), uc.RemoteAgent().HasAuthenticationInfo())
		_ = found
	})

	var app *application.ApplicationConnection
	t.Run("02-authenticate", func(t *testing.T) {
		var quirks []string
		defer report.record(t, &quirks)
		requireStage(t, uc != nil, "01-connect")
		conn, blocker := authenticateOrSkip(t, uc, ctrl, &quirks)
		app = application.NewApplicationConnection(conn)
		top.Cleanup(func() { _ = app.Close() })
		if blocker != "" {
			t.Skip(blocker)
		}
	})

	availOK := false
	t.Run("03-presentation-url-availability", func(t *testing.T) {
		var quirks []string
		defer report.record(t, &quirks)
		requireStage(t, app != nil, "02-authenticate")
		ctx := withTimeout(t, stepTimeout)
		ctrl.Send("avail " + testURL)
		m, err := expectMessage(ctx, t, app, tkPresentationURLAvailabilityReq)
		if err != nil {
			skipIfDemoCrashed(t, ctrl)
			skipIfGated(t, app, ctrl, err)
			t.Fatalf("no presentation-url-availability-request: %v", err)
		}
		send(ctx, t, app, tkPresentationURLAvailabilityResp, map[uint64]any{0: m[0], 1: []uint64{0}})
		l, ok := ctrl.WaitFor(`available! |unavailable! |request failed`, stepTimeout)
		if !ok || !strings.Contains(l, "available! "+name) || strings.Contains(l, "unavailable") {
			t.Fatalf("osp_demo did not accept the availability response: %q", l)
		}
		t.Logf("osp_demo: %s", l)
		availOK = true
	})

	var connectionID uint64 = 7
	startOK := false
	t.Run("04-presentation-start", func(t *testing.T) {
		var quirks []string
		defer report.record(t, &quirks)
		requireStage(t, availOK, "03-presentation-url-availability")
		ctx := withTimeout(t, stepTimeout)
		ctrl.Send("start " + testURL + " " + name)
		m, err := expectMessage(ctx, t, app, tkPresentationStartRequest)
		if err != nil {
			t.Fatal(err)
		}
		send(ctx, t, app, tkPresentationStartResponse, map[uint64]any{0: m[0], 1: uint64(1), 2: connectionID})
		l, ok := ctrl.WaitFor(`request successful|on request error|presentation connection connected`, stepTimeout)
		if !ok || strings.Contains(l, "error") {
			t.Fatalf("osp_demo did not accept the start response: %q", l)
		}
		t.Logf("osp_demo: %s", l)
		startOK = true
	})

	t.Run("05-presentation-connection-message", func(t *testing.T) {
		var quirks []string
		defer report.record(t, &quirks)
		requireStage(t, startOK, "04-presentation-start")
		ctx := withTimeout(t, stepTimeout)
		ctrl.Send("msg hello from cpp")
		m, err := expectMessage(ctx, t, app, tkPresentationConnectionMessage)
		if err != nil {
			t.Fatal(err)
		}
		if got := fmt.Sprint(m[1]); got != "hello from cpp" {
			t.Errorf("message = %q", got)
		}
		send(ctx, t, app, tkPresentationConnectionMessage, map[uint64]any{0: connectionID, 1: "hello from go"})
		if l, ok := ctrl.WaitFor(`got message: hello from go`, stepTimeout); ok {
			t.Logf("osp_demo: %s", l)
		} else {
			t.Errorf("osp_demo did not log our message")
		}
	})
}
