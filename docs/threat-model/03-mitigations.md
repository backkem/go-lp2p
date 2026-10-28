# LP2P threat model: mitigations

Step 3. One disposition per threat in `02-threats.md`:

- **SH** spec-handled, citing the section;
- **SF-n** proposed spec fix (section 4);
- **BO-n** binding obligation (`04-binding-obligations.md`);
- **AR** accepted risk, with the reason.

A row can carry more than one; the first is the main one.

## 1. Dispositions

| Threat | Sev | Disposition | How |
|---|---|---|---|
| `T-DISC-1` | M | SH, BO-2, BO-14 | The ceremony ("Peer authentication") is the defence; names are hints ("Peers and sessions"). Residual with a captured secret: `T-CER-2` |
| `T-DISC-2` | M | SF-6, BO-13 | Treat "known identity, fails to chain" as an identity change, not a new peer |
| `T-DISC-3` | M | SF-10, BO-14 | Display rules for nickname and display name in native UI |
| `T-DISC-4` | H | **SF-6** | The session must authenticate the identity the user picked; a known mark is shown only after it does |
| `T-CER-1` | M | SF-15, BO-4, BO-16 | Server-side ceremony only while advertising with consent; UI names the requester; rate limits |
| `T-CER-2` | M | AR; BO-3 | Shoulder-surfing a one-time PIN is accepted: the same as any pairing code. Single use and rotation limit it |
| `T-CER-5` | H | BO-3 | Static secrets only in a pairing mode entered by physical action on the device |
| `T-CER-3` | H | SF-10, SF-21; AR (residual) | Pairings visible and removable (must), indicator while connected, review of unused anchors. Residual: someone with the unlocked device can do anything the user can |
| `T-CER-4` | M | BO-2, BO-3 | PAKE plus online-guess limits |
| `T-DSP-1` | H | **SF-1** | Remove the fallback; refuse instead (section 2) |
| `T-DSP-2` | M | BO-6, SF-1 | Advertisement chosen by the client and covered by authentication |
| `T-DSP-3` | M | SF-7 | Key server grants on the requesting origin too; define the prompt |
| `T-DSP-5` | M | SF-24 (with SF-1) | Bind the advertisement before it can report sessions; `stop()` takes down the prompt and the pending advertisement |
| `T-DSP-4` | M | SH ("Peers and sessions": asserted, Origin-header model); SF-2, SF-11, SF-18 | Apps authenticate each other with the keying material exporter; spec text corrected |
| `T-PEER-1` | H | SF-10, SF-28, SF-21; AR (no revocation offline) | User removal is the only revocation: make it findable, make "remove" actually revoke, prompt review |
| `T-PEER-2` | M | AR; BO-15; open (LP2P issue 24) | Inherent to pairing (Local HTTPS says so). Per-partition client credentials would cut it; decision for the group |
| `T-PEER-3` | M | SH (BO-5); SF-5 | Only authenticated peers learn it; two-stage vouching narrows "authenticated" to confirmed |
| `T-PEER-4` | M | SF-10 | Indicator while a session is open, on both sides |
| `T-PEER-5` | M | SF-29, BO-16 | Rate-limit grant prompts per peer and remember a decline for a while; bound held channels and sessions |
| `T-VCH-1` | H | **SF-2**, **SF-5** | Correct the claim; two-stage confirmation (`05-vouching-settle.md`) |
| `T-VCH-2` | H | SF-12 (BO-11), SF-20 | Contract minimum for vouching; namespace confirmed in the ceremony |
| `T-VCH-3` | H | SF-5, BO-11(d); AR (residual) | Confirmation per device; bounded validity. Residual: a stolen key still authenticates what it mints until removed |
| `T-VCH-4` | M | SF-5, BO-11(b) | Confirmation shows "via <hub>"; attested names. Residual accepted: confirmation is not verification |
| `T-VCH-5` | M | SF-26, SF-10 | Merging runs the removal cascade (site data of absorbed namespaces); ceremony UI says what the choice absorbs and what removal drops |
| `T-VCH-6` | M | SF-27 | Local HTTPS validation also checks the subject of a one-peer anchor |
| `T-STO-1` | M | **SF-3** | Commit trust only on success |
| `T-STO-2` | M | SF-8, BO-9, BO-10 | Clear with browsing data; separate per profile and private mode |
| `T-STO-3` | M | SF-5, SF-20; open (LP2P issue 24) | Local HTTPS effect bounded by confirming hosts and namespaces; the timing signal is accepted (a ceremony's human delay) |
| `T-STO-4` | M | BO-12, SF-25 | Proof of possession of both keys; replacement limited to that peer's anchor, with trust anchor checks |
| `T-STO-6` | M | **SF-22** | Revocation also ends sessions in setup; later steps re-check before granting or delivering |
| `T-STO-5` | M | SF-20 | No nesting names; namespace confirmed in the ceremony |
| `T-API-1` | M | SF-9 | Transient activation for `start()` in a window |
| `T-API-2` | M | SF-14 | Key grants by (top-level site, origin) |
| `T-API-3` | L | SF-13; AR (timing) | Uniform `NotAllowedError`, capability filtering. Timing of human actions is accepted |
| `T-API-4` | M | SF-10, SF-17 | Advertising indicator; stop advertising when not fully active |
| `T-API-5` | M | SF-4 | Define the UI owner for worker requests |
| `T-API-6` | M | **SF-4** | Exclude service workers until LP2P issue 49 is decided |
| `T-API-7` | M | **SF-23**, SF-17 | Teardown steps when a document or worker goes away |
| `T-NET-1` | H | BO-7 | No long-lived client credential before selection |
| `T-NET-2` | M | BO-8; AR (traffic shape) | No stable advertised identifier. Traffic analysis of who talks to whom is accepted, as for any LAN protocol |
| `T-NET-3` | M | SF-10, BO-8 | Advertise prompt says who can see it; nickname bounded |
| `T-NET-4` | M | BO-4, SF-19 | Ceremony needs the advertisement token; UI discloses non-direct paths |
| `T-NET-5` | L | BO-16; AR | Inherited; the ceremony bounds steering |
| `T-ABU-1` | M | SF-16; AR (device behaviour) | Policy hook (CSP or enterprise); what devices relay is out of scope |
| `T-ABU-2` | L | SH ("Peer discovery", "Personally identifiable information") | Origins never see discovery; residual presence signal accepted |
| `T-ABU-3` | M | SF-11 | Say that peers, not only UAs, must not authorise on the requesting origin |

Candidate surface (decision input, not findings):

| Threat | Sev | If LP2P adds it, it needs |
|---|---|---|
| `T-SW-1` | H | No receivers in service workers, or an always-visible advertising indicator and per-page consent |
| `T-SW-2` | H | No picker-less access from service workers; sessions only handed over from a document; never combine with `T-GD-*` (absorbs `T-GD-3`) |
| `T-SW-3` | M | Accept: same as other SW-persisted risks; document it |
| `T-SW-4` | H | A Permissions Policy definition for service workers (none exists today); otherwise do not expose |
| `T-SW-5` | L | UA lifetime caps |
| `T-GD-1` | H | Re-access only while a document of the origin is visible, with an indicator; not from workers |
| `T-GD-2` | M | Return opaque per-origin handles, no names; nothing before a grant |
| `T-GD-4` | M | Grant expiry, or the picker after a period of non-use |
| `T-GD-5` | M | Keep the picker as the default; re-access is an opt-in per grant |

**Recommendation on the candidates:** keep both out of this version.
Service-worker exposure has no consent anchor and no policy definition
(`T-SW-1`, `T-SW-4`); picker-less re-access turns discovery from a user
action into a presence oracle (`T-GD-1`). Each is H without new machinery.

## 2. Receiver fallback: the fix and the determinism check

**Today.** "Incoming sessions" step 2: the receiver the client picked, "if
its `[[State]]` is "advertising" and it allows the requesting origin.
Otherwise, let |receiver| be another LP2PReceiver for which this holds,
chosen by the user agent, for example by asking the user." The paragraph in
"Peer advertisement" says the same ("only to a started LP2PReceiver that
allows ...").

**Proposed wording** (replaces step 2):

> 2. Let |receiver| be the {{LP2PReceiver}} whose {{LP2PReceiver/[[Advertisement]]}}
>    is the [=advertisement=] |session| was reported for. If there is none, its
>    {{LP2PReceiver/[[State]]}} is not "`advertising`", or it does not
>    [=allows an origin|allow=] the [=protocol/requesting origin=], [=refuse=]
>    |session| and abort these steps.

and in "Peer advertisement":

> The user agent must deliver an [=incoming session=] only to the
> {{LP2PReceiver}} whose [=advertisement=] it was reported for, if that
> receiver is started and [=allows an origin|allows=] the session's
> [=protocol/requesting origin=], and [=refuse=] the session otherwise. A
> refusal is indistinguishable, to the [=protocol/client=], from the server's
> user declining.

**Determinism check** (after the fix):

| Property | Holds? | Why, or what is still needed |
|---|---|---|
| One advertisement <-> one receiver | Yes | `start an LP2PReceiver` creates one advertisement per receiver; `lp2p.makeDiscoverable()` creates one receiver |
| The client names the advertisement | **No, contract gap** | `establish a session` takes a peer and an origin, not an advertisement; the picker shows advertisements (nickname) but hands back a peer. Fix: the picker returns an advertisement, `establish a session` takes it (`SF-1`, `BO-6`) |
| The advertisement can't be swapped in transit | Binding | `BO-6`. OSP fails it today (`04-binding-obligations.md`) |
| The origin is checked only against that receiver | Yes | After the fix, step 2 has a single candidate |
| Grant lookup uses that receiver's origin | Yes | Step 3; with `SF-7` also the requesting origin |
| Repeat sessions | Yes | Each session is matched to the advertisement it was opened against; a stopped receiver's advertisement is gone, so a new `start()` needs a new pick |
| Several tabs of one origin, each with a receiver | Yes | Distinct advertisements; the user picks one. Same nicknames look alike in the picker, but stay within one origin |
| Receivers of two origins with the same nickname on one device | Picker ambiguity, not dispatch | A page on B can imitate another origin's nickname; covered by `T-DISC-3`, `SF-10` |
| Receiver stops after the grant prompt | Defined | Step 7 closes the session after accepting; the grant from step 5 stays. Acceptable: User B granted it |
| Session to peer on the client | Needs `SF-6` | The authenticated identity must equal the picked one; today a paired peer can stand in for another known peer (`T-DISC-4`) |
| Session in the receiver start window | Needs `SF-24` | With the fallback gone, it is refused, but only after a ceremony may have run |

## 3. Two-stage vouching

Evaluated in `05-vouching-settle.md` section 3. **Verdict: adopt** (`SF-5`).
It reuses the picker (client) and the grant prompt (server) as the
confirmation, so the LP2P paths get no new prompt; Local HTTPS's
user-initiated navigation gets one per new vouched host. It narrows
`T-VCH-1` and `T-VCH-3` from silent to user-visible, and lets the "lying peer"
sentence be true again in a weaker form. It does not replace vouching limits
(`BO-11`).

## 4. Proposed spec fixes, ranked

**PR** = goes in the security and privacy PR; **FU** = follow-up.

| Rank | ID | Fix | Threats | Where | PR/FU |
|---|---|---|---|---|---|
| 1 | `SF-1` | **Remove the receiver fallback**; deliver only to the picked advertisement's receiver, refuse otherwise; `establish a session` takes the advertisement. Wording in section 2 | `T-DSP-1`, `T-DSP-2` | "Incoming sessions" step 2; "Peer advertisement"; "Operations" | PR |
| 2 | `SF-2` | **Correct the "lying peer" sentence.** Replace "but a lying peer can only be one the user already authenticated and trusts with what they send it" with: "Such a peer is [=/authenticated=]: the user paired with it in a [=trust ceremony=], or trusts a party that vouches for it, which the user may never have seen. Authentication identifies the device, not its intent or the origin of the page using it." (With `SF-5`: "... vouches for it and the user confirmed it.") | `T-VCH-1`, `T-DSP-4` | "Impact on same-origin policy" | PR |
| 3 | `SF-6` | **Identity change is not a new peer.** "If a discovered peer claims the [=peer/peer identity=] of a [=known peer=] and its credentials do not chain to that peer's [=known peer/anchor key=], the user agent must not mark it as known, and must warn the user before running a [=trust ceremony=] with it." And, in `start an LP2PRequest`: "If |session| is authenticated with a [=peer/peer identity=] other than the one the user selected, [=close a session|close=] it and run |fail|." | `T-DISC-2`, `T-DISC-4` | "Peer grant"; "Peer authentication" | PR |
| 4 | `SF-3` | **A declined peer and its anchor don't persist.** On the server, trust added by a ceremony is committed only when User B grants. In "Incoming sessions", move step 4 after step 5, and on decline or dismissal: "if a [=trust ceremony=] in step 1 added a [=trust anchor=], [=remove a trust anchor|remove=] it". On the client, keep the anchor after a refusal: User A chose to pair, and the refusal is about the origin, not the device (this avoids a repeat ceremony per origin) | `T-STO-1` | "Incoming sessions"; "Peer authentication" | PR |
| 5 | `SF-22` | **Revocation reaches sessions in setup.** `close the sessions with a peer` also covers sessions not yet exposed: "and each [=incoming session=] or session being established with the peer that is not yet exposed to script: [=close a session|close=] it, which makes the steps waiting on it abort." Before `grant a peer`, `Accept` and delivery, re-check that the peer is still authenticated; on abort after `grant a peer`, revoke it | `T-STO-6` | "Local state"; "Incoming sessions"; `start an LP2PRequest` | PR |
| 6 | `SF-4` | **Fix the worker mismatch.** `Exposed=(Window,DedicatedWorker,SharedWorker)` on every interface, until LP2P issue 49 is decided. Add: "The user interface for a request or receiver in a worker is shown for the worker's owner document, or, for a shared worker, an owning document that is [=fully active=] and visible; if there is none, the user agent treats it as dismissed." | `T-API-5`, `T-API-6`, `T-SW-4` | IDL; "Permission Policy Integration" | PR |
| 7 | `SF-5` | **Two-stage vouching**: an "unconfirmed" state for vouched peers; known only after the user confirms. Text in `05-vouching-settle.md` section 4 | `T-VCH-1`, `T-VCH-3`, `T-VCH-4`, `T-PEER-3` | "Peer authentication"; `add a known peer`; `acquire a local peer grant`; "Incoming sessions"; Local HTTPS "Scopes" | PR |
| 8 | `SF-23` | **Teardown.** When a document stops being fully active (unload, iframe removal) or a worker ends: stop its receivers, abort its pending requests (taking down UI), close its connections. Correct questionnaire Q18 | `T-API-7` | New "Lifecycle" section; questionnaire | PR |
| 9 | `SF-24` | **Receiver start race.** Set `[[Advertisement]]` before the advertisement can report an incoming session (or hold sessions until it is set); `stop()` takes down the advertise prompt and stops an advertisement that is starting | `T-DSP-5` | `start an LP2PReceiver`; `stop an LP2PReceiver` | PR |
| 10 | `SF-7` | **Server grant matches the consent.** Key server-side grants on (receiver origin, requesting origin, peer identity). The prompt shows the peer's verified display name, the receiver's origin, and the requesting origin as a claim | `T-DSP-3` | "Local state" (peer grants); "Incoming sessions" step 5; "User interface guidelines" | PR |
| 11 | `SF-28` | **"Remove" revokes.** The user-facing removal of a peer runs `forget a known peer` *and* removes its one-peer anchor; `remove a trust anchor` also forgets known peers that are no longer authenticated (no stale "known" marks) | `T-PEER-1` | "Local state" | PR |
| 12 | `SF-8` | **Clearing and private mode.** "When the user clears all browsing data, the user agent clears its [=local trust store=] and [=known peers map=] and replaces its own credentials. Private browsing sessions and user profiles each have their own." | `T-STO-2` | "Local state"; "Persistent State" | PR |
| 13 | `SF-9` | **User activation.** `start()` of `LP2PRequest` and `LP2PReceiver` in a `Window` consumes transient activation, else `NotAllowedError` | `T-API-1` | Both `start` algorithms | PR |
| 14 | `SF-10` | **Visibility and UI.** "Should let the user view ... and remove" becomes must; an indicator while advertising and while a session is open; nickname and unverified names shown as such, length-bounded, never as instructions; the advertise prompt says that everyone on the network can see the nickname; the vouching choice says what it absorbs | `T-CER-3`, `T-PEER-1`, `T-PEER-4`, `T-API-4`, `T-DISC-3`, `T-NET-3`, `T-VCH-5` | "Local state"; "User interface guidelines"; "Peer advertisement" | PR |
| 15 | `SF-11` | **Peers don't authorise on the requesting origin.** After "User agents must not grant anything ... on the strength of it", add: "Nor should any other implementation of a peer, such as device logic: to authorize an application, use credentials its origin issued, bound to the session with [=export keying material=]." | `T-ABU-3`, `T-DSP-4` | "Peers and sessions" | PR |
| 16 | `SF-12` | **Binding contract additions.** Add `BO-6`, `BO-7`, `BO-8`, `BO-11` (and the rest of `04-binding-obligations.md` as the group agrees) as requirements in "Transport abstraction" | many | "Transport abstraction" | PR (`BO-6`, `BO-11`); FU (rest) |
| 17 | `SF-13` | **Uniform errors.** Let requests declare the features they need, filter the picker on them, and have `lp2p.connect()` reject with `NotAllowedError` when the picked peer lacks WebTransport | `T-API-3` | "Peer grant"; `connect()` | FU |
| 18 | `SF-14` | **Partition grants** by (top-level site, origin) | `T-API-2` | "Local state"; the granularity issue | FU |
| 19 | `SF-15` | **Server-side ceremony rules.** A ceremony on the server runs only for an advertisement the user consented to; its UI names the requesting device; showing a PIN is not consent | `T-CER-1` | "Peer authentication"; "Incoming sessions" step 1 | FU |
| 20 | `SF-16` | **Policy hook.** A CSP directive or enterprise policy for LP2P connections | `T-ABU-1` | New section | FU |
| 21 | `SF-17` | **Non-fully-active documents.** Stop advertising when a receiver's document is not fully active; decide BFCache behaviour (questionnaire item 17) | `T-API-4` | `LP2PReceiver`; lifecycle | FU |
| 22 | `SF-18` | **Expose the requesting origin** (asserted) to the receiving page, as servers see `Origin` | `T-DSP-4` | `LP2PConnection` | FU |
| 23 | `SF-19` | **Off-LAN disclosure.** The UI shows when a session is not direct (`BO-4`); the spec states that locality is not verified | `T-NET-4` | "Transport abstraction"; "User interface guidelines" | FU |
| 24 | `SF-20` | **Local HTTPS namespaces.** The ceremony confirms the namespace; a one-peer device's name may not nest another anchor's hosts | `T-STO-5`, `T-VCH-2` | Local HTTPS "Trust anchors" | FU |
| 25 | `SF-21` | **Review of stale trust.** The UA may surface anchors unused for a long period for review or removal | `T-PEER-1`, `T-CER-3` | "Local state" | FU |
| 26 | `SF-25` | **Narrow key replacement.** Replace only the anchor(s) of the peer that proved continuity, run the trust anchor checks, and let Local HTTPS clear or keep site data explicitly | `T-STO-4` | `replace a trust anchor key`; Local HTTPS | FU |
| 27 | `SF-26` | **Merge runs the cascade.** In `add a trust anchor` step 4, absorbed one-peer anchors go through the removal steps that other specifications hook (Local HTTPS clears namespaces no longer covered) | `T-VCH-5` | `add a trust anchor` | FU |
| 28 | `SF-27` | **Local HTTPS checks the subject** of a one-peer anchor, not only its key and namespace | `T-VCH-6` | Local HTTPS "Validation" | FU |
| 29 | `SF-29` | **Grant-prompt limits.** Rate-limit grant prompts per peer, remember a decline for a period, time out pending incoming sessions | `T-PEER-5` | "Incoming sessions" step 5 | FU |

## 5. Accepted risks

| Risk | From | Why accepted |
|---|---|---|
| A paired peer recognises the browser across sites | `T-PEER-2` | Inherent to a device identity; bounded by the per-origin pick. Revisit with LP2P issue 24 |
| No revocation while offline | `T-PEER-1`, `T-VCH-3` | No online authority by premise; user removal and bounded validity are the substitutes |
| Shoulder-surfing a one-time PIN | `T-CER-2` | As with every pairing code |
| Timing of human actions | `T-API-3` | Can't be hidden without harming UX |
| Traffic analysis on the medium | `T-NET-2` | As for any LAN protocol; the contract does not promise metadata privacy |
| What a granted device does with the bytes | `T-ABU-1` | Device robustness is out of scope (`00-framing.md` section 8) |

## 6. Spec claims the model contradicts

| Claim | Where | Contradicted by | Fix |
|---|---|---|---|
| "a lying peer can only be one the user already authenticated" | "Impact on same-origin policy" | `T-VCH-1` | `SF-2` |
| "This is handled by the appropriate teardown logic & events" | Questionnaire Q18 | `T-API-7`: no teardown steps exist | `SF-23` |
| "By design no such information should be exposed" | Questionnaire Q6 | `T-API-3`: a capability bit via `NotSupportedError` | `SF-13` |
| "No peer attributes reach origins" | "Personally identifiable information" | `T-API-3` | `SF-13` |
| "other devices of the hub are not trusted" | "Peer authentication", example 2 | `T-VCH-6` (true for LP2P, not Local HTTPS) | `SF-27` |
| "the tuple is unambiguous within a profile"; "at most one key can certify a given host" | Local HTTPS "Origins ...", "Trust anchors" | `T-STO-4`, `T-VCH-5` | `SF-25`, `SF-26` |
| "A referee never widens a page's grants" | Local HTTPS security considerations | `T-STO-5` | `SF-20` |
| Delivery "only to a started LP2PReceiver that allows" the origin, read as safe | "Peer advertisement" | `T-DSP-1` | `SF-1` |
