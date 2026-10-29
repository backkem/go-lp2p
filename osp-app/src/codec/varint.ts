// QUIC variable-length integers (RFC 9000, section 16). OSP prefixes every
// CBOR message with its type key in this encoding.

import { IncompleteError } from "./cbor.ts";

/** JS numbers are exact up to 2^53; type keys and IDs never get near it. */
const MAX_VARINT = 2 ** 53 - 1;

export function encodeVarint(n: number): Uint8Array {
  if (!Number.isSafeInteger(n) || n < 0 || n > MAX_VARINT) throw new RangeError(`varint out of range: ${n}`);
  if (n < 0x40) return Uint8Array.of(n);
  if (n < 0x4000) return Uint8Array.of(0x40 | (n >> 8), n & 0xff);
  if (n < 0x40000000) return Uint8Array.of(0x80 | (n >>> 24), (n >>> 16) & 0xff, (n >>> 8) & 0xff, n & 0xff);
  const out = new Uint8Array(8);
  const view = new DataView(out.buffer);
  view.setUint32(0, Math.floor(n / 2 ** 32));
  view.setUint32(4, n >>> 0);
  out[0] = (out[0] as number) | 0xc0;
  return out;
}

/**
 * Decodes a varint at offset and returns [value, length]. Throws
 * IncompleteError when buf ends inside the varint.
 */
export function decodeVarint(buf: Uint8Array, offset = 0): [value: number, length: number] {
  const first = buf[offset];
  if (first === undefined) throw new IncompleteError("varint: unexpected end of input");
  const length = 1 << (first >> 6);
  if (offset + length > buf.length) throw new IncompleteError("varint: unexpected end of input");
  let value = first & 0x3f;
  for (let i = 1; i < length; i++) value = value * 256 + (buf[offset + i] as number);
  if (value > MAX_VARINT) throw new RangeError("varint: value exceeds 2^53");
  return [value, length];
}
