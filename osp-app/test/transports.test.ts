import { test } from "node:test";
import assert from "node:assert/strict";
import { encodeCBOR, OspSession, PresentationController, PresentationReceiver, WebTransportTransport, type Frame, type PresentationConnection } from "../src/index.ts";
import { FakeDataChannel, FakeWebTransport, tick } from "./helpers.ts";
import { DataChannelTransport } from "../src/index.ts";

const frame = (n: number): Frame => ({ typeKey: 16, body: encodeCBOR(new Map<number, string | number>([[0, 1], [1, `m${n}`]])) });

function sinkOf(t: WebTransportTransport | DataChannelTransport): Frame[] {
  const out: Frame[] = [];
  t.start({ frame: (f) => out.push(f), malformed: (e) => assert.fail(e) });
  return out;
}

test("webtransport: an ungrouped message gets its own stream; a group shares one until endGroup", async () => {
  const [x, y] = FakeWebTransport.pair();
  const tx = new WebTransportTransport(x);
  const got = sinkOf(new WebTransportTransport(y));
  await tx.send(frame(0));
  await tx.send(frame(1));
  assert.equal(x.opened, 2);
  await Promise.all([1, 2, 3].map((n) => tx.send(frame(n), { group: "g" })));
  assert.equal(x.opened, 3);
  await tx.endGroup("g");
  await tx.send(frame(4), { group: "g" });
  assert.equal(x.opened, 4, "a new stream after endGroup");
  await tick(10);
  assert.equal(got.length, 6);
});

test("webtransport: messages split across chunks are reassembled", async () => {
  const [x, y] = FakeWebTransport.pair();
  x.chunkSize = 1;
  const tx = new WebTransportTransport(x);
  const got = sinkOf(new WebTransportTransport(y));
  for (let n = 0; n < 5; n++) await tx.send(frame(n), { group: "g" });
  await tick(20);
  assert.deepEqual(got, [0, 1, 2, 3, 4].map(frame));
});

test("webtransport: a presentation over byte-at-a-time streams", async () => {
  const [x, y] = FakeWebTransport.pair();
  x.chunkSize = 3;
  y.chunkSize = 2;
  const receiver = new PresentationReceiver();
  receiver.attach(new OspSession(new WebTransportTransport(y)));
  const controller = new PresentationController(new OspSession(new WebTransportTransport(x)));
  const rconn = new Promise<PresentationConnection>((r) => receiver.on("connection", r));
  const conn = await controller.start("https://example.com/");
  const got: unknown[] = [];
  (await rconn).on("message", (m) => got.push(m));
  await conn.send("é".repeat(100));
  await tick(20);
  assert.deepEqual(got, ["é".repeat(100)]);
  controller.session.close();
});

test("webtransport: a stream ending inside a message is malformed", async () => {
  const [x, y] = FakeWebTransport.pair();
  const errors: Error[] = [];
  new WebTransportTransport(y).start({ frame: () => assert.fail("no frame"), malformed: (e) => errors.push(e) });
  const w = (await x.createUnidirectionalStream()).getWriter();
  await w.write(Uint8Array.of(0x10, 0xa2, 0x00));
  await w.close();
  await tick(10);
  assert.equal(errors.length, 1);
});

test("datachannel: one channel message per OSP message, ArrayBuffer, binaryType set", async () => {
  const [x, y] = FakeDataChannel.pair();
  const tx = new DataChannelTransport(x);
  assert.equal(x.binaryType, "arraybuffer");
  const got = sinkOf(new DataChannelTransport(y));
  await Promise.all([0, 1, 2].map((n) => tx.send(frame(n), { group: "ignored" })));
  assert.equal(x.sent, 3);
  await tick(10);
  assert.deepEqual(got, [0, 1, 2].map(frame));
});

test("datachannel: ready rejects and send fails on a closed channel", async () => {
  const [x] = FakeDataChannel.pair();
  x.readyState = "closed";
  const t = new DataChannelTransport(x);
  await assert.rejects(t.ready, /closed/);
  await assert.rejects(t.send(frame(0)), /closed/);
});
