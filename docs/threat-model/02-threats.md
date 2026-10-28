# LP2P threat model: threats

Step 2. Threats against the **intended behaviour** of `index.bs` (upstream
main) and the abstract transport-binding contract. Elements, flows and IDs come
from `00-framing.md` and `01-dfd.md`. Countermeasures are in step 3
(`03-mitigations.md`).

## 1. Method

- **STRIDE** on the novel elements and flows only: dispatch (`P2`, `F2.19`),
  native UI with mixed-trust strings (`P3`, `F1.7`, `F2.22`), the binding
  contract (`P4`), the stores (`S1`-`S3`), the asserted origin (`F1.21`,
  `F2.17`), the assumed peer-page link (`TB5`), hub-issued credentials
  (`F4.8`).
- **LINDDUN**, limited to Linkability (L), Identifiability (I),
  Detectability (D) and Unawareness (U). Written `L/I/D/U` in the Cat column;
  STRIDE as `S/T/R/I/D/E`. Where a letter could be either, the row says
  `STRIDE-D` or `LINDDUN-D`.
- **Abuse of functionality** (section 4): tunnelling, discovery-based
  profiling, the TAG's UPnP comparison.
- Inherited analysis is not repeated: TLS 1.3 and QUIC channel security,
  SPAKE2, mDNS, the permission model within one UA.

### Severity

- **H**: lasting loss of trust integrity or cross-site/cross-location
  tracking, reachable without unusual conditions, or the user explicitly
  ruled it unacceptable (`T-DSP-1`).
- **M**: needs a precondition (first contact with someone nearby, a paired
  peer, a user mistake) or the impact is bounded.
- **L**: nuisance, inherited and well understood, or low-value information.

### Axis columns

| Col | Axis | Values |
|---|---|---|
| C | Contact | `1` first contact, `R` repeat |
| N | Network | `H` home, `P` public (cafe, hotel, airport; LAN or WAN) |
| X | Context | `W` window, `D` dedicated or shared worker, `S` service worker (candidate surface only) |
| O | Origin match | `=` same-origin, `+` via `allowedOrigins` |
| K | Peer kind | `B` browser, `H` headless |
| A | Anchor | `S` self-anchored or one-peer, `V` vouched |

`·` = applies alike to every value. Two values = applies to both. A value
followed by `!` = applies to both, worst there.

## 2. Threat table

Areas: `DISC` discovery and picker, `CER` ceremony, `DSP` dispatch and
origin, `PEER` paired peer, `VCH` vouching, `STO` trust store and state,
`API` page-facing surface, `NET` hostile network, `ABU` abuse of
functionality.

| ID | DFD | Adversary | Cat | Threat | C | N | X | O | K | A | Sev | Why this severity |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| `T-DISC-1` | `F1.4`-`F1.9`, `F1.20` | `ADV-PEER-NEW`, `ADV-NET-LAN`, `ADV-PAGE` (advertising look-alike nicknames from visitors' browsers) | S | **Evil twin.** Advertise the display name or nickname of the device the user means; the user picks the twin and pairs with it. | 1 | P! | · | · | · | S | M | The ceremony stops a remote twin unless the twin learns the secret (`T-CER-2`); look-alike names are free |
| `T-DISC-2` | `F1.6`, `F1.13`, `F3.22` | `ADV-PEER-NEW`, `ADV-NET-LAN` | S | **Identity-change downgrade.** Advertise a known peer's (unverified) identity: the picker marks it "known"; auth fails to chain, and the UA silently falls into a fresh ceremony instead of warning that a known device changed key. | R | P! | · | · | · | S | M | Users conditioned that "known" means safe enter the PIN; no SSH-style "key changed" stop in the spec |
| `T-DISC-3` | `F1.7`, `F2.3`, `F2.22`, `TB6` | `ADV-PAGE`, `ADV-PEER-NEW` | S | **Untrusted strings in native UI.** Script-chosen nicknames and peer-asserted display names appear in the picker, advertise prompt, ceremony and grant prompt; can mimic UI text ("Confirm to continue"), brands, homoglyphs, bidi tricks. | · | · | · | · | · | · | M | Trusted UI carrying attacker text; spec says only "never verified", no display rules |
| `T-DISC-4` | `F1.4`-`F1.6`, `F1.10`-`F1.13`, `F1.24`, `F3.22` | `ADV-PEER-PAIRED`, `ADV-NET-LAN` | S | **Known-peer substitution.** A peer the user already paired (ex-partner's phone) advertises another known peer's identity and name. The picker marks it known; it chains to its *own* anchor, so no ceremony runs. `start()` never checks that the authenticated identity is the one picked, and the authenticated name is never shown again. *(Red team.)* | R | P! | · | · | · | · | H | Headline adversary, silent, no unusual conditions; unlike `T-DISC-2` no ceremony warns anyone |
| `T-CER-1` | `F2.8`, `F2.10`-`F2.14` | `ADV-NET-LAN`, `ADV-NEARBY` | STRIDE-D, S | **Unsolicited ceremony on the server.** Anyone who can reach an advertisement starts a session and triggers a PIN display or entry prompt on B, at will. Showing a PIN is not User B's consent, yet a nearby reader of it completes the pairing. | 1 | P! | · | · | H! | S | M | Fatigue and a pairing path User B never initiated; on headless displays nobody watches |
| `T-CER-2` | `F1.16`-`F1.19`, `AS-SECRET` | `ADV-NEARBY` | S | **One-time secret capture.** Shoulder-surf or camera: whoever reads the PIN in time pairs in place of, or alongside, the intended device. | 1 | · | · | · | · | S | M | Needs presence at the moment of the ceremony |
| `T-CER-5` | `F1.16`-`F1.19`, `AS-SECRET` | `ADV-NEARBY`, `ADV-PEER-NEW` | S | **Static secret.** A sticker or printed code on a headless device: anyone who saw it once pairs with the device, or plays the device in a ceremony with the user (an evil twin that passes). *(Red team split.)* | 1 | · | · | · | H | S | H | One look defeats the ceremony for the device's lifetime |
| `T-CER-3` | `F1.16`-`F1.20`, `S1`, `F4.18` | `ADV-NEARBY` | E, U | **Coerced or covert pairing.** Someone with brief access (partner, housemate) pairs their device, or ticks "trust for its devices". The anchor persists with no expiry, no indicator, and removal UI that is only a "should". | 1 | H | · | · | · | · | H | Intimate-partner abuse; durable and invisible; also the TAG's abuse question |
| `T-CER-4` | `F2.10`, `AS-SECRET` | `ADV-NET-LAN`, `ADV-NET-WAN` | S | **Online secret guessing.** Repeated ceremonies against a low-entropy PIN (OSP floor: 20 bits), from the LAN or off-LAN. | 1 | P | · | · | H! | S | M | Binding-level; bounded by PAKE plus rate limits, if the binding has them |
| `T-DSP-1` | `F2.19`, `F2.30`, `P2` | `ADV-PAGE` on B, `ADV-PEER-PAIRED` | E, S, I | **Receiver fallback.** If the picked receiver has stopped or doesn't allow the origin, "Incoming sessions" step 2 delivers to "another LP2PReceiver ... chosen by the user agent". User A consented to one receiver (by nickname); the session lands on another, possibly another origin on B that lists A's origin in `allowedOrigins`. A page on B can camp with a permissive receiver and catch sessions when the intended one stops. | · | · | · | +! | B | · | H | Non-deterministic, cross-origin delivery without User A's consent; ruled unacceptable at checkpoint 1 |
| `T-DSP-2` | `F2.8`, `F2.19`, `P4` | `ADV-NET-LAN` | T | **Advertisement not bound to the session.** The contract says which advertisement a session was reported for, but not that this is authenticated. "Advertisement" has no authenticated binding to the session: when several advertisements share one device key, an on-path attacker redirects the client's connection to another advertisement of the same device; authentication still succeeds and the origin is checked against the wrong receiver. | · | P! | · | +! | B | · | M | Needs an on-path attacker and a second receiver allowing the origin; breaks deterministic matching |
| `T-DSP-3` | `F2.21`-`F2.25`, `S3` | `ADV-PEER-PAIRED`, `ADV-PAGE` | E, U | **Server grant broader than the consent.** B's grant is keyed on (receiver origin, peer), not on the requesting origin, and the spec doesn't say which origin(s) the prompt shows. User B accepts A once for origin O1; later sessions from A asserting any origin the receiver allows skip the prompt. | R | · | · | +! | B | · | M | Consent scope mismatch; only with `allowedOrigins` or a lying peer |
| `T-DSP-4` | `F2.17`, `F2.28`, `TB5` | `ADV-PEER-PAIRED` | S | **Asserted origin, self-anchored liar.** A paired peer claims the receiver's own origin; page B treats it as its own app. Page B can't even see the requesting origin, so with `allowedOrigins` it can't tell partners apart. | R | · | · | · | B | S | M | Spec-acknowledged (Origin-header model); limited to peers the user paired |
| `T-DSP-5` | `F2.3`-`F2.7`, `F2.19`, `F3.19` | `ADV-PAGE`, `ADV-NET-LAN` | E, S | **Receiver start race.** `start advertising` runs in parallel before the queued task sets `[[Advertisement]]`; a session in that window matches no receiver (today: falls back, `T-DSP-1`), after step 1 already ran a ceremony. `stop()` rejects the promise but takes down neither the advertise prompt nor an advertisement about to go live. *(Red team.)* | 1 | · | W D | · | B | · | M | Reliable trigger for the fallback; consent after `stop()` |
| `T-PEER-1` | `F3.5`-`F3.7`, `S1`, `S3` | `ADV-PEER-PAIRED` | E | **Formerly trusted peer keeps access.** Resold device, ex-partner's phone: the anchor never expires and nothing revokes it offline; on B its server grant makes later sessions promptless. "Forget" keeps the anchor, so the next session re-adds the known peer silently: the UI's remove doesn't revoke; and "remove a trust anchor" leaves a stale "known" mark (red team). | R | · | · | · | · | · | H | Durable and silent; the offline premise removes any revocation feed |
| `T-PEER-2` | `F1.21`, `F3.6`, `AS-DEVID` | `ADV-TRACKER` | L | **Cross-site linking by a peer.** The peer sees UA A's one device credential plus the requesting origin on every session, for every origin; origins that share a peer (a TV with many apps) join their profiles. | R | · | · | · | H! | · | M | Needs the user to pick the same peer from several sites; Local HTTPS calls it "inherent to pairing" |
| `T-PEER-3` | `F1.21`, `F1.22`, `AS-ORIGIN` | `ADV-PEER-PAIRED`, `ADV-HUB` | I | **Origin disclosed to any authenticated peer.** The requesting origin reaches every authenticated peer, including one that then refuses and a vouched peer the user never met; the accept/refuse answer tells UA A which sites B's user uses. | · | · | · | · | · | V! | M | Contract scopes it to authenticated peers; under vouching that is a wide set |
| `T-PEER-4` | `F3.6`, `F3.7`, `F2.28` | `ADV-PEER-PAIRED` | U | **Silent sessions on B.** Once granted, A connects whenever any page of the receiver origin advertises; no indicator for User B while a session is open. | R | · | · | · | B | · | M | Background data channel the user forgot about |
| `T-PEER-5` | `F3.12`, `F3.13`, `F2.22`-`F2.24` | `ADV-PEER-PAIRED`, `ADV-HUB` | STRIDE-D (`AS-ATTN`) | **Peer-driven flooding.** Any authenticated (or vouched) peer re-triggers B's grant prompt at will: no rate limit, no memory of a decline, no timeout on pending incoming sessions. Incoming data channels held before exposure are unbounded; stream and datagram floods. *(Grant-prompt part: red team.)* | R | · | · | · | B | V! | M | Attention attack on User B; raised from L with the prompt part |
| `T-VCH-1` | `F4.8`-`F4.10`, `F2.17` | `ADV-HUB` | S, E | **Liar never met.** A hub-minted device is authenticated with no ceremony and asserts any origin, display name and capabilities. "A lying peer can only be one the user already authenticated" is false: the user authenticated the hub, never this device. | 1 | · | · | · | · | V | H | Breaks the spec's stated bound on liars; scales with the hub |
| `T-VCH-2` | `F4.1`, `F4.2`, `F4.13`, `E5` | `ADV-HUB`, `ADV-PEER-NEW` | E | **Vouching scope unbounded.** Limits are "defined by the transport binding" with no contract minimum; how a peer is recognised as a vouching party, and how the Local HTTPS namespace is conveyed and confirmed, are unspecified. A device that merely claims to vouch can be given a vouching anchor. | 1 | · | · | · | · | V | H | One ceremony, unbounded transitive trust plus a CA for a namespace |
| `T-VCH-3` | `S1`, `F4.8`, `F4.13` | `ADV-HUB` | S, E | **Compromised hub or stolen key.** Every device and every `*.home.local` host it certifies is trusted until the user removes the anchor; offline, nobody tells them to. | R | H | · | · | · | V | H | Single key, many peers, no revocation channel |
| `T-VCH-4` | `F4.9`, `F1.7` | `ADV-HUB`, a co-tenant of the hub | S | **Vouched look-alike.** The hub (or anyone it enrols, e.g. a guest's device) presents a device with the name of another one; no ceremony to break the tie. | 1 | H | · | · | · | V | M | Needs a hub that enrols devices the user doesn't control |
| `T-VCH-5` | `F4.7`, `F4.20`, `F4.24`, `S4` | `ADV-HUB`, `ADV-PEER-NEW` | E, STRIDE-D, L | **Merge side effects.** Adding a vouching anchor silently widens a one-peer pairing (example 2) to all the hub's devices; removing it drops devices the user paired one by one. The merge uses a plain "remove", not `remove a trust anchor`: no cascade, no Local HTTPS site-data clearing, and the device's old namespace (`nas.local`, outside `*.home.local`) keeps its site data with no anchor. *(Bypass: red team.)* | R | H | · | · | · | V | M | Raised from L: a correctness gap, not just surprise |
| `T-VCH-6` | `F4.13`, `F0.25`, `F4.8` | `ADV-HUB` | S, E | **One-peer hub anchor certifies any hub device for HTTPS.** Local HTTPS validation checks the key and the namespace, never the subject. With example 2's one-peer anchor (hub key, one device), any device the hub issues a `nas.local` certificate to passes; "other devices of the hub are not trusted" holds for LP2P, not for Local HTTPS. *(Red team.)* | R | H | · | · | · | S | M | Needs the hub to misissue; contradicts the spec's example |
| `T-STO-1` | `F1.20`, `F2.16`, `F2.20` | `ADV-PEER-NEW`, `ADV-NEARBY` | E, U | **Declined peer persists.** The anchor (ceremony) and known peer ("Incoming sessions" step 4) are added before the grant prompt or the server's answer. If User B declines, the peer stays authenticated: next time there's no ceremony. On the client, a refusal after a ceremony leaves an anchor but no known-peer entry (silent next time, yet not marked known). | 1 | · | · | · | · | S | M | Converts a "no" into lasting trust; nothing tells the user |
| `T-STO-2` | `F4.26`, `S1`, `S2`, `AS-DEVID` | `ADV-TRACKER`, `ADV-PEER-PAIRED` | L, U | **State survives clearing and private mode.** Clearing an origin's data revokes its grants only; anchors, known peers and the UA's device credential stay. Private browsing and profiles are not specified (OSP only recommends separation). A peer re-identifies the user after clearing, or across private and normal browsing. | R | · | · | · | · | · | M | Undermines a user control; precedent: cookies, HSTS |
| `T-STO-3` | `F0.3`, `S1`, `S2` | `ADV-PAGE` | LINDDUN-D | **Shared store across origins.** A ceremony run for origin X (or an embedded third party) makes the peer authenticated for every origin; `start()` timing (ceremony or not) hints at pairing done elsewhere. LP2P issue 24. Through Local HTTPS, the device's advertised name becomes trusted for user-initiated navigations ("every anchor"), whichever origin (or iframe) ran the ceremony. | R | · | · | · | · | · | M | Raised from L (red team): a lasting cross-origin effect via Local HTTPS, not just timing |
| `T-STO-4` | `F4.15`-`F4.17` | `ADV-PEER-PAIRED`, `ADV-HUB` | S | **Silent key replacement.** "Replace a trust anchor key" runs whenever the binding claims continuity; a weak binding lets a key holder (or thief) move the anchor to a new key with no user step. It rewrites *every* anchor with that key (vouching ones too), runs no trust anchor check, and leaves `.local` site data in place, breaking Local HTTPS's "at most one key per host" (red team). | R | · | · | · | · | · | M | Binding-defined; OSP has none today, a future binding may |
| `T-STO-5` | `F4.3`-`F4.6`, `F4.14`, `S4` | `ADV-PEER-NEW`, `ADV-HUB` | STRIDE-D, E | **Name squatting and nesting.** A one-peer namespace is the name the device advertises (trusted after auth). The first paired device claims `printer.local` or `home.local`; the trust anchor check then fails the real one's ceremony. A granted device advertising a broad name gives pages a grant constraint covering names under it. | 1 | · | · | · | · | · | M | Lasting denial of pairing; grant widening via a name the device chose |
| `T-STO-6` | `F3.18`, `F4.19`-`F4.25`, `F1.24`, `F1.25`, `F2.25` | `ADV-PEER-PAIRED`, `ADV-PAGE` | E, U | **Revocation and abort races.** `close the sessions with a peer` walks only `LP2PConnection` objects: a session at the grant prompt, between `Accept` and delivery, or between "grant a peer" and resolve on the client survives "forget", "remove a trust anchor" or a revoked grant, and its later steps re-add the grant and known peer. Abort after the grant step leaves a grant (both sides) for a promise that rejected. *(Red team.)* | · | · | · | · | · | · | M | User removal silently undone; a narrow window but the user's main control |
| `T-API-1` | `F1.1`, `F2.1`, `F2.3` | `ADV-PAGE` | STRIDE-D (`AS-ATTN`) | **Prompt spam.** Neither `LP2PRequest.start()` nor `LP2PReceiver.start()` needs user activation; a page (or a delegated iframe) re-opens pickers and advertise prompts until the user gives in. | · | · | W | · | · | · | M | Consent quality; Design Principles 2.12 |
| `T-API-2` | `F0.3`, `S3`, `F2.21` | `ADV-PAGE` (embedded), `ADV-TRACKER` | L, E | **Third-party embedding.** Grants are keyed by origin, not top-level site. An iframe granted on site 1 (via `allow="local-peer-to-peer"`) keeps that grant on site 2; its server-side receiver auto-accepts there. | R | · | W | · | · | · | M | Cross-site reuse of a grant; open granularity issue |
| `T-API-3` | `F1.27`, `F3.8` | `ADV-PAGE` | LINDDUN-D, I | **Script-observable distinctions.** `lp2p.connect()` and `new LP2PQuicTransport()` throw `NotSupportedError` when the picked peer lacks WebTransport, and `createDataChannel()` when it lacks data channels: script learns that the user picked a peer, and one capability bit of it. Timing separates "dismissed fast" from "ceremony then refusal". | 1 | · | · | · | · | · | L | One bit, contradicts "no peer attributes reach origins" |
| `T-API-4` | `F2.7`, `F3.19`, `F3.20` | `ADV-PAGE` | U, LINDDUN-D | **Background advertising.** A receiver keeps advertising and accepting from a hidden tab, a worker, or a BFCached document; nothing shows the user it is discoverable. | R | P! | W D | · | · | · | M | Consent given once, presence leaks indefinitely; BFCache answer is open |
| `T-API-5` | `F3.20`, `E3D` | `ADV-PAGE` | S, U | **Worker UI attribution.** No rule for which document hosts the picker or prompt of a dedicated or shared worker; a prompt can appear over an unrelated tab of the origin, or none. | · | · | D | · | · | · | M | Mis-attributed consent |
| `T-API-6` | `F3.21`, `E3S` | `ADV-PAGE` | E | **Service worker gate undefined.** `Exposed=(Window,Worker)` includes `ServiceWorkerGlobalScope`, but "Permission Policy Integration" covers only dedicated and shared workers. The policy check has no defined answer for a service worker, so the gate is implementation-defined today. | · | · | S | · | · | · | M | A current-spec bug, independent of the candidate surface |
| `T-API-7` | `F2.6`, `F2.7`, `F2.28`, `F1.26`, `F3.17`, `F3.9` | `ADV-PAGE`, `ADV-PEER-PAIRED` | U, LINDDUN-D, STRIDE-D | **No teardown.** Nothing stops advertising, aborts in-parallel steps or finalizes connections when a document unloads or a worker ends; an accepted session whose delivery task never runs is neither delivered nor closed. Questionnaire Q18 says teardown is handled. Also: after `makeDiscoverable()`'s stream is cancelled, data-channel-only connections stay open with no handle to close them. *(Red team.)* | · | · | W D | · | · | · | M | Presence and open channels outlive the page the user consented on |
| `T-NET-1` | `F1.3` (outbound metadata connections), `F1.12`, `AS-DEVID` | `ADV-NET-LAN`, `ADV-PEER-NEW` | L, I | **Client identity leaks before selection.** If the binding has UA A present its long-lived credential on every connection (OSP: the agent certificate, also on metadata connections during discovery), any advertiser on cafe Wi-Fi collects a stable identifier for every browser that opens a picker near it. Conditional on the binding (`01-dfd.md` draws no outbound discovery flow; `F1.3` stands for it). | 1 | P | · | · | · | · | H | Passive, scalable, cross-location tracking with no user action beyond opening a picker |
| `T-NET-2` | `F2.7`, `AS-PRESENCE` | `ADV-NET-LAN`, `ADV-NET-WAN` | L, LINDDUN-D | **Stable identifiers and traffic shape.** A fixed fingerprint or serial in advertisements tracks a portable device across networks (presence itself: `T-NET-3`); per-origin connections and their timing show who pairs with whom and how many origins are in use. | R | P | · | · | · | · | M | Only while advertising, which the user consented to; still cross-location |
| `T-NET-3` | `F2.7`, `F2.3`, `AS-NAMES` | `ADV-NET-LAN` | I, LINDDUN-D | **Presence and nickname leak.** The advertisement reveals the device and its nickname (which may name the site or the person) to everyone on the medium; the advertise prompt need not say who can see it. | · | P | · | · | · | · | M | PII to strangers on public networks |
| `T-NET-4` | `F0.14`, `F0.15`, `E7` | `ADV-NET-WAN` | S, E | **Off-LAN reach.** A NAT, VPN or relay exposes an advertiser; locality is not promised, so a remote host starts sessions and ceremonies, and phones the user for the code ("read me the number on your TV"). | 1 | P | · | · | H! | S | M | Contract promises C+I only; OSP's `at` token helps where present |
| `T-NET-5` | `F1.4`, `F2.8`, `E6` | `ADV-NET-LAN` | STRIDE-D, S | **Flooding and suppression.** Connection floods, mDNS cache floods, suppressing the real device's adverts so the picker shows only decoys. | 1 | P | · | · | · | · | L | Inherited, binding-level; steering is bounded by the ceremony |
| `T-ABU-1` | `F3.10`, `F3.11`, `E4` | `ADV-PAGE`, `ADV-PEER-PAIRED` | E | **Tunnelling and C2.** A page with a granted peer gets a bulk, bidirectional channel to a device that may relay into the LAN; it is not governed by CSP `connect-src` or any enterprise policy. | R | · | · | · | H! | · | M | The TAG's protocol-tunnelling point; needs a grant |
| `T-ABU-2` | `F1.5`, `F2.29` | `ADV-PAGE`, `ADV-TRACKER` | I, L | **Discovery-based profiling.** The origin never sees discovery results ("Peer discovery"); residual: page B learns when A is nearby each time A connects, and a granted origin sees repeat success as a presence signal. | R | · | · | · | · | · | L | Needs user action per connect on the client; `T-GD-1` changes this |
| `T-ABU-3` | `F2.17`, `F0.16`, `E4` | `ADV-PAGE`, `ADV-PEER-PAIRED` | E | **UPnP-like trust in devices.** Headless peers authorise on the asserted origin or on being local, as UPnP did; the spec's "must not grant anything ... on the strength of it" binds UAs, not device logic. | R | H | · | · | H | · | M | Real-world precedent; spec text has a gap |

46 threats: 9 H, 34 M, 3 L. Drafted as 40 (7 H, 27 M, 6 L); the red team added five rows, split one, and raised three severities (section 7).

## 3. STRIDE coverage of novel elements

| Element or flow | S | T | R | I | D | E |
|---|---|---|---|---|---|---|
| `P2` dispatch, `F2.19` | `T-DSP-1`, `T-DSP-5` | `T-DSP-2`, `T-STO-6` | n/a: no audit need found | `T-DSP-1` | | `T-DSP-1`, `T-DSP-3` |
| `P3` UI, `F1.7`, `F2.22` | `T-DISC-1`-`4`, `T-API-5` | | | | `T-API-1`, `T-CER-1`, `T-PEER-5` | |
| `P4` binding contract | `T-CER-4`, `T-STO-4` | `T-DSP-2` | | `T-NET-1` | `T-NET-5`, `T-PEER-5` | `T-NET-4` |
| `S1` trust store | `T-VCH-3`, `T-VCH-6` | `T-STO-4`, `T-VCH-5` | | | | `T-CER-3`, `T-STO-1`, `T-STO-6`, `T-VCH-2` |
| `S2`, `S3` | `T-DISC-2`, `T-DISC-4` | | | `T-STO-3` | | `T-DSP-3`, `T-API-2` |
| `F1.21`, `F2.17` asserted origin | `T-DSP-4`, `T-VCH-1` | | | `T-PEER-3` | | `T-ABU-3` |
| `TB5` peer page | `T-DSP-4` | | | | | |
| `F4.8` hub credentials | `T-VCH-1`, `T-VCH-4` | | | | | `T-VCH-2` |

Repudiation: no threat. There is no audit trail to protect, and the spec
promises none; noted, not pursued.

## 4. Abuse of functionality

- **Tunnelling** (TAG: MITRE T1572): `T-ABU-1`. The channel reaches only a
  peer the user picked and that accepted; what the device does with the bytes
  is out of scope. Policy hooks (CSP, enterprise) are the gap.
- **Discovery-based profiling** (TAG: MITRE TA0007): `T-ABU-2` for origins,
  `T-NET-1`-`T-NET-3` for the network. The origin side is handled by design
  ("Peer discovery": never exposes the full result); the network side is
  where the exposure is.
- **UPnP comparison** (TAG: RFC 6970 section 6): UPnP's failures were
  unauthenticated control, automatic exposure to the WAN, and trust in "being
  on the LAN". LP2P has an authenticated, user-picked peer and no port
  mapping; the residues are `T-ABU-3` (devices trusting the asserted origin)
  and `T-NET-4` (off-LAN exposure through a NAT or relay).

## 5. Candidate surface (decision input)

Not findings against the current spec. Deltas on D3 (`E3S`, `F3.21`) and on
`F3.1`-`F3.4`, for LP2P issue 49 (service workers) and LP2P issue 55
(`getDevices`-style re-access without the picker).

| ID | Delta | Adversary | Cat | Threat | Sev | Why |
|---|---|---|---|---|---|---|
| `T-SW-1` | `E3S` + `LP2PReceiver` | `ADV-PAGE` | U, LINDDUN-D | **Always-on advertising.** A receiver in a service worker advertises and accepts with no page open, beyond any document whose user consented. | H | Presence leak and inbound channel with no UI anchor |
| `T-SW-2` | `E3S` + existing grants | `ADV-PAGE`, `ADV-PEER-PAIRED` | U, E | **Background use of grants; silent C2.** Woken by push, sync or fetch, the worker uses sessions or, with re-access (`T-GD-*`), connects to a device at any time with no user action. No UI to prompt from, so only existing grants work: use the user never sees. Absorbs `T-GD-3`. | H | Strengthens `T-ABU-1` to unattended |
| `T-SW-3` | `E3S`, offline premise | `ADV-PAGE` | E | **Cached code with device access.** Offline, the worker serves old or compromised code; the origin is unreachable so it can't be fixed; a persisted XSS keeps device access. | M | Offline is a condition; same as other SW-persisted risks but with a device behind it |
| `T-SW-4` | `E3S`, `F3.21` | `ADV-PAGE` (embedded) | E | **Permissions Policy bypass.** A third party denied `local-peer-to-peer` in an iframe registers a (partitioned) service worker and calls the API there, where no owning document's policy applies. | H | Defeats the embedder's control unless policy is defined for SWs |
| `T-SW-5` | `E3S` | `ADV-PAGE` | STRIDE-D, U | **Lifetime extension.** Open sessions or advertisements keep the worker alive. | L | Resource and tracking window; UAs already cap SW lifetime |
| `T-GD-1` | `F3.1`-`F3.4` without `F3.3`/`F3.4` | `ADV-PAGE`, `ADV-TRACKER` | L, I | **Presence oracle.** An origin re-connects to granted peers whenever it likes: "is the user near device X now" turns into home/work location tracking. | H | Turns `T-ABU-2` from L into H |
| `T-GD-2` | `F3.1`-`F3.4` | `ADV-PAGE` | I | **Grant enumeration.** The list (count, stable handles) is a per-origin fingerprinting surface and reveals which devices the user owns. | M | Per-origin only; Design Principles 9.2 |
| `T-GD-3` | (merged into `T-SW-2`, red team) | | | | | |
| `T-GD-4` | `S3`, `T-PEER-1` | `ADV-PEER-PAIRED` | I | **Stale grant to a changed owner.** A device resold with its key keeps getting connections and data from the old owner's origin, silently. | M | Today the per-use picker catches it |
| `T-GD-5` | `F3.3`, `F3.4` | (loss of a control) | U | **Picker as awareness point lost.** The per-use picker is the client side's only recurring consent and the point where stale or unexpected peers become visible. | M | Removes the mitigation for `T-PEER-1` on the client |

## 6. Gaps from step 1 mapped to threats

| Gap (`01-dfd.md`, "Where the spec is silent") | Threats |
|---|---|
| Which side shows and which enters the PIN | `T-CER-1`, `T-CER-2` |
| Consent on a headless peer | `T-CER-1`, `T-CER-5`, `T-ABU-3` |
| Advertise consent persistence | `T-API-4`, `T-NET-3` |
| Which origin(s) the server's grant prompt shows | `T-DSP-3` |
| How the UA chooses "another" receiver | `T-DSP-1` |
| What a client-side grant is for | `T-GD-5` (it matters once re-access exists) |
| Service workers | `T-API-6`, `T-SW-*` |
| Which document hosts UI for a worker | `T-API-5` |
| Re-access without a picker | `T-GD-*` |
| Limits on what a vouching key may certify | `T-VCH-2`, `T-VCH-3` |
| How the ceremony conveys a Local HTTPS namespace | `T-VCH-2`, `T-STO-5` |
| Key continuity rules | `T-STO-4` |
| Incognito, profiles, clearing anchors and known peers | `T-STO-2` |
| Grants per origin or per site | `T-API-2` |
| Locality over a relay or exposed NAT | `T-NET-4` |
| Pre-authentication filtering and picker context | `T-DISC-1`, `T-DISC-4`, `T-API-3` |
| Document teardown, worker end (added after the red team) | `T-API-7` |
| Advertising goes live before the receiver is bound (added after the red team) | `T-DSP-5` |
| Nickname transport in the binding | `T-NET-3`, `T-DISC-3` |
| Rate limits on ceremonies | `T-CER-1`, `T-CER-4` |

## 7. Red-team pass

A fresh agent (Opus) saw only `00-framing.md`, `01-dfd.md`, this file and the
spec (`index.bs`, `local-https.bs`, the questionnaire), not the code.

| Red-team item | Decision | Result |
|---|---|---|
| NEW-1 known-peer substitution by a paired peer, no ceremony | Accepted, H | `T-DISC-4` |
| NEW-2 revocation misses sessions in setup; later steps restore grants | Accepted | `T-STO-6` |
| NEW-3 no teardown on unload; questionnaire Q18 claim | Accepted | `T-API-7` |
| NEW-4 Local HTTPS ignores the subject of a one-peer hub anchor | Accepted | `T-VCH-6` |
| NEW-5 replace-key blast radius, skipped checks | Merged | into `T-STO-4` |
| NEW-6 receiver start race; `stop()` leaves the prompt | Accepted | `T-DSP-5`. The general "in-parallel steps read `[[State]]` unsynchronised" point is rejected as a threat: an editorial pattern across the spec, noted for the editors |
| NEW-7 vouching merge bypasses cascade and site-data clearing | Merged | into `T-VCH-5`, raised L -> M |
| NEW-8 "forget" is cosmetic; stale "known" mark after removal | Merged | into `T-PEER-1` |
| NEW-9 grant-prompt flooding by a peer | Merged | into `T-PEER-5`, raised L -> M |
| NEW-10 abort leaves durable state | Merged | into `T-STO-6` (abort) and `T-STO-1` (client anchor, no known peer) |
| NEW-11 `makeDiscoverable()` leaves unclosable connections | Merged | into `T-API-7` |
| SEV `T-CER-2` split static vs one-time secret | Accepted | `T-CER-2` M, new `T-CER-5` H |
| SEV `T-STO-3` L -> M (Local HTTPS effect) | Accepted | |
| SEV `T-DISC-2` stays M given NEW-1 | Accepted | |
| FIX `T-DSP-2` implementation reference | Accepted | Rephrased as a contract gap |
| FIX `T-DISC-1` add `ADV-PAGE` | Accepted | |
| FIX `T-DISC-3` contact is not first-only | Accepted | `·` |
| FIX `T-STO-1` client side | Accepted | |
| FIX `T-STO-4` scope, checks, `S4`; STRIDE table | Accepted | |
| FIX `T-VCH-5` merge bypass | Accepted | see NEW-7 |
| FIX `T-NET-1` no outbound discovery flow | Accepted | Cites `F1.3`, marked conditional on the binding. `01-dfd.md` not redrawn |
| FIX `01-dfd.md` teardown and start race | Accepted | Listed in section 6 here, not in the step-1 file |
| FIX spec claims contradicted (Q18, Q6, Local HTTPS "unambiguous", example 2) | Accepted | `03-mitigations.md` section 6 |
| DUP `T-SW-2` / `T-GD-3` | Accepted | `T-GD-3` merged into `T-SW-2`; ID retired |
| DUP `T-NET-2` / `T-NET-3` | Accepted | Presence only in `T-NET-3` |
