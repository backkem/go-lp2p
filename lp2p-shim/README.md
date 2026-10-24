# lp2p-shim

**An experimental polyfill for the [Local Peer-to-Peer API](https://github.com/WICG/local-peer-to-peer):
`LP2PReceiver`, `LP2PRequest`, `LP2PConnection`, `LP2PDataChannel` and their events, working in today's
browsers over pluggable transports.**

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

Other code is the [spec's examples](https://wicg.github.io/local-peer-to-peer/) unchanged.

## Files

| Path | What |
|---|---|
| `dist/lp2p-shim.js` | Standalone script (IIFE). It defines the six LP2P interfaces on `globalThis` where they are missing (in secure contexts only) and exposes the rest as `LP2PShim`. |
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
                      │
 user agent  ── consent prompts, device picker, PIN pairing (UserAgentUI hooks)  src/ua
                      │
 session     ── OSP/LP2P messages: agent-info, auth-*, 1101/1102, data-frame     src/protocol, src/codec
                      │
 transport   ── discovery, connections, reliable ordered message streams         src/transport
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
4. **Channel IDs**: the requester allocates even IDs and the receiver odd ones, as SCTP does with
   DTLS roles. An explicit `id` from `LP2PDataChannelInit` is used as given. A collision throws
   `OperationError` locally, and an open request for an ID in use is answered with
   `permanent-error`.

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
max-message-size (about 256 KiB in Chrome) fail. The shim does not chunk them.

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
| `LP2PConnection.close()` / `onclose` | Added. Closing closes every channel on both sides. |
| `LP2PReceiver.stop()` | Added: stops advertising. Existing connections stay open. |
| Peer identity | The page is told nothing about the peer (spec issue #15). Nicknames are shown in the browser UI only and marked "(unverified)". |
| Not implemented | `LP2PQuicTransport`, `LP2PQuicTransportListener`, `bufferedAmount`, local HTTPS, and remembering paired peers. |

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
