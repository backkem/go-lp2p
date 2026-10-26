# lp2p-shim

**An experimental polyfill for the [Local Peer-to-Peer API](https://github.com/WICG/local-peer-to-peer):
`LP2PReceiver`, `LP2PRequest`, `LP2PConnection`, `LP2PDataChannel`, `LP2PQuicTransport`,
`LP2PQuicTransportListener` and their events, working in today's browsers over pluggable transports.**

> [!WARNING]
> The shim offers **no security**. Pairing is simulated (a toy PIN check, not SPAKE2), nothing is
> authenticated, the "browser UI" is drawn inside the page where the page can fake or click it, and
> data is only as private as the transport underneath. Use it to prototype and to demo the API, never
> to protect anything.

The point is to let web developers try the API in a real page, such as a local chat, a multiplayer
game or a file drop, and to find out where the spec is incomplete. A page cannot do what the
native API does: discover devices on the LAN with mDNS and connect to them over authenticated QUIC.
So the shim fakes the parts a page can't do and keeps the rest close to the real thing. The page
calls the WebIDL API, the protocol layer speaks the Open Screen Protocol and LP2P message shapes,
and only the transport at the bottom is a stand-in.

## Try it

```sh
cd lp2p-shim
npm run serve        # http://localhost:8080/demo/
```

- **[Two frames, one page](demo/iframes.html)**: two "devices" side by side using the postMessage
  transport, with the permission prompt, the device picker and PIN pairing.
- **[WebRTC chat](demo/chat.html?transport=webrtc)**: open it in two tabs. Signaling goes over
  BroadcastChannel and data over RTCDataChannel.
- **[WebTransport in two frames](demo/transport-iframes.html)**: `LP2PQuicTransport` and
  `LP2PQuicTransportListener`: a stream echo, datagram pings and a 1 MiB upload with a hash check.
- **[WebTransport over WebRTC](demo/transport.html?transport=webrtc)**: the same between two tabs. Datagrams
  go over an unreliable, unordered RTCDataChannel.
- **[WebRTC between two devices](demo/chat.html?transport=manual)**: open it on two devices on the same LAN
  and copy three codes across by hand. No server is involved. It needs HTTPS (or `localhost`) on
  both devices, since the API is `[SecureContext]`.

Using it from a page:

```html
<script src="lp2p-shim/dist/lp2p-shim.js"></script>
<script>
  // Optional: pick a transport and UI before calling start().
  LP2PShim.configure({ transport: new LP2PShim.WebRTCTransport() });

  const receiver = new LP2PReceiver({ nickname: "Living room TV" });
  receiver.onconnection = (e) => {
    e.connection.ondatachannel = (e) => (e.channel.onmessage = (m) => console.log(m.data));
  };
  await receiver.start();
</script>
```

The WebTransport side works the same way (`LP2PQuicTransport` has WebTransport's API):

```js
const transport = new LP2PQuicTransport(new LP2PRequest({ nickname: "Phone" }));
await transport.ready;
const { readable, writable } = await transport.createBidirectionalStream();
```

Other code is the [spec's examples](https://wicg.github.io/local-peer-to-peer/) unchanged.

## Files

| Path | What |
|---|---|
| `dist/lp2p-shim.js` | Standalone script (IIFE). It defines the eight LP2P interfaces on `globalThis` where they are missing (in secure contexts only) and exposes the rest as `LP2PShim`. |
| `dist/lp2p-shim.min.js` | The same, minified. |
| `dist/lp2p-shim.esm.js` | ES module. It exports everything and installs nothing; call `install()` to install the globals. |
| `dist/types/` | `.d.ts` files for the ES module. |
| `src/` | TypeScript sources (strict, ES2022, no runtime dependencies). |
| `demo/` | Static demo pages. They use only `dist/lp2p-shim.js`. |
| `test/` | Unit tests (`node:test`, run on Node's built-in type stripping), browser tests (`test/browser/run.mjs`, puppeteer-core with a system Chromium) and an IDL conformance check. |

`dist/` is checked in. After changing `src/`, run `npm run build` and commit the result; CI
(`.github/workflows/lp2p-shim.yml`) rebuilds and fails if the output differs.

```sh
npm ci
npm run typecheck     # sources, tests, and conformance with ../src/lp2p.d.ts (the WebIDL typings)
npm test              # unit + end-to-end API tests over the postMessage transport (Node ≥ 22.18)
npm run test:browser  # demos in headless Chromium: iframes + PIN, WebRTC tabs, WebRTC manual signaling
npm run build         # regenerate dist/
```

## Layers

```
 page script ── LP2PReceiver · LP2PRequest · LP2PConnection · LP2PDataChannel   src/api
                LP2PQuicTransport · LP2PQuicTransportListener (WHATWG streams)
                      │
 user agent  ── consent prompts, device picker, PIN pairing (UserAgentUI hooks)  src/ua
                      │
 session     ── OSP/LP2P messages: agent-info, auth-*, 1101/1102, data-frame,    src/protocol, src/codec
                pooled WebTransport 1201-1205
                      │
 transport   ── discovery, connections, reliable ordered message streams,        src/transport
                datagrams
               ├─ PostMessageTransport  (BroadcastChannel / MessageChannel; same browser)
               ├─ WebRTCTransport       (RTCPeerConnection; signaling via a Signaler)
               └─ (future) a bridge to a native go-lp2p agent, e.g. WebSocket → OSP over QUIC
```

These mirror go-lp2p: `src/api` matches [`lp2p-api`](../lp2p-api), `src/ua` matches
[`lp2p-api/useragent`](../lp2p-api/useragent) and `src/protocol` matches [`openscreen-go/network`](../openscreen-go/network).

### The transport seam

A transport knows nothing about LP2P. It moves opaque messages. This is the whole contract
(`src/transport/types.ts`):

```ts
interface Transport {
  readonly name: string;
  listen(ad: { id: string; nickname: string }, onconnection: (c: TransportConnection) => void): Closer;
  discover(onevent: (e: { kind: "added" | "updated" | "removed"; peer: { id: string; nickname: string } }) => void): Closer;
  connect(peerId: string, local: { id: string; nickname: string }, options?: { signal?: AbortSignal }): Promise<TransportConnection>;
}
interface TransportConnection {
  readonly remoteId: string;
  readonly closed: boolean;
  openStream(): TransportStream;                  // usable at once; early sends are queued
  onstream: ((s: TransportStream) => void) | null;   // held until set
  onclose: ((reason: string) => void) | null;
  close(reason?: string): void;
  readonly maxDatagramSize: number;               // 0: no datagrams
  readonly datagramsReliable: boolean;            // true if they cannot actually be lost
  sendDatagram(d: Uint8Array): void;              // may be lost, duplicated or reordered
  ondatagram: ((d: Uint8Array) => void) | null;
}
interface TransportStream {                       // reliable, ordered, message-preserving, bidirectional
  readonly id: number;
  readonly closed: boolean;
  send(message: Uint8Array): void;
  onmessage: ((message: Uint8Array) => void) | null; // held until set
  onclose: (() => void) | null;                  // after all earlier messages were delivered
  close(): void;
}
interface Closer { close(): void }
```

The transport does no policy: it accepts every incoming connection and leaves consent to the user
agent. The model is OSP's own. A QUIC connection exists before the agents authenticate, and each
stream is a QUIC stream. Streams carry *messages*, not bytes. Both postMessage and RTCDataChannel
are message-oriented, and each message is one OSP message (a varint type key plus CBOR). A byte-stream
transport can concatenate them unchanged, because both parts are self-delimiting. So a future
transport that bridges a page to a local go-lp2p helper can map each stream 1:1 to a QUIC stream and
forward the bytes, and nothing above the seam changes. `BaseConnection` and `BaseStream` in
`src/transport/base.ts` implement the holding and closing rules for new transports.

Datagrams are the one unreliable service, for WebTransport's `datagrams`. QUIC has them as DATAGRAM
frames (RFC 9221). A transport that can't lose messages may still carry them and set
`datagramsReliable`. A WebTransport session on it then reports `reliability: "reliable-only"`, as
WebTransport over HTTP/2 does.

### Session protocol

The shim speaks the [OSP network messages](https://w3c.github.io/openscreenprotocol/network.html)
and the LP2P extension messages (spec Appendix A) on the wire:

1. The **requester** opens the first stream as the **control stream**. Both sides send
   `agent-info-request` (10) and answer with `agent-info-response` (11). The response carries
   `display-name` = nickname, `model-name` = `lp2p-shim` and `capabilities` = `[1100]` (data channels).
2. **Consent and pairing** run on the control stream. The receiver's user is asked to accept the
   connection. If they decline, the receiver sends `auth-status` with `unknown-error` and closes.
   Otherwise, with `pairing: "pin"`, it sends `auth-capabilities` (1001) and
   `auth-spake2-handshake` (1005, `psk-shown`, a random nonce), and shows a 9-digit PIN. The requester's
   user enters the PIN. The requester sends `auth-spake2-handshake` (`psk-input`, its nonce) and
   `auth-spake2-confirmation` (1003), which holds `SHA-512(tag ‖ PIN ‖ nonceR ‖ nonceQ)`, 64 bytes as
   the CDDL requires. The receiver answers with `auth-status` (1004): `authenticated` or `proof-invalid`.
   This is **not SPAKE2** and is brute-forceable offline. Only the message shapes and the UX match.
   With `pairing: "none"` the receiver sends `auth-status authenticated` straight away.
3. **Every data channel gets its own stream.** Its first message is `data-channel-open-request` (1101:
   request-id, channel-id, label, protocol). The acceptor answers with `data-channel-open-response` (1102,
   `result` = 1 success) on the same stream *before* any data. The opener's channel opens when the
   response arrives. Then both sides send `data-frame` (24: key 0 = encoding-id, key 4 = payload; the
   other OSP data-frame keys are ignored on receipt). Closing the stream closes the channel.
4. **Channel IDs**: the requester (QUIC client) allocates the lowest free even ID and the receiver
   the lowest free odd one, as go-lp2p and openscreen do. An explicit `id` from
   `LP2PDataChannelInit` that is in use or has the peer's parity throws `OperationError`. Open
   requests are accepted whatever the ID's parity, but one for an ID in use is answered with
   `permanent-error`.
5. **WebTransport sessions are pooled on the connection**, as go-lp2p's WIP pooled WebTransport is
   (`src/protocol/webtransport.ts`). A session is a stream that begins with
   `data-transport-start-request` (1201: request-id, exchange-id). The peer answers
   `data-transport-start-response` (1202) once its page takes the session, through a listener or its own
   `LP2PQuicTransport`. `data-transport-close` (1205: code, reason; the shim's own) ends it, and so does
   closing the stream (abruptly). Exchange-ids are even for the requester and odd for the receiver.
   Each WebTransport stream is a stream that begins with `data-transport-stream-request` (1203:
   request-id, exchange-id, key 2 = unidirectional, added by the shim). It is answered by 1204 without
   the opener waiting. After that, each message is a frame: a tag byte, then DATA bytes, FIN, RESET
   (varint code) or STOP_SENDING (varint code). These stand in for QUIC's own stream bytes, FIN bit,
   RESET_STREAM and STOP_SENDING, which the shim's message streams don't have. A datagram is
   `varint(exchange-id) ‖ payload`, as HTTP/3 datagrams carry a quarter stream ID. The spec's binding
   dials a separate QUIC connection with ALPN `q2q` instead. The shim can't, for the reasons in spec
   issue W1 below. go-lp2p's version is bidirectional only: it has no
   unidirectional key, no 1205 and no datagrams, and doesn't wait for 1202.

### Transports

**postMessage** (`PostMessageTransport`) works between windows, iframes and workers of one
browser. Discovery and connection requests go through a `Signaler`. By default that is a
`BroadcastChannel`, which means same-origin contexts only. Each connection then gets a private port.
If the signaler can transfer ports (`PortSignaler` over a `MessagePort`/`Worker`, which also works
across origins), the port is a fresh `MessageChannel` port. Otherwise it is a `BroadcastChannel`
named after the connection, used only by its two ends. Streams are multiplexed over the port, and
the platform's per-port ordering gives ordered delivery.

**WebRTC** (`WebRTCTransport`) uses one `RTCPeerConnection` per connection and **one reliable,
ordered `RTCDataChannel` per stream**. That mirrors OSP's QUIC stream per LP2P channel: SCTP
streams are independent, so a bulk transfer on one channel doesn't hold up another (no head-of-line
blocking), and closing a stream is a native close. Multiplexing everything over one RTCDataChannel
would mean writing our own framing, flow control and close handshake, for no gain. A
pre-negotiated channel (id 0) gets SCTP into the offer, signals readiness and carries the
goodbye. There are no ICE servers by default, so only host candidates are used, which is right for
the same LAN. Pass `rtcConfiguration` to change that. Messages above the browser's SCTP
max-message-size (about 256 KiB in Chrome) fail. The shim does not chunk them. (WebTransport stream
writes are cut into 16 KiB frames, so any write size works there.) Datagrams use a second pre-negotiated
channel (id 1), which is unordered with `maxRetransmits: 0`. That is real loss, like QUIC DATAGRAM
frames, so sessions report `reliability: "supports-unreliable"`. `maxDatagramSize` is 1200 bytes
minus the exchange-id prefix, to stay within one SCTP packet. In the postMessage transport,
datagrams share the port and are never lost (`"reliable-only"`).

Signaling is out of band, so it is pluggable (`Signaler`: a broadcast medium with addressed
messages):

- `BroadcastChannelSignaler`: tabs of the same browser (the default).
- `ManualSignaler`: the user copies codes between devices: the receiver's announcement, the
  requester's offer and the receiver's answer. ICE is gathered fully up front, so there are only
  three codes.
- Anything else (a WebSocket relay, QR codes, a shared server) is about 20 lines that implement `post`
  and `onmessage`.

Discovery is pull-based. The page showing the picker broadcasts `query` every second, receivers
answer with `announce`, and peers silent for 3.5 s are dropped. This keeps working when the
receiver is a background tab whose timers the browser throttles. Connections still open when a
page goes away (`pagehide`) are closed, so peers find out at once.

**Real LAN discovery is out of reach for a web page.** No web API browses mDNS or opens a
connection to an arbitrary local device. Every shim transport needs something both sides already
share: the same browser, a server, or a human copying text. That gap is why the native API is
needed.

### User agent and UI

`UserAgent` (`src/ua/useragent.ts`) is the policy layer between the API and the session, like
go-lp2p's `useragent`:

- `LP2PReceiver.start()` asks `requestPermission({permission: "advertise"})`, then advertises.
- For each incoming connection it runs the agent-info handshake, asks
  `requestPermission({permission: "accept-connection", peer})`, then pairs (`presentPIN`) and fires
  `connection`.
- `LP2PRequest.start()` shows `pickPeer(picker)` with a live peer list, connects, and pairs
  (`enterPIN`).

The UI is the `UserAgentUI` interface, with `requestPermission`, `pickPeer`, `presentPIN` and
`enterPIN`, each given an `AbortSignal` that says when to take the UI down. There are two built-in
implementations:

- `DomUI`: minimal dialogs in a shadow root, labeled "simulated browser UI".
- `AutoUI`: scripted, for tests, workers, and pages that draw their own UI.

`LP2PShim.configure({ transport, ui, pairing, origin, log })` sets them. The defaults are
`PostMessageTransport`, plus `DomUI` with `pairing: "pin"` where there is a DOM, or `AutoUI` (allow
everything, pick the first peer) with `pairing: "none"` in workers. Each object binds to the
configuration current when its `start()` runs.

## Behavior where the spec is silent

| Topic | Shim behavior |
|---|---|
| Errors | `NotAllowedError`: advertising denied, picker dismissed, peer declined, or pairing failed (wrong PIN or canceled). `NetworkError`: peer unreachable or connection lost during setup. `TimeoutError` (wrapped as `NetworkError`): no agent-info within 10 s. |
| `start()` twice | Returns the same promise while pending or after success. After a failure it starts over. |
| `LP2PDataChannel.readyState` | Added (`connecting`/`open`/`closing`/`closed`, as on RTCDataChannel). Without it a page cannot know when `send()` is allowed. |
| `binaryType` | Defaults to `"arraybuffer"`, as on RTCDataChannel. The receiver's `binaryType` decides whether binary data arrives as an ArrayBuffer or a Blob. The sender's encoding-id (blob or array-buffer) is informational. |
| `send()` | Strings are sent as encoding-id string (UTF-8). ArrayBuffer and views are sent as array-buffer. Blobs are sent as blob, read asynchronously, and later sends wait for them so order is kept. `send()` throws `InvalidStateError` unless the channel is open. |
| Events | Channels from `datachannel` are already open and also fire `open` right after the event, as RTCDataChannel does. `close()` fires `closing` and then `close`, as go-lp2p does. A remote close fires the same pair. |
| `new LP2PDataChannel()` | Allowed, because the IDL has a constructor. The channel is attached to nothing and permanently `closed`. |
| `new LP2PConnection()` | `TypeError` (no constructor in the IDL). |
| `LP2PConnection.close()` / `onclose` | Closing closes every channel and transport on both sides. |
| `LP2PReceiver.stop()` | Added: stops advertising. Existing connections stay open. |
| Peer identity | The page is told nothing about the peer (spec issue #15). Nicknames are shown in the browser UI only and marked "(unverified)". |
| `new LP2PQuicTransport(source)` | `source` may be an `LP2PConnection`, an `LP2PRequest` (started if needed, then its connection is used) or an `LP2PReceiver` (started if needed, then its *next* connection is used). If the peer already opened a session on that connection that no page has taken, the transport joins it instead of opening another. If both pages open at once on the receiver side, the receiver's transport joins the requester's session. So the spec's example of both sides calling `new LP2PQuicTransport(conn)` gives one session. |
| `LP2PQuicTransportListener(source)` | Takes the peer's sessions on the one connection of an `LP2PRequest`, or on every connection (current and future) of an `LP2PReceiver`. It also takes an `LP2PConnection` (an extension). `ready` resolves once `source` started. `incomingTransports` closes when a request's connection closes. Cancel it to stop listening. The first listener takes a session. A session nobody takes stays unanswered, so the opener's `ready` stays pending, until a listener or a local `LP2PQuicTransport` takes it. |
| Lifecycle | Closing the `LP2PConnection` (either side) closes its transports abruptly: `closed` rejects with a `WebTransportError` (source `"session"`). `transport.close()` doesn't close the connection. `close()` before `ready` rejects `ready` and `closed`. |
| WebTransport members | `close({closeCode, reason})` reaches the peer's `closed`. Stream `abort()`/`cancel()` with a `WebTransportError` carry its `streamErrorCode` as RESET/STOP_SENDING. `reliability` depends on the transport (above), and `requireUnreliable` rejects `ready` on `"reliable-only"`. `congestionControl`, `sendOrder`, `sendGroup`, `anticipatedConcurrentIncoming*` and the datagram max ages are stored but have no effect. `getStats()` counts payload bytes only (no RTT or packets). `protocol` is `""`, `responseHeaders` is `null` and `exportKeyingMaterial()` rejects with `NotSupportedError`. `draining` resolves only at close. Oversized datagrams are dropped silently, as in WebTransport. |
| `instanceof WebTransport` | False. The browser's `WebTransport` can't be subclassed without a URL connection. `WebTransportError` is the shim's own (Chromium's constructor still takes a dictionary), chained to the browser's, so `instanceof WebTransportError` holds. |
| Not implemented | `bufferedAmount` and backpressure on WebTransport writes (writes resolve once handed to the transport), local HTTPS, and remembering paired peers. |

## LP2P spec issues found while building this

1. **`11OO` / `12OO`** in the `agent-capability` CDDL use the letter O and should be 1100 and 1200. Also,
   `quick-transport` should be `quic-transport`.
2. **The stream mapping is not specified.** `data-frame` has no channel ID, so it only works if each
   channel is its own stream with the open request first. The spec should say so normatively:
   one stream per channel, 1101 first, 1102 on the same stream before data, and close = stream
   FIN/reset. It should also say whether the opener must wait for 1102 (the shim waits; go-lp2p
   originally didn't).
3. **`data-frame` (type key 24) is OSP's streaming message**, but the LP2P CDDL redefines it with only
   keys 0 and 4. The spec should either reference OSP's definition (and say keys 1–3 and 5 are ignored)
   or use a new type key.
4. **`data-channel-open-response` uses `&result`**, which comes from the *application* protocol
   CDDL (success = 1). The network-level extension depends on it without saying so, and no
   failure codes are defined for refusing a channel.
5. **Channel ID allocation is unspecified.** `LP2PDataChannelInit.id` is optional and there is no
   `negotiated` flag and no even/odd rule, so two sides that pick IDs on their own collide (go-lp2p
   defaulted every channel to 0). The meaning of the nullable `id` attribute is also unclear.
6. **`LP2PDataChannel` has a public constructor** but no way to attach the result to a connection.
7. **It is missing RTCDataChannel essentials**: `readyState`, `bufferedAmount`,
   `bufferedAmountLowThreshold`/`onbufferedamountlow`, and `ordered`/reliability options. The spec
   says the interface is kept as close as possible to RTCDataChannel.
8. **The default `binaryType` is unspecified** (RTCDataChannel uses `"arraybuffer"`, WebSocket uses `"blob"`),
   and so is how the sender's blob/array-buffer encoding-id relates to the receiver's `binaryType`.
9. **There are no algorithms** for `send()`, `close()`, `start()` or the events. For example, it is
   undefined whether `closing` fires on a locally initiated close, what `start()` called twice does,
   and which `DOMException` each failure rejects with.
10. **`LP2PConnection` has no `close()`, `onclose` or peer attributes**, and `LP2PReceiver` cannot stop
    advertising.
11. **There is no discovery scoping.** `LP2PRequest` cannot say what kind of receiver it wants
    (service or protocol name, origin), so the picker lists every LP2P receiver nearby by nickname
    alone. The purpose of `LP2PRequestOptions.nickname` is also unstated (the shim shows it in the
    receiver's consent prompt).
12. **Events and UI**: the interfaces are `Exposed=(Window,Worker)`, but the picker, permission and PIN
    UI need a window. The spec doesn't say how a worker gets them. The `LP2PDataChannelEventInit`
    constructor argument is spelled `DataChannelEventInitDict` (capital D).
13. Related OSP nit: Appendix B's padding rule ("zero-pad with `3 - len(N) mod 3` digits") adds three
    zeros when the length is already a multiple of three.

### WebTransport (`LP2PQuicTransport`)

Section names are those of `index.bs` on the `lp2p2/04-security-privacy` branch.

- **W1. The OSP binding can't be implemented as written** (*WebTransport extension*). It says to dial a new
  QUIC connection with ALPN `q2q`, but `q2q` is not defined anywhere, and draft-vvv-webtransport-quic
  expired. Nothing maps streams, datagrams or session close onto it. A receiver can't dial a requester
  that isn't advertising. Nothing binds the new connection to the authenticated session, or routes it
  to an origin or `LP2PReceiver`. The shim pools sessions on the existing connection instead (1201–1205;
  go-lp2p does the same). The spec should choose pooled or dedicated, which WebTransport's `allowPooling`
  already distinguishes.
- **W2. The abstraction quietly requires unreliable delivery** (*Operations*: "open a WebTransport
  session"). "The capabilities of a WebTransport session" include datagrams. The OSP binding then needs
  QUIC DATAGRAM (RFC 9221) negotiated, and says so nowhere. Bindings without loss (postMessage, a WebSocket
  bridge) can only do `reliable-only`. The spec should allow that, map it to `reliability`, and say how
  `requireUnreliable` fails.
- **W3. No one is defined to receive an incoming session** (*LP2PQuicTransportListener*, *Operations*).
  "The other side is notified" has no accept or refuse step, unlike data channels. Nothing says what
  happens with no listener (the shim leaves it unanswered), with several listeners (the shim lets the first
  take it), or when the LP2PQuicTransport was built on an `LP2PConnection`, which no listener accepts (the
  shim accepts one as an extension).
- **W4. The connection examples assume a symmetric open** (*LP2PConnection Examples*). Both sides call
  `new LP2PQuicTransport(conn)` and await `ready`, but the operation is asymmetric (open, then incoming).
  The spec doesn't say whether that makes one session or two. The shim makes one: a constructor joins an
  untaken incoming session, and on a simultaneous open the requester's session wins.
- **W5. `new LP2PQuicTransport(receiver)` is ambiguous** (*LP2PQuicTransport*). A receiver has zero or many
  connections. The shim uses the next one. More generally, there are no constructor steps. A constructor
  that "must start" an `LP2PRequest` shows a device picker, and nothing says whether that needs user
  activation.
- **W6. `LP2PQuicTransport : WebTransport` can't inherit WebTransport's constructor or internal slots**.
  Those assume a URL and HTTP/3 or HTTP/2. LP2P must define the constructor steps and slot setup itself.
  The exposure also differs: WebTransport is `(Window,Worker)`, LP2P is `(Window,DedicatedWorker)`.
- **W7. `LP2PQuicTransportInit` and `LP2PQuicTransportListenerInit` are empty**, with an open Issue.
  `headers`, `responseHeaders`, `serverCertificateHashes` and `allowPooling` are HTTP or server
  concepts. `protocols`/`protocol` would be useful, as a counterpart to the data channel `protocol`, but
  have no wire field. A listener has no way to set options such as `datagramsReadableType` or
  `requireUnreliable` for the transports it hands out. The shim accepts `requireUnreliable`,
  `congestionControl`, `anticipatedConcurrentIncoming*` and `datagramsReadableType`.
- **W8. The lifecycle coupling is one-way and has no error defined** (*Operations*: "close a session";
  *LP2PConnection*). Closing the session closes its WebTransport sessions, but the spec doesn't say with
  which `closed` outcome. The shim rejects with a session `WebTransportError`. A transport built from
  an `LP2PRequest` hides its `LP2PConnection`, so closing the transport leaves the LP2P session open, and
  the page can only close it by calling `start()` again. The spec doesn't say what `LP2PReceiver.stop()`
  does to listeners.
- **W9. Nothing on the wire covers close codes, stream errors or draining**. `close({closeCode, reason})`
  needs a message like HTTP/3's CLOSE_WEBTRANSPORT_SESSION (the shim uses 1205). Reset and STOP_SENDING
  codes are 32-bit in WebTransport and need a mapping onto QUIC application error codes. `draining`
  has no trigger between peers, since there is no GOAWAY and neither page can ask for a drain.
- **W10. Pooling leaks across origins**. Keying material from `exportKeyingMaterial()` on a shared OSP
  connection is shared by every origin on it. RTT and packet counts in `getStats()` would show other
  origins' traffic. Both need per-session separation, or must be unsupported. The shim rejects the
  first and counts only the session's own bytes.
- **W11. The `quic-transport` capability (1200) has no defined use** (*Appendix A*). The spec doesn't say
  what happens when the peer lacks it (which error `ready` rejects with), and the capability is per agent,
  not per origin.
- **W12. `incomingTransports` is an untyped `ReadableStream`, and the listener has no `close()`.** Only
  `cancel()` stops it. The spec doesn't say whether the stream ends when the request's connection
  closes (the shim ends it) or when the receiver stops.
- **W13. Evidence for Issue(29)** (*Operations*). In the shim, a data channel and a bidirectional
  WebTransport stream use the same wire pattern (a request/response header on a stream, then frames).
  A data channel is a WebTransport stream with a label, a protocol and message framing, which argues
  for converging the two.
