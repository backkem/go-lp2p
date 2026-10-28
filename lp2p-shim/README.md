# lp2p-shim

**An experimental polyfill for the [Local Peer-to-Peer API](https://github.com/WICG/local-peer-to-peer):
`LP2PReceiver`, `LP2PRequest`, `LP2PConnection`, `LP2PDataChannel`, `LP2PQuicTransport`,
`LP2PQuicTransportListener`, their events and the explainer's `lp2p.makeDiscoverable()` /
`lp2p.connect()` shorthand, working in today's browsers over pluggable transports.**

> [!WARNING]
> The shim offers **no security**. Pairing is simulated (a toy PIN check, not SPAKE2), keys are
> proven but nothing is encrypted, the "browser UI" is drawn inside the page where the page can fake or click it, and
> data is only as private as the transport underneath. Use it to prototype and to demo the API, never
> to protect anything.

The point is to let web developers try the API in a real page, such as a local chat, a multiplayer
game or a file drop, and to find out where the spec is incomplete. A page cannot do what the
native API does with the spec's OSP binding: discover devices on the LAN with mDNS and connect to them
over authenticated QUIC.
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
| `dist/lp2p-shim.js` | Standalone script (IIFE). It defines the eight LP2P interfaces and the explainer's `lp2p` shorthand namespace on `globalThis` where they are missing (in secure contexts only) and exposes the rest as `LP2PShim`. |
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
and the LP2P extension messages (spec Appendix A) on the wire, as the spec's OSP binding lays them
out on one QUIC connection per LP2P session. The side that connects is the **client**, the side that
accepts is the **server**.

1. **Handshake.** The client opens the first stream as the **control stream**. Both sides first run
   the shim's stand-in for the TLS handshake underneath OSP (`src/protocol/identity.ts`, private-use
   type keys 32001/32002): each sends its agent key (ECDSA P-256, non-extractable, one per device) and
   an ephemeral ECDH key, and signs the transcript. This proves who holds which key, gives each side the
   peer's **identity** (the key's SHA-256 fingerprint, like OSP's agent fingerprint) and a shared secret
   for keying material exporters. It does not encrypt anything. Then both sides exchange
   `agent-info` (10/11): `display-name` is the **device name** (`deviceName`, default
   `"lp2p-shim device"`), not the page's nickname, which only travels in discovery.
2. **Verify the server first.** Both agents send `auth-capabilities` (1001) at connection start, as in
   OSP; it starts nothing. A client that doesn't trust the server's key runs the trust ceremony now,
   before the server learns anything about the page.
3. **Trust ceremony, client-initiated.** The server presents the PSK, so the client asks for
   presentation: `auth-spake2-handshake` (1005, `psk-needs-presentation`) with **no public-value**, as
   the OSP fix the LP2P binding assumes (OSP today requires `pA` there, which a consumer can't have
   before the PSK is shown). The server shows a 9-digit PIN and runs OSP's presenter-initiated flow:
   `psk-shown` with its nonce; the client's user enters the PIN and the client answers `psk-input` with
   its nonce and `auth-spake2-confirmation` (1003) =
   `SHA-512(tag ‖ PIN ‖ nonceS ‖ nonceQ ‖ fpClient ‖ fpServer)`; the server answers `auth-status` (1004).
   The nonces stand in for SPAKE2's `pA`/`pB`. (Before, the client's request carried a nonce and the
   server answered `psk-input`; that didn't follow OSP's message rules.) Success adds a one-peer trust anchor on each side; a failure
   closes the connection. This is **not SPAKE2** and is brute-forceable offline. With
   `pairing: "none"` the server answers the client's handshake with `auth-status authenticated`.
4. **Session request and answer.** Only with a verified server, the client opens a stream with
   `lp2p-session-request` (1110: request-id, requesting origin). A server that doesn't trust the
   client's key answers a provisional `lp2p-session-response` (1111) `authentication-required` (1110),
   without looking at the origin, and the client runs the ceremony. Once the client is verified, the
   server picks a receiver that allows the origin (the one whose advertisement the client picked, if
   it does), asks its user to grant the client (skipped if granted before), and answers `success` or
   `permanent-error`. So the origin only reaches a verified server and the answer only a verified
   client, and peers that trust each other run no ceremony. The client accepts on `success` and holds
   streams that arrive before it. After a refusal the client ends its side of the stream and the
   server closes the connection once it has, or after 2 s. A second 1110 on a connection closes it.
5. **Data channels** each get their own stream, starting with `data-channel-open-request` (1101); the
   acceptor answers `data-channel-open-response` (1102) on it before any `data-frame` (24). The client
   allocates the lowest free even channel ID, the server the lowest free odd one. Closing the stream
   closes the channel.
6. **WebTransport sessions are pooled on the connection** (`src/protocol/webtransport.ts`). A session's
   control stream starts with `data-transport-start-request` (1201: request-id, exchange-id, even for the
   client and odd for the server, never reused). The peer's user agent answers
   `data-transport-start-response` (1202): `success` if a listener takes it, else `permanent-error` and
   the end of the stream. A WebTransport stream is a transport stream of the same direction starting with
   `data-transport-stream` (1203: exchange-id only, no response); 1204 is reserved. After the header each
   message is one frame: a tag byte, then DATA bytes, FIN, RESET (varint code) or STOP_SENDING (varint
   code), standing in for QUIC's stream bytes, FIN, RESET_STREAM and STOP_SENDING. Codes from 2^32 are
   the binding's: **session-gone** (2^32) resets and stops a closed session's streams, and refuses a
   stream for an unknown session; script sees it without a stream error code. `data-transport-close`
   (1205: code, reason) and then the end of the control stream close a session; ending it without 1205
   is code 0 with an empty reason; losing the connection is abrupt. An agent that sent 1205 keeps the
   connection until the peer ended the control stream, or 2 s. `data-transport-drain` (1206)
   resolves the peer's `draining`. A datagram is `varint(exchange-id) ‖ payload`. The binding's optional
   dedicated `q2q` connections are not implemented.

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
- For each incoming connection it runs the handshake, answers the ceremony the client starts
  (`presentPIN`), asks for one with `authentication-required` if the client's key isn't trusted, then
  picks a receiver that allows the requesting origin (preferring the one the client picked), asks
  `requestPermission({permission: "accept-connection", peer})` unless the peer is granted already,
  and fires `connection`.
- `LP2PRequest.start({signal})` shows `pickPeer(picker)` with a live peer list, connects, pairs
  (`enterPIN`) if it doesn't trust the server, sends the requesting origin, and pairs again only if the
  server asks. Aborting the signal takes down the picker or closes the session being set up.

The UI is the `UserAgentUI` interface, with `requestPermission`, `pickPeer`, `presentPIN` and
`enterPIN`, each given an `AbortSignal` that says when to take the UI down. There are two built-in
implementations:

- `DomUI`: minimal dialogs in a shadow root, labeled "simulated browser UI".
- `AutoUI`: scripted, for tests, workers, and pages that draw their own UI.

`LP2PShim.configure({ transport, ui, pairing, origin, deviceName, localState, quirks, log })` sets
them. The defaults are `PostMessageTransport`, plus `DomUI` with `pairing: "pin"` where there is a DOM,
or `AutoUI` (allow everything, pick the first peer) with `pairing: "none"` in workers. `origin` is the
page's origin as the requesting origin (default `location.origin`), `deviceName` the agent-info
`display-name`, and `localState` the device's trust store, known peers and grants (default: one per
transport object; `LocalState` has methods to view and remove them, as the spec says user agents
should offer). Each object binds to the configuration current when its `start()` runs.

#### Quirks

Behavior that departs from the spec goes behind `configure({ quirks })`, each flag off by default, like
go-lp2p's `ospc.InteropQuirks`. There are none at the moment: `exposeShorthandConnection` went away once
the spec gave `LP2PQuicTransport` a `connection` attribute. Some departures are not flags because a
page can't avoid them: the TLS stand-in and toy PIN pairing, reliable-only datagrams on postMessage,
and pooled-only WebTransport.

## Behavior where the spec is silent

| Topic | Shim behavior |
|---|---|
| Errors | `start()` of `LP2PRequest` rejects with the abort reason when its signal aborts, and otherwise with `NotAllowedError` on every failure, as the spec requires: picker dismissed, peer unreachable, pairing failed or session refused. The message differs, for debugging. `LP2PReceiver.start()` rejects with `NotAllowedError` when advertising is denied and `InvalidStateError` after `stop()` (checked first, as P1 asked). |
| `start()` twice | Returns the same promise while pending or after success. After a failure it starts over. |
| `LP2PDataChannel.readyState` | Added (`connecting`/`open`/`closing`/`closed`, as on RTCDataChannel). Without it a page cannot know when `send()` is allowed. |
| `binaryType` | Defaults to `"arraybuffer"`, as on RTCDataChannel. The receiver's `binaryType` decides whether binary data arrives as an ArrayBuffer or a Blob. The sender's encoding-id (blob or array-buffer) is informational. |
| `send()` | Strings are sent as encoding-id string (UTF-8). ArrayBuffer and views are sent as array-buffer. Blobs are sent as blob, read asynchronously, and later sends wait for them so order is kept. `send()` throws `InvalidStateError` unless the channel is open. |
| Events | Channels from `datachannel` are already open and also fire `open` right after the event, as RTCDataChannel does. `close()` fires `closing` and then `close`, as go-lp2p does. A remote close fires the same pair. |
| `new LP2PDataChannel()` | Allowed, because the IDL has a constructor. The channel is attached to nothing and permanently `closed`. |
| `new LP2PConnection()` | `TypeError` (no constructor in the IDL). |
| `LP2PConnection.close()` / `onclose` | As specified: closes every channel and transport on both sides; `close` fires once, after the transports failed and the listener closed. |
| `LP2PReceiver.stop()` | As specified: stops advertising; delivered connections, their transports and listeners stay. |
| Peer identity | The page is told nothing about the peer (spec issue #15). The browser UI shows the page's nickname from discovery, marked "(unverified)", and the device name once the session is authenticated. |
| `new LP2PQuicTransport(connection)` | As specified: each construction opens its own session, so both sides constructing at once gives two sessions; `transport.connection` is the connection. Fails with a session `WebTransportError` if the peer lacks `quic-transport` or refuses. |
| `LP2PQuicTransportListener(connection)` | As specified: one per connection (`InvalidStateError` otherwise), `close()` ends `incomingTransports` but keeps what is queued. Incoming sessions are refused with no listener, when its queue (8) is full, or with `requireUnreliable` on a reliable-only connection, and held until the connection is exposed (the task after `start()` resolves or `connection` fires). Incoming data channels are held the same way, and their messages until the `datachannel` event has run. |
| Lifecycle | `transport.close()` leaves the connection open; the peer's `closed` gets code and reason. Closing the `LP2PConnection` fails its transports with a session `WebTransportError`. `close()` before `ready` rejects `ready` and `closed`. |
| WebTransport members | Stream `abort()`/`cancel()` with a `WebTransportError` carry its `streamErrorCode`. `reliability` depends on the transport (above). `congestionControl`, `sendOrder`, `sendGroup`, `anticipatedConcurrentIncoming*` and the datagram max ages are stored but have no effect. `getStats()` counts the session's payload bytes only. `exportKeyingMaterial(label, context, length = 32)` derives per session from the handshake stand-in with the binding's label and WebTransport Exporter Context: both sides agree, other sessions differ, but it is not a TLS exporter. `draining` resolves on the peer's 1206 (no page can send one) or at close. Oversized datagrams are dropped silently. |
| `instanceof WebTransport` | False. The browser's `WebTransport` can't be subclassed without a URL connection. `WebTransportError` is the shim's own (Chromium's constructor still takes a dictionary), chained to the browser's, so `instanceof WebTransportError` holds. |
| Local state | Trust anchors, known peers and peer grants per device (`LocalState`, by default one per transport object), in memory only. A trusted key skips the PIN; a granted peer skips the prompt. Removing an anchor drops the grants of peers it no longer covers. Vouching anchors are modeled, never added. |
| Not implemented | `bufferedAmount` and backpressure on WebTransport writes (writes resolve once handed to the transport), dedicated `q2q` connections, persisting the local state, and local HTTPS (out of scope: a separate draft). |

## Shorthand APIs

`lp2p.makeDiscoverable(options)` and `lp2p.connect(options)` (`src/api/shorthand.ts`, installed as the
global `lp2p`) follow the explainer, built only from what a page can use:

```js
// Peer A: an LP2PReceiver plus an LP2PQuicTransportListener on each connection, merged.
for await (const transport of lp2p.makeDiscoverable({ nickname: "Peer A" })) { /* ready */ }

// Peer B: an LP2PRequest plus new LP2PQuicTransport(connection).
const transport = await lp2p.connect({ nickname: "Peer B" });
```

`makeDiscoverable()` takes the `LP2PReceiverOptions` and `LP2PQuicTransportListenerInit` members and
returns a `ReadableStream` of ready `LP2PQuicTransport`s. It owns the listener of each connection it
creates and moves transports from it as fast as the page reads, so an unread stream fills the queues
and further sessions are refused. Cancelling it stops the receiver and closes the listeners.
`connect()` takes the `LP2PRequestOptions` and `LP2PQuicTransportInit` members and a `signal`, and
resolves with a ready `LP2PQuicTransport`; it creates no listener. The shorthand covers transports
only: a connection on which the peer only opens data channels never shows up in
`makeDiscoverable()`. On both sides `transport.connection` is the
connection, which stays the page's to use and close.

How they coexist with the full API (`test/shorthand.test.ts`):

- **Both directions work**: `makeDiscoverable()` against `LP2PRequest` + `new LP2PQuicTransport(conn)`,
  and an `LP2PReceiver` with a listener against `lp2p.connect()`.
- **After `connect()`** the page can construct its own `LP2PQuicTransportListener` on
  `transport.connection` and receive the transports the peer opens; until then they are refused.
- **On a `makeDiscoverable()` connection**, `new LP2PQuicTransportListener(transport.connection)`
  throws `InvalidStateError` (the shorthand holds the listener); further transports from that peer
  arrive in the shorthand's stream. Data channels and `close()` work through `transport.connection`.
- **A page with `makeDiscoverable()` and its own `LP2PReceiver`** advertises twice. A session goes to
  the receiver whose advertisement the client picked when it allows the origin, else to another one
  that does.

## LP2P spec issues found while building this

Status against the spec as of fork PR 17 (`lp2p2/03-connection-lifecycle`, which builds on trust
anchors, local state, roles and the requesting origin). **Resolved** means the spec now says what the
issue asked for; "PR 17" marks the ones that change resolved.

### Data channels and the API

1. **Resolved.** `11OO` / `12OO` in the `agent-capability` CDDL: now 1100 and 1200, `quic-transport`.
2. **Resolved.** The stream mapping: one stream per channel, 1101 first, 1102 on it before data, and the
   channel opens on a success response.
3. **Resolved.** `data-frame` (24) is OSP's; keys 1, 2, 3 and 5 are unused and ignored.
4. **Open.** `data-channel-open-response` uses `&result` from the *application* protocol CDDL, and no
   failure codes say why a channel was refused.
5. **Half resolved.** Binding-chosen IDs follow the role (client even, server odd). Still open: whether a
   page-chosen `id` must too (the shim throws `OperationError`; otherwise it can collide with the peer's
   next binding-chosen ID), and what the nullable `id` attribute means.
6. **Open.** `LP2PDataChannel` has a public constructor but no way to attach the result to a connection.
7. **Open.** RTCDataChannel essentials are missing: `readyState`, `bufferedAmount`,
   `bufferedAmountLowThreshold`/`onbufferedamountlow`, `ordered`/reliability options.
8. **Open.** The default `binaryType`, and how the sender's blob/array-buffer encoding-id relates to the
   receiver's `binaryType`.
9. **Half resolved (PR 17).** `LP2PReceiver.start()`/`stop()`, `LP2PRequest.start()` and the connection's
   `close()` now have algorithms and errors (`NotAllowedError` for every request failure). `send()`,
   `close()` and the events of `LP2PDataChannel` still have none.
10. **Resolved (PR 17).** `LP2PConnection.close()`/`onclose` and `LP2PReceiver.stop()`. Peer attributes
    are still absent, deliberately (see the trust model).
11. **Narrowed (PR 17).** Origin filtering happens at delivery (`allowedOrigins`), and a refusal after
    the user picked a peer is now expected and indistinguishable from a dismissed picker. Discovery still
    has no scoping (service or protocol), and `LP2PRequestOptions.nickname`'s purpose is unstated.
12. **Open.** Interfaces are `Exposed=(Window,Worker)`, but the picker, permission and PIN UI need a
    window. (The `DataChannelEventInitDict` spelling is fixed.)
13. Related OSP nit: Appendix B's padding rule ("zero-pad with `3 - len(N) mod 3` digits") adds three
    zeros when the length is already a multiple of three.

### WebTransport (`LP2PQuicTransport`)

- **W1. Resolved (PR 17).** The binding is defined: sessions pooled on the LP2P session's QUIC
  connection (1201/1202), or on a client-opened dedicated `q2q` connection bound by an exporter
  (`session-binding`). The shim pools only.
- **W2. Resolved (PR 17).** Sessions are "supports-unreliable" or "reliable-only", `quic-transport`
  requires QUIC DATAGRAM, and `requireUnreliable` fails the transport.
- **W3. Resolved (PR 17).** The listener accepts; with no listener or a full queue the session is
  refused; sessions are held until the connection is exposed.
- **W4. Resolved (PR 17).** Each construction opens its own session; the examples have one side open
  and the other listen.
- **W5. Resolved (PR 17).** The constructor takes an `LP2PConnection` and has steps.
- **W6. Acknowledged (PR 17).** The spec initializes the WebTransport slots itself, with an Issue asking
  the WebTransport editors for a hook.
- **W7. Resolved (PR 17).** `LP2PQuicTransportInit` keeps `requireUnreliable`, `congestionControl` and
  `anticipatedConcurrentIncoming*`; the listener's options apply to its transports.
  `protocols`/`protocol` remain unavailable.
- **W8. Resolved (PR 17).** A lifecycle section: closing a transport leaves the connection, closing the
  connection fails its transports with a session `WebTransportError` and closes the listener before
  `close`, and `stop()` leaves both alone.
- **W9. Mostly resolved (PR 17).** 1205 carries close code and reason, 1206 drains, stream error codes
  map onto QUIC codes with session-gone reserved. Still open: no page can ask for a drain (WebTransport
  has no `drain()`), so 1206 is only sent at the user agent's discretion.
- **W10. Half resolved (PR 17).** Each session has its own exporter, with the exchange-id in the
  context. `getStats()` on a pooled connection still needs per-session separation of RTT and packets.
- **W11. Resolved (PR 17).** `quic-transport` gates the extension, and opening fails towards an agent
  without it.
- **W12. Resolved (PR 17).** `incomingTransports` carries `LP2PQuicTransport` objects, and the listener
  has `close()`.
- **W13. Open.** Evidence for Issue(29): a data channel and a bidirectional WebTransport stream use the
  same wire pattern (a header on a stream, then frames). A data channel is a WebTransport stream with a
  label, a protocol and message framing, which argues for converging the two.

### Raised on PR 17

All **resolved** by the spec's follow-up commits, and the shim follows them:

- **P1–P5**: `start()` checks the stopped state first; `connect()` returns a promise for a transport;
  `transport.connection`, with the shorthand owning its connections' listeners; the picked
  advertisement wins; early streams are held until `success`.
- **P6, S1**: auth-capabilities is sent at connection start as in OSP and starts nothing; the client
  verifies the server before it sends the origin, and a server that doesn't trust the client asks
  with a provisional `authentication-required`. So the requesting origin is never exposed to an
  unverified server, nor the answer to an unverified client, and peers that trust each other run no
  ceremony. The cost, which the spec states: a first ceremony with a peer can end in a refusal.
- **S2**: `LP2PRequest.start({signal})` and `lp2p.connect({signal})`.
- **S3**: the explainer says the shorthand covers transports only.
- **S4**: incoming data channels are held until the connection is exposed, like WebTransport
  sessions. The shim also holds their messages until the `datachannel` event has run.
