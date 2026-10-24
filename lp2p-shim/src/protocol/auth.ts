// Toy PIN pairing, shaped like OSP's SPAKE2 authentication but NOT SPAKE2
// and NOT secure.
//
// It exists so the user-facing flow (one device shows a PIN, the other
// enters it) and the message sequence look like the real thing:
//
//   receiver (PSK presenter)                     requester (PSK consumer)
//   auth-capabilities                       -->
//   auth-spake2-handshake (psk-shown, nR)   -->  user enters PIN
//                                           <--  auth-spake2-handshake (psk-input, nQ)
//                                           <--  auth-spake2-confirmation
//                                                  SHA-512(tag | PIN | nR | nQ)
//   auth-status (authenticated | proof-invalid) -->
//
// Why this is not security: the transports underneath are not encrypted or
// authenticated (no TLS certificate to bind to), and the confirmation hash of
// a 9-digit PIN can be brute-forced offline by anyone who sees it. A page
// cannot do better than this; that is part of the case for a native API.
//
// With pairing "none" the receiver sends auth-status authenticated right
// away and no PIN is involved.

import { AuthResult, PskStatus, TypeKey } from "./messages.ts";
import type { ControlChannel } from "./session.ts";

const TAG = new TextEncoder().encode("lp2p-shim toy pairing v1");

export class AuthError extends Error {
  override name = "AuthError";
  readonly result: number;

  constructor(message: string, result: number) {
    super(message);
    this.result = result;
  }
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

async function proof(pin: string, presenterNonce: Uint8Array, consumerNonce: Uint8Array): Promise<Uint8Array> {
  const p = new TextEncoder().encode(normalizePIN(pin));
  const input = new Uint8Array(TAG.length + p.length + presenterNonce.length + consumerNonce.length);
  input.set(TAG);
  input.set(p, TAG.length);
  input.set(presenterNonce, TAG.length + p.length);
  input.set(consumerNonce, TAG.length + p.length + presenterNonce.length);
  return new Uint8Array(await crypto.subtle.digest("SHA-512", input));
}

function equal(a: Uint8Array, b: Uint8Array): boolean {
  return a.length === b.length && a.every((x, i) => x === b[i]);
}

/** Receiver side without pairing. */
export function skipAuthentication(control: ControlChannel): void {
  control.send({ type: TypeKey.AuthStatus, result: AuthResult.Authenticated });
}

/**
 * Receiver side: shows a PIN via showPIN and checks the requester's proof.
 * Throws AuthError if the requester gets it wrong.
 */
export async function presentPIN(control: ControlChannel, showPIN: (pin: string) => void, signal?: AbortSignal): Promise<void> {
  const pin = generatePIN();
  const nR = nonce();
  control.send({ type: TypeKey.AuthCapabilities, pskEaseOfInput: 50, pskInputMethods: [0], pskMinBitsOfEntropy: 20 });
  control.send({ type: TypeKey.AuthSpake2Handshake, pskStatus: PskStatus.Shown, publicValue: nR });
  showPIN(pin);
  const hs = await control.next([TypeKey.AuthSpake2Handshake], signal);
  const conf = await control.next([TypeKey.AuthSpake2Confirmation], signal);
  const ok = equal(conf.confirmationValue, await proof(pin, nR, hs.publicValue));
  const result = ok ? AuthResult.Authenticated : AuthResult.ProofInvalid;
  control.send({ type: TypeKey.AuthStatus, result });
  if (!ok) throw new AuthError("the peer entered a wrong PIN", result);
}

/**
 * Requester side: waits for the receiver to accept and, if it asks for a
 * PIN, gets one from enterPIN (null means the user canceled). Resolves when
 * the receiver reports authenticated.
 */
export async function awaitAuthentication(
  control: ControlChannel,
  enterPIN: () => Promise<string | null>,
  signal?: AbortSignal,
): Promise<void> {
  for (;;) {
    const m = await control.next([TypeKey.AuthSpake2Handshake, TypeKey.AuthStatus], signal);
    if (m.type === TypeKey.AuthStatus) {
      if (m.result === AuthResult.Authenticated) return;
      throw new AuthError(m.result === AuthResult.ProofInvalid ? "wrong PIN" : `authentication failed (${m.result})`, m.result);
    }
    if (m.pskStatus !== PskStatus.Shown) continue;
    const pin = await enterPIN();
    if (pin === null) throw new AuthError("the user canceled pairing", AuthResult.UnknownError);
    const nQ = nonce();
    control.send({ type: TypeKey.AuthSpake2Handshake, pskStatus: PskStatus.Input, publicValue: nQ });
    control.send({ type: TypeKey.AuthSpake2Confirmation, confirmationValue: await proof(pin, m.publicValue, nQ) });
  }
}
