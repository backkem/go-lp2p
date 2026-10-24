# Interop: openscreen-go (ospc) vs. Chromium Open Screen `osp_demo`

Exploratory interop between `openscreen-go/network` (package `ospc`) and the
Open Screen Library's C++ OSP stack, driven through its `osp_demo` binary.

- C++: `/work/openscreen` at 2a78d154, `out/debug` (debug build: DCHECKs abort).
- Go: this repo, quic-go v0.39.0, fxamacker/cbor v2.9.0, grandcat/zeroconf v1.0.0.
- Spec: w3c/openscreenprotocol `network.bs`, `application.bs`, `*.cddl` (main @ 71b1412).

**Bottom line:** mDNS discovery, QUIC/TLS 1.3 with ALPN `osp`, and Presentation API
application messages (availability, start, connection messages in both directions)
interoperate on the wire. Getting there needs a Go peer that follows osp_demo's
conventions rather than the spec. Neither unmodified implementation completes a
connection with the other. The blockers sit on both sides:

1. **Fingerprint definition (Chromium).** osp_demo's `fp` is SHA-256 over the whole
   DER certificate, not the SPKI hash `network.bs` specifies. Each side rejects the
   other's fingerprint.
2. **Zero-length connection IDs (Chromium, quiche integration).** osp_demo's QUIC
   *server* is broken for any RFC 9000 client, including C++ to C++.
3. **No authentication and no agent-info (Chromium).** osp_demo never sends or answers
   `agent-info-request` or any `auth-*` message, and it accepts application messages
   from unauthenticated peers. The SPAKE2 classes exist but nothing instantiates them.
   ospc can't get past its metadata/auth phase with it.
4. **ospc strictness beyond the spec (Go).** `Dial` requires a TXT `sn` key that is not
   in the spec, and both `Dial` and `Listener` do x509 hostname verification that
   osp_demo's certificate (no SAN) can't pass.

So SPAKE2 interop can't be tested: the C++ side has no wired-up authentication.

## How to run

```sh
ninja -C /work/openscreen/out/debug osp_demo
OPENSCREEN_OUT_DIR=/work/openscreen/out/debug \
OSP_DEMO_LOG_DIR=/tmp/osp-logs \   # optional: full osp_demo logs per test
  go test -tags interop -v -count=1 ./openscreen-go/test/osp-demo-interop/
```

- The tests skip when `$OPENSCREEN_OUT_DIR/osp_demo` is missing.
- Each numbered subtest is one stage. A stage PASSes when it interoperates and SKIPs
  with a `BLOCKER (...)` reason at a known incompatibility. Later stages skip when a
  stage they depend on didn't pass. The `-v` output is the progress gauge.
- The harness takes care of osp_demo's quirks:
  - It runs osp_demo in a temp cwd containing `osp/impl/quic/certificates/`. Without
    that directory osp_demo crashes, because it writes its certificate cwd-relative.
  - It reads osp_demo's log from the `_recv_fifo`/`_cntl_fifo` FIFO.
  - It drives osp_demo through stdin.
  - It kills osp_demo's process group on cleanup, with Pdeathsig as a backstop.
- Instance names carry a random suffix, and tests only act on their own instances
  because other OSP agents may be on the LAN.
- The osp_demo publisher always binds port 6667 on the first address of each
  non-loopback interface (`osp_demo.cc:592-607`). Only one publisher can run per host,
  and there is no loopback-only mode.

Two client paths run per direction:

- **ospc**: the public `openscreen-go/network` API, unmodified.
- **raw**: quic-go, zeroconf and ospc's exported framing (`WriteTypeKey`/`ReadTypeKey`/
  `DecodeCBOR`) with generic CBOR maps. It is configured the way osp_demo expects: DER
  fingerprint, no auth, deterministic CBOR key order. It shows how far the wire protocol
  gets once the blockers above are sidestepped.

## Per-stage results

Recorded result (2026-10-24, two consecutive runs, identical):
`ok  github.com/backkem/go-lp2p/openscreen-go/test/osp-demo-interop  15.7s`.
The package passes because blocked stages SKIP. In total 9 stages pass and 10 skip
(7 at a blocker, 3 dependent).

### osp_demo publisher (receiver) → Go controller: `TestOSPDemoPublisherToGo`

| Stage | Result | Evidence |
|---|---|---|
| 01 mDNS, ospc `Discoverer` | PASS | Discovered by nickname. TXT has `fp` and `at`. **No `mv`** (spec-required) and no `sn`. |
| 02 ospc `Dial` | SKIP (Go) | `ospc Dial failed: failed to get sn record: no value for key sn`. |
| 03 mDNS, raw resolve | PASS | `port 6667`, `v4 [10.89.16.2]`. |
| 04 QUIC + TLS, raw | PASS | `TLS 0x0304, ALPN "osp"`. Peer cert: `CN=Open Screen Certificate`, no SAN, serial 1. `TXT fp` == DER-cert hash != SPKI hash → spec drift. |
| 05 agent-info | SKIP (C++) | No `agent-info-response`. osp_demo log: `no message handler matched` for type 10. |
| 06 auth-capabilities | SKIP (C++) | osp_demo has already aborted (see 07). Source reading shows no `auth-*` messages either way. |
| 07 URL availability | SKIP (C++) | osp_demo aborts: `Server: Ignoring packet from unexpected server connection ID …`, then `quic_connection.cc:1075 Check failed: Perspective::IS_SERVER != perspective_`. |
| 08-10 start / message / terminate | SKIP | Depend on 07. |

Results were stable over 8+ runs. Each run had exactly one client of osp_demo (our
quic-go port), so this is not cross-talk.

### Go receiver → osp_demo controller: `TestGoToOSPDemoController`

**`ospc-listener`** (unmodified `ospc.Listen`):

| Stage | Result | Evidence |
|---|---|---|
| 01 C++ discovers Go via mDNS | PASS | osp_demo: `found! GoIopOspc…`. |
| 02 QUIC + TLS | SKIP (fp) | osp_demo: `Cert chain verification failed: Certificate does not match any fingerprint`, then `TLS handshake failure … certificate unknown`. ospc advertises the spec SPKI fingerprint. |
| 03 ospc `Accept` | SKIP | Never returns. It would also need `agent-info-response` and `auth-capabilities` from the peer. |

**`raw-receiver`** (quic-go server, zeroconf TXT `fp=<DER hash>`, `at`):

| Stage | Result | Evidence |
|---|---|---|
| 01 C++ discovers Go | PASS | `found! GoIopRaw…`. |
| 02 QUIC + TLS | PASS | ALPN `"osp"`, SNI `"GoIopRaw….local"`. osp_demo presents a client cert (`CN=Open Screen Certificate`). osp_demo: `Controller succeed to build the underlying connection to: GoIopRaw…`. |
| 03 URL availability (14/15) | PASS | `<-- type=14 body=map[0:0 1:[https://example.com/] 2:20000000 3:1]`, `--> type=15 map[0:0 1:[0]]`. osp_demo: `available! GoIopRaw…`. |
| 04 presentation start (104/105) | PASS | `<-- type=104 map[0:2 1:GoIopRaw…:https://example.com/ 2:https://example.com/ 3:[]]`, `--> type=105 map[0:2 1:1 2:7]`. osp_demo: `presentation connection connected`. |
| 05 connection messages (16) | PASS | `<-- type=16 map[0:7 1:hello from cpp]`. osp_demo: `got message: hello from go`. |
| 06 network messages / auth | SKIP (C++) | Types received over the whole connection: only 14, 104, 16. No `agent-info-request` and no `auth-*`. |

Our quic-go server uses non-empty CIDs, so the CID bug below doesn't trigger in this
direction. With the ospc listener, which uses zero-length server CIDs, osp_demo logs
`Accepting packet with new connection ID 0` and continues. It then fails on the
fingerprint check.

## Comparison

| Aspect | Spec (network.bs) | ospc (Go) | osp_demo (C++) |
|---|---|---|---|
| DNS-SD service | `_openscreen._udp` | same | same (`osp_constants.h:17`) |
| TXT `fp` | base64 SHA-256 of **SPKI** (RFC 7469) | SPKI | base64 SHA-256 of **whole DER cert** (`quic_agent_certificate.cc:80`) |
| TXT `mv` | required, varint | set. Parsed if present, tolerated if absent | **not set** (constant defined, unused) |
| TXT `at` | required | 9 random chars. Sent in the first auth message and checked on receipt (wired in after this table was first written) | 16 chars (`quic_server.cc:32`). Watcher requires it, nothing checks it |
| TXT `sn` | — (not specified, see #293/#365) | set, and **required by `Dial`** | absent |
| Other TXT | — | `fn=TODO:Remove?` | — |
| ALPN | `osp` | `osp` | `osp` |
| TLS | 1.3 | 1.3 only | 1.3 (quiche/BoringSSL) |
| SNI | agent hostname `<b64 sn>.<instance>.<domain>` | agent hostname (URL-safe b64, #365) | `<instance>.local` |
| Cert subject / issuer | CN=agent hostname / CN=model-name | CN=hostname + SAN=hostname / CN=display name | CN=`Open Screen Certificate` both, no SAN, `CA:TRUE`, serial 1, valid 1 year |
| Client verifies server | `fp` must match | `fp` (SPKI) + serial==`sn` + x509 `Verify(DNSName=hostname)` | quiche `WebTransportFingerprintProofVerifier`: DER hash, validity ≤3650 days, key type; hostname ignored |
| Server verifies client | both present certs | `RequireAnyClientCert` + x509 `Verify(DNSName=peer CN)`. Would reject osp_demo's SAN-less cert (code reading; not reached) | accepts any (`open_screen_server_session.cc:22-31`) |
| QUIC connection IDs | "should be zero length" (network.bs:263) | listener: zero-length server CIDs. Dialer: quic-go defaults | zero-length via `DeterministicConnectionIdGenerator(0)`. **Broken**, see below |
| Framing | varint type key + CBOR, unidirectional stream per message (group) | network phase: one uni stream per message. **`application` package: bidirectional streams** | uni stream per protocol connection, many messages |
| CBOR map key order | unspecified | struct field order (ascending in practice) | tinycbor decoders **require ascending keys**: `parse error: -519` otherwise |
| Unknown type key | close connection with error 404 | stops reading that stream, no close | buffers unhandled, no close (`message_demuxer.cc:281-296`) |
| Type keys | appendix | same 47 | same 47 (identical numbers) |
| CDDL shapes | current | follows spec | older copy: `presentation-connection-close-event` keys 2/3 swapped (`connection-count`/`error-message`); `remote-playback-state.supports` keys 1-5 vs 0-4; spec adds `null` to `epoch`/`duration`/`resolution` |
| Metadata discovery | `agent-info-request`/`-response` | sends `auth-capabilities` + `agent-info-request` on connect, **waits for both** from the peer (listener gives up after 10s; `Dial` bounded by ctx) | never sends 10, no handler for 10 |
| Authentication | SPAKE2 mandatory before other messages | SPAKE2 implemented | `authentication_{alice,bob}` exist but unused. **No auth**; app messages accepted unauthenticated |

## The connection-ID bug (Chromium side)

Evidence: `-v` logs from both directions, quiche and quic-go sources.

- osp's QUIC factories use `DeterministicConnectionIdGenerator` with
  `expected_connection_id_length = 0` (`osp/impl/quic/quic_connection_factory_base.h:61-62`,
  `quic_connection_factory_server.cc:80`).
- The dispatcher drops the empty replacement (`quic_dispatcher.cc:1334-1337`), so the
  server session keeps the client's original DCID (14-20 bytes) as its own CID. The
  framer then parses short headers with that length (`quic_connection.cc:203-204`).
- The server still issues `NEW_CONNECTION_ID{seq 1, cid: empty}`. RFC 9000 requires
  CIDs in this frame to be 1-20 bytes.
- **quiche client** (C++ ↔ C++): rejects that frame and closes the connection with
  `QUIC_INVALID_NEW_CONNECTION_ID_DATA: Connection IDs in NEW_CONNECTION_ID cannot be empty`
  about 50 ms after the handshake. osp_demo's own controller → publisher path therefore
  doesn't work: `QUIC stream is closed`, and the availability request is silently dropped.
- **quic-go client**: accepts the frame and switches to the new CID right after the
  handshake (`conn_id_manager.go:196-198`). It then sends short-header packets with an
  empty DCID. The server reads 14-20 bytes of payload as the DCID, rejects it as
  unexpected, and reaches a client-only branch guarded by
  `QUICHE_DCHECK_NE(IS_SERVER, perspective_)` (`quic_connection.cc:1067-1076`). A debug
  build aborts there; a release build would silently drop every later packet.
- quic-go v0.39 has no setting that disables the switch.
- Fix (C++): use a non-zero CID length, e.g. `kQuicDefaultConnectionIdLength` as
  `platform/impl/quiche_web_transport_server.cc:102` already does. Alternatively,
  implement truly zero-length server CIDs: set the framer length to 0 and issue no
  NEW_CONNECTION_ID.
- Spec: network.bs:263 could note that agents using zero-length CIDs must not issue
  NEW_CONNECTION_ID (RFC 9000 §5.1.1).

## What would need to change

**Go (ospc).** Documented, not changed here. Since this was written, the connection
lifecycle rework on main has addressed some of these; they are marked below.

- `Dial` (`dialer.go`):
  - Don't require TXT `sn`; it isn't in the spec.
  - Verify the server by `fp` pinning only, as network.bs §TLS requires. The serial
    number and x509 `DNSName` checks go beyond the spec and fail against certs that
    lack SAN or the spec CN.
- `Listener` (`listener.go`): `peerCert.Verify(DNSName: peer CN)` fails for any client
  certificate without a SAN. The spec doesn't ask the server to verify a hostname.
  The listener could pin nothing, or verify after authentication.
- `exchangeInfo`/`checkAgentInfoComplete` (`connection_network.go`): the handshake
  phase waits for the peer's `auth-capabilities` and `agent-info-response`. The
  listener now gives up after 10s and `Dial` is bounded by its context, but there is
  still no clear error for peers that don't speak the metadata/auth phase.
- Unknown type key: network.bs says to close the connection with application error
  404; ospc deliberately doesn't while the protocol is in flux and only stops reading
  that stream.
- `application` package: sends on bidirectional streams. application.bs:337 requires
  unidirectional streams, which is what osp_demo sends and expects.
- Auth initiation token: now wired in on main (the advertised `at` is sent in the
  first auth message and checked on receipt).

**Chromium (Open Screen Library)**

- Fingerprint: SPKI hash per network.bs, in both `QuicAgentCertificate` and the pinned
  verifier.
- The zero-length connection ID bug above. It also breaks C++ ↔ C++.
- Implement metadata discovery (`agent-info-request` handler) and SPAKE2
  authentication. The `AuthenticationAlice`/`AuthenticationBob` classes exist; they
  need wiring, plus gating of application messages until authentication succeeds.
- TXT `mv`. Certificate fields per network.bs: CN=agent hostname, issuer=model-name,
  serial per spec. SNI = agent hostname.
- Sync `osp/msgs/osp_messages.cddl` with the spec (presentation-connection-close-event,
  remote-playback-state).
- Optional: accept CBOR maps in any key order, or the spec should mandate
  deterministic encoding.
- Demo ergonomics: the cwd-relative certificate path crashes when missing; logs go only
  to a FIFO; the publisher port is fixed at 6667.

**Spec**

- `sn` TXT key and the base64 alphabet in hostnames (#293, #365). ospc's `sn` is a
  local workaround.
- Zero-length CID guidance (above).
- CBOR map key order: say whether deterministic encoding (RFC 8949 §4.2) is required.
  At least one implementation depends on it.
