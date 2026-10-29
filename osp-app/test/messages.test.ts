import { test } from "node:test";
import assert from "node:assert/strict";
import { decodeBody, encodeBody, messageName, messages, SchemaError, typeKeyOf, type MessageName } from "../src/messages.ts";
import { cddlVectors, goVectors, PID, URL1 } from "./vectors.ts";
import { hex, unhex } from "./hex.ts";

test("type keys match application.bs appendix A", () => {
  const expected: Record<string, number> = {
    "agent-info-request": 10, "agent-info-response": 11, "agent-status-request": 12, "agent-status-response": 13,
    "presentation-url-availability-request": 14, "presentation-url-availability-response": 15, "presentation-connection-message": 16,
    "remote-playback-availability-request": 17, "remote-playback-availability-response": 18, "remote-playback-modify-request": 19,
    "remote-playback-modify-response": 20, "remote-playback-state-event": 21, "audio-frame": 22, "video-frame": 23, "data-frame": 24,
    "presentation-url-availability-event": 103, "presentation-start-request": 104, "presentation-start-response": 105,
    "presentation-termination-request": 106, "presentation-termination-response": 107, "presentation-termination-event": 108,
    "presentation-connection-open-request": 109, "presentation-connection-open-response": 110, "presentation-connection-close-event": 113,
    "remote-playback-availability-event": 114, "remote-playback-start-request": 115, "remote-playback-start-response": 116,
    "remote-playback-termination-request": 117, "remote-playback-termination-response": 118, "remote-playback-termination-event": 119,
    "agent-info-event": 120, "presentation-change-event": 121, "streaming-capabilities-request": 122, "streaming-session-receiver-stats-event": 132,
  };
  for (const [name, key] of Object.entries(expected)) {
    assert.equal(typeKeyOf(name as MessageName), key, name);
    assert.equal(messageName(key), name);
  }
  assert.equal(Object.keys(messages).length, 43);
  assert.equal(messageName(1001), undefined, "network-layer auth messages are not application messages");
});

for (const [set, vectors] of [["openscreen-go", goVectors], ["CDDL", cddlVectors]] as const) {
  test(`round-trips the ${set} vectors byte for byte`, () => {
    for (const v of vectors) {
      const name = v.name as MessageName;
      const bytes = encodeBody(name, v.body as never);
      assert.equal(hex(bytes), v.hex, `encode ${v.name}`);
      const msg = decodeBody(typeKeyOf(name), unhex(v.hex));
      assert.equal(msg.name, v.name);
      assert.deepEqual(msg.body, v.body, `decode ${v.name}`);
    }
  });
}

test("schema: required fields, types, ranges and closed choices", () => {
  const bad: [MessageName, unknown, RegExp][] = [
    ["presentation-start-request", { requestId: 1, presentationId: PID, url: URL1 }, /headers: required field \(key 3\)/],
    ["presentation-url-availability-request", { requestId: 1, urls: [], watchDuration: 0, watchId: 1 }, /urls: expected \[1\* text\]/],
    ["presentation-start-response", { requestId: 1, result: 2, connectionId: 0 }, /result: expected &\(/],
    ["presentation-connection-message", { connectionId: -1, message: "x" }, /connectionId: expected uint/],
    ["presentation-connection-message", { connectionId: 1, message: 5 }, /message: expected bytes \/ text/],
    ["presentation-start-request", { requestId: 1, presentationId: PID, url: URL1, headers: [["a"]] }, /headers\[0\]: expected \[text, text\]/],
  ];
  for (const [name, body, re] of bad) {
    assert.throws(() => encodeBody(name, body as never), (e: unknown) => e instanceof SchemaError && re.test(e.message), name);
  }
  // Decoding: a missing required key, and a wrong type.
  assert.throws(() => decodeBody(16, unhex("a10007")), SchemaError);
  assert.throws(() => decodeBody(16, unhex("a20007016568656c6c6f00")), RangeError, "trailing bytes");
  assert.throws(() => decodeBody(105, unhex("a3000301f50200")), /result: expected/);
  assert.throws(() => decodeBody(9999, unhex("a0")), /unknown type key 9999/);
});

test("schema: unknown keys are ignored, floats accept integers", () => {
  // presentation-change-event with an extension field at key 99.
  const msg = decodeBody(121, unhex("a300781a534433493742564d33344b5a344d324354345947374b454e574a01011863f5"));
  assert.deepEqual(msg.body, { presentationId: PID, connectionCount: 1 });
  // remote-playback-modify-request with volume sent as the integer 1.
  const m2 = decodeBody(19, unhex("a30001010102a10501"));
  assert.deepEqual(m2.body, { requestId: 1, remotePlaybackId: 1, controls: { volume: 1 } });
});

test("streaming messages are recognized with opaque bodies", () => {
  const msg = decodeBody(24, unhex("a2000004430a0b0c"));
  assert.equal(msg.name, "data-frame");
  assert.ok(msg.body instanceof Map);
});
