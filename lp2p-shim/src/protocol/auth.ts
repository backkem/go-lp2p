// Toy PIN pairing (the shim's trust ceremony), shaped like OSP's SPAKE2
// authentication but NOT SPAKE2 and NOT secure.
//
// It exists so the user-facing flow (one device shows a PIN, the other
// enters it) and the message sequence look like the real thing. As the LP2P
// session extension requires, only the client starts it, once the server
// sent auth-capabilities as its cue (or accepted without it, see
// ../ua/useragent.ts):
//
//   server (PSK presenter)                       client (PSK consumer)
//   auth-capabilities (cue)                 -->
//                                           <--  auth-spake2-handshake (psk-needs-presentation, nQ)
//   shows the PIN
//   auth-spake2-handshake (psk-input, nS)   -->  user enters the PIN
//                                           <--  auth-spake2-confirmation
//                                                  SHA-512(tag | PIN | nS | nQ | fpClient | fpServer)
//   auth-status (authenticated | proof-invalid) -->
//
// The proof covers both agent fingerprints from the handshake in
// identity.ts, so a successful ceremony vouches for those keys, and the
// trust anchors it adds mean something on later sessions. Why this is still
// not security: the transports underneath are not encrypted, and the
// confirmation hash of a 9-digit PIN can be brute-forced offline by anyone
// who sees it. A page cannot do better; that is part of the case for a
// native API.
//
// With pairing "none" the server answers the client's handshake with
// auth-status authenticated, and no PIN is involved.

import { AuthResult, PskStatus, TypeKey } from "./messages.ts";
import type { ControlChannel } from "./session.ts";

const TAG = new TextEncoder().encode("lp2p-shim toy pairing v2");

export class AuthError extends Error {
  override name = "AuthError";
  readonly result: number;

  constructor(message: string, result: number) {
    super(message);
    this.result = result;
  }
}

/** The fingerprints the ceremony binds. */
export interface Parties {
  client: string;
  server: string;
}

/** A 9-digit PIN, about 27 bits, shown as "123-456-789" (OSP Appendix B grouping). */
export function generatePIN(): string {
  const v = new Uint32Array(1);
  crypto.getRandomValues(v);
  const digits = String((v[0] as number) % 1_000_000_000).padStart(9, "0");
  return `${digits.slice(0, 3)}-${digits.slice(3, 6)}-${digits.slice(6)}`;
}

function normalizePIN(pin: string): string {
  return pin.replace(/[^0-9]/g, "");
}

function nonce(): Uint8Array {
  return crypto.getRandomValues(new Uint8Array(32));
}

async function proof(pin: string, serverNonce: Uint8Array, clientNonce: Uint8Array, parties: Parties): Promise<Uint8Array> {
  const e = new TextEncoder();
  const parts = [TAG, e.encode(normalizePIN(pin)), serverNonce, clientNonce, e.encode(parties.client), e.encode(parties.server)];
  const input = new Uint8Array(parts.reduce((n, p) => n + p.length, 0));
  let off = 0;
  for (const p of parts) {
    input.set(p, off);
    off += p.length;
  }
  return new Uint8Array(await crypto.subtle.digest("SHA-512", input));
}

function equal(a: Uint8Array, b: Uint8Array): boolean {
  return a.length === b.length && a.every((x, i) => x === b[i]);
}

/** Server: the cue that it checked the requesting origin hint and needs a ceremony. */
export function sendAuthCapabilities(control: ControlChannel): void {
  control.send({ type: TypeKey.AuthCapabilities, pskEaseOfInput: 50, pskInputMethods: [0], pskMinBitsOfEntropy: 20 });
}

/**
 * Server: answers the ceremony the client starts. With showPIN null
 * (pairing "none") it reports success at once. Throws AuthError if the
 * client proves the wrong PIN.
 */
export async function answerCeremony(
  control: ControlChannel,
  parties: Parties,
  showPIN: ((pin: string) => void) | null,
  signal?: AbortSignal,
): Promise<void> {
  const hs = await control.next([TypeKey.AuthSpake2Handshake], signal);
  if (hs.pskStatus !== PskStatus.NeedsPresentation) {
    control.send({ type: TypeKey.AuthStatus, result: AuthResult.UnknownError });
    throw new AuthError("unexpected psk-status from the client", AuthResult.UnknownError);
  }
  if (!showPIN) {
    control.send({ type: TypeKey.AuthStatus, result: AuthResult.Authenticated });
    return;
  }
  const pin = generatePIN();
  const nS = nonce();
  showPIN(pin);
  control.send({ type: TypeKey.AuthSpake2Handshake, pskStatus: PskStatus.Input, publicValue: nS });
  const conf = await control.next([TypeKey.AuthSpake2Confirmation], signal);
  const ok = equal(conf.confirmationValue, await proof(pin, nS, hs.publicValue, parties));
  const result = ok ? AuthResult.Authenticated : AuthResult.ProofInvalid;
  control.send({ type: TypeKey.AuthStatus, result });
  if (!ok) throw new AuthError("the peer entered a wrong PIN", result);
}

/**
 * Client: starts the ceremony and, if the server shows a PIN, gets it from
 * enterPIN (null means the user canceled). Resolves when the server reports
 * authenticated.
 */
export async function startCeremony(
  control: ControlChannel,
  parties: Parties,
  enterPIN: () => Promise<string | null>,
  signal?: AbortSignal,
): Promise<void> {
  const nQ = nonce();
  control.send({ type: TypeKey.AuthSpake2Handshake, pskStatus: PskStatus.NeedsPresentation, publicValue: nQ });
  for (;;) {
    const m = await control.next([TypeKey.AuthSpake2Handshake, TypeKey.AuthStatus], signal);
    if (m.type === TypeKey.AuthStatus) {
      if (m.result === AuthResult.Authenticated) return;
      throw new AuthError(m.result === AuthResult.ProofInvalid ? "wrong PIN" : `authentication failed (${m.result})`, m.result);
    }
    if (m.pskStatus !== PskStatus.Input) continue;
    const pin = await enterPIN();
    if (pin === null) throw new AuthError("the user canceled pairing", AuthResult.UnknownError);
    control.send({ type: TypeKey.AuthSpake2Confirmation, confirmationValue: await proof(pin, m.publicValue, nQ, parties) });
  }
}
