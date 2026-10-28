# LP2P threat model: binding obligations

Step 3. What any transport binding must provide so that the API's
mitigations hold. Abstract: no protocol names in the obligations. Threat
IDs from `02-threats.md`; dispositions in `03-mitigations.md`.

Results of the conformance passes below are **binding findings**
("binding X fails BO-n"), not LP2P findings.

## 1. Obligations

`Contract` = already required by "Transport abstraction"; `New` = proposed
for the contract.

| ID | Obligation | Status | Threats |
|---|---|---|---|
| `BO-1` | End-to-end confidentiality and integrity for all communication with a peer, including through an intermediary. | Contract | (inherited) |
| `BO-2` | Mutual authentication. The peer identity is derived from the anchor key and the subject under it. A ceremony is resistant to offline guessing from a transcript (a PAKE or equivalent), and a man in the middle needs the secret. | Contract (mutual auth); New (offline-guess resistance) | `T-DISC-1`, `T-CER-4` |
| `BO-3` | Ceremony secret hygiene: generated from a CSPRNG, single-use, with a stated minimum entropy; online guesses limited (backoff, attempts per secret). A static secret (sticker, printed code) works only while the device is in a pairing mode entered by physical action on it. | New | `T-CER-2`, `T-CER-4`, `T-CER-5` |
| `BO-4` | Ceremony reachability: only a party that received the advertisement over the local medium can start a ceremony (for example, an unguessable token in the advertisement). The binding tells the UA whether a session's path is direct or through an intermediary. | New | `T-NET-4`, `T-CER-1` |
| `BO-5` | Origin confidentiality and binding: the requesting origin, and the server's answer to it, are exchanged only after mutual authentication and are bound to the authenticated session. Any early-decision mechanism exposes neither side's origins. | Contract | `T-PEER-3`, `T-DSP-4` |
| `BO-6` | Advertisement binding: an incoming session identifies exactly one advertisement, the one the client picked, and that identification is covered by authentication, so it can't be redirected to another advertisement of the same device. `establish a session` takes the advertisement, not only the peer. | New | `T-DSP-1`, `T-DSP-2` |
| `BO-7` | Client credential withholding: the client presents its long-lived credential only to a server it has authenticated or, for a ceremony, to the peer the user selected. Discovery and metadata exchanges use no long-lived client credential. | New | `T-NET-1` |
| `BO-8` | Advertisement minimisation: no identifier in an advertisement stable across networks or advertisements; the nickname carried only as consented and length-bounded; nothing that names an origin. | New | `T-NET-2`, `T-NET-3` |
| `BO-9` | Separate credentials and state per user profile and for private browsing; the UA's own credential can be regenerated when the user clears state. | New | `T-STO-2` |
| `BO-10` | Persistent state in step with LP2P state: when the UA removes an anchor, forgets a peer or clears state, binding-level trust and caches that refer to it go too. | New (sharpens "persistent state rules") | `T-STO-1`, `T-STO-2`, `T-PEER-1` |
| `BO-11` | Vouching, if supported: (a) a vouching party is recognisable from its credential, not from a claim; (b) each vouched credential carries a subject stable and unique under the voucher, and an attested display name; (c) limits are expressible and enforced (at least a namespace, as name constraints); (d) vouched credentials have bounded validity, so a compromise ages out offline; (e) the UA can tell, at authentication, a peer vouched for from one anchored directly. | New | `T-VCH-1`-`T-VCH-4` |
| `BO-12` | Key continuity only with proof of possession of both the old and the new key; no other party can trigger a replacement. | New | `T-STO-4` |
| `BO-13` | Identity-change signal: the binding reports a discovered peer that claims a known identity but fails to chain, as distinct from a new peer. | New | `T-DISC-2` |
| `BO-14` | Display-name integrity: a display name or capability is trusted only when obtained on an authenticated session; unverified or truncated names are marked as such. | Contract (as hints) | `T-DISC-1`, `T-DISC-3` |
| `BO-15` | Session isolation: a session exposed to an origin is not shared with another origin; each WebTransport session has its own keying material exporter. | Contract | `T-PEER-2` (bounds it) |
| `BO-16` | Resource bounds: limits on concurrent connections, ceremonies per period, and channels or sessions held before exposure. | New | `T-PEER-5`, `T-NET-5`, `T-CER-1` |

## 2. OSP conformance pass (light)

Against the Open Screen Network Protocol (W3C ED, this repo's
`network.bs`) plus the LP2P OSP binding ("Binding to the Open Screen
Network Protocol", "Session extension").

| BO | OSP | Evidence |
|---|---|---|
| `BO-1` | Meets | TLS 1.3 over QUIC |
| `BO-2` | Meets | SPAKE2 with agent fingerprints as identities ("Authentication with SPAKE2") |
| `BO-3` | Partially meets | One-time PSK, CSPRNG and backoff advice ("Local active network attackers"); the floor is 20 bits; nothing on static secrets |
| `BO-4` | Partially meets | `at` token must be in the TXT record and in the first auth message ("Remote active network attackers"); no direct/relayed signal (OSP has no relays) |
| `BO-5` | Meets | Via the LP2P session extension: `lp2p-session-request` only to a verified server, answer only to a verified client; bound by the certificates. Assumes the `psk-needs-presentation` fix the binding notes |
| `BO-6` | **Fails** | `lp2p-session-request` carries no advertisement; nothing says one agent per advertisement. A UA that reuses one agent certificate across advertisements (go-lp2p does) cannot bind a session to the picked one, and an on-path attacker can redirect between them |
| `BO-7` | **Fails** | Agents "must use the agent certificate ... when making a QUIC connection", including the metadata connection made during discovery ("Agent Certificates", "Transport and metadata discovery") |
| `BO-8` | **Fails** | `fp` (fingerprint) in the TXT record, stable; OSP declares mDNS data public ("Personally Identifiable Information") |
| `BO-9` | Partially meets | Separate authentication contexts for private browsing are "recommended" ("Private Browsing Mode"); profiles not mentioned |
| `BO-10` | Partially meets | Clearing is "an implementation choice" ("Persistent State") |
| `BO-11` | N/A | Self-signed agent certificates only: no vouching. OSP cannot express the hub examples of "Peer authentication" |
| `BO-12` | Meets (trivially) | No continuity; a new key needs a new ceremony |
| `BO-13` | Partially meets | "Suspicious agent" rules flag fingerprint collisions and name changes (should); not surfaced as a distinct signal |
| `BO-14` | Meets | "Instance and Display Names": verified display names, truncation rules |
| `BO-15` | Meets | One QUIC connection per LP2P session; exporter per WebTransport session ("Keying material exporters") |
| `BO-16` | Partially meets | DoS advice only ("Denial of service"); held-before-exposure limits are LP2P-side |

**Summary:** OSP meets the channel, authentication, origin-binding,
display-name and isolation obligations; partially meets secret hygiene,
reachability, private mode, state and DoS; **fails `BO-6` (advertisement
binding), `BO-7` (client certificate before selection) and `BO-8` (stable
fingerprint in mDNS)**; vouching is not applicable.

Smallest OSP-side fixes, for the binding owners:

- `BO-6`: one agent certificate (hence fingerprint) per advertisement, or an
  advertisement identifier inside `lp2p-session-request`, checked by the
  server against the listener the connection arrived on.
- `BO-7`: an ephemeral certificate for metadata connections; present the
  agent certificate only on the connection to the selected agent.
- `BO-8`: a per-network or per-advertisement fingerprint, which `BO-6`'s
  first option already implies, plus a stable identity proven only inside
  the authenticated session.

## 3. Sketch: WebTransport over HTTP/3 with SETTLE-style authentication

Assumed shape: the receiver side is an HTTPS server with a name under
`.local`, discovered by DNS-SD; the server certificate chains to a local
trust anchor (Local HTTPS); the first contact uses a PAKE-based ceremony
that SETTLE would define; a session is a WebTransport CONNECT whose `Origin`
is the requesting origin and whose `:path` selects the receiver.

| BO | Likely | Note |
|---|---|---|
| `BO-1` | Meets | TLS 1.3 |
| `BO-2` | Open | Server auth by path validation; client auth needs mTLS or a PAKE-derived client credential. SETTLE's choice decides it |
| `BO-3`, `BO-4` | Open | Depends on the SETTLE ceremony; no `at`-like token yet |
| `BO-5` | Partially meets | `Origin` goes only to a server the client has authenticated; but the server's answer (HTTP status) reaches a client it has not authenticated unless mTLS is used |
| `BO-6` | Meets | The receiver is named by `:authority` and `:path` inside TLS: matching is deterministic by construction |
| `BO-7` | Meets, if mTLS | TLS 1.3 sends the client certificate only after verifying the server |
| `BO-8` | Can meet | Path validation needs no fingerprint in DNS-SD |
| `BO-9`, `BO-10` | Can meet | UA-side |
| `BO-11` | Meets (a)-(c) | A referee as a name-constrained CA (SETTLE REQ-ARCH-02); (d) short-lived certificates help only with a trusted clock; (e) visible from the chain |
| `BO-12` | Open | Local HTTPS: "Rotating an anchor key needs a proof of continuity, which is out of scope" |
| `BO-13`, `BO-14` | Can meet | UA-side, given BO-11(b) |
| `BO-15` | Meets | Exporter per WebTransport session (WebTransport over HTTP/3, section 4.8) |
| `BO-16` | Meets | Standard HTTP/3 limits |

Structural catch: a **browser** as receiver must itself be an HTTPS server
under a `.local` name with a certificate a peer accepts, which means the
browser must be enrolled like a device. The sketch fits device-as-server
well and browser-to-browser poorly.
