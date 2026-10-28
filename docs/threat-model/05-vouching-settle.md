# LP2P threat model: vouching and delegated trust (SETTLE scope)

Step 3. Our position on vouching, framed as input to the IETF SETTLE work
(REQ-ARCH-02, the referee). Threats from `02-threats.md`; obligations from
`04-binding-obligations.md`.

## 1. What the spec does today

- A trust anchor's scope is "one peer" or "vouching". A vouching anchor comes
  only from a ceremony with the vouching party itself ("Peer
  authentication").
- From then on, any peer whose credentials chain to that key, "within limits
  defined by the transport binding", is **authenticated**: no ceremony, no
  user step (`F4.8`-`F4.10`).
- The same key is a Local HTTPS CA for a namespace (`*.home.local`), and
  every anchor applies to user-initiated navigations (Local HTTPS "Scopes").
- Adding a vouching anchor absorbs one-peer anchors with the same key; removing
  it drops all the hub's devices.

## 2. What goes wrong

| Threat | Sev | In one line |
|---|---|---|
| `T-VCH-1` | H | A device the user never met is authenticated and may assert any origin |
| `T-VCH-2` | H | The contract sets no minimum limits; "is a vouching party" is undefined |
| `T-VCH-3` | H | A compromised hub key is a silent, offline, unrevocable skeleton key |
| `T-VCH-4` | M | Vouched look-alikes: no ceremony breaks a name tie |
| `T-VCH-5` | L | Merge and removal side effects surprise the user |
| `T-PEER-3` | M | Requesting origins reach every vouched device |

And one spec sentence becomes false: "a lying peer can only be one the user
already authenticated" ("Impact on same-origin policy").

## 3. Two-stage vouching (evaluated)

**Proposal.** Split what the vouching anchor gives:

1. **Authenticatable.** The vouching anchor makes the hub's devices
   *verifiable*: their credentials chain, their identity is trusted, their
   display name is attested by the hub.
2. **Confirmed on first use.** Each device still needs an explicit,
   lightweight user confirmation the first time it is used: "Connect to
   printer.home.local? Vouched for by Home." No PIN, no ceremony. Only then
   does it become a **known peer**, or usable in Local HTTPS's user scope.

**Where the confirmation already exists.** The API already has a user moment
on both sides for a device it has never granted:

| Path | Today | Stage 2 needs |
|---|---|---|
| Client: `LP2PRequest.start()` | The user picks the device in the picker | The picker marks "new, vouched for by Home" instead of "known"; the pick is the confirmation |
| Server: incoming session from a vouched device | Grant prompt (step 5), because grants are keyed on peer identity | Move "add a known peer" (step 4) after the prompt; the prompt names the voucher |
| Server, headless | Nobody | Device policy; out of the user's model, as today |
| Local HTTPS, user-initiated navigation | Every anchor applies; a hub-minted host is trusted at once | New: a one-time confirmation for a host under a vouching namespace that isn't a known peer's |
| Local HTTPS, page request | Grant constraint: only granted peers' names | Nothing new: a grant implies a pick |

So the cost is low: no new prompt on the LP2P paths, one new confirmation on
the Local HTTPS navigation path, and a "vouched, unconfirmed" state.

**What it buys.**

- `S2` again means "devices the user has seen": it stays a list of devices
  the user has met, and `T-VCH-1`'s liar must first get past a user
  confirmation that names the voucher.
- A stolen hub key (`T-VCH-3`) can no longer add trusted `.local` hosts or
  known peers silently; it still authenticates whatever it mints, so it is a
  phishing aid, not a skeleton key.
- The "lying peer" sentence can be restored in a weaker, true form: a liar is
  a device the user paired with, or one a party they trust vouches for and
  they confirmed.
- A natural place to show provenance ("via Home"), which Local HTTPS's UI
  section already wants.

**What it doesn't fix.**

- A malicious or compromised hub can still mint a convincing look-alike that
  the user confirms (`T-VCH-4`): confirmation is not verification.
- Headless UAs have nobody to confirm.
- It does not bound what a hub may vouch for (`T-VCH-2`); that needs `BO-11`.
- Requesting origins still reach confirmed vouched devices (`T-PEER-3`).

**Verdict: adopt.** Two-stage costs one state and one prompt on the Local
HTTPS navigation path, keeps the hub's value (no PIN per device, free
credential rotation), and turns the hub key from a silent skeleton key into
something that needs the user once per device. It does not replace limits
(`BO-11`) or bounded validity.

## 4. Spec changes it implies

- **"Peer authentication"**: a peer whose credentials chain to a vouching
  anchor is authenticated, but becomes a known peer only after the user
  confirms it. Suggested text: "A peer authenticated through a [=trust
  anchor/scope/vouching=] [=trust anchor=] that is not a [=known peer=] is
  *unconfirmed*. The user agent adds it as a [=known peer=] only once the user
  has confirmed it, by selecting it in [=acquire a local peer grant=] or
  accepting it in [[#lp2p-receiver-incoming-session]]."
- **`add a known peer`**: precondition "the peer is self-anchored or
  one-peer anchored, or confirmed".
- **`acquire a local peer grant`**: mark unconfirmed vouched peers as such,
  with the voucher's name; selecting one is the confirmation.
- **"Incoming sessions"**: move step 4 ("Add a known peer") after step 5, and
  have the prompt name the voucher. This also fixes `T-STO-1` on that path.
- **`add a trust anchor`**: unchanged. The ceremony UI for "trust it for its
  devices" should say that each device will be confirmed on first use.
- **"Impact on same-origin policy"**: replace the "lying peer" sentence (see
  `03-mitigations.md`, `SF-2`).
- **Local HTTPS "Scopes"**: for a user-initiated navigation, a vouching
  anchor certifies only hosts of known peers; another host in its namespace
  needs a one-time confirmation first.

## 5. Requirements for SETTLE (our position)

1. **Referee credentials are recognisable** as such (CA-like, with name
   constraints), not by a claim in discovery (`BO-11`a, `T-VCH-2`).
2. **Per-device subject and attested name** in every vouched credential, so a
   UA can key known peers and confirmations on them and show "printer, via
   Home" (`BO-11`b).
3. **Enforced limits**: at least a namespace; ideally also roles (which
   services a device may offer) (`BO-11`c).
4. **Bounded validity and renewal from the referee**, so an offline
   compromise ages out without a revocation feed; acknowledge that without a
   trusted clock this is weak (`BO-11`d, offline premise).
5. **Vouched vs direct is visible at authentication** (`BO-11`e), which
   two-stage confirmation needs.
6. **Referee consent is explicit and separate** from pairing a single device:
   "trust Home for its devices" is its own choice in the ceremony, as the LP2P
   spec already requires.
7. **Removal is total and immediate**: removing a referee removes trust in
   everything under it, and the UA clears the namespace's site data (Local
   HTTPS already requires this).
8. **Origin privacy toward vouched devices** follows REQ-NAME-05
   ("Privacy-Preserving Discovery"): a device should not learn requesting
   origins before the user picked or confirmed it.
9. **A referee never widens a page's grants.** Local HTTPS claims it; it holds
   only if a device under the referee cannot advertise a name that nests
   other devices' names (`T-STO-5`). SETTLE should forbid a vouched device a
   name equal to or above another device's.
