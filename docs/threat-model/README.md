# LP2P threat model

Threat model for the WICG Local Peer-to-Peer API (`index.bs`) and its abstract
transport-binding contract. It lives in go-lp2p for now. The intent is to move it
to the spec repo as `threat-model.md`, pending a decision on where the model
lives (plan, "Decisions for the user").

**Status:** steps 1-3 done; awaiting checkpoint 2 (user review of steps 2 and 3).

## Files

- `00-framing.md`: novelty statement, premises, principals, adversaries,
  assets, trust boundaries, scope, vouching, assumptions, axes.
- `01-dfd.md`: Mermaid data-flow diagrams D0 (context) to D4 (trust store),
  with a table per diagram and a list of where the spec is silent.
- `02-threats.md`: threat table (STRIDE, LINDDUN L/I/D/U, abuse of
  functionality) with the axes as columns; candidate surface (service
  workers, `getDevices`); red-team record.
- `03-mitigations.md`: a disposition per threat, the receiver-fallback fix
  and determinism check, ranked proposed spec fixes, accepted risks, spec
  claims the model contradicts.
- `04-binding-obligations.md`: `BO-n` list; light OSP conformance pass;
  WebTransport+SETTLE sketch.
- `05-vouching-settle.md`: vouching analysis and two-stage confirmation, as
  SETTLE input.
- `06-tag-challenge.md`: Design Principles, Privacy Principles and the earlier
  TAG review, mapped to threats and fixes.
- `README.md`: this file.

## Steps

| Step | Content | Status |
|---|---|---|
| 0 | Framing: novelty, premises, out of scope | Done, in `00-framing.md` |
| 1 | Decompose: principals, boundaries, DFDs | Done, in `01-dfd.md` |
| 2 | Threats: STRIDE on novel elements, LINDDUN (L, I, D, U), abuse of functionality; threat table with the axes as columns | Done, in `02-threats.md`, red-teamed |
| 3 | Countermeasures and binding obligations; light OSP conformance pass | Done, in `03`-`05` |
| 4 | Challenge against the TAG review, Design Principles, Privacy Principles | Done early, in `06-tag-challenge.md` |
| 5 | Security and privacy questionnaire, derived from the table | |

(The plan names framing "Phase 0" and decomposition "Phase 1"; this draft
covers both as step 1.)

## IDs

- `PR-*` principals, `ADV-*` adversaries, `AS-*` assets, `TB1`-`TB7`
  boundaries, `ASM-*` assumptions: defined in `00-framing.md`.
- `E*` external entities, `P*` processes, `S*` stores: defined in the element
  catalogue of `01-dfd.md`, stable across diagrams.
- `F0.n`-`F4.n` flows, one prefix per diagram (D0-D4); the ID is in each edge
  label.
- `T-<area>-<n>` threats (`02-threats.md`); areas `DISC`, `CER`, `DSP`,
  `PEER`, `VCH`, `STO`, `API`, `NET`, `ABU`; candidate surface `T-SW-n`,
  `T-GD-n`.
- `SF-n` proposed spec fixes (`03-mitigations.md`; the ID is stable, the rank
  may change); `BO-n` binding obligations (`04-binding-obligations.md`).
- IDs are never reused. A merged ID stays in its table, marked as merged.
