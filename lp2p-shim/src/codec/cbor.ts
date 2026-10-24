// A deliberately small CBOR (RFC 8949) codec covering what Open Screen
// Protocol messages use: unsigned/negative integers, byte and text strings,
// arrays, maps, booleans, null/undefined and floats. Tags are decoded by
// returning the tagged value; indefinite-length items are rejected.
//
// Maps are plain objects. OSP maps use integer keys ("0: text ; display-name"),
// so object keys that look like canonical non-negative integers ("0", "12")
// are encoded as CBOR unsigned integers, and integer keys decode back to the
// same property names. Any other key is encoded as a text string.

export type CBORValue =
  | number
  | string
  | boolean
  | null
  | undefined
  | Uint8Array
  | CBORValue[]
  | { [key: string]: CBORValue };

const INT_KEY = /^(0|[1-9][0-9]*)$/;
const textEncoder = new TextEncoder();
const textDecoder = new TextDecoder("utf-8", { fatal: true });

class Writer {
  private buf = new Uint8Array(64);
  private len = 0;

  private reserve(n: number): void {
    if (this.len + n <= this.buf.length) return;
    let size = this.buf.length * 2;
    while (size < this.len + n) size *= 2;
    const next = new Uint8Array(size);
    next.set(this.buf.subarray(0, this.len));
    this.buf = next;
  }

  byte(b: number): void {
    this.reserve(1);
    this.buf[this.len++] = b;
  }

  bytes(b: Uint8Array): void {
    this.reserve(b.length);
    this.buf.set(b, this.len);
    this.len += b.length;
  }

  head(major: number, n: number): void {
    const m = major << 5;
    if (n < 24) {
      this.byte(m | n);
    } else if (n < 0x100) {
      this.byte(m | 24);
      this.byte(n);
    } else if (n < 0x10000) {
      this.byte(m | 25);
      this.byte(n >> 8);
      this.byte(n & 0xff);
    } else if (n < 0x100000000) {
      this.byte(m | 26);
      const b = new Uint8Array(4);
      new DataView(b.buffer).setUint32(0, n);
      this.bytes(b);
    } else {
      this.byte(m | 27);
      const b = new Uint8Array(8);
      const view = new DataView(b.buffer);
      view.setUint32(0, Math.floor(n / 2 ** 32));
      view.setUint32(4, n >>> 0);
      this.bytes(b);
    }
  }

  result(): Uint8Array {
    return this.buf.slice(0, this.len);
  }
}

function encodeInto(w: Writer, v: CBORValue): void {
  switch (typeof v) {
    case "number":
      if (Number.isSafeInteger(v)) {
        if (v >= 0) w.head(0, v);
        else w.head(1, -1 - v);
      } else {
        w.byte(0xfb);
        const b = new Uint8Array(8);
        new DataView(b.buffer).setFloat64(0, v);
        w.bytes(b);
      }
      return;
    case "string": {
      const b = textEncoder.encode(v);
      w.head(3, b.length);
      w.bytes(b);
      return;
    }
    case "boolean":
      w.byte(v ? 0xf5 : 0xf4);
      return;
    case "undefined":
      w.byte(0xf7);
      return;
    case "object":
      if (v === null) {
        w.byte(0xf6);
      } else if (v instanceof Uint8Array) {
        w.head(2, v.length);
        w.bytes(v);
      } else if (Array.isArray(v)) {
        w.head(4, v.length);
        for (const item of v) encodeInto(w, item);
      } else {
        // Skip undefined members, so optional fields can be left unset.
        const entries = Object.entries(v).filter(([, val]) => val !== undefined);
        w.head(5, entries.length);
        for (const [key, val] of entries) {
          if (INT_KEY.test(key) && Number.isSafeInteger(Number(key))) w.head(0, Number(key));
          else encodeInto(w, key);
          encodeInto(w, val);
        }
      }
      return;
    default:
      throw new TypeError(`cbor: cannot encode ${typeof v}`);
  }
}

export function encodeCBOR(v: CBORValue): Uint8Array {
  const w = new Writer();
  encodeInto(w, v);
  return w.result();
}

const MAX_DEPTH = 64;

class Reader {
  pos = 0;
  private readonly view: DataView;

  private readonly buf: Uint8Array;

  constructor(buf: Uint8Array) {
    this.buf = buf;
    this.view = new DataView(buf.buffer, buf.byteOffset, buf.byteLength);
  }

  private need(n: number): void {
    if (this.pos + n > this.buf.length) throw new RangeError("cbor: unexpected end of input");
  }

  private u8(): number {
    this.need(1);
    return this.buf[this.pos++] as number;
  }

  private arg(info: number): number {
    if (info < 24) return info;
    switch (info) {
      case 24:
        return this.u8();
      case 25: {
        this.need(2);
        const v = this.view.getUint16(this.pos);
        this.pos += 2;
        return v;
      }
      case 26: {
        this.need(4);
        const v = this.view.getUint32(this.pos);
        this.pos += 4;
        return v;
      }
      case 27: {
        this.need(8);
        const v = this.view.getUint32(this.pos) * 2 ** 32 + this.view.getUint32(this.pos + 4);
        this.pos += 8;
        if (!Number.isSafeInteger(v)) throw new RangeError("cbor: integer exceeds 2^53");
        return v;
      }
      default:
        throw new RangeError(`cbor: unsupported additional info ${info} (indefinite lengths are not supported)`);
    }
  }

  value(depth = 0): CBORValue {
    if (depth > MAX_DEPTH) throw new RangeError("cbor: nesting too deep");
    const ib = this.u8();
    const major = ib >> 5;
    const info = ib & 0x1f;
    switch (major) {
      case 0:
        return this.arg(info);
      case 1:
        return -1 - this.arg(info);
      case 2: {
        const n = this.arg(info);
        this.need(n);
        const b = this.buf.slice(this.pos, this.pos + n);
        this.pos += n;
        return b;
      }
      case 3: {
        const n = this.arg(info);
        this.need(n);
        const s = textDecoder.decode(this.buf.subarray(this.pos, this.pos + n));
        this.pos += n;
        return s;
      }
      case 4: {
        const n = this.arg(info);
        const out: CBORValue[] = [];
        for (let i = 0; i < n; i++) out.push(this.value(depth + 1));
        return out;
      }
      case 5: {
        const n = this.arg(info);
        const out: { [key: string]: CBORValue } = {};
        for (let i = 0; i < n; i++) {
          const key = this.value(depth + 1);
          if (typeof key !== "number" && typeof key !== "string") {
            throw new TypeError("cbor: only integer and text map keys are supported");
          }
          Object.defineProperty(out, String(key), {
            value: this.value(depth + 1),
            enumerable: true,
            writable: true,
            configurable: true,
          });
        }
        return out;
      }
      case 6:
        this.arg(info); // tag number: ignored
        return this.value(depth + 1);
      default: // 7: simple values and floats
        switch (info) {
          case 20:
            return false;
          case 21:
            return true;
          case 22:
            return null;
          case 23:
            return undefined;
          case 25: {
            this.need(2);
            const v = decodeFloat16(this.view.getUint16(this.pos));
            this.pos += 2;
            return v;
          }
          case 26: {
            this.need(4);
            const v = this.view.getFloat32(this.pos);
            this.pos += 4;
            return v;
          }
          case 27: {
            this.need(8);
            const v = this.view.getFloat64(this.pos);
            this.pos += 8;
            return v;
          }
          default:
            throw new RangeError(`cbor: unsupported simple value ${info}`);
        }
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

/** Decodes one CBOR item starting at offset. Returns the value and the offset after it. */
export function decodeCBORPrefix(buf: Uint8Array, offset = 0): [value: CBORValue, end: number] {
  const r = new Reader(buf);
  r.pos = offset;
  const v = r.value();
  return [v, r.pos];
}

/** Decodes exactly one CBOR item; trailing bytes are an error. */
export function decodeCBOR(buf: Uint8Array): CBORValue {
  const [v, end] = decodeCBORPrefix(buf);
  if (end !== buf.length) throw new RangeError("cbor: trailing bytes");
  return v;
}
