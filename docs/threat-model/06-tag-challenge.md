# LP2P threat model: TAG challenge

Step 3 (the plan's step 4, pulled forward). The model walked against the TAG
Web Platform Design Principles (https://w3ctag.github.io/design-principles/),
the Privacy Principles (https://www.w3.org/TR/privacy-principles/) and the
earlier TAG review (https://github.com/w3ctag/design-reviews/issues/932,
with the 2024-06-12 breakout and plenary minutes). Only relevant principles
are kept. `SF-n` are proposed spec fixes, `BO-n` binding obligations, both
defined in `03-mitigations.md` and `04-binding-obligations.md`.

Verdicts: **Meets**, **Partial**, **Gap** (a proposed spec fix exists), or
**Open** (a decision for the group).

## 1. Design Principles

| Principle | Threats | Verdict | Mitigation |
|---|---|---|---|
| 1.2 Safe to visit a web page | `T-API-1`, `T-ABU-1` | Partial | Picker and grants gate access; activation missing: `SF-9` |
| 1.3 Trusted UI should be trustworthy | `T-DISC-3`, `T-CER-1`, `T-DSP-3` | Gap | Display rules for script- and peer-chosen strings, prompt contents: `SF-10`, `SF-7` |
| 1.4 / 1.4.1 Design for user intent, help users decide | `T-DSP-1`, `T-DISC-4`, `T-VCH-1`, `T-CER-3` | Gap | Deterministic delivery `SF-1`; the picked peer is the one authenticated `SF-6`; two-stage vouching `SF-5`; visible pairings `SF-10` |
| 1.5 Use identity appropriately in context | `T-PEER-2`, `T-NET-1`, `T-STO-2` | Partial | One device credential for all origins; `BO-7`, `BO-9`, `SF-8`; partitioning is LP2P issue 24 (Open) |
| 1.7 Add new capabilities with care | `T-VCH-*`, `T-ABU-*` | Partial | This model; `SF-2` |
| 1.10 Minimize user data | `T-PEER-3`, `T-NET-3` | Partial | Origin only after auth (Meets, `BO-5`); advertisement contents `BO-8` |
| 2.8 Secure contexts | | Meets | `[SecureContext]` on every interface |
| 2.9 / 2.10 Private browsing | `T-STO-2` | Gap | `SF-8`, `BO-9` |
| 2.12 Require user activation for powerful APIs | `T-API-1` | Gap | `SF-9` |
| 2.13 / 2.14 Non-fully-active and BFCache | `T-API-4`, `T-API-7` | Gap | Teardown `SF-23`; BFCache `SF-17` (questionnaire item 17 is still "open point") |
| 6.12 Cancel with AbortSignal | | Meets | `LP2PRequestStartOptions.signal` |
| 9.1 Don't expose unnecessary information about capabilities | `T-API-3` | Partial | One capability bit via `NotSupportedError`: `SF-13` |
| 9.2 Care when selecting or enumerating devices | `T-ABU-2`, `T-GD-*` | Meets (today) | UA picker, nothing enumerated. A `getDevices` addition would need the analysis in `02-threats.md` section 5 |
| 9.4 Be proactive about safety | `T-ABU-3`, `T-PEER-1` | Gap | `SF-11`; expiry or review `SF-21` |
| 10.2 Dedicated workers | `T-API-5` | Partial | Exposed, but no UI-owner rule: `SF-4` |
| 10.3 Only computational features everywhere | `T-API-6`, `T-SW-*` | Gap | `Exposed=Worker` reaches service workers: `SF-4` |
| 10.8 Best crypto, expect it to evolve | `T-CER-4` | Partial | Binding-defined; PAKE required by `BO-2`; OSP's 20-bit floor |
| 11.2 Specify completely, avoid ambiguity | `T-DSP-1`, `T-DSP-2`, `T-DSP-5` | Gap | "chosen by the user agent" is the ambiguity: `SF-1`, `BO-6`; start race `SF-24` |

## 2. Privacy Principles

| Principle | Threats | Verdict | Mitigation |
|---|---|---|---|
| 1.1.1 Consent; 2.12 Consent and withdrawal | `T-STO-1`, `T-STO-6`, `T-DSP-3`, `T-PEER-1` | Gap | A "no" leaves trust behind (`SF-3`); removal must exist and revoke (`SF-10`, `SF-28`), also for sessions in setup (`SF-22`); grant scope (`SF-7`) |
| 1.2 Vulnerability; 2.9 Protecting users from abusive behaviour | `T-CER-3`, `T-PEER-1`, `T-DISC-4` | Gap | Visible pairings and indicators `SF-10`; "remove" revokes `SF-28`; review or expiry `SF-21`; no silent substitution `SF-6` |
| 2.1 Identity; A.5 Recognition | `T-PEER-2`, `T-NET-1`, `T-NET-2`, `T-STO-2` | Partial | Cross-context recognition by a peer is inherent to pairing; network-level recognition is not: `BO-7`, `BO-8`, `SF-8` |
| 2.2 Data minimization | `T-PEER-3`, `T-NET-3`, `T-API-3` | Partial | `BO-5`, `BO-8`, `SF-13` |
| 2.3.1 Unavoidable information exposure | `T-NET-3` | Partial | Advertising is presence by definition; the consent prompt should say who can see it: `SF-10` |
| 2.7 Collective privacy | `T-NET-3`, `T-NET-1` | Partial | Other people's devices appear in a user's picker on public networks; `BO-8` limits what they carry |
| 2.8 Device owners and administrators | `T-ABU-1` | Gap | No enterprise or CSP hook: `SF-16` |
| 2.11 Transparency | `T-CER-3`, `T-PEER-4`, `T-API-4` | Gap | `SF-10` |
| 2.13 Notifications and interruptions | `T-API-1`, `T-CER-1` | Gap | `SF-9`, `SF-15` |

## 3. Earlier TAG review (design-reviews 932)

| Point (who) | Threats | Status now | Remaining |
|---|---|---|---|
| Provide a threat model; abuse of functionality (Simone Onofri) | all | This model | Publish it with the spec (plan output 1) |
| Protocol tunnelling, MITRE T1572 (Simone Onofri) | `T-ABU-1` | Partial: a picked, accepting, authenticated peer only | `SF-16` |
| Discovery phase used for fingerprinting and profiling (Simone Onofri) | `T-ABU-2`, `T-NET-1`, `T-NET-2` | Origins: Meets ("Peer discovery"). Network: OSP fails `BO-7`, `BO-8` | Binding fixes |
| Similarities with UPnP (Simone Onofri) | `T-ABU-3`, `T-NET-4` | Partial | `SF-11`, `BO-4` |
| A device that gives away my position (Simone Onofri) | `T-NET-2`, `T-GD-1` | Partial | `BO-8`; do not add picker-less re-access without the `T-GD-*` analysis |
| Abuse, e.g. abusive partner (Dan Appelquist) | `T-CER-3`, `T-PEER-1`, `T-DISC-4` | Gap | `SF-6`, `SF-10`, `SF-28`, `SF-21` |
| "What's the origin you're talking to"; bind it to the TLS connection (Martin Thomson) | `T-DSP-4`, `T-VCH-1` | Meets for binding (`BO-5`; "Peers and sessions"); asserted, Origin-header model | `SF-2`, `SF-11` |
| Assumes physical access? Not clear in the description (Martin Thomson) | `T-NET-4`, `T-CER-2` | Partial: `ASM-5`; locality is not promised | State it in the spec; `BO-4`, `SF-19` |
| Use WebTransport, client/server roles, drop WebRTC-style (Martin Thomson) | | Largely done: roles, `LP2PQuicTransport`; data channels still open (LP2P issue 29) | Group decision |
| Do the protocol in the IETF (Martin Thomson) | | Done in structure: abstract binding contract; SETTLE as the venue | `04-binding-obligations.md`, `05-vouching-settle.md` |
| Bootstrapping devices without I/O; accessibility (Matthew Atkinson) | `T-CER-5` | Gap | Static secrets only in a physical pairing mode (`BO-3`); accessible ceremonies (OSP UI item 6) |
| Security risk deflected onto end users, as with WebUSB and Web Bluetooth (plenary) | `T-API-1`, `T-DISC-3`, `T-VCH-1`, `T-STO-1` | Partial | `SF-9`, `SF-10`, `SF-5`, `SF-3` |
| Security of the handshake (plenary) | `T-CER-*`, `T-DISC-1` | Partial | `BO-2`, `BO-3`; the `psk-needs-presentation` bug the OSP binding works around |
| HTTPS without Web PKI; mTLS (Peter Linss) | `T-VCH-*`, `T-STO-5` | Partial: Local HTTPS sketch, shared store | `SF-5`, `SF-20` |

## 4. Gaps as proposed spec fixes

Every Gap above maps to an `SF-n` in `03-mitigations.md` section 4. Nothing
found in this walk lacks one.
