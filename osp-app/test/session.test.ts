import { test } from "node:test";
import assert from "node:assert/strict";
import { encodeCBOR, encodeFrame, OspSession, TransportClosedError, UNKNOWN_TYPE_KEY_CODE, type AgentInfo } from "../src/index.ts";
import { FakeDataChannel, FakeWebTransport, kinds, next, sessionPair, tick, unhex } from "./helpers.ts";

const info: AgentInfo = { displayName: "Receiver", modelName: "TS", capabilities: [3, 5], stateToken: "abcd1234", locales: ["en"] };

/** Writes raw bytes to the peer of the session's first end: a data channel message or a fresh stream. */
async function sendRaw(raw: FakeDataChannel | FakeWebTransport, bytes: Uint8Array): Promise<void> {
  if (raw instanceof FakeDataChannel) {
    await new Promise<void>((r) => (raw.readyState === "open" ? r() : raw.addEventListener("open", () => r())));
    raw.send(bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.byteLength) as ArrayBuffer);
    return;
  }
  const w = (await raw.createUnidirectionalStream()).getWriter();
  await w.write(bytes);
  await w.close();
}

for (const kind of kinds) {
  test(`${kind}: agent-info and agent-status are answered`, async () => {
    const { a, b } = sessionPair(kind, {}, { agentInfo: info });
    assert.deepEqual(await a.requestAgentInfo(), info);
    assert.deepEqual(a.remoteAgentInfo, info);
    assert.deepEqual(await a.request("agent-status-request", {}), { requestId: 2 });
    b.close();
  });

  test(`${kind}: request IDs count from 1 or come from the allocator`, async () => {
    let id = 100;
    const { a, b } = sessionPair(kind, { nextRequestId: () => id++ }, { agentInfo: info });
    const seen: number[] = [];
    b.handle("agent-info-request", (body) => seen.push(body.requestId));
    await a.requestAgentInfo();
    await a.requestAgentInfo();
    assert.deepEqual(seen, [100, 101]);
    a.close();
  });

  test(`${kind}: concurrent requests are matched by request ID`, async () => {
    const { a, b } = sessionPair(kind);
    // Answer out of order.
    const held: number[] = [];
    b.handle("agent-status-request", (body) => {
      held.push(body.requestId);
      if (held.length === 3) for (const id of held.reverse()) void b.send("agent-status-response", { requestId: id, status: { status: `s${id}` } });
    });
    const out = await Promise.all([1, 2, 3].map(() => a.request("agent-status-request", {})));
    assert.deepEqual(out.map((r) => r.status?.status), ["s1", "s2", "s3"]);
    a.close();
  });

  test(`${kind}: an unknown type key closes the connection with 404`, async () => {
    const { a, b, pair } = sessionPair(kind);
    const unknown = next<{ typeKey: number }>((fn) => b.on("unknown", fn));
    await sendRaw(pair.rawA, encodeFrame({ typeKey: 7777, body: encodeCBOR(1) }));
    assert.equal((await unknown).typeKey, 7777);
    const closedB = await b.closed;
    assert.equal(closedB.code, UNKNOWN_TYPE_KEY_CODE);
    assert.match(closedB.reason, /7777/);
    const closedA = await a.closed;
    // A data channel cannot carry the close code to the peer; WebTransport can.
    assert.equal(closedA.code, kind === "webtransport" ? 404 : 0);
  });

  test(`${kind}: an unknown type key can be ignored instead`, async () => {
    const { a, b, pair } = sessionPair(kind, { agentInfo: info }, { onUnknownTypeKey: "ignore", agentInfo: info });
    const unknown = next((fn) => b.on("unknown", fn));
    await sendRaw(pair.rawA, encodeFrame({ typeKey: 7777, body: encodeCBOR(1) }));
    await unknown;
    assert.deepEqual(await a.requestAgentInfo(), info, "still open");
    a.close();
  });

  test(`${kind}: unsupported (streaming) messages are reported and dropped, not fatal`, async () => {
    const { a, b, pair } = sessionPair(kind, {}, { agentInfo: info });
    const unsupported = next<{ name: string }>((fn) => b.on("unsupported", fn));
    await sendRaw(pair.rawA, unhex("18" + "a2000004430a0b0c")); // data-frame (24)
    assert.equal((await unsupported).name, "data-frame");
    await assert.rejects(a.send("data-frame", new Map()), /not supported/);
    assert.deepEqual(await a.requestAgentInfo(), info);
    a.close();
  });

  test(`${kind}: malformed messages are reported and dropped`, async () => {
    const { a, b, pair } = sessionPair(kind, {}, { agentInfo: info });
    const errors: Error[] = [];
    b.on("malformed", (e) => errors.push(e));
    await sendRaw(pair.rawA, unhex("10a10007")); // connection-message missing key 1
    await sendRaw(pair.rawA, unhex("10ff")); // not CBOR
    await tick(10);
    assert.equal(errors.length, 2);
    assert.match(errors[0]!.message, /required field/);
    assert.deepEqual(await a.requestAgentInfo(), info);
    a.close();
  });

  test(`${kind}: unhandled messages and stray responses are reported`, async () => {
    const { a, b } = sessionPair(kind);
    const unhandled: string[] = [];
    b.on("unhandled", (m) => unhandled.push(m.name));
    await a.send("presentation-change-event", { presentationId: "p".repeat(16), connectionCount: 1 });
    await a.send("agent-info-response", { requestId: 42, agentInfo: info });
    await tick(10);
    assert.deepEqual(unhandled, ["presentation-change-event", "agent-info-response"]);
    a.close();
  });

  test(`${kind}: closing rejects pending requests and later sends`, async () => {
    const { a, b } = sessionPair(kind);
    const pending = a.request("agent-info-request", {});
    await tick(5);
    const closeEvent = next((fn) => a.on("close", fn));
    b.close({ code: 7, reason: "bye" });
    await assert.rejects(pending, TransportClosedError);
    const info = await a.closed;
    assert.deepEqual(await closeEvent, info);
    if (kind === "webtransport") assert.deepEqual(info, { code: 7, reason: "bye" });
    assert.equal(a.isClosed, true);
    await assert.rejects(a.send("agent-status-request", { requestId: 1 }), TransportClosedError);
    await assert.rejects(a.request("agent-status-request", {}), TransportClosedError);
  });

  test(`${kind}: requests time out`, async () => {
    const { a } = sessionPair(kind);
    await assert.rejects(a.request("agent-info-request", {}, { timeout: 20 }), /timed out/);
    a.close();
  });
}

test("datachannel: messages received before the session starts are queued", async () => {
  const [x, y] = FakeDataChannel.pair();
  const { DataChannelTransport } = await import("../src/index.ts");
  const tx = new DataChannelTransport(x);
  const ty = new DataChannelTransport(y);
  await tx.send({ typeKey: 10, body: unhex("a10001") });
  await tick(5);
  const s = new OspSession(ty, { agentInfo: info });
  const got = next<{ requestId: number }>((fn) => s.handle("agent-info-request", fn));
  assert.deepEqual(await got, { requestId: 1 });
  s.close();
});

test("datachannel: a text message is malformed", async () => {
  const [x, y] = FakeDataChannel.pair();
  const { DataChannelTransport } = await import("../src/index.ts");
  const s = new OspSession(new DataChannelTransport(y));
  void new DataChannelTransport(x);
  const err = next<Error>((fn) => s.on("malformed", fn));
  await tick(5);
  x.send("hello");
  assert.match((await err).message, /binary/);
  s.close();
});
