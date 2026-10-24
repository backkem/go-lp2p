# presentation-messages

Sends the Open Screen Presentation Protocol messages
([application.bs "Presentation Protocol"](https://w3c.github.io/openscreenprotocol/application.html#presentation-protocol))
by hand, against Chromium Open Screen's `osp_demo`. This is not a
Presentation API implementation: each mode is the fixed message sequence a
Presentation API implementation would produce for one presentation, step by
step, with the Presentation API section that calls for each message.

| Mode | Go | osp_demo | Messages |
|---|---|---|---|
| `controller` | controlling user agent | receiver (`publisher`) | availability, start, message, close, reconnect, message, terminate |
| `receiver` | receiving user agent | controller (`listener`), its commands typed by the example | the same, sent by osp_demo and answered by Go |

Everything below the application protocol is
[openscreen-go/network](../../openscreen-go/network) (mDNS, QUIC, TLS 1.3,
agent-info, SPAKE2), and the messages are the typed structs in
[openscreen-go/application/presentation.go](../../openscreen-go/application/presentation.go).

## Run

Needs a built `osp_demo` ([interop notes](../../openscreen-go/test/osp-demo-interop/INTEROP.md)):

```sh
go run ./examples/presentation-messages -mode controller -spake2-p256 -osp-demo $OPENSCREEN_OUT_DIR/osp_demo
go run ./examples/presentation-messages -mode receiver   -spake2-p256 -osp-demo $OPENSCREEN_OUT_DIR/osp_demo
```

With `-osp-demo` the example starts osp_demo itself, with the same PSK, and in
receiver mode types osp_demo's commands (`connect`, `avail`, `start`, `msg`,
`close`, `reconnect`, `msg`, `term`). Without it, start osp_demo yourself: in
controller mode pass its name with `-peer`, in receiver mode the example prints
the osp_demo command line and the commands to type. A hand-started osp_demo
needs `osp/impl/quic/certificates/` in its working directory, and it waits
until something reads its log FIFO there (`cat _recv_fifo` or
`cat _cntl_fifo`). Names carry a random suffix per run.

**The one deviation from the spec is `-spake2-p256`.** network.bs authenticates
with SPAKE2 over edwards25519, which neither ospc nor Chromium's openscreen
implements. Both implement the RFC 9382 P-256 suite as an opt-in (ospc
`InteropQuirks.Spake2SuiteP256`, osp_demo `--spake2_suite=p256`). It is off by
default, and without it authentication fails with
`ospc.ErrSpake2SuiteUnsupported`.

The interop test runs both modes:

```sh
OPENSCREEN_OUT_DIR=/work/openscreen/out/debug \
  go test -tags interop -p 1 ./examples/presentation-messages/
```

## Output

`-->` is sent by Go and `<--` is received by Go. `osp_demo |` lines are
osp_demo's log, and `osp_demo $` lines are commands typed into it. quic-go's
warnings go to stderr.

Controller mode:

```
 0.000s == Start osp_demo as receiver "OspDemoReceiver1714"
 0.001s     started [/work/openscreen/out/debug/osp_demo -v --psk=123456 --spake2_suite=p256 OspDemoReceiver1714]
 0.001s == Discover "OspDemoReceiver1714" (network.bs "Discovery with mDNS")
 0.004s    osp_demo | Running publisher demo...
 0.004s    osp_demo | signal handlers setup
 0.006s    osp_demo | publisher started!
 0.972s     found "OspDemoReceiver1714"
 0.972s == Connect and authenticate (network.bs: QUIC, TLS 1.3, agent-info, SPAKE2)
 1.018s     connected to "OspDemoReceiver1714"
 1.018s     SPAKE2 as PSK Presenter, PSK 123456
 1.027s     authenticated
 1.027s == URL availability (Presentation API 6.4.2)
 1.027s --> presentation-url-availability-request {RequestID:2 URLs:[https://example.com/] WatchDuration:60000000 WatchID:1}
 1.028s    osp_demo | got availability request for: https://example.com/
 1.029s <-- presentation-url-availability-response {RequestID:2 URLAvailabilities:[0]}
 1.029s == Start the presentation (Presentation API 6.3.4)
 1.029s --> presentation-start-request {RequestID:3 PresentationID:SD3I7BVM34KZ4M2CT4YG7KENWJ URL:https://example.com/ Headers:[]}
 1.030s    osp_demo | Got an initiation request for: https://example.com/
 1.030s    osp_demo | presentation connection connected
 1.030s <-- presentation-start-response {RequestID:3 Result:1 ConnectionID:0 HTTPResponseCode:<nil>}
 1.030s == Send a message (Presentation API 6.5.2)
 1.030s --> presentation-connection-message {ConnectionID:0 Message:hello from the Go controller}
 1.032s    osp_demo | got message: hello from the Go controller
 1.032s    osp_demo | sending '--echo-- hello from the Go controller' to (SD3I7BVM34KZ4M2CT4YG7KENWJ, 0)
 1.032s <-- presentation-connection-message {ConnectionID:0 Message:--echo-- hello from the Go controller}
 1.032s == Close the connection (Presentation API 6.5.5)
 1.032s --> presentation-connection-close-event {ConnectionID:0 Reason:1 ErrorMessage:<nil> ConnectionCount:0}
 1.032s == Reconnect to the presentation (Presentation API 6.3.5)
 1.032s --> presentation-connection-open-request {RequestID:4 PresentationID:SD3I7BVM34KZ4M2CT4YG7KENWJ URL:https://example.com/}
 1.033s    osp_demo | presentation connection closed by remote
 1.034s    osp_demo | presentation connection connected
 1.034s <-- presentation-connection-open-response {RequestID:4 Result:1 ConnectionID:1 ConnectionCount:0}
 1.034s --> presentation-connection-message {ConnectionID:1 Message:hello again}
 1.035s    osp_demo | got message: hello again
 1.036s    osp_demo | sending '--echo-- hello again' to (SD3I7BVM34KZ4M2CT4YG7KENWJ, 1)
 1.036s <-- presentation-connection-message {ConnectionID:1 Message:--echo-- hello again}
 1.036s == Terminate the presentation (Presentation API 6.5.6)
 1.036s --> presentation-termination-request {RequestID:5 PresentationID:SD3I7BVM34KZ4M2CT4YG7KENWJ Reason:1}
 1.037s    osp_demo | Got termination request for: 1
 1.037s    osp_demo | presentation terminated
 1.037s <-- presentation-termination-response {RequestID:5 Result:1}
 1.040s    osp_demo | publisher stopped!
 1.394s done
```

Receiver mode:

```
 0.000s == Advertise as "GoReceiver2e12" (network.bs "Discovery with mDNS")
 0.001s == Start osp_demo as controller
 0.002s     started [/work/openscreen/out/debug/osp_demo -v --psk=123456 --spake2_suite=p256]
 0.005s    osp_demo | Running listener demo...
 0.005s    osp_demo | signal handlers setup
 0.007s    osp_demo | listener started!
 0.068s    osp_demo | found! GoReceiver2e12
 0.068s    osp_demo $ connect GoReceiver2e12
 0.069s    osp_demo | changed! GoReceiver2e12
 0.176s == Accepted "osp_demo controller", authenticate (network.bs SPAKE2)
 0.176s     SPAKE2 as PSK Presenter, PSK 123456
 0.183s    osp_demo $ avail https://example.com/
 0.183s    osp_demo | Controller succeed to build the underlying connection to: GoReceiver2e12
 0.184s     authenticated
 0.185s <-- presentation-url-availability-request {RequestID:1 URLs:[https://example.com/] WatchDuration:20000000 WatchID:1}
 0.185s --> presentation-url-availability-response {RequestID:1 URLAvailabilities:[0]}
 0.186s    osp_demo | available! GoReceiver2e12
 0.186s    osp_demo $ start https://example.com/ GoReceiver2e12
 0.187s <-- presentation-start-request {RequestID:2 PresentationID:GoReceiver2e12:https://example.com/ URL:https://example.com/ Headers:[]}
 0.187s --> presentation-start-response {RequestID:2 Result:1 ConnectionID:1 HTTPResponseCode:<nil>}
 0.188s    osp_demo | presentation started for https://example.com/
 0.188s    osp_demo | presentation connection connected
 0.188s    osp_demo $ msg hello from osp_demo
 0.189s    osp_demo | request successful
 0.189s    osp_demo | sending 'hello from osp_demo' to (GoReceiver2e12:https://example.com/, 1)
 0.189s <-- presentation-connection-message {ConnectionID:1 Message:hello from osp_demo}
 0.189s --> presentation-connection-message {ConnectionID:1 Message:Go receiver got "hello from osp_demo"}
 0.190s    osp_demo | got message: Go receiver got "hello from osp_demo"
 0.190s    osp_demo $ close
 0.191s <-- presentation-connection-close-event {ConnectionID:1 Reason:1 ErrorMessage:<nil> ConnectionCount:1}
 0.391s    osp_demo $ reconnect
 0.393s <-- presentation-connection-open-request {RequestID:3 PresentationID:GoReceiver2e12:https://example.com/ URL:https://example.com/}
 0.393s --> presentation-connection-open-response {RequestID:3 Result:1 ConnectionID:2 ConnectionCount:1}
 0.394s    osp_demo | presentation connection opened to GoReceiver2e12:https://example.com/
 0.394s    osp_demo | presentation connection connected
 0.394s    osp_demo | request successful
 0.394s    osp_demo $ msg hello again from osp_demo
 0.395s    osp_demo | sending 'hello again from osp_demo' to (GoReceiver2e12:https://example.com/, 2)
 0.395s <-- presentation-connection-message {ConnectionID:2 Message:hello again from osp_demo}
 0.395s --> presentation-connection-message {ConnectionID:2 Message:Go receiver got "hello again from osp_demo"}
 0.396s    osp_demo | got message: Go receiver got "hello again from osp_demo"
 0.396s    osp_demo $ term
 0.397s <-- presentation-termination-request {RequestID:4 PresentationID:GoReceiver2e12:https://example.com/ Reason:2}
 0.397s --> presentation-termination-response {RequestID:4 Result:1}
 0.397s     presentation GoReceiver2e12:https://example.com/ (https://example.com/) terminated
 0.595s done
```

## What osp_demo does differently

These are observed in the traces above. None of them breaks the exchange.

- **Termination reason.** osp_demo's controller always sends
  `reason: 2` (user-request), even for `Connection::Terminate(...,
  kApplicationTerminated)` (`presentation_controller.cc`, `TerminatePresentation`).
  application.bs maps Presentation API 6.5.6 `terminate()` to
  application-request (1).
- **connection-count.** osp_demo's receiver leaves `connection-count` out of
  its `presentation-connection-open-response`, so it decodes as 0. It should
  be the new number of open connections (here 1).
- **presentation-id.** osp_demo's controller uses
  `<receiver instance name>:<url>`, a placeholder per its own TODO. That is
  valid for application.bs (at least 16 ASCII characters) but not random. The
  Go controller uses `application.NewPresentationID`, 26 random base32
  characters.

## Spec gaps

- **Controller's close event.** `presentation-connection-close-event` requires
  `connection-count` in the CDDL, but application.bs defines it only for the
  receiver's event. The Go controller sends 0, and osp_demo's controller sends
  its own count.
- **Events to other controllers.** With one controller there are no
  `presentation-change-event`s or `presentation-termination-event`s to send,
  so the example sends none (see the comments in `receiver.go`).
