# osp-app

**The [Open Screen Protocol application protocol](https://w3c.github.io/openscreenprotocol/application.html)
(presentation, remote playback, agent-info) in TypeScript, decoupled from OSP's network layer: it
runs over any transport shaped like `RTCDataChannel` or like `WebTransport`, including the LP2P
API's `LP2PDataChannel` and `LP2PQuicTransport`.**

- Zero runtime dependencies. The only dev dependency is `typescript`. Ships ESM (`dist/`, checked in).
- Typed messages for every message in `application_messages.cddl`, with their own CBOR and varint code.
- Wire format checked byte for byte against openscreen-go's encoder.
- Experimental, like the rest of this repo. It has no security of its own: authentication is the transport's job.

## Layers

```
 PresentationController / PresentationReceiver     RemotePlaybackController / RemotePlaybackReceiver
                         \                          /
                          OspSession   request IDs, response matching, agent-info, unknown type keys
                               |
                          OspTransport  framed messages + ordering groups + close info
                         /                          \
     DataChannelTransport                            WebTransportTransport
     RTCDataChannel, LP2PDataChannel                 WebTransport, LP2PQuicTransport
```

| File | What |
|---|---|
| `src/codec/` | CBOR (deterministic map order, lossless float64) and QUIC varints |
| `src/schema.ts`, `src/messages.ts` | The CDDL as schemas. Message types are derived from them; there are type keys, encode/decode and framing |
| `src/transport.ts` | The `OspTransport` interface |
| `src/transports/` | The data channel and WebTransport transports |
| `src/session.ts` | `OspSession` |
| `src/presentation.ts`, `src/remote-playback.ts` | Controller and receiver APIs |

## The transport interface

```ts
interface OspTransport {
  readonly ready: Promise<void>;
  readonly closed: Promise<CloseInfo>;              // { code, reason }
  start(sink: { frame(f: Frame): void; malformed(e: Error): void }): void;
  send(frame: Frame, options?: { group?: string }): Promise<void>;
  endGroup(group: string): Promise<void>;
  close(info?: { code?: number; reason?: string }): void;
}
interface Frame { typeKey: number; body: Uint8Array }  // body: exactly one CBOR item
```

The **ordering group** stands in for application.bs's "to send a group of messages in order,
that group of messages must be sent in one QUIC stream". Messages in the same group arrive in
order. Ungrouped messages, and messages in different groups, may be reordered. The protocol
layers put everything about one presentation in the group `presentation:<id>`, and everything
about one remote playback in `remote-playback:<id>`. So a connection message never overtakes
the start response that announced its connection ID. Availability, agent-info and agent-status
messages are ungrouped.

## The two transports

| | `DataChannelTransport` | `WebTransportTransport` |
|---|---|---|
| Wraps (structurally, no imports) | `binaryType`, `readyState`, `send(ArrayBuffer)`, `close()`, `open`/`message`/`close` events | `ready`, `closed`, `close()`, `createUnidirectionalStream()`, `incomingUnidirectionalStreams` |
| Works with | `RTCDataChannel`, `LP2PDataChannel` | `WebTransport`, `LP2PQuicTransport` |
| One OSP message is | one channel message: type-key varint + CBOR, as an ArrayBuffer | type-key varint + CBOR on a unidirectional stream |
| Ungrouped message | one channel message | its own stream: open, write, close. openscreen-go does the same |
| Ordering group | ignored, because the channel must be reliable and ordered | one stream per group, opened by its first message and closed by `endGroup()` |
| Receiving | each message is one frame | every incoming unidirectional stream is read to its end and split into frames |
| Unknown type key (close with 404) | `close()`: **the code and reason are lost** | `close({closeCode: 404, reason: "unknown type key N"})` |
| Not used | | Bidirectional streams and datagrams. The application protocol only uses unidirectional streams; incoming bidirectional streams are left alone |

`test/conformance.ts` checks at type level that `RTCDataChannel`, `WebTransport` and lp2p-shim's
`LP2PDataChannel` and `LP2PQuicTransport` all fit these shapes.

## Quick start

```ts
import { DataChannelTransport, OspSession, PresentationController, PresentationReceiver, Result, WebTransportTransport } from "osp-app";

// Receiver ("TV"): serve each incoming channel.
const receiver = new PresentationReceiver({ start: async ({ url }) => ({ result: Result.success }) });
receiver.on("connection", (conn) => conn.on("message", (m) => conn.send(`echo ${m}`)));
receiver.attach(new OspSession(new DataChannelTransport(channel), { agentInfo }));

// Controller ("phone").
const controller = new PresentationController(new OspSession(new WebTransportTransport(lp2pQuicTransport)));
const { availabilities } = await controller.checkAvailability(["https://example.com/slides"]);
const conn = await controller.start("https://example.com/slides");
conn.on("message", console.log);
await conn.send("next slide");                  // string = text message, Uint8Array = binary
await conn.close();                             // or: await conn.terminate()
```

Remote playback works the same way: `RemotePlaybackController.start({ sources })` returns a
`RemotePlayback` with `modify(controls)`, `terminate()`, a merged `state`, and `state` and
`terminate` events. `RemotePlaybackReceiver` answers requests and has `updateState()` and
`terminate()`.

## Examples

```sh
npm install
npm run example        # examples/node-inmemory.ts: agent-info, presentation and remote playback over an in-memory channel
npm run serve          # http://localhost:8080/osp-app/examples/browser/
```

The **browser example** is runnable. It puts two "devices" in frames, which connect through
[lp2p-shim](../lp2p-shim/)'s checked-in `dist/lp2p-shim.js` over postMessage. The controller
connects with `new LP2PRequest().start()` + `createDataChannel()` (`?transport=datachannel`) or
with `lp2p.connect()` (`?transport=webtransport`). The receiver uses `LP2PReceiver` or
`lp2p.makeDiscoverable()`. Both run the presentation protocol from `osp-app/dist`. It has been
driven end to end in headless Chromium over both transports.

## Scope

| In | Out |
|---|---|
| All type keys of application.bs, so none is "unknown" | **The streaming protocol**: audio, video and data frames, and the streaming-session and streaming-capabilities messages. They are recognized, reported as `unsupported` and dropped. `remoting` in the remote playback start messages is passed through as opaque CBOR |
| agent-info request, response and event; agent-status request and response (answered automatically) | The OSP network layer: mDNS, QUIC, TLS, SPAKE2 and the auth messages (type keys 1001-1005). The transport provides these, or nothing does |
| The presentation protocol: availability with watches and events, start, reconnect, connection messages, close, change events, termination by either side, and a receiver that serves several controllers | Agent-status keepalive timers and power-saving reconnects |
| Remote playback: availability, start, modify, state events and termination by either side | Rendering. Receivers decide through callbacks (`start`, `terminate`, `modify`, `availabilityOf`) |

Behaviours the spec leaves open, and what this package does:

- **Unknown type key:** the session closes the transport with 404 by default (`onUnknownTypeKey: "ignore"` keeps it open).
- **Malformed messages:** the message is dropped and a `malformed` event fires. On a stream, framing is lost, so the rest of that stream is dropped.
- **Unsupported messages:** messages for an out-of-scope protocol are dropped with an `unsupported` event.
- **Unhandled messages and stray responses:** these raise an `unhandled` event.
- **Request IDs:** the counter is per session and starts at 1. Pass `nextRequestId` to share a counter per remote agent, as openscreen-go does.
- **Closing the transport:** this closes every presentation connection on it with reason `unrecoverable-error-while-sending-or-receiving-message`. The presentation itself keeps running on the receiver.

## Wire format vs. openscreen-go

`test/vectors.ts` holds 18 messages encoded by openscreen-go's `application` and `ospc` structs
(regenerate them with `go run ./osp-app/test/govectors`). This package encodes and decodes all 18
**byte for byte identically**: integer keys in ascending order, empty arrays rather than null,
and optional keys omitted. The differences:

- **`model-name`:** openscreen-go omits it when empty. The CDDL makes it required, but the prose calls it optional. This package accepts it absent and sends it when set.
- **Streams:** openscreen-go sends every message on a new stream, connection messages included, so their order isn't guaranteed. This package keeps each presentation's messages on one stream.
- **Request IDs:** openscreen-go counts per remote agent (by certificate fingerprint) across reconnects. This package counts per session by default.
- **Remote playback:** openscreen-go has none. These messages are checked against hand-encoded CDDL vectors instead.

A live Go to TS run wasn't done because there is no easy path. Go speaks OSP over its own QUIC (and LP2P
WebTransport sessions are pooled inside that QUIC connection after mDNS and SPAKE2). Node has no
QUIC or WebTransport client, and there is no Go-side bridge to a data channel a Node process
could reach.

## Spec findings

Where the application protocol silently assumes OSP's network protocol, and what a mapping onto
a data channel or WebTransport has to define.

1. **Framing and ordering exist only as QUIC streams** (Issue 343). "Message delivery using CBOR" is defined only as type key + CBOR on unidirectional QUIC streams, with ordered groups sharing a stream. Nothing says which messages form a group. The Presentation API needs connection messages in order, and a connection message must not overtake the response that carries its connection ID. A non-QUIC mapping must define the framing (here, one message per channel message), the ordering groups, and a maximum message size. Data channels have one and OSP has no fragmentation.
2. **Errors are QUIC connection-close codes.** An unknown type key must "close the QUIC connection with application error code 404" and the type key in the reason phrase. `RTCDataChannel.close()` carries neither; WebTransport's `close({closeCode, reason})` does. Nothing is said about malformed CBOR, schema violations, recognized but unsupported messages (such as streaming to an agent without the capability), or requests nobody handles. There is no generic error response.
3. **Request IDs have no defined scope.** "An unsigned integer chosen by the requester." The streaming section says IDs "should be treated like request IDs with regards to the state-token as specified in …", but neither spec specifies it. openscreen-go uses one counter per remote agent, keyed by certificate fingerprint, that survives reconnects. A page-level transport has no stable remote agent identity to key it on.
4. **Agent identity, authentication and capabilities come from the network layer.** The application protocol assumes an authenticated peer whose agent-info (capabilities, display name, state-token) was exchanged first. Capabilities decide which messages may be sent. Over LP2P, the user agent authenticated the peer, but the page never sees the peer's OSP agent-info. An in-band agent-info from a page is self-asserted. A mapping must say whether agent-info is exchanged in band and what it is trusted for. LP2P's own capabilities (`data-channels` 1100, `quic-transport` 1200) live in the same agent-info.
5. **Presentations outlive connections, but IDs aren't scoped.** The controller may drop QUIC to save power and reconnect ("the QUIC connection ID may or may not be the same"). Watches, presentations and connection IDs are meant to survive that. Neither spec says whether connection IDs and watch IDs belong to a connection, an agent pair or a receiver. A transport-scoped mapping must decide what a closed channel or session does to presentation connections. This package closes them and keeps the presentation.
6. **Keepalive is QUIC-specific.** agent-status keepalive is defined against QUIC's `idle_timeout`, PING frames and close code 5139 (Issue 343). None of these exist for a data channel or a WebTransport session.
7. **The CDDL and the prose disagree.**
   - The prose calls agent-info's `model-name` optional; the CDDL requires it.
   - `presentation-connection-close-event` requires `connection-count`, but the prose gives a controller no value to send. This package sends the count it knows minus one.
   - The availability events (both protocols) refer to "the watch-id given in the response", but the responses carry no watch ID. It is in the request.
   - `remote-playback-start-response` has no `result`, but the prose says a receiver that won't load "must respond with the appropriate failure result". This package answers, then sends a termination event. The `remote-playback-modify-response` prose omits the `result` that the CDDL has.
   - The prose for `remote-playback-availability-request` lists `headers` and says sources "must not be empty". The CDDL has no headers and allows `[*]`.
   - The prose says `remote-playback-id` should be universally unique and recommends a UUID. The CDDL makes it a `uint`. This package uses a random 48-bit integer.
8. **Over LP2P, OSP gets nested inside OSP.** An `LP2PDataChannel` on the OSP binding already carries its payload as OSP `data-frame` messages (type key 24) on the OSP connection, and an `LP2PQuicTransport` is a WebTransport session pooled on the OSP QUIC connection. Running the application protocol in the page frames OSP inside OSP. That works, but it means a page re-implements what a user agent with OSP already speaks natively, under a self-asserted identity (finding 4).

## Development

```sh
npm install
npm run typecheck      # src, tests, examples, and the transport shape conformance check
npm test               # node:test on Node's type stripping (Node >= 22.18)
npm run build          # dist/, checked in; CI rebuilds and diffs it (npm run check-dist)
```
