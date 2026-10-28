# LP2P threat model

Threat model for the WICG Local Peer-to-Peer API (`index.bs`) and its abstract
transport-binding contract. It lives in go-lp2p for now. The intent is to move it
to the spec repo as `threat-model.md`, pending a decision on where the model
lives (plan, "Decisions for the user").

**Status:** step 1 done (framing and data-flow diagrams); awaiting checkpoint 1.

## Files

- `00-framing.md`: novelty statement, premises, principals, adversaries,
  assets, trust boundaries, scope, vouching, assumptions, axes.
- `01-dfd.md`: Mermaid data-flow diagrams D0 (context) to D4 (trust store),
  with a table per diagram and a list of where the spec is silent.
- `README.md`: this file.

## Steps

| Step | Content | Status |
|---|---|---|
| 0 | Framing: novelty, premises, out of scope | Done, in `00-framing.md` |
| 1 | Decompose: principals, boundaries, DFDs | **This draft** |
| 2 | Threats: STRIDE on novel elements, LINDDUN (L, I, D, U), abuse of functionality; threat table with the axes as columns | Next |
| 3 | Countermeasures and binding obligations; light OSP conformance pass | |
| 4 | Challenge against the TAG review, Design Principles, Privacy Principles | |
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
- IDs are never reused. Threats in step 2 cite them.
