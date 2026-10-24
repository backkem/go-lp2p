<h1 align="center">
  Cast Go
</h1>
<h4 align="center">A pure Go implementation of the Google Cast (CastV2) channel</h4>
<p align="center">
  <a href="https://chromium.googlesource.com/openscreen/+/refs/heads/main/cast/">Reference (openscreen/cast)</a>
</p>
<p align="center">
  <a href="../LICENSE"><img src="https://img.shields.io/badge/License-MIT-yellow.svg" alt="License: MIT"></a>
</p>
<br>

Chromium's [openscreen](https://chromium.googlesource.com/openscreen/) library
contains two protocol stacks: the W3C Open Screen Protocol (QUIC + CBOR,
implemented in Go in [`../openscreen-go`](../openscreen-go)) and Google Cast,
which is what Chrome actually uses to talk to Chromecasts and Cast-enabled TVs.
This package implements the Cast transport ("cast channel") next to it, ported
from `openscreen/cast/common`, `cast/sender/channel` and `cast/receiver/channel`.

```
mDNS _googlecast._tcp  ->  TLS over TCP :8009  ->  [u32 BE length][CastMessage protobuf]
                                                   source/destination ids + namespace
```

### Features

- CastV2 channel
  - `CastMessage` / `DeviceAuthMessage` protobuf encoding, hand-written to
    match `cast_channel.proto` byte for byte (no protobuf dependency)
  - Length-prefixed framing with the 64 KiB body limit
  - TLS socket: sender `Dial`, receiver `Listen`/`Accept`
- Channel (`VirtualConnectionRouter` + `ConnectionNamespaceHandler` for one socket)
  - Virtual connections: `CONNECT` / `CONNECTED` / `CLOSE` with protocol
    version negotiation, connection policy and close reasons
  - Namespace routing; messages without a virtual connection are dropped
    (transport `tp.*` namespaces and broadcasts excepted)
  - Heartbeat: answers `PING`, optional keep-alive and liveness timeout with
    an injectable clock (defaults 5s / 10s, as in Chromium; openscreen leaves
    this to the embedder)
- Device authentication (`tp.deviceauth`)
  - Sender `Authenticate` and receiver `DeviceAuthResponder`
  - Pluggable `Verifier`; `CertVerifier` ports openscreen's checks: TLS
    certificate lifetime, nonce echo, device chain, SHA1/SHA256 policy,
    RSASSA-PKCS1v15 / PSS signature over `nonce || TLS cert DER`
  - Device chain validation ported from openscreen's `FindCertificatePath`
    (path building by subject name, RSA >= 2048, SHA-1/SHA-2 RSA
    signatures as genuine devices use, key usage, basic constraints,
    path length, name constraints); checked against openscreen's
    Chromecast/Nexus Player/Vizio test chains
  - `GoogleCastRoots()`: the Cast Root CA and Eureka Root CA built into
    openscreen
  - `InsecureVerifier` for tests and trusted networks
- Discovery: `_googlecast._tcp` browse / advertise with `id`, `ve`, `ca`,
  `st`, `fn`, `md` TXT records
- Pure Go, no Cgo

### Usage

```go
sock, _ := cast.Dial(ctx, receiver.Addr(), nil)
ch := cast.NewChannel(sock, &cast.Config{Heartbeat: cast.DefaultHeartbeat})
ch.Handle(cast.ReceiverNamespace, func(ch *cast.Channel, m *cast.CastMessage) { /* ... */ })
ch.Start()

err := cast.Authenticate(ctx, ch, &cast.CertVerifier{Roots: cast.GoogleCastRoots()})
vc := cast.VirtualConnection{LocalID: cast.PlatformSenderID, PeerID: cast.PlatformReceiverID}
err = ch.Connect(ctx, vc)
err = ch.SendString(vc, cast.ReceiverNamespace, `{"type":"GET_STATUS","requestId":1}`)
```

See `example_test.go` for a loopback sender and receiver.

### Limitations

- CRLs are not checked (openscreen senders treat them as optional too), and
  the audio-only device policy is not reported.
- Application protocols on top of the channel (receiver/app control, media,
  mirroring) are out of scope.
- No adapter to `openscreen-go`'s `NetworkTransport`: Cast is a
  message-oriented channel multiplexed by namespace and endpoint ids, while
  the OSP interfaces are QUIC stream oriented.

### Interop tests

`interop_test.go` (build tag `interop`) runs against the standalone Cast
binaries of Chromium's [openscreen](https://chromium.googlesource.com/openscreen/):

```sh
# in an openscreen checkout with a configured build dir
ninja -C out/debug cast_receiver cast_sender
OPENSCREEN_OUT_DIR=/path/to/openscreen/out/debug go test -tags interop -run Interop -v ./cast/
```

`OPENSCREEN_OUT_DIR` defaults to `/work/openscreen/out/debug`; tests skip
when a binary is missing. They use free ports, temp dirs for credentials,
and kill the C++ process group on exit. Scenarios:

- Go sender -> `cast_receiver` (credentials from `cast_receiver -g`,
  discovery off, dialled on 127.0.0.1): TLS, PING/PONG, device auth
  verified against the generated root, CONNECT, `GET_STATUS`,
  `GET_APP_AVAILABILITY`, `LAUNCH` of the mirroring app (`0F5096E8`),
  CONNECT to its `transportId`, `OFFER` -> `ANSWER` on
  `urn:x-cast:com.google.cast.webrtc`, `STOP`, keep-alive.
- `_googlecast._tcp` browse finds `cast_receiver` (by a unique `-f` name).
- `cast_sender` -> Go receiver, dialled directly and via mDNS discovery of
  an `Advertise`d receiver: TLS, device auth with a Go developer PKI
  (`cast_sender -d root.pem`), `LAUNCH`, app CONNECT and the streaming
  `OFFER`, which the test declines (Cast Streaming/RTP is out of scope).

Notes: openscreen's sender never answers `PING`, so a Go receiver must not
enable a liveness timeout against it. `Advertise` publishes all interface
addresses, so listen on all of them.

### Roadmap

- [x] Interop testing against openscreen's standalone sender/receiver
- [ ] Interop testing against a real Cast device
- [ ] CRL verification
- [ ] Receiver / media namespace helpers

### License

MIT License - see [LICENSE](../LICENSE) for full text
