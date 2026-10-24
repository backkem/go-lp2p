# go-lp2p

**A Go playground for the [Local Peer-to-Peer API](https://github.com/WICG/local-peer-to-peer),
built on a from-scratch implementation of the [Open Screen Protocol](https://github.com/w3c/openscreenprotocol).**

The Local Peer-to-Peer (LP2P) API is a WICG proposal that lets web pages
discover devices on the local network and open secure, direct connections to
them without a server in between. This repository explores what such an API
could feel like and what it takes to build it on a real wire protocol: the
W3C's Open Screen Protocol (OSP), which already specifies local discovery
(mDNS/DNS-SD), mutual authentication (SPAKE2 over TLS 1.3) and transport over
QUIC.

> [!WARNING]
> This is experimental research code. It is not final, not audited and not
> secure. Don't use it in production.

## What's in here

The repository is a stack of layers, each usable on its own:

```
 examples/                 runnable end-to-end demos
     │
 lp2p-api/        (lp2p)   the LP2P web API, mirrored in Go
     │      ├── useragent/         (ua)            a mock browser: permission prompts, device picker, PIN pairing
     │      ├── web-api/           (web)           events, the bits of the web platform it needs
     │      ├── streams-api/       (streams)       WHATWG Streams-style readers and writers
     │      └── webtransport-api/  (webtransport)  WebTransport-style sessions and streams
     │
 openscreen-go/            a Go implementation of the Open Screen Protocol
     ├── network/          (ospc)  discovery, QUIC transport, SPAKE2 auth, agent-info
     └── application/              application protocol messages on top of the network layer

 cast/                     a Go implementation of the Google Cast channel, as a sibling transport

 lp2p-shim/                the LP2P web API as a JavaScript polyfill for today's browsers (TypeScript)
     ├── api, ua, protocol         same layering as lp2p-api / useragent / ospc, speaking OSP messages
     └── transports                postMessage (same browser) and WebRTC (pluggable signaling)
```

| Directory | Package | What it does |
|---|---|---|
| [`lp2p-api`](./lp2p-api/) | `lp2p` | The `LP2PReceiver`, `LP2PRequest`, `LP2PDataChannel` and `LP2PQuicTransport` interfaces from the proposal. It deliberately stays close to the web API instead of idiomatic Go (see [Coding conventions](#coding-conventions)). |
| [`lp2p-api/useragent`](./lp2p-api/useragent/) | `ua` | A mock user agent hosting the API. It discovers peers continuously and drives a pluggable UI for permission prompts, the device picker and PIN pairing. The UI can be scripted (examples, tests) or interactive (terminal). It exists to illustrate the API, not as a product. |
| [`openscreen-go/network`](./openscreen-go/network/) | `ospc` | The OSP network layer: mDNS advertising and browsing, QUIC with ALPN and certificate fingerprints, SPAKE2 PSK authentication, CBOR message framing, the data-channel extension and WebTransport over OSP. It also has an experimental WebRTC (DTLS/SCTP) transport. |
| [`openscreen-go/application`](./openscreen-go/application/) | `application` | OSP application-protocol messages, plus examples and interop tests against [openscreen-rs](https://github.com/youtube/openscreen-rs). |
| [`cast`](./cast/) | `cast` | The Google Cast channel as used by Chromium's Open Screen Library: TLS over TCP, length-prefixed protobuf `CastMessage`s, virtual connections, heartbeat, device authentication and `_googlecast._tcp` discovery. Not yet wired into the LP2P API; see its [README](./cast/README.md). |
| [`streams-api`](./streams-api/), [`webtransport-api`](./webtransport-api/), [`web-api`](./web-api/) | `streams`, `webtransport`, `web` | Small Go shims for the web platform APIs that LP2P builds on. |
| [`lp2p-shim`](./lp2p-shim/) | — | A polyfill for web pages: `LP2PReceiver`, `LP2PRequest`, `LP2PConnection` and `LP2PDataChannel` over a clean transport seam, with postMessage and WebRTC transports, simulated browser UI and demos. Ships a checked-in standalone bundle (`lp2p-shim/dist/lp2p-shim.js`). Not secure; see its [README](./lp2p-shim/README.md). |
| [`src`](./src/) | — | TypeScript typings generated from the proposal's WebIDL (`npm run idl`), plus a sketch of how the API reads from JavaScript. Reference only; the Go code doesn't depend on it. |

## Quick start

You need Go 1.24 or newer.

```sh
git clone https://github.com/backkem/go-lp2p
cd go-lp2p

# Chat over an LP2PDataChannel
go run ./examples/lp2p-data-channel

# Pick one of several devices in the device picker
go run ./examples/device-picker

# See how denied permissions, a dismissed picker and a wrong PIN surface
go run ./examples/pairing-failures

# Stream 4 MiB over an LP2PQuicTransport and verify it
go run ./examples/webtransport-pooled
```

Each example runs every device in one process, each with its own mock
browser, and simulates the users: granting permissions, picking a device, and
reading the pairing PIN off one screen to type it on the other. ospc prints
protocol debug output on stdout, so add `>/dev/null` to see just the example:

```
[Receiver device] https://receiver-device.example wants to be discoverable as "Chat receiver #3fa9": allowed
[Requester device] https://requester-device.example ("Chat requester") shows the device picker
[Requester device] user picked Chat receiver #3fa9 (unverified)
[Receiver device] shows PIN 1406-6647 to pair with Chat requester (unverified)
[Requester device] user enters PIN 1406-6647
Receiver: got a connection
Requester: got a connection
Receiver: got "Hello receiver!"
Requester: got "Hi requester, receiver here."
...
```

Pass `-interactive` to pick the device and type the PIN yourself. See
[examples](./examples/) for more.

Lower-level OSP examples live next to the code they demonstrate, in
[`openscreen-go/network/examples`](./openscreen-go/network/examples/) and
[`openscreen-go/application/examples`](./openscreen-go/application/examples/).

Run the tests with:

```sh
go test ./...
# Interop with openscreen-rs needs cargo and a checkout at ../openscreen-rs
go test -tags interop ./openscreen-go/application/...
```

## Status

- **LP2P API**
  - [x] `LP2PReceiver` and `LP2PRequest`, with WebIDL names and
        `DOMException` errors
  - [x] `LP2PDataChannel`, including events and a closing handshake
  - [x] `LP2PQuicTransport` and `LP2PQuicTransportListener` over an OSP
        connection (pooled)
  - [ ] `LP2PQuicTransport` over a dedicated QUIC connection
  - [ ] Teardown in the proposal itself: `LP2PConnection.Close` and
        `LP2PReceiver.Close` are Go-only additions
- **Mock user agent**
  - [x] Permission prompts for advertising and for each incoming connection
  - [x] Continuous discovery and a live device picker (peer listing and
        selection)
  - [x] PIN presentation and entry
  - [x] Scripted and interactive (terminal) UIs
  - [ ] Peers disappearing from the picker (ospc discovery doesn't report
        removals yet)
  - [ ] Remembered grants: every connection prompts and pairs again
- **Open Screen Protocol**
  - [x] Discovery, listen and dial
  - [x] SPAKE2 authentication
  - [x] Data-channel protocol extension
  - [ ] Complete application protocol (presentation, remote playback, streaming)
- **Beyond OSP**
  - [ ] Abstract the LP2P API from the underlying transport, so others (Wi-Fi
        Direct, Cast) can plug in
  - [ ] Lots of cleanup

## Related work

- **[Open Screen Protocol](https://github.com/w3c/openscreenprotocol/)** (W3C
  Second Screen Working Group). The specifications this repository implements:
  the [network protocol](https://www.w3.org/TR/openscreen-network/) and the
  [application protocol](https://www.w3.org/TR/openscreen-application/). Open
  Screen grew out of the Presentation API and Remote Playback API, and LP2P
  reuses its discovery, authentication and transport layers.
- **[Open Screen Library](https://chromium.googlesource.com/openscreen)**
  (Chromium). The C++ reference implementation used by Chrome. Besides OSP it
  includes mDNS/DNS-SD and the Cast protocols (discovery, application control
  and media streaming); the [`cast`](./cast/) package follows its Cast channel.
- **[openscreen-rs](https://github.com/youtube/openscreen-rs)** (YouTube, now
  archived). A Rust implementation of the Open Screen Protocol, used here as an
  interop target: the application examples mirror its examples, and the
  `interop` tests run against it.
- **[Local Peer-to-Peer API](https://github.com/WICG/local-peer-to-peer)**
  (WICG). The API proposal this playground is meant to inform.

## Coding conventions

The `lp2p` package mirrors a web API and favours similarity to it over
idiomatic Go; async functions are implemented as blocking calls. All other
packages, such as `ospc`, aim to be idiomatic Go.

## License

[MIT](./LICENSE)
