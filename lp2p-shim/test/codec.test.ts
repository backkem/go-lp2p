import { test } from "node:test";
import assert from "node:assert/strict";
import { decodeCBOR, encodeCBOR, type CBORValue } from "../src/codec/cbor.ts";
import { decodeVarint, encodeVarint } from "../src/codec/varint.ts";
import { TypeKey, decodeMessage, encodeMessage, type Message } from "../src/protocol/messages.ts";

const hex = (b: Uint8Array): string => Buffer.from(b).toString("hex");
const unhex = (s: string): Uint8Array => new Uint8Array(Buffer.from(s.replace(/\s+/g, ""), "hex"));

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
  assert.equal(hex(encodeVarint(1101)), "444d");
  assert.throws(() => encodeVarint(-1), RangeError);
  assert.throws(() => decodeVarint(unhex("40")), RangeError);
});

test("cbor: RFC 8949 appendix A examples", () => {
  const cases: [CBORValue, string][] = [
    [0, "00"],
    [23, "17"],
    [24, "1818"],
    [100, "1864"],
    [1000, "1903e8"],
    [1000000, "1a000f4240"],
    [1000000000000, "1b000000e8d4a51000"],
    [-1, "20"],
    [-1000, "3903e7"],
    [1.1, "fb3ff199999999999a"],
    [false, "f4"],
    [true, "f5"],
    [null, "f6"],
    [undefined, "f7"],
    [new Uint8Array([1, 2, 3, 4]), "4401020304"],
    ["", "60"],
    ["IETF", "6449455446"],
    ["ü", "62c3bc"],
    ["水", "63e6b0b4"],
    [[], "80"],
    [[1, [2, 3], [4, 5]], "8301820203820405"],
    [{}, "a0"],
    [{ 1: 2, 3: 4 }, "a201020304"],
    [{ a: 1, b: [2, 3] }, "a26161016162820203"],
  ];
  for (const [v, h] of cases) {
    assert.equal(hex(encodeCBOR(v)), h, `encode ${JSON.stringify(v)}`);
    assert.deepEqual(decodeCBOR(unhex(h)), v, `decode ${h}`);
  }
  // Decode-only: half floats, single floats, tags.
  assert.equal(decodeCBOR(unhex("f93c00")), 1);
  assert.equal(decodeCBOR(unhex("f97bff")), 65504);
  assert.equal(decodeCBOR(unhex("fa47c35000")), 100000);
  assert.equal(decodeCBOR(unhex("c11a514b67b0")), 1363896240);
});

test("cbor: rejects malformed input", () => {
  assert.throws(() => decodeCBOR(unhex("19")), RangeError); // truncated
  assert.throws(() => decodeCBOR(unhex("0000")), RangeError); // trailing
  assert.throws(() => decodeCBOR(unhex("5f")), RangeError); // indefinite
  assert.throws(() => decodeCBOR(unhex("62c328")), TypeError); // invalid UTF-8
  assert.throws(() => decodeCBOR(unhex("a1f600")), TypeError); // null map key
});

test("messages: wire format matches OSP framing", () => {
  // varint(1101) then {0: 7, 1: 2, 2: "chat", 3: ""}
  const open: Message = { type: TypeKey.DataChannelOpenRequest, requestId: 7, channelId: 2, label: "chat", protocol: "" };
  assert.equal(hex(encodeMessage(open)), "444d" + "a4" + "0007" + "0102" + "026463686174" + "0360");
  // varint(1102) then {0: 7, 1: 1 (success)}
  assert.equal(hex(encodeMessage({ type: TypeKey.DataChannelOpenResponse, requestId: 7, result: 1 })), "444e" + "a2" + "0007" + "0101");
  // varint(24) then {0: 1 (string), 4: h'6869'}
  assert.equal(
    hex(encodeMessage({ type: TypeKey.DataFrame, encodingId: 1, payload: new TextEncoder().encode("hi") })),
    "18" + "a2" + "0001" + "04426869",
  );
});

test("messages: round trip and tolerance of OSP optional fields", () => {
  const msgs: Message[] = [
    { type: TypeKey.AgentInfoRequest, requestId: 1 },
    {
      type: TypeKey.AgentInfoResponse,
      requestId: 1,
      agentInfo: { displayName: "TV", modelName: "lp2p-shim", capabilities: [1100], stateToken: "ab", locales: ["en"] },
    },
    { type: TypeKey.AuthCapabilities, pskEaseOfInput: 50, pskInputMethods: [0], pskMinBitsOfEntropy: 20 },
    { type: TypeKey.AuthSpake2Handshake, pskStatus: 1, publicValue: new Uint8Array(32) },
    { type: TypeKey.AuthSpake2Handshake, initiationToken: "tok", pskStatus: 2, publicValue: new Uint8Array(3) },
    { type: TypeKey.AuthSpake2Confirmation, confirmationValue: new Uint8Array(64).fill(9) },
    { type: TypeKey.AuthStatus, result: 0 },
  ];
  for (const m of msgs) assert.deepEqual(decodeMessage(encodeMessage(m)), m);

  // A data-frame from a full OSP implementation may carry sequence-number (1) etc.
  const frame = unhex("18" + "a3" + "0002" + "0105" + "04" + "4401020304");
  assert.deepEqual(decodeMessage(frame), { type: 24, encodingId: 2, payload: new Uint8Array([1, 2, 3, 4]) });

  assert.throws(() => decodeMessage(unhex("444da0"))); // missing fields
  assert.throws(() => decodeMessage(unhex("3fa0"))); // unknown type key 63
});

test("messages: LP2P session and WebTransport messages match the CDDL (PR 17 Appendix A)", () => {
  const binding = new Uint8Array(32).fill(0xaa);
  // Each vector is varint(type key) then the CBOR map, derived by hand from the CDDL.
  const vectors: [Message, string][] = [
    // lp2p-session-request = { request, 1: text } (type key 1110)
    [
      { type: TypeKey.SessionRequest, requestId: 1, requestingOrigin: "https://a.example" },
      "4456" + "a2" + "0001" + "01" + "71" + Buffer.from("https://a.example").toString("hex"),
    ],
    // lp2p-session-response = { response, 1: &result } (1111), success = 1
    [{ type: TypeKey.SessionResponse, requestId: 1, result: 1 }, "4457" + "a2" + "0001" + "0101"],
    // data-transport-start-request = { request, 1: exchange-id, ? 2: bytes .size 32 } (1201)
    [{ type: TypeKey.DataTransportStartRequest, requestId: 2, exchangeId: 4 }, "44b1" + "a2" + "0002" + "0104"],
    [
      { type: TypeKey.DataTransportStartRequest, requestId: 2, exchangeId: 2 ** 40, sessionBinding: binding },
      "44b1" + "a3" + "0002" + "01" + "1b0000010000000000" + "02" + "5820" + "aa".repeat(32),
    ],
    // data-transport-start-response = { response, 1: &result } (1202), permanent-error = 102
    [{ type: TypeKey.DataTransportStartResponse, requestId: 2, result: 102 }, "44b2" + "a2" + "0002" + "011866"],
    // data-transport-stream = { 1: exchange-id } (1203): no request-id
    [{ type: TypeKey.DataTransportStream, exchangeId: 4 }, "44b3" + "a1" + "0104"],
    // data-transport-close = { 0: uint .size 4, 1: text .size (0..1024) } (1205)
    [{ type: TypeKey.DataTransportClose, code: 7, reason: "bye" }, "44b5" + "a2" + "0007" + "0163627965"],
    [{ type: TypeKey.DataTransportClose, code: 0xffffffff, reason: "" }, "44b5" + "a2" + "001affffffff" + "0160"],
    // data-transport-drain = {} (1206)
    [{ type: TypeKey.DataTransportDrain }, "44b6" + "a0"],
  ];
  for (const [m, wire] of vectors) {
    assert.equal(hex(encodeMessage(m)), wire, `encode ${m.type}`);
    assert.deepEqual(decodeMessage(unhex(wire)), m, `decode ${m.type}`);
  }

  // 1204 is reserved: no message, so an unknown type key.
  assert.throws(() => decodeMessage(unhex("44b4" + "a1" + "0104")), /unknown type key 1204/);
  // exchange-id = uint .lt 2^62
  assert.throws(() => decodeMessage(unhex("44b3" + "a1" + "01" + "1b4000000000000000")));
  // close-code is uint .size 4
  assert.throws(() => decodeMessage(unhex("44b5" + "a2" + "00" + "1b0000000100000000" + "0160")), /close-code/);
  assert.throws(() => encodeMessage({ type: TypeKey.DataTransportClose, code: 2 ** 32, reason: "" }), /close-code/);
  // reason is at most 1024 bytes of UTF-8
  const long = "é".repeat(513); // 1026 bytes
  assert.throws(() => encodeMessage({ type: TypeKey.DataTransportClose, code: 0, reason: long }), /1024/);
  const wireLong = "44b5a20000017904" + "02" + Buffer.from("x".repeat(1026)).toString("hex");
  assert.throws(() => decodeMessage(unhex(wireLong)), /1024/);
  // session-binding is exactly 32 bytes
  assert.throws(() => decodeMessage(unhex("44b1" + "a3" + "0002" + "0104" + "02" + "5401" + "00".repeat(19))), /32 bytes/);
});
