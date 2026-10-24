# Interop: openscreen-go (ospc) vs. Chromium Open Screen `osp_demo`

Interop between `openscreen-go/network` (package `ospc`) plus `openscreen-go/application`
and the Open Screen Library's C++ OSP stack, driven through its `osp_demo` binary.

- C++: `/work/openscreen`, branch `osp-spake2-interop` at 2639d78d plus uncommitted
  work in progress (`agent_authenticator.cc`, message gate), `out/debug` built
  2026-10-24 12:32. Debug build: DCHECKs abort.
- Go: branch `osp-interop-relax`. quic-go v0.39.0, fxamacker/cbor v2.9.0,
  grandcat/zeroconf v1.0.0, backkem/spake2-go ef71e299b11a.
- Spec: w3c/openscreenprotocol `network.bs`, `application.bs`, `*.cddl` (main @ 71b1412).

**Bottom line.** Both directions now get through mDNS, QUIC/TLS 1.3 with SPKI
fingerprint pinning, and the metadata/auth-capabilities exchange using the real
ospc `Dial`/`Listen` paths, **with no interop quirks**. The one remaining blocker
is SPAKE2. ospc's SPAKE2 parameters don't match network.bs or osp_demo (see
[SPAKE2](#spake2-the-remaining-blocker)). osp_demo now correctly holds back
application messages from unauthenticated peers, so no application stage gets
through. With the `AllowSkipAuthentication` quirk ospc hands the connection over
unauthenticated, but osp_demo still holds the messages back. That is the
spec-correct behavior.

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
- `OSP_DEMO_TRY_SPAKE2=1` attempts real SPAKE2 with the PSK that osp_demo shows as
  presenter, or with `$OSP_DEMO_PSK`. It is expected to fail until the SPAKE2
  mismatches are fixed.
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

### Before / after (ospc paths)

"Before" is ospc on main (f7b7205) against the osp_demo of the first exploration
(2a78d154: whole-certificate `fp`, zero-length CID bug, no network phase).
"After" is this branch against the current osp_demo, recorded 2026-10-24 in two
consecutive identical runs: `ok ... 33.4s`.

**osp_demo publisher → ospc `Dial`** (`TestOSPCDialPublisher`)

| Stage | Before | After | Quirks needed (after) | Evidence (after) |
|---|---|---|---|---|
| 01 mDNS discover | PASS | PASS | none | TXT `fp` (SPKI), `mv`, `at`; no `sn`. |
| 02 Dial (TLS + agent-info + auth-capabilities) | SKIP (Go: `failed to get sn record`) | PASS | none | fp pinning only; agent-info-request resent once (see below). |
| 03 authenticate | n/a | SKIP (Go: SPAKE2 params) | `AllowSkipAuthentication` | osp_demo presents and sends `auth-spake2-handshake`; ospc can't complete it. |
| 04 URL availability (14/15) | n/a | SKIP (auth) | none | osp_demo: `holding back message type 14 from instance 1`. |
| 05-07 start / message / terminate | n/a | SKIP | — | depend on 04 |

**osp_demo controller → ospc `Listen`** (`TestOSPCListenController`)

| Stage | Before | After | Quirks needed (after) | Evidence (after) |
|---|---|---|---|---|
| 01 connect (mDNS, TLS, info exchange, `Accept`) | SKIP (fp definition; `Accept` would also wait for agent-info) | PASS | none | osp_demo: `agent-info from GoIopOspc… (unverified)`. |
| 02 authenticate | n/a | SKIP (Go: SPAKE2 params) | `AllowSkipAuthentication` | |
| 03 URL availability | n/a | SKIP (auth) | none | osp_demo controller sends no request on the unauthenticated connection. |
| 04-05 start / message | n/a | SKIP | — | depend on 03 |

Against the old osp_demo the quirks would have been needed as follows. These are
covered by unit tests; that binary no longer exists.

| Old osp_demo behavior | Quirk |
|---|---|
| Whole-certificate `fp` | `AcceptCertificateFingerprint` (Dial) and `AdvertiseCertificateFingerprint` (Listen) |
| No agent-info / auth-capabilities | `TolerateMissingAgentInfo` |
| No authentication | `AllowSkipAuthentication` |

The zero-length-CID crash had no Go-side workaround. It is fixed in C++ (6a9786ae).

### Raw probe (fallback gauge), after

| Test / stage | Result | Notes |
|---|---|---|
| Publisher 01 discover, 03 resolve, 04 QUIC+TLS | PASS | `fp` matches the SPKI definition. |
| Publisher 05 agent-info | SKIP | The probe sends one request right after the handshake. osp_demo drops a request that arrives before its network phase starts. |
| Publisher 06 auth-capabilities | SKIP | Not in reply to the probe's. |
| Publisher 07-10 presentation | SKIP | osp_demo holds back application messages until authentication. |
| Controller (SPKI fp) 01 discover, 02 QUIC+TLS | PASS | osp_demo connects to an SPKI-advertising receiver. |
| Controller 03-05 presentation | SKIP | osp_demo doesn't send application messages unauthenticated. |
| Controller 06 network messages | PASS | osp_demo sent `auth-capabilities` (1001) and `agent-info-request` (10). |

Before (old osp_demo), the raw probe got through availability, start and
connection messages in the controller direction (9 PASS / 10 SKIP overall). That
was only possible because osp_demo didn't authenticate.

## Interop quirks (`ospc.AgentConfig.InteropQuirks`)

All quirks are off by default. Enabling any of them logs a warning when the agent
is created.

| Quirk | Effect | Spec rule violated |
|---|---|---|
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

## SPAKE2: the remaining blocker

Comparison of `agent_authenticator.cc` / `util/crypto/spake2.cc` with spake2-go
and `connection_network.go`:

| Item | network.bs | osp_demo | ospc | Who deviates |
|---|---|---|---|---|
| Curve | edwards25519 | P-256 | P-256 | both |
| Password hash | SHA-512 | SHA-512 | **SHA-256** (spake2-go has no option) | Go |
| `pw` bytes | "the PSK presented to the user" (underspecified) | canonical decimal digit string | raw bytes given to `AuthenticatePSK` | unclear, spec gap |
| Identities A/B | fingerprints of client / server | client / server | **presenter / consumer** | Go |
| Consumer starts | `auth-spake2-handshake` with `psk-needs-presentation` | same | **`auth-spake2-need-psk` (99001)**, a deprecated WIP type | Go |
| Hash / KDF / MAC, M/N, transcript | SHA-256, HKDF, HMAC | match ospc | match osp_demo | — |

Fixing the Go side means a SHA-512 password hash in spake2-go, client/server
identities, and the `psk-needs-presentation` start. These change Go-to-Go and
Go-to-Rust behavior, so they are not part of this branch. Both implementations
also need to move to edwards25519, or the spec to P-256.

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
| Authentication | SPAKE2 mandatory | SPAKE2 (parameters differ, see above) | SPAKE2 (P-256) |
| App messages before auth | not allowed | held until hand-off | held back until authenticated |
| Unknown type key | close with 404 | not implemented (deliberately) | buffered |
| Extension capability IDs (≥1000) | allowed | sends 1100, 1200 | decoded without range check |

## Remaining work

**Go**
- SPAKE2 parameters (table above): password hash, identities, consumer start.
- Pre-existing, found here: `Listener.Close` closes connections already returned
  by `Accept`, contrary to its doc comment. quic-go v0.39 `Listener.Close`:
  "All active connections will be closed."
- Unknown type key → close with 404: not implemented, by owner decision.

**Chromium**
- Drop of an agent-info-request that arrives before the network phase starts
  (`handling message type 10` is logged before `Network phase with …`).
- A PSK input path for the consumer role: osp_demo has no `--psk`, so as a
  controller against an ospc presenter it can't authenticate.
- Curve: P-256 vs. the spec's edwards25519.
- Older items: certificate fields per network.bs (CN=agent hostname), SNI = agent
  hostname, sync `osp_messages.cddl`, cwd-relative certificate path, fixed port.

**Spec**
- `sn` / hostname computation (#293, #365); SPAKE2 `pw` encoding; CBOR map key
  order (osp_demo depends on deterministic order); zero-length CID guidance
  (agents using zero-length CIDs must not issue NEW_CONNECTION_ID).
