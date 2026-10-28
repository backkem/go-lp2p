# LP2P threat model: framing

## 1. Purpose and status

- Step 1 of the plan: framing (this file) and data-flow diagrams (`01-dfd.md`).
- Target: the Local Peer-to-Peer API (`index.bs`) plus the abstract
  transport-binding contract ("Transport abstraction").
- Later steps add: threats per element and flow (step 2), countermeasures and
  binding obligations plus a light OSP pass (step 3), the TAG challenge (step 4),
  the security questionnaire (step 5).
- Status: draft for review. Nothing here is a finding yet.

## 2. Novelty statement

What LP2P adds to the web threat model, in order:

1. **A second user agent as a principal** (`PR-UA-B`): as powerful as the
   first, under someone else's control, or nobody's.
2. **An asserted origin consumed in new ways**: it routes a session to a
   receiver, gates the server's grant prompt, and is matched against
   `allowedOrigins` ("Peer advertisement", "Incoming sessions").
3. **Pairing state that outlives pages and sites**: trust anchors, known peers,
   grants (`S1`-`S3`); one device identity presented to every origin's sessions.
4. **Transitive trust**: vouching anchors, and the same key acting as a Local
   HTTPS CA for a namespace.
5. **Headless peers**: no user to consent; the device asserts any origin.
6. **Offline as a condition**: no online authority behind any of the above.

The hostile network: the channel (TLS 1.3, QUIC) is inherited; new are
unauthenticated discovery feeding a user-facing picker, a ceremony run in the
network's presence, and advertised presence to everyone on the medium.

Everything else inherits existing analysis: permissions, Permissions Policy,
secure contexts and the same-origin policy within one UA (web platform);
channel security (TLS 1.3, QUIC); PAKE and mDNS (IETF).

## 3. Premises

- **Authenticated != honest.** The ceremony binds a key to "the thing in the
  room", nothing more. It says nothing about that device's intent, its
  software, its owner next year, or the origin its page was served from.
  Under vouching it binds to whatever the hub vouches for, which the user may
  never have seen.
- **Offline is a condition, not a boundary.** There is no online authority:
  - no revocation feed: an anchor stays trusted until the user removes it;
  - no safe-browsing or reputation for peers or `.local` hosts;
  - an untrusted clock: certificate validity and "short-lived" leaves
    (Local HTTPS "Validation") mean little without time;
  - page code may come from a service worker cache, so the running code can be
    old and its origin unreachable.
- **The hostile network is first-class.** The feature must be safe on cafe,
  hotel or airport Wi-Fi. What matters is where the network touches the user
  (picker contents, ceremony prompts, display names), discovery (presence,
  spoofing, suppression) and persistence (what first contact leaves behind),
  not TLS boilerplate.
- **Two users.** Pairing is between two people. Each side's own user is honest
  towards their own UA; the other side may not be. The server-side grant prompt
  is User B's consent, not User A's; on a headless peer nobody consents.

What the contract promises over the network, including relayed paths:

| Property | Contract ("Transport abstraction" and below) | Left to the binding or not promised |
|---|---|---|
| Confidentiality, integrity | "must provide end-to-end confidentiality and integrity for all communication with a peer, including when the path to the peer goes through an intermediary" | Mechanism |
| Peer authentication | `authenticate a session`: mutual, verifies the peer identity | Ceremony mechanics, brute-force resistance |
| Origin binding | The binding "binds it to the authenticated session" | Mechanism |
| Origin confidentiality | Requesting and accepted origins: "Neither is exposed to a peer that is not authenticated" | Any authenticated peer learns them |
| Discovery authenticity | Not promised: "A discovered peer identity is not verified"; names and capabilities are hints | Everything |
| Locality, physical presence | Not promised. "Local communication medium" is a definition, not a check | Binding. A relay, or a NAT exposing an agent to the Internet (OSP "Remote active network attackers"), removes it |
| Metadata (addresses, timing, session count, presence) | Not promised. "No IP information is exposed to an origin" covers origins only | Binding. OSP treats IPs and mDNS data as public |
| Availability | Not promised | Binding (OSP "Denial of Service") |
| Ceremony reachability off-LAN | Not addressed | Binding. OSP: the `at` TXT field, so off-LAN hosts can't start authentication |

## 4. Principals

The network is not a principal: it is an adversary (`ADV-NET-*`) and an edge
(`TB3`).

| ID | Principal | Controls | Trusted by the local UA for | Note |
|---|---|---|---|---|
| `PR-USR-A` | User A | UA A, its UI | Choosing a peer, ceremony input, removals | Honest towards UA A; may be coerced (`ADV-NEARBY`) |
| `PR-USR-B` | User B, or nobody | UA B's UI | Advertise consent, ceremony, grant prompt on B | Absent on a headless peer |
| `PR-PAGE-A` | Page A, origin O | Script in UA A (window or worker) | Nothing: every argument is hostile (`TB1`) | Requests or advertises |
| `PR-PAGE-X` | Another origin X in UA A | Iframe, other tab, worker | Nothing | Shares `S1`, `S2` with O (`TB2`) |
| `PR-PAGE-B` | Page B, origin O', or device logic on a headless peer | Script in UA B, or firmware | Nothing; its origin is asserted by UA B | Behind `TB5`, not enforced |
| `PR-UA-A` | User agent A | Its stores, UI, binding | It is the TCB (`ASM-1`) | Client in D1, server in D2 when roles swap |
| `PR-UA-B` | User agent B | Its stores, UI, binding, the origin it asserts | Only what the ceremony bound: its key | May be a browser, a TV, a NAS, a hub |
| `PR-HUB` | Hub, vouching party | A key that vouches for devices "within limits defined by the transport binding" | Whatever the user accepted at the hub ceremony | SETTLE scope, see section 9 |
| `PR-LHTTPS` | Local HTTPS consumer inside UA A | Path validation for `.local` hosts | Reads `S1`; its trust anchor check gates `add a trust anchor` | Shares the store; rest of Local HTTPS deferred |

## 5. Adversaries

| ID | Name | Position | Capabilities | Wants | Scope note |
|---|---|---|---|---|---|
| `ADV-PAGE` | Malicious page (either side) | Script in a window, a third-party iframe given `allow="local-peer-to-peer"`, a dedicated, shared or service worker; or an abusive first party | Call the API repeatedly, set `nickname` and `allowedOrigins`, time prompts, send any bytes to a granted peer, keep a connection open | Reach or enumerate devices without consent, spoof prompts, use a granted peer as exfiltration, C2 or tunnel, profile the user | In. XSS as such is out; its consequences through the API are in |
| `ADV-NET-LAN` | Hostile local network | Another client on the same Wi-Fi or LAN, a rogue access point, the AP operator | Observe, inject, drop, replay; spoof or suppress advertisements; clone display names; start sessions to any advertiser; flood | Presence and topology, impersonation at first contact, PIN brute force, metadata (who pairs with whom, how many origins), DoS | First-class. Includes home networks with a compromised device |
| `ADV-NET-WAN` | Wider or relayed network | Relay operator, ISP, Internet host reaching an agent exposed by a misconfigured NAT | As `ADV-NET-LAN` minus multicast, plus off-LAN reach | Same, at scale and remotely | First-class. Contract promises C+I end to end; locality and metadata are left to the binding |
| `ADV-PEER-NEW` | Unpaired malicious peer | On the medium, advertising, before any ceremony | Advertise any display name, capabilities, nickname; trigger ceremonies; refuse or accept | Be picked and paired instead of the real device (evil twin), learn the requesting origin | In. First contact only |
| `ADV-PEER-PAIRED` | Paired-then-malicious or formerly trusted peer | Holds a key in the user's `S1` (directly or via a hub) | Assert any requesting origin, display name and capabilities on an authenticated session; accept every session and record origins; recognise UA A across sites; send arbitrary data; flood channels and transports | Origins visited, cross-site linking, a channel into pages, persistence after the relationship ended | **Headline.** Resold device, ex-partner's phone, compromised NAS, headless device |
| `ADV-HUB` | Compromised or untrustworthy hub, stolen vouching key | Holds a vouching key in `S1` | Mint credentials for new peers that never met the user; certify any host in its Local HTTPS namespace | Everything `ADV-PEER-PAIRED` wants, for any number of identities | SETTLE scope; framed here, analysed later |
| `ADV-NEARBY` | Nearby person at pairing time | Physically present, or able to coerce the user | Shoulder-surf a PIN or QR, choose a display name equal to the real device's, trigger a PIN display at will on a TV, tell the user "enter this code", take the device | Get paired in place of, or in addition to, the intended peer | In. Covers intimate-partner abuse |
| `ADV-TRACKER` | Cross-site tracker via a cooperating peer | Two or more origins that collude with one peer, or a peer operator | Correlate UA A's device identity, the requesting origins and timing across sites and locations | Cross-site and cross-location profile of the user | In. Also via `.local` site data and anything `getDevices` (LP2P issue 55) would add |

## 6. Assets

| ID | Asset | Exposed to | Why it matters |
|---|---|---|---|
| `AS-ANCHOR` | Trust anchors in `S1`; the private keys behind them on peers and hubs | UA A's TCB; private keys held by peers and hubs | Integrity of `S1` decides who is authenticated. Vouching anchors are critical: one key, many peers, a Local HTTPS namespace |
| `AS-DEVID` | Peer identity, and UA A's own device credential | Every authenticated peer, for every origin's session; the network to the extent the binding exposes it | A stable cross-site and cross-location identifier for a portable device |
| `AS-ORIGIN` | Requesting origin; origins a server accepts | Authenticated peers | Browsing history of either user |
| `AS-GRANT` | Peer grants (`S3`) | UA only; their effects are visible to the peer | Decide which origin may use which peer without a prompt (server side) and which `.local` hosts a page may reach |
| `AS-KNOWN` | Known peers (`S2`) | UA only; shown in the picker | History of devices paired with; decides "known" marks |
| `AS-TOPO` | Discovery results, topology | The network; never origins in full ("Peer discovery") | Fingerprinting the network and locating the user |
| `AS-NAMES` | Display name (asserted by a UA), nickname (chosen by script) | Users in pickers and prompts; the network for advertised names | Spoofing in UI; PII ("Michiel's phone") |
| `AS-SECRET` | PIN or QR secret | Both users, people nearby, the screen showing it | Its secrecy is the ceremony's only defence against a MITM at first contact |
| `AS-CONTENT` | Session content on data channels and WebTransport | Both pages and both UAs | User data |
| `AS-EKM` | Exported keying material | The page that owns the WebTransport session | Lets an application prove its origin on its own; a leak lets another party pose as it |
| `AS-ATTN` | User attention, consent quality | Pages (by triggering prompts), peers (by triggering ceremonies) | Prompts only protect if the user understands them; fatigue and spoofing erode them |
| `AS-PRESENCE` | Advertised presence | Everyone on the medium while advertising | Reveals that a device and a site are active, and where |
| `AS-LSITE` | Site data of `.local` origins | The `.local` origin's pages | Cleared when the anchor goes (Local HTTPS "Origins, Fetch and Local Network Access"); a tracking vector if not |

## 7. Trust boundaries

| ID | Boundary | What crosses | Enforced? |
|---|---|---|---|
| `TB1` | Page <-> UA | WebIDL calls; `nickname`, `allowedOrigins`, labels, data; connections, events, one error | Yes: secure context, Permissions Policy, opaque-origin check |
| `TB2` | Origin <-> origin in one UA | Shared `S1` and `S2` (LP2P issue 24), per-origin `S3`, iframes, workers | Yes, by `S3` and the requesting origin per session |
| `TB3` | UA <-> network | Advertisements, discovery hints, session setup, ceremony, all traffic | Only after `authenticate a session`; everything before is untrusted |
| `TB4` | **UA A <-> UA B (primary)** | Credentials, requesting origin, accept or refuse, display name, capabilities, channels | Integrity and peer key: yes. Claims inside (origin, name, capabilities): asserted |
| `TB5` | Peer UA <-> peer page | Which origin's page really receives the session | **Assumed, not enforced** (drawn dashed) |
| `TB6` | User <-> UI | Picker, ceremony, advertise consent, grant prompt, management UI | Native UI; carries page-controlled and peer-asserted strings |
| `TB7` | Persistent state | `S1`, `S2`, `S3`; clearing, incognito, profiles, Local HTTPS site data | Per binding's persistent-state rules ("Local state") |

## 8. Scope

**In scope**

- The API: every interface, algorithm and note in `index.bs`.
- The transport-binding contract: "Transport abstraction", "Peers and
  sessions", "Operations".
- Peer management: "Local state", "Peer advertisement", "Peer discovery",
  "Peer authentication", "Peer grant".
- Local HTTPS's **shared trust store** only: the same anchors validate HTTPS
  servers under `.local`; removing an anchor clears site data; the Local HTTPS
  trust anchor check keeps one key per host.
- Vouching, framed in section 9.
- The hostile network, LAN and wider or relayed, as in section 3.
- Headless peers and the second user.
- As deltas against the diagrams, because they are not in the spec today:
  service workers (LP2P issue 49) and a `getDevices`-style re-access API (LP2P
  issue 55).

**Out of scope**

- The rest of Local HTTPS (fetch, upgrade signal, UI, scopes): a later annex.
- OSP as a protocol: it gets a light conformance pass against the binding
  obligations in step 3.
- SPAKE2, QUIC and mDNS internals: they inherit IETF analysis.
- A compromised local UA or OS.
- Cryptographic breaks.
- The device's robustness against bytes a page sends it.
- Implementation bugs.
- Page compromise (XSS) as such.

## 9. Vouching and delegated trust (SETTLE scope)

- In the spec: a trust anchor has scope "one peer" or "vouching" ("Peer
  authentication"). A vouching anchor is added only by a ceremony with the
  vouching party itself, and covers "any peer the anchor vouches for, within
  limits defined by the transport binding".
- The three examples: a self-anchored device; a hub-issued device anchored as
  one peer; a hub trusted for its devices (vouching), which replaces the
  one-peer anchors with its key.
- What it changes: a peer can be authenticated without ever meeting the user.
  The ceremony binds the hub, not each device.
- So "a lying peer can only be one the user already authenticated and trusts
  with what they send it" ("Impact on same-origin policy") is false under
  vouching: the liar may be a device the hub minted yesterday.
- The same key doubles as a Local HTTPS CA for a namespace such as
  `*.home.local` (Local HTTPS "Trust anchors").
- The binding defines the limits; the contract does not yet say what a binding
  must enforce about them.
- Local HTTPS's claim "A referee never widens a page's grants" is to be tested,
  not assumed.
- Step 1 only frames this; analysis comes in later steps.

## 10. Assumptions

- `ASM-1` The local UA is the TCB: its UI, stores and binding behave as
  specified.
- `ASM-2` The link from the remote UA to the remote page (`TB5`) is assumed,
  not enforced.
- `ASM-3` The remote origin is asserted, not authenticated ("Peers and
  sessions": "consistent for the session, not that it is authentic").
- `ASM-4` The user's own UA is honest; the other side's may not be.
- `ASM-5` Physical presence holds where the binding's ceremony needs it.
- `ASM-6` UI is implementation-defined but follows the spec's notes: dismissal
  is not detectable by script, and every failure rejects with the same error.
- `ASM-7` The binding meets the contract's MUSTs; whether OSP does is step 3.

## 11. Cross-cutting axes (future threat-table columns)

| Axis | Values | Why it matters | Where in the spec it bites |
|---|---|---|---|
| Adversary | `ADV-*` from section 5 | Who acts | All |
| Contact | first contact, repeat | Spoofing and ceremony threats live in first contact; tracking and stale trust in repeat | "Peer authentication", "Incoming sessions" step 1, `start()` step "Wait until" |
| Network | home, public | Who else is on the medium; whether presence and names leak to strangers | "Peer advertisement", "Peer discovery", "Transport abstraction" |
| Context | window, dedicated or shared worker, service worker | Who owns the prompt; whether a document exists; background reach | "Permission Policy Integration", `Exposed=(Window,Worker)` on every interface |
| Origin match | same-origin, `allowedOrigins` | The TAG's origin model; cross-origin delivery on an asserted origin | "Peer advertisement" (allows an origin), "LP2PReceiverOptions" |
| Peer kind | browser, headless | Whether `TB5` exists, whether anyone consents on B | "Impact on same-origin policy", "Incoming sessions" step 5 |
| Anchor | self-anchored, vouched | Whether the user ever met the peer | "Peer authentication", "Local state" (`add a trust anchor`) |

## 12. How to cite

- Diagram elements: `E*` external entities, `P*` processes, `S*` stores, from
  `01-dfd.md`; IDs are stable across diagrams.
- Flows: `F0.n` context, `F1.n` connect, `F2.n` advertise and incoming,
  `F3.n` steady state and lifecycle, `F4.n` trust store; for example `F1.13`.
- From this file: `PR-*` principals, `ADV-*` adversaries, `AS-*` assets,
  `TB1`-`TB7` boundaries, `ASM-*` assumptions.
- Spec text: by section name in quotes, for example per "Peer grant".
