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
	"strconv"
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
	if q.Spake2SuiteP256 {
		names = append(names, "Spake2SuiteP256")
	}
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
	// application.bs "Presentation API": controllers advertise
	// control-presentation, receivers receive-presentation. The harness
	// agents act as either.
	conf.Capabilities = []ospc.AgentCapability{ospc.CapabilityReceivePresentation, ospc.CapabilityControlPresentation}
	// The one accepted deviation: SPAKE2 over P-256, as osp_demo with
	// --spake2_suite=p256 (see demoAuthArgs). network.bs requires
	// edwards25519, which neither side implements.
	conf.InteropQuirks.Spake2SuiteP256 = true
	if ease := os.Getenv("OSP_GO_EASE_OF_INPUT"); ease != "" {
		n, err := strconv.Atoi(ease)
		if err != nil {
			t.Fatalf("OSP_GO_EASE_OF_INPUT: %v", err)
		}
		conf.WithPSKConfig(n, 0)
	}
	conf.WithLogger(slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	a, err := ospc.NewAgent(conf)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

// authenticateOrSkip authenticates uc with osp_demo using SPAKE2 with the
// harness PSK (see harnessPSK), which osp_demo gets with --psk. If osp_demo
// didn't take part in the auth-capabilities exchange it hands the connection
// over unauthenticated with the AllowSkipAuthentication quirk instead and
// returns the BLOCKER reason, for the caller to skip the stage with once it
// stored the connection. It fails t if neither works.
func authenticateOrSkip(t *testing.T, uc *ospc.UnauthenticatedConnection, d *ospDemo, quirks *[]string) (*ospc.Connection, string) {
	t.Helper()
	remote := uc.RemoteAgent()
	if remote.HasAuthenticationInfo() && remote.Info() != nil {
		role := uc.GetAuthenticationRole()
		t.Logf("authenticating as PSK %v with PSK %q", role, harnessPSK)
		ctx := withTimeout(t, 3*stepTimeout)
		conn, err := uc.AuthenticatePSK(ctx, []byte(harnessPSK))
		if err != nil {
			for _, line := range d.Grep(`(?i)spake2|auth-|PSK|authenticat`) {
				t.Logf("osp_demo: %s", line)
			}
			t.Fatalf("SPAKE2 authentication with osp_demo failed: %v", err)
		}
		*quirks = append(*quirks, "Spake2SuiteP256")
		t.Logf("authenticated with osp_demo as PSK %v (Go quirks: Spake2SuiteP256; osp_demo flags: %s)", role, strings.Join(demoAuthArgs(), " "))
		return conn, ""
	}

	reason := "(Chromium side) osp_demo did not send auth-capabilities / agent-info-response, so SPAKE2 can't start"
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

// expectMessage waits for an application message of type typeKey and
// decodes it into v. It answers agent-info-request and logs and discards
// anything else.
func expectMessage(ctx context.Context, t *testing.T, app *application.ApplicationConnection, typeKey ospc.TypeKey, v any) error {
	for {
		msg, err := app.ReceiveMessage(ctx)
		if err != nil {
			return err
		}
		if msg.TypeKey == typeKey {
			if err := msg.Decode(v); err != nil {
				return fmt.Errorf("%s: %w", application.MessageName(typeKey), err)
			}
			t.Logf("<-- %s %+v", application.MessageName(typeKey), v)
			return nil
		}
		var body any
		_ = msg.Decode(&body)
		t.Logf("<-- %s %v (not waited for)", application.MessageName(msg.TypeKey), body)
		if msg.TypeKey == ospc.TypeKeyAgentInfoRequest {
			_ = app.AnswerAgentInfoRequest(ctx, msg)
		}
	}
}

// request sends req, whose request-id the caller allocated with
// NextRequestID, and decodes the correlated response into resp.
func request(ctx context.Context, t *testing.T, app *application.ApplicationConnection, reqType ospc.TypeKey, id ospc.RequestID, req any, respType ospc.TypeKey, resp any) error {
	t.Logf("--> %s %+v", application.MessageName(reqType), req)
	if err := app.Request(ctx, reqType, id, req, respType, resp); err != nil {
		return err
	}
	t.Logf("<-- %s %+v", application.MessageName(respType), resp)
	return nil
}

func send(ctx context.Context, t *testing.T, app *application.ApplicationConnection, typeKey ospc.TypeKey, msg any) {
	t.Helper()
	t.Logf("--> %s %+v", application.MessageName(typeKey), msg)
	if err := app.SendMessage(ctx, typeKey, msg); err != nil {
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

	// application.bs: at least 16 ASCII characters.
	presentationID := application.NewPresentationID()
	var connectionID uint64
	availOK, startOK := false, false

	t.Run("04-presentation-url-availability", func(t *testing.T) {
		var quirks []string
		defer report.record(t, &quirks)
		requireStage(t, app != nil, "03-authenticate")
		id := app.NextRequestID()
		var resp application.PresentationURLAvailabilityResponse
		err := request(withTimeout(t, stepTimeout), t, app,
			application.TypeKeyPresentationURLAvailabilityRequest, id, &application.PresentationURLAvailabilityRequest{
				RequestID: id, URLs: []string{testURL}, WatchDuration: 60_000_000, WatchID: 1,
			}, application.TypeKeyPresentationURLAvailabilityResponse, &resp)
		if err != nil {
			skipIfDemoCrashed(t, pub)
			skipIfGated(t, app, pub, err)
			t.Fatal(err)
		}
		if len(resp.URLAvailabilities) != 1 {
			t.Fatalf("url-availabilities = %v, want one entry", resp.URLAvailabilities)
		}
		if v := resp.URLAvailabilities[0]; v != application.URLAvailable {
			t.Fatalf("url-availability = %v, want 0 (available)", v)
		}
		availOK = true
	})

	t.Run("05-presentation-start", func(t *testing.T) {
		var quirks []string
		defer report.record(t, &quirks)
		requireStage(t, availOK, "04-presentation-url-availability")
		id := app.NextRequestID()
		var resp application.PresentationStartResponse
		err := request(withTimeout(t, stepTimeout), t, app,
			application.TypeKeyPresentationStartRequest, id, &application.PresentationStartRequest{
				RequestID: id, PresentationID: presentationID, URL: testURL,
			}, application.TypeKeyPresentationStartResponse, &resp)
		if err != nil {
			t.Fatal(err)
		}
		if resp.Result != application.ResultSuccess {
			t.Fatalf("result = %v, want 1 (success)", resp.Result)
		}
		connectionID = resp.ConnectionID
		startOK = true
	})

	t.Run("06-presentation-connection-message", func(t *testing.T) {
		var quirks []string
		defer report.record(t, &quirks)
		requireStage(t, startOK, "05-presentation-start")
		const text = "hello from go"
		ctx := withTimeout(t, stepTimeout)
		send(ctx, t, app, application.TypeKeyPresentationConnectionMessage, &application.PresentationConnectionMessage{ConnectionID: connectionID, Message: text})
		if l, ok := pub.WaitFor(`got message: `+text, stepTimeout); ok {
			t.Logf("osp_demo: %s", l)
		} else {
			t.Errorf("osp_demo did not log the message")
		}
		var m application.PresentationConnectionMessage
		if err := expectMessage(ctx, t, app, application.TypeKeyPresentationConnectionMessage, &m); err != nil {
			t.Fatal(err)
		}
		if got := fmt.Sprint(m.Message); got != "--echo-- "+text {
			t.Errorf("echo = %q, want %q", got, "--echo-- "+text)
		}
	})

	t.Run("07-presentation-termination", func(t *testing.T) {
		var quirks []string
		defer report.record(t, &quirks)
		requireStage(t, startOK, "05-presentation-start")
		id := app.NextRequestID()
		var resp application.PresentationTerminationResponse
		err := request(withTimeout(t, stepTimeout), t, app,
			application.TypeKeyPresentationTerminationRequest, id, &application.PresentationTerminationRequest{
				RequestID: id, PresentationID: presentationID, Reason: application.TerminationReasonApplicationRequest,
			}, application.TypeKeyPresentationTerminationResponse, &resp)
		if err != nil {
			t.Fatal(err)
		}
		if resp.Result != application.ResultSuccess {
			t.Errorf("result = %v, want 1 (success)", resp.Result)
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
		var req application.PresentationURLAvailabilityRequest
		if err := expectMessage(ctx, t, app, application.TypeKeyPresentationURLAvailabilityRequest, &req); err != nil {
			skipIfDemoCrashed(t, ctrl)
			skipIfGated(t, app, ctrl, err)
			t.Fatalf("no presentation-url-availability-request: %v", err)
		}
		send(ctx, t, app, application.TypeKeyPresentationURLAvailabilityResponse, &application.PresentationURLAvailabilityResponse{
			RequestID: req.RequestID, URLAvailabilities: []application.URLAvailability{application.URLAvailable},
		})
		l, ok := ctrl.WaitFor(`available! |unavailable! |request failed`, stepTimeout)
		if !ok || !strings.Contains(l, "available! "+name) || strings.Contains(l, "unavailable") {
			t.Fatalf("osp_demo did not accept the availability response: %q", l)
		}
		t.Logf("osp_demo: %s", l)
		availOK = true
	})

	var connectionID uint64 = 7
	var presentationID string
	startOK := false
	t.Run("04-presentation-start", func(t *testing.T) {
		var quirks []string
		defer report.record(t, &quirks)
		requireStage(t, availOK, "03-presentation-url-availability")
		ctx := withTimeout(t, stepTimeout)
		ctrl.Send("start " + testURL + " " + name)
		var req application.PresentationStartRequest
		if err := expectMessage(ctx, t, app, application.TypeKeyPresentationStartRequest, &req); err != nil {
			t.Fatal(err)
		}
		if err := application.ValidatePresentationID(req.PresentationID); err != nil {
			send(ctx, t, app, application.TypeKeyPresentationStartResponse, &application.PresentationStartResponse{
				RequestID: req.RequestID, Result: application.ResultInvalidPresentationID,
			})
			t.Fatalf("osp_demo sent an invalid presentation-id: %v", err)
		}
		presentationID = req.PresentationID
		send(ctx, t, app, application.TypeKeyPresentationStartResponse, &application.PresentationStartResponse{
			RequestID: req.RequestID, Result: application.ResultSuccess, ConnectionID: connectionID,
		})
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
		var m application.PresentationConnectionMessage
		if err := expectMessage(ctx, t, app, application.TypeKeyPresentationConnectionMessage, &m); err != nil {
			t.Fatal(err)
		}
		if got := fmt.Sprint(m.Message); got != "hello from cpp" {
			t.Errorf("message = %q", got)
		}
		send(ctx, t, app, application.TypeKeyPresentationConnectionMessage, &application.PresentationConnectionMessage{ConnectionID: connectionID, Message: "hello from go"})
		if l, ok := ctrl.WaitFor(`got message: hello from go`, stepTimeout); ok {
			t.Logf("osp_demo: %s", l)
		} else {
			t.Errorf("osp_demo did not log our message")
		}
	})

	t.Run("06-presentation-termination", func(t *testing.T) {
		var quirks []string
		defer report.record(t, &quirks)
		requireStage(t, startOK, "04-presentation-start")
		ctx := withTimeout(t, stepTimeout)
		ctrl.Send("term")
		var req application.PresentationTerminationRequest
		if err := expectMessage(ctx, t, app, application.TypeKeyPresentationTerminationRequest, &req); err != nil {
			t.Fatal(err)
		}
		if req.PresentationID != presentationID {
			t.Errorf("presentation-id = %q, want %q", req.PresentationID, presentationID)
		}
		send(ctx, t, app, application.TypeKeyPresentationTerminationResponse, &application.PresentationTerminationResponse{
			RequestID: req.RequestID, Result: application.ResultSuccess,
		})
		if l, ok := ctrl.WaitFor(`(?i)terminat`, stepTimeout); ok {
			t.Logf("osp_demo: %s", l)
		}
	})
}
