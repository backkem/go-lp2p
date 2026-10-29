// A small CBOR (RFC 8949) codec for what OSP messages use: integers, byte and
// text strings, arrays, maps, booleans, null/undefined and floats. Tags are
// decoded as their content; indefinite lengths are rejected.
//
// The value model is lossless for OSP:
// - maps are Map objects, so integer keys stay integers;
// - floats decode to Float64 and Float64 always encodes as a float64, so a
//   float field holding 1.0 doesn't turn into the integer 1 on the wire;
// - a number that is a safe integer encodes as an integer, any other number
//   as a float64.
// Map keys are written in the core deterministic order (RFC 8949 4.2.1):
// some decoders, Chromium's Open Screen library among them, reject others.

/** A float that encodes as a CBOR float64 even when it is integral. */
export class Float64 {
  readonly value: number;
  constructor(value: number) {
    this.value = value;
  }
}

export type CBORKey = number | string;
export type CBORValue =
  | number
  | Float64
  | string
  | boolean
  | null
  | undefined
  | Uint8Array
  | CBORValue[]
  | Map<CBORKey, CBORValue>;

/** Thrown when the input ends inside an item: more bytes may complete it. */
export class IncompleteError extends RangeError {
  override name = "IncompleteError";
}

const textEncoder = new TextEncoder();
const textDecoder = new TextDecoder("utf-8", { fatal: true });

class Writer {
  #buf = new Uint8Array(64);
  #len = 0;

  #reserve(n: number): void {
    if (this.#len + n <= this.#buf.length) return;
    let size = this.#buf.length * 2;
    while (size < this.#len + n) size *= 2;
    const next = new Uint8Array(size);
    next.set(this.#buf.subarray(0, this.#len));
    this.#buf = next;
  }

  byte(b: number): void {
    this.#reserve(1);
    this.#buf[this.#len++] = b;
  }

  bytes(b: Uint8Array): void {
    this.#reserve(b.length);
    this.#buf.set(b, this.#len);
    this.#len += b.length;
  }

  head(major: number, n: number): void {
    const m = major << 5;
    if (n < 24) this.byte(m | n);
    else if (n < 0x100) this.bytes(Uint8Array.of(m | 24, n));
    else if (n < 0x10000) this.bytes(Uint8Array.of(m | 25, n >> 8, n & 0xff));
    else {
      const b = new Uint8Array(n < 0x100000000 ? 5 : 9);
      const view = new DataView(b.buffer);
      if (n < 0x100000000) {
        b[0] = m | 26;
        view.setUint32(1, n);
      } else {
        b[0] = m | 27;
        view.setUint32(1, Math.floor(n / 2 ** 32));
        view.setUint32(5, n >>> 0);
      }
      this.bytes(b);
    }
  }

  float64(v: number): void {
    const b = new Uint8Array(9);
    b[0] = 0xfb;
    new DataView(b.buffer).setFloat64(1, v);
    this.bytes(b);
  }

  result(): Uint8Array {
    return this.#buf.slice(0, this.#len);
  }
}

function encodeInto(w: Writer, v: CBORValue): void {
  if (typeof v === "number") {
    if (!Number.isSafeInteger(v)) w.float64(v);
    else if (v >= 0) w.head(0, v);
    else w.head(1, -1 - v);
  } else if (typeof v === "string") {
    const b = textEncoder.encode(v);
    w.head(3, b.length);
    w.bytes(b);
  } else if (typeof v === "boolean") {
    w.byte(v ? 0xf5 : 0xf4);
  } else if (v === undefined) {
    w.byte(0xf7);
  } else if (v === null) {
    w.byte(0xf6);
  } else if (v instanceof Float64) {
    w.float64(v.value);
  } else if (v instanceof Uint8Array) {
    w.head(2, v.length);
    w.bytes(v);
  } else if (Array.isArray(v)) {
    w.head(4, v.length);
    for (const item of v) encodeInto(w, item);
  } else if (v instanceof Map) {
    // Core deterministic order: sort by the encoded key, shorter first.
    const entries = [...v].map(([k, val]) => [encodeCBOR(k), val] as const);
    entries.sort(([a], [b]) => compareBytes(a, b));
    w.head(5, entries.length);
    for (const [k, val] of entries) {
      w.bytes(k);
      encodeInto(w, val);
    }
  } else {
    throw new TypeError(`cbor: cannot encode ${Object.prototype.toString.call(v)}`);
  }
}

function compareBytes(a: Uint8Array, b: Uint8Array): number {
  if (a.length !== b.length) return a.length - b.length;
  for (let i = 0; i < a.length; i++) {
    const d = (a[i] as number) - (b[i] as number);
    if (d !== 0) return d;
  }
  return 0;
}

export function encodeCBOR(v: CBORValue): Uint8Array {
  const w = new Writer();
  encodeInto(w, v);
  return w.result();
}

const MAX_DEPTH = 64;

class Reader {
  pos: number;
  readonly #buf: Uint8Array;
  readonly #view: DataView;

  constructor(buf: Uint8Array, pos: number) {
    this.#buf = buf;
    this.#view = new DataView(buf.buffer, buf.byteOffset, buf.byteLength);
    this.pos = pos;
  }

  #need(n: number): void {
    if (this.pos + n > this.#buf.length) throw new IncompleteError("cbor: unexpected end of input");
  }

  #u8(): number {
    this.#need(1);
    return this.#buf[this.pos++] as number;
  }

  #arg(info: number): number {
    if (info < 24) return info;
    let v: number;
    switch (info) {
      case 24:
        return this.#u8();
      case 25:
        this.#need(2);
        v = this.#view.getUint16(this.pos);
        this.pos += 2;
        return v;
      case 26:
        this.#need(4);
        v = this.#view.getUint32(this.pos);
        this.pos += 4;
        return v;
      case 27:
        this.#need(8);
        v = this.#view.getUint32(this.pos) * 2 ** 32 + this.#view.getUint32(this.pos + 4);
        this.pos += 8;
        if (!Number.isSafeInteger(v)) throw new RangeError("cbor: integer exceeds 2^53");
        return v;
      default:
        throw new RangeError(`cbor: unsupported additional info ${info} (indefinite lengths are not supported)`);
    }
  }

  value(depth = 0): CBORValue {
    if (depth > MAX_DEPTH) throw new RangeError("cbor: nesting too deep");
    const ib = this.#u8();
    const major = ib >> 5;
    const info = ib & 0x1f;
    switch (major) {
      case 0:
        return this.#arg(info);
      case 1:
        return -1 - this.#arg(info);
      case 2:
      case 3: {
        const n = this.#arg(info);
        this.#need(n);
        const b = this.#buf.subarray(this.pos, this.pos + n);
        this.pos += n;
        return major === 2 ? b.slice() : textDecoder.decode(b);
      }
      case 4: {
        const n = this.#arg(info);
        const out: CBORValue[] = [];
        for (let i = 0; i < n; i++) out.push(this.value(depth + 1));
        return out;
      }
      case 5: {
        const n = this.#arg(info);
        const out = new Map<CBORKey, CBORValue>();
        for (let i = 0; i < n; i++) {
          const key = this.value(depth + 1);
          if (typeof key !== "number" && typeof key !== "string") throw new TypeError("cbor: only integer and text map keys are supported");
          if (out.has(key)) throw new TypeError(`cbor: duplicate map key ${key}`);
          out.set(key, this.value(depth + 1));
        }
        return out;
      }
      case 6:
        this.#arg(info); // The tag number is ignored.
        return this.value(depth + 1);
      default:
        return this.#simple(info);
    }
  }

  #simple(info: number): CBORValue {
    let v: number;
    switch (info) {
      case 20:
        return false;
      case 21:
        return true;
      case 22:
        return null;
      case 23:
        return undefined;
      case 25:
        this.#need(2);
        v = decodeFloat16(this.#view.getUint16(this.pos));
        this.pos += 2;
        return new Float64(v);
      case 26:
        this.#need(4);
        v = this.#view.getFloat32(this.pos);
        this.pos += 4;
        return new Float64(v);
      case 27:
        this.#need(8);
        v = this.#view.getFloat64(this.pos);
        this.pos += 8;
        return new Float64(v);
      default:
        throw new RangeError(`cbor: unsupported simple value ${info}`);
    }
  }
}

function decodeFloat16(h: number): number {
  const sign = h & 0x8000 ? -1 : 1;
  const exp = (h >> 10) & 0x1f;
  const frac = h & 0x3ff;
  if (exp === 0) return sign * 2 ** -14 * (frac / 1024);
  if (exp === 0x1f) return frac ? NaN : sign * Infinity;
  return sign * 2 ** (exp - 15) * (1 + frac / 1024);
}

/**
 * Decodes one item starting at offset and returns [value, end offset].
 * Throws IncompleteError if buf ends inside the item.
 */
export function decodeCBORPrefix(buf: Uint8Array, offset = 0): [value: CBORValue, end: number] {
  const r = new Reader(buf, offset);
  const v = r.value();
  return [v, r.pos];
}

/** Decodes exactly one item; trailing bytes are an error. */
export function decodeCBOR(buf: Uint8Array): CBORValue {
  const [v, end] = decodeCBORPrefix(buf);
  if (end !== buf.length) throw new RangeError("cbor: trailing bytes");
  return v;
}
