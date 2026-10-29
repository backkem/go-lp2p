import { test } from "node:test";
import assert from "node:assert/strict";
import { decodeCBOR, decodeCBORPrefix, encodeCBOR, Float64, IncompleteError, type CBORValue } from "../src/codec/cbor.ts";
import { decodeVarint, encodeVarint } from "../src/codec/varint.ts";
import { decodeFrame, encodeFrame, FrameReader } from "../src/messages.ts";
import { hex, unhex } from "./hex.ts";

test("varint: RFC 9000 appendix A.1 examples", () => {
  const cases: [number, string][] = [
    [37, "25"],
    [15293, "7bbd"],
    [494878333, "9d7f3e7d"],
    [2 ** 40, "c000010000000000"],
  ];
  for (const [n, h] of cases) {
    assert.equal(hex(encodeVarint(n)), h);
    assert.deepEqual(decodeVarint(unhex(h)), [n, h.length / 2]);
  }
  assert.deepEqual(decodeVarint(unhex("4025")), [37, 2], "non-minimal encodings decode");
  assert.throws(() => decodeVarint(unhex("7b")), IncompleteError);
  assert.throws(() => encodeVarint(-1), RangeError);
});

test("cbor: RFC 8949 appendix A examples", () => {
  const cases: [CBORValue, string][] = [
    [0, "00"],
    [23, "17"],
    [24, "1818"],
    [1000, "1903e8"],
    [1000000, "1a000f4240"],
    [1000000000000, "1b000000e8d4a51000"],
    [-1, "20"],
    [-1000, "3903e7"],
    [new Float64(1.1), "fb3ff199999999999a"],
    [false, "f4"],
    [true, "f5"],
    [null, "f6"],
    [undefined, "f7"],
    [new Uint8Array(0), "40"],
    [Uint8Array.of(1, 2, 3, 4), "4401020304"],
    ["", "60"],
    ["IETF", "6449455446"],
    ["ü", "62c3bc"],
    ["水", "63e6b0b4"],
    [[], "80"],
    [[1, [2, 3], [4, 5]], "8301820203820405"],
    [new Map([[1, 2], [3, 4]]), "a201020304"],
    [new Map<number | string, CBORValue>([["a", 1], ["b", [2, 3]]]), "a26161016162820203"],
  ];
  for (const [v, h] of cases) {
    assert.equal(hex(encodeCBOR(v)), h, `encode ${h}`);
    assert.deepEqual(decodeCBOR(unhex(h)), v, `decode ${h}`);
  }
});

test("cbor: floats", () => {
  assert.equal(hex(encodeCBOR(1.5)), "fb3ff8000000000000", "non-integral numbers are float64");
  assert.equal(hex(encodeCBOR(new Float64(2))), "fb4000000000000000", "Float64 stays a float when integral");
  assert.deepEqual(decodeCBOR(unhex("f93c00")), new Float64(1), "float16");
  assert.deepEqual(decodeCBOR(unhex("fa47c35000")), new Float64(100000), "float32");
  assert.ok(Number.isNaN((decodeCBOR(unhex("f97e00")) as Float64).value));
});

test("cbor: map keys are written in deterministic order", () => {
  const m = new Map<number | string, CBORValue>([[10, 0], ["a", 0], [1, 0], [100, 0], [0, 0]]);
  assert.equal(hex(encodeCBOR(m)), "a5" + "0000" + "0100" + "0a00" + "186400" + "616100");
});

test("cbor: rejects what OSP doesn't use or allow", () => {
  assert.throws(() => decodeCBOR(unhex("9f01ff")), RangeError, "indefinite length");
  assert.throws(() => decodeCBOR(unhex("a201000100")), TypeError, "duplicate key");
  assert.throws(() => decodeCBOR(unhex("0000")), RangeError, "trailing bytes");
  assert.throws(() => decodeCBOR(unhex("1b0020000000000000")), RangeError, "beyond 2^53");
  assert.throws(() => decodeCBOR(unhex("62c3")), IncompleteError);
  assert.throws(() => decodeCBOR(unhex("62c328")), TypeError, "invalid UTF-8");
  assert.deepEqual(decodeCBOR(unhex("c11a514b67b0")), 1363896240, "tags are skipped");
  assert.deepEqual(decodeCBORPrefix(unhex("0102"), 1), [2, 2]);
});

test("frames: encode, decode and split a byte stream", () => {
  const f1 = { typeKey: 16, body: encodeCBOR(new Map([[0, 1]])) };
  const f2 = { typeKey: 1000, body: encodeCBOR("x".repeat(300)) };
  const wire = new Uint8Array([...encodeFrame(f1), ...encodeFrame(f2)]);
  assert.equal(hex(encodeFrame(f1)), "10a10001");
  assert.deepEqual(decodeFrame(encodeFrame(f2)), f2);
  assert.throws(() => decodeFrame(wire), RangeError, "one frame per data channel message");

  for (const size of [1, 2, 7, wire.length]) {
    const r = new FrameReader();
    const out = [];
    for (let i = 0; i < wire.length; i += size) out.push(...r.push(wire.subarray(i, i + size)));
    assert.deepEqual(out, [f1, f2], `chunks of ${size}`);
    assert.equal(r.pending, false);
  }
  const r = new FrameReader();
  assert.deepEqual(r.push(wire.subarray(0, 3)), []);
  assert.equal(r.pending, true);
  assert.throws(() => new FrameReader().push(unhex("10ff")), RangeError, "garbage is not incomplete");
});
