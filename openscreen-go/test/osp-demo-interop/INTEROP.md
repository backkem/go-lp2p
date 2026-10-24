# Interop: openscreen-go (ospc) vs. Chromium Open Screen `osp_demo`

Interop between `openscreen-go/network` (package `ospc`) plus `openscreen-go/application`
and the Open Screen Library's C++ OSP stack, driven through its `osp_demo` binary.

- C++: `/work/openscreen`, branch `osp-spec-gaps` at 33430be1, `out/debug/osp_demo`
  built 2026-10-24 13:44. Debug build: DCHECKs abort.
- Go: branch `osp-spec-gaps`. quic-go v0.39.0, fxamacker/cbor v2.9.0,
  grandcat/zeroconf v1.0.0, backkem/spake2-go ef71e299b11a.
- Spec: w3c/openscreenprotocol `network.bs`, `application.bs`, `*.cddl` (main @ 71b1412).

**Bottom line.** With **one** accepted deviation on each side, the SPAKE2
P-256 suite (ospc `InteropQuirks.Spake2SuiteP256`, osp_demo
`--spake2_suite=p256`), both directions pass every stage: mDNS, QUIC/TLS 1.3
with SPKI fingerprint pinning, metadata discovery, spec SPAKE2 authentication
(SHA-512 password hash, client/server fingerprint identities, Appendix B PSK),
and the full Presentation API flow (availability, start, connection messages
both ways, termination), whichever side presents the PSK. No other quirk and no
other non-spec osp_demo flag (`--spake2_identities`, `--spake2_password_hash`,
`--no_auth`, `--allow_unauthenticated`) is used.

## How to run

```sh
ninja -C /work/openscreen/out/debug osp_demo
OPENSCREEN_OUT_DIR=/work/openscreen/out/debug \
OSP_DEMO_LOG_DIR=/tmp/osp-logs \   # optional: osp_demo and ospc logs per test
  go test -tags interop -v -count=1 ./openscreen-go/test/osp-demo-interop/
```

- The tests skip when `$OPENSCREEN_OUT_DIR/osp_demo` is missing.
- Each numbered subtest is one stage. A stage PASSes when it interoperates and SKIPs
  with a `BLOCKER (...)` reason at a known incompatibility. A stage that depends on
  an earlier one is skipped when that one didn't pass. The `-v` output is the
  progress gauge.
- Authentication always runs. osp_demo is started with
  `--spake2_suite=p256 --psk=<PSK>`, ospc agents with `Spake2SuiteP256`. The PSK
  is `$OSP_DEMO_PSK` or a random 6-digit one per run; since both sides know it,
  either can be the PSK presenter.
- Who presents (network.bs: lower `psk-ease-of-input`, the server on a tie):
  `OSP_GO_EASE_OF_INPUT` sets the ospc agents' ease of input (default 0),
  `OSP_DEMO_EASE_OF_INPUT` osp_demo's `--psk_ease_of_input` (default 50).
- Harness mechanics: osp_demo runs in a temp cwd containing
  `osp/impl/quic/certificates/`. Its log is read from the `_recv_fifo`/`_cntl_fifo`
  FIFO, and it is driven through stdin. Cleanup kills its process group.
  Instance names carry a random suffix. The osp_demo publisher always binds port
  6667, so only one can run per host.

### Test paths

- **ospc** (`ospc_test.go`): `TestOSPCDialPublisher` and `TestOSPCListenController`
  use the public `ospc` API (`Discoverer`, `DiscoveredAgent.Dial`, `Listen`/`Accept`,
  `AuthenticatePSK`) and `application.ApplicationConnection` for the Presentation
  API messages.
  - Each direction starts with **no quirks**. It enables a quirk only when an
    attempt fails the way that quirk addresses:
    - fingerprint mismatch: `AcceptCertificateFingerprint` when dialing,
      `AdvertiseCertificateFingerprint` when listening;
    - `ErrNoAgentInfo`: `TolerateMissingAgentInfo`.
  - `AllowSkipAuthentication` is only used when real authentication isn't
    possible.
  - Each stage logs `QUIRKS NEEDED: ...`, and the test ends with a
    `STAGE SUMMARY` table.
- **raw** (`interop_test.go`): `TestRawProbePublisher` and `TestRawProbeController`
  use quic-go, zeroconf and ospc's exported framing with generic CBOR maps. They
  have no ospc state machine and no authentication. This is the fallback gauge for
  the wire protocol.
  - The raw receiver advertises the SPKI `fp` first. It falls back to the
    whole-certificate hash only if osp_demo doesn't connect.

## Per-stage results

Recorded 2026-10-24 with `OSP_DEMO_PSK=123456`,
`go test -tags interop -count=1 -v -run TestOSPC`. Flags on each side, every run:
ospc `InteropQuirks{Spake2SuiteP256: true}` (plus `AllowSkipAuthentication`,
which only enables the API and was not used); osp_demo
`-v --spake2_suite=p256 --psk=123456` (plus `--psk_ease_of_input=0` in run C).

| Run | Env | Dial direction: PSK presenter (SPAKE2 Alice) | Listen direction: PSK presenter (SPAKE2 Alice) |
|---|---|---|---|
| A | defaults (Go ease 0, osp_demo 50) | ospc (ospc, `psk-shown`) | ospc (ospc, `psk-shown`) |
| B | `OSP_GO_EASE_OF_INPUT=100` | osp_demo (osp_demo, `psk-shown`) | osp_demo (osp_demo, `psk-shown`) |
| C | `OSP_DEMO_EASE_OF_INPUT=0` (tie: server presents) | osp_demo (server) | ospc (server) |

The Bob side answered with `psk-input` in every run.

**osp_demo publisher → ospc `Dial`** (`TestOSPCDialPublisher`), runs A, B, C

| Stage | A | B | C | Quirks |
|---|---|---|---|---|
| 01 mDNS discover | PASS | PASS | PASS | none |
| 02 Dial (TLS + agent-info + auth-capabilities) | PASS | PASS | PASS | none |
| 03 authenticate (SPAKE2) | PASS | PASS | PASS | `Spake2SuiteP256` |
| 04 URL availability | PASS | PASS | PASS | none |
| 05 presentation start (26-char presentation-id) | PASS | PASS | PASS | none |
| 06 connection message (Go → C++, echo C++ → Go) | PASS | PASS | PASS | none |
| 07 termination (controller-initiated) | PASS | PASS | PASS | none |

**osp_demo controller → ospc `Listen`** (`TestOSPCListenController`), runs A, B, C

| Stage | A | B | C | Quirks |
|---|---|---|---|---|
| 01 connect (mDNS, TLS, info exchange, `Accept`) | PASS | PASS | PASS | none |
| 02 authenticate (SPAKE2) | PASS | PASS | PASS | `Spake2SuiteP256` |
| 03 URL availability | PASS | PASS | PASS | none |
| 04 presentation start (presentation-id validated, ≥16 chars) | PASS | PASS | PASS | none |
| 05 connection message (C++ → Go, Go → C++) | PASS | PASS | PASS | none |
| 06 termination (controller-initiated, new) | PASS | PASS | PASS | none |

Not exercised against osp_demo: the PSK consumer initiating with
`psk-needs-presentation`. osp_demo never initiates as consumer, and as presenter
it initiates as soon as it has the peer's auth-capabilities, before ospc's
consumer is given the PSK. ospc covers it Go-to-Go (`TestAuthenticationFlows`),
including the both-initiate race, which it resolves the way osp_demo does (the
presenter's initiation wins).

### Raw probe (fallback gauge)

`interop_test.go` speaks the wire protocol without the ospc state machine and
without authentication. Since osp_demo correctly holds back application
messages from unauthenticated peers, its presentation stages SKIP; it remains a
gauge for discovery, QUIC/TLS and the network-phase messages only. Run A
(`-run TestRawProbe`): publisher 01, 03, 04, 05 (agent-info) PASS, 06
auth-capabilities and 07-10 presentation SKIP; controller (SPKI fp) 01, 02, 06
PASS, 03-05 SKIP. Its
presentation-ids now come from `application.NewPresentationID` (the previous
`"go-iop-" + 8 chars` was 15 characters, below application.bs's minimum of 16;
osp_demo answers such an id with `invalid-presentation-id` (11)).

## Interop quirks (`ospc.AgentConfig.InteropQuirks`)

All quirks are off by default. Enabling any of them logs a warning when the agent
is created.

| Quirk | Effect | Spec rule violated |
|---|---|---|
| `Spake2SuiteP256` | SPAKE2 with the RFC 9382 P-256 suite (SPAKE2-P256-SHA256-HKDF-HMAC) and otherwise network.bs parameters. Without it `AuthenticatePSK` fails with `ErrSpake2SuiteUnsupported`. **The accepted deviation**: neither side implements edwards25519 | network.bs "Authentication with SPAKE2": "Elliptic curve is edwards25519" |
| `AcceptCertificateFingerprint` | Dial also accepts `fp` = base64 SHA-256 of the whole DER certificate | network.bs "Computing the Agent Fingerprint" (SPKI per RFC 7469) and "TLS 1.3" (computed fingerprint must match `fp`) |
| `AdvertiseCertificateFingerprint` | Listener advertises the whole-certificate hash as `fp`. PeerID and SPAKE2 identities stay SPKI | network.bs "Computing the Agent Fingerprint" |
| `TolerateMissingAgentInfo` | Dial/Accept proceed after a 2s grace period without agent-info-response / auth-capabilities. Remote `Info()` is then nil and equal auth capabilities are assumed | network.bs metadata discovery / authentication (peer must answer agent-info-request; auth starts from exchanged auth-capabilities) |
| `AllowSkipAuthentication` | Enables `UnauthenticatedConnection.SkipAuthentication()`, which hands the connection to the application protocol without SPAKE2 and logs a warning. `Connection.Authenticated()` reports false | network.bs "Authentication": SPAKE2 is mandatory before application messages. **Insecure, debugging only** |

## Spec-compatible changes (on by default)

- **`Dial` doesn't need `sn`.** `sn` isn't in network.bs (#293). If it is
  present, the TLS server_name is the agent hostname. Without it, the hostname
  can't be computed, so the server_name is `<encoded instance>.<encoded domain>`.
- **Server verified by `fp` only.** network.bs "TLS 1.3" requires only that "the
  agent fingerprint calculated for the received agent certificate must match the
  `fp` mDNS TXT record". The serial-number and x509 `DNSName` checks are gone; no
  spec rule required them.
- **Listener doesn't verify a hostname in client certificates.** The spec defines
  no check of the listening agent's certificate beyond its presence ("Both
  advertising agents and listening agents must use the agent certificate"). The
  peer is verified by SPAKE2, whose identities are bound to the fingerprints.
- **`ErrNoAgentInfo`.** When the peer doesn't finish metadata discovery within
  10s, or within the `Dial` context, the error names the missing messages.
- **agent-info-request resent** every 1.5s until answered, with responses to any
  of the requests accepted. osp_demo drops a request that arrives before its
  network phase starts.
- **Unidirectional application streams** (application.bs / network.bs "for each
  message, the OSP agent must write into a unidirectional QUIC stream"):
  - ospc routes incoming streams by the first type key. agent-status messages go
    to the keep-alive; agent-info/auth go to the network phase while it runs;
    everything else goes to `Connection.AcceptStream`.
  - Application streams that arrive before the hand-off are held, up to 64, and
    delivered after it. A peer that completes authentication first may already
    send them.
  - `Connection.OpenUniStream` is new.
  - `application.ApplicationConnection` sends every message on its own
    unidirectional stream, reads all incoming streams itself, and correlates
    responses by request-id (`Request`). Other messages come from
    `ReceiveMessage`.
- **Deterministic CBOR:** map keys in RFC 8949 §4.2.1 order (ascending integer
  keys). The spec doesn't require an order, so this is spec-neutral; osp_demo's
  decoders reject other orders.
- **Nil slices encoded as empty arrays**, not `null`: CDDL `[* text]` doesn't
  allow null. osp_demo rejected ospc's agent-info-response (`parse error: -1`)
  because of a null `locales`.
- **Dial prefers IPv4.** mDNS gives link-local IPv6 addresses without a zone,
  which can't be dialed. Before this change, ospc dialed osp_demo's `fe80::`
  address and timed out.
- **Missing `mv` stays tolerated.** network.bs obliges the *advertising* agent to
  include it but sets no rule for a listener that finds it missing.

## SPAKE2

Comparison of `agent_authenticator.cc` / `util/crypto/spake2.cc` with
`connection_network.go` / `psk.go`, both at their defaults plus the P-256 opt-in:

| Item | network.bs | osp_demo | ospc |
|---|---|---|---|
| Curve | edwards25519 | P-256 (`--spake2_suite=p256`) | P-256 (`Spake2SuiteP256`) |
| Transcript hash / KDF / MAC | SHA-256 / HKDF / HMAC | same | same |
| Password hash | SHA-512 | SHA-512 | SHA-512 (`newSpake2`; spake2-go has a single hash, so ospc builds the instance with SHA-512 and switches the transcript hash back to SHA-256) |
| `pw` bytes | "the PSK presented to the user"; Appendix B: remove dashes and leading zeros | canonical decimal digits | canonical decimal digits (`ParsePSK`) |
| Identities A/B | fingerprints of client / server | same (base64 text) | same (base64 text) |
| Presenter | lower ease of input; server on tie | same | same |
| Presenter initiates | `psk-shown` + pA; reply `psk-input` + pB | same | same |
| Consumer initiates | `psk-needs-presentation` + pA; reply `psk-input` + pB | answers it; never initiates | initiates when given the PSK before the presenter's handshake; answers it |
| Both initiate | unspecified | presenter's wins | presenter's wins |
| confirmation-value | CDDL `bytes .size 64` | 32 bytes (HMAC-SHA-256) | 32 bytes; size not checked (spec inconsistency, commented) |
| Bob's cB | after pA, pB known | sent with pB | sent after verifying cA (spake2-go API); Alice doesn't wait for it |

## Comparison (current)

| Aspect | Spec | ospc | osp_demo (current) |
|---|---|---|---|
| TXT `fp` | SPKI SHA-256 | SPKI | SPKI (ea1b2da1) |
| TXT `mv` | required | set; tolerated if absent | set (2639d78d) |
| TXT `sn` | — (#293) | set; optional for `Dial` | absent |
| SNI | agent hostname | agent hostname, or instance+domain without `sn` | `<instance>.local` |
| Server verification | `fp` match | `fp` match only | `fp` match |
| Client cert verification | — | presence only | — |
| QUIC CIDs | "should be zero length" | listener zero-length | non-zero (6a9786ae) |
| Framing | uni stream per message (group) | uni streams in both phases | uni stream per protocol connection |
| Metadata discovery | agent-info-request/response | sends, answers, resends request | sends, answers; drops a request that arrives before its network phase |
| Authentication | SPAKE2 mandatory, edwards25519 | SPAKE2 per spec, P-256 opt-in | SPAKE2 per spec, P-256 opt-in |
| App messages before auth | not allowed | held until hand-off | held back until authenticated |
| Unknown type key | close with 404 | not implemented (deliberately); the deprecated `auth-spake2-need-psk` (99001) is gone | buffered |
| presentation-id | ≥ 16 ASCII chars | `application.ValidatePresentationID` / `NewPresentationID` | enforced (result 11) |
| Extension capability IDs (≥1000) | allowed | sends 1100, 1200 | decoded without range check |

## Remaining work

**Both / spec**
- edwards25519 (network.bs) vs. the P-256 opt-in on both sides; BoringSSL has
  no general edwards25519 group operations. Owner decision: P-256 is the
  accepted, documented deviation.
- Spec issues: confirmation-value `.size 64` vs. 32-byte HMAC-SHA-256; the
  consumer-initiated flow needs pA, i.e. the PSK, before the presenter has shown
  it; both-initiate resolution unspecified; Appendix B padding rule ("3 - len(N)
  mod 3" pads a full group, 9 digits uncovered); `pw` encoding only implied by
  Appendix B; `sn` / hostname computation (#293, #365); CBOR map key order;
  zero-length CID guidance.

**Go**
- `Listener.Close` closes connections already returned by `Accept`, contrary to
  its doc comment (quic-go v0.39 `Listener.Close`).
- Unknown type key → close with 404: not implemented, by owner decision.
- Older Go/Rust peers (SHA-256 password hash, presenter/consumer identities,
  `auth-spake2-need-psk`) no longer authenticate with ospc; the Rust interop in
  `application/test/integration` needs the Rust side updated.

**Chromium**
- Drop of an agent-info-request that arrives before the network phase starts
  (ospc works around it by resending, which is spec-compatible).
- Older items: certificate fields per network.bs (CN=agent hostname), SNI = agent
  hostname, cwd-relative certificate path, fixed port.
