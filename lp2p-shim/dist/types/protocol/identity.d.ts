import type { ControlChannel } from "./session.js";
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
export declare class HandshakeError extends Error {
    name: string;
}
/** The agent fingerprint of an SPKI: base64(SHA-256(spki)). */
export declare function fingerprintOf(spki: Uint8Array): Promise<string>;
/** Creates a new identity. The private key is not extractable. */
export declare function createIdentity(): Promise<Identity>;
/**
 * Runs the handshake on the control stream. role is "client" for the side
 * that established the connection. Rejects with HandshakeError if the peer's
 * signature does not verify.
 */
export declare function handshake(control: ControlChannel, identity: Identity, role: "client" | "server", signal?: AbortSignal): Promise<HandshakeResult>;
/**
 * The TLS exporter stand-in: HKDF-SHA-256 with the handshake secret as input
 * keying material, the transcript hash as salt, and label ‖ 0x00 ‖ context as
 * info. Both sides of a connection derive the same bytes for the same label
 * and context.
 */
export declare function exportKeyingMaterial(h: HandshakeResult, label: string, context: Uint8Array, length: number): Promise<Uint8Array>;
/**
 * The WebTransport Exporter Context of WebTransport over HTTP/3, Section 4.8,
 * with the exchange-id as the session ID: session ID (64 bits), label length
 * (8), label, context length (8), context.
 */
export declare function webTransportExporterContext(exchangeId: number, label: Uint8Array, context: Uint8Array): Uint8Array;
