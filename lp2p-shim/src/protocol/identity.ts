// The shim's stand-in for the TLS 1.3 handshake underneath OSP.
//
// In the OSP binding, each agent presents its agent certificate in the TLS
// handshake of the QUIC connection, and its peer identity is derived from
// that certificate's key (the agent fingerprint). The shim's transports have
// no TLS, so the session layer runs a small handshake of its own first on the
// control stream (private-use type keys, not OSP messages):
//
//   both: shim-hello    {signing key (SPKI), ephemeral ECDH key, nonce}
//   both: shim-finished {ECDSA signature over the transcript and the role}
//
// It gives each side what TLS would: proof that the peer holds the private
// key of the identity it presents (so a trust anchor on that key means
// something on later sessions), and a shared secret for keying material
// exporters. Unlike TLS, it does not encrypt the transport, and on a first
// session nothing stops a man in the middle except the PIN ceremony that
// follows, which binds both fingerprints. NOT secure; see README.md.

import { TypeKey } from "./messages.ts";
import type { ControlChannel } from "./session.ts";

const TAG = new TextEncoder().encode("lp2p-shim tls v1");
const subtle = (): SubtleCrypto => globalThis.crypto.subtle;

/** A long-lived agent key pair: the shim's agent certificate. */
export interface Identity {
  readonly keys: CryptoKeyPair;
  /** The public key, DER SubjectPublicKeyInfo. */
  readonly spki: Uint8Array;
  /** base64 SHA-256 of spki, like OSP's agent fingerprint. The peer identity. */
  readonly fingerprint: string;
}

/** What the handshake establishes about one connection. */
export interface HandshakeResult {
  /** The peer's identity, proven by its signature. */
  peerFingerprint: string;
  /** ECDH shared secret, 32 bytes; the exporter's input keying material. */
  secret: Uint8Array;
  /** SHA-256 of the handshake transcript; the exporter's salt. */
  transcriptHash: Uint8Array;
}

export class HandshakeError extends Error {
  override name = "HandshakeError";
}

function concat(...parts: Uint8Array[]): Uint8Array {
  const out = new Uint8Array(parts.reduce((n, p) => n + p.length, 0));
  let off = 0;
  for (const p of parts) {
    out.set(p, off);
    off += p.length;
  }
  return out;
}

function base64(b: Uint8Array): string {
  let s = "";
  for (const x of b) s += String.fromCharCode(x);
  return btoa(s);
}

async function sha256(b: Uint8Array): Promise<Uint8Array> {
  return new Uint8Array(await subtle().digest("SHA-256", b as Uint8Array<ArrayBuffer>));
}

/** The agent fingerprint of an SPKI: base64(SHA-256(spki)). */
export async function fingerprintOf(spki: Uint8Array): Promise<string> {
  return base64(await sha256(spki));
}

/** Creates a new identity. The private key is not extractable. */
export async function createIdentity(): Promise<Identity> {
  const keys = await subtle().generateKey({ name: "ECDSA", namedCurve: "P-256" }, false, ["sign", "verify"]);
  const spki = new Uint8Array(await subtle().exportKey("spki", keys.publicKey));
  return { keys, spki, fingerprint: await fingerprintOf(spki) };
}

/**
 * Runs the handshake on the control stream. role is "client" for the side
 * that established the connection. Rejects with HandshakeError if the peer's
 * signature does not verify.
 */
export async function handshake(
  control: ControlChannel,
  identity: Identity,
  role: "client" | "server",
  signal?: AbortSignal,
): Promise<HandshakeResult> {
  const eph = await subtle().generateKey({ name: "ECDH", namedCurve: "P-256" }, false, ["deriveBits"]);
  const ephRaw = new Uint8Array(await subtle().exportKey("raw", eph.publicKey));
  const nonce = crypto.getRandomValues(new Uint8Array(32));
  control.send({ type: TypeKey.ShimHello, signingKey: identity.spki, exchangeKey: ephRaw, nonce });
  const hello = await control.next([TypeKey.ShimHello], signal);

  const mine = concat(identity.spki, ephRaw, nonce);
  const theirs = concat(hello.signingKey, hello.exchangeKey, hello.nonce);
  const transcript = role === "client" ? concat(TAG, mine, theirs) : concat(TAG, theirs, mine);
  const transcriptHash = await sha256(transcript);
  const toSign = (r: "client" | "server"): Uint8Array => concat(transcriptHash, new TextEncoder().encode(r));

  const sig = new Uint8Array(
    await subtle().sign({ name: "ECDSA", hash: "SHA-256" }, identity.keys.privateKey, toSign(role) as Uint8Array<ArrayBuffer>),
  );
  control.send({ type: TypeKey.ShimFinished, signature: sig });
  const fin = await control.next([TypeKey.ShimFinished], signal);

  let peerKey: CryptoKey;
  let peerEph: CryptoKey;
  try {
    peerKey = await subtle().importKey(
      "spki",
      hello.signingKey as Uint8Array<ArrayBuffer>,
      { name: "ECDSA", namedCurve: "P-256" },
      false,
      ["verify"],
    );
    peerEph = await subtle().importKey(
      "raw",
      hello.exchangeKey as Uint8Array<ArrayBuffer>,
      { name: "ECDH", namedCurve: "P-256" },
      false,
      [],
    );
  } catch (err) {
    throw new HandshakeError(`bad peer key: ${String(err)}`);
  }
  const peerRole = role === "client" ? "server" : "client";
  const ok = await subtle().verify(
    { name: "ECDSA", hash: "SHA-256" },
    peerKey,
    fin.signature as Uint8Array<ArrayBuffer>,
    toSign(peerRole) as Uint8Array<ArrayBuffer>,
  );
  if (!ok) throw new HandshakeError("the peer's signature does not verify");
  const secret = new Uint8Array(await subtle().deriveBits({ name: "ECDH", public: peerEph }, eph.privateKey, 256));
  return { peerFingerprint: await fingerprintOf(hello.signingKey), secret, transcriptHash };
}

/**
 * The TLS exporter stand-in: HKDF-SHA-256 with the handshake secret as input
 * keying material, the transcript hash as salt, and label ‖ 0x00 ‖ context as
 * info. Both sides of a connection derive the same bytes for the same label
 * and context.
 */
export async function exportKeyingMaterial(
  h: HandshakeResult,
  label: string,
  context: Uint8Array,
  length: number,
): Promise<Uint8Array> {
  const ikm = await subtle().importKey("raw", h.secret as Uint8Array<ArrayBuffer>, "HKDF", false, ["deriveBits"]);
  const info = concat(new TextEncoder().encode(label), new Uint8Array([0]), context);
  const bits = await subtle().deriveBits(
    { name: "HKDF", hash: "SHA-256", salt: h.transcriptHash as Uint8Array<ArrayBuffer>, info: info as Uint8Array<ArrayBuffer> },
    ikm,
    length * 8,
  );
  return new Uint8Array(bits);
}

/**
 * The WebTransport Exporter Context of WebTransport over HTTP/3, Section 4.8,
 * with the exchange-id as the session ID: session ID (64 bits), label length
 * (8), label, context length (8), context.
 */
export function webTransportExporterContext(exchangeId: number, label: Uint8Array, context: Uint8Array): Uint8Array {
  if (label.length > 255 || context.length > 255) throw new RangeError("label and context are at most 255 bytes");
  const out = new Uint8Array(8 + 1 + label.length + 1 + context.length);
  const view = new DataView(out.buffer);
  view.setBigUint64(0, BigInt(exchangeId));
  out[8] = label.length;
  out.set(label, 9);
  out[9 + label.length] = context.length;
  out.set(context, 10 + label.length);
  return out;
}
