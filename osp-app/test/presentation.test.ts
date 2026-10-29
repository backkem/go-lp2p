import { test } from "node:test";
import assert from "node:assert/strict";
import {
  ConnectionCloseReason,
  OspSession,
  PresentationController,
  PresentationError,
  PresentationReceiver,
  Result,
  TerminationReason,
  TerminationSource,
  UrlAvailability,
  type PresentationConnection,
  type PresentationMessage,
} from "../src/index.ts";
import { kinds, next, sessionPair, tick, transportPair, type Kind } from "./helpers.ts";

const URL1 = "https://example.com/slides";

function setup(kind: Kind, receiver = new PresentationReceiver()) {
  const { a, b } = sessionPair(kind);
  receiver.attach(b);
  return { controller: new PresentationController(a), receiver, a, b };
}

/** A second controller on its own transport to the same receiver. */
function second(kind: Kind, receiver: PresentationReceiver): PresentationController {
  const pair = transportPair(kind);
  receiver.attach(new OspSession(pair.b));
  return new PresentationController(new OspSession(pair.a));
}

function collect(conn: PresentationConnection): PresentationMessage[] {
  const out: PresentationMessage[] = [];
  conn.on("message", (m) => out.push(m));
  return out;
}

for (const kind of kinds) {
  test(`${kind}: URL availability with a watch`, async () => {
    const { controller, receiver, a } = setup(kind);
    const r = await controller.checkAvailability([URL1, "ftp://x/"], { watchDuration: 10_000_000 });
    assert.deepEqual(r.availabilities, [UrlAvailability.available, UrlAvailability.invalid]);
    const ev = next<{ watchId: number; urls: string[]; availabilities: number[] }>((fn) => controller.on("availability", fn));
    receiver.setAvailability(URL1, UrlAvailability.unavailable);
    assert.deepEqual(await ev, { watchId: r.watchId, urls: [URL1, "ftp://x/"], availabilities: [1, 10] });
    a.close();
  });

  test(`${kind}: start, exchange text and binary messages, close`, async () => {
    const { controller, receiver, a } = setup(kind);
    const incoming = next<PresentationConnection>((fn) => receiver.on("connection", fn));
    const conn = await controller.start(URL1, { headers: [["Accept-Language", "en"]] });
    const rconn = await incoming;
    assert.equal(conn.connectionId, rconn.connectionId);
    assert.equal(conn.presentationId, rconn.presentationId);
    assert.equal(receiver.presentations.get(conn.presentationId), URL1);

    const atReceiver = collect(rconn);
    const atController = collect(conn);
    await conn.send("hello");
    await conn.send(Uint8Array.of(1, 2, 3));
    await rconn.send("hi back");
    await tick(10);
    assert.deepEqual(atReceiver, ["hello", Uint8Array.of(1, 2, 3)]);
    assert.deepEqual(atController, ["hi back"]);

    const closed = next<{ reason: number; local: boolean }>((fn) => rconn.on("close", fn));
    await conn.close();
    assert.deepEqual(await closed, { reason: ConnectionCloseReason.closeMethodCalled, local: false });
    assert.equal(conn.state, "closed");
    await assert.rejects(conn.send("late"), /closed/);
    assert.equal(controller.connections.size, 0);
    a.close();
  });

  test(`${kind}: many messages stay in order`, async () => {
    const { controller, receiver, a } = setup(kind);
    const incoming = next<PresentationConnection>((fn) => receiver.on("connection", fn));
    const conn = await controller.start(URL1);
    const rconn = await incoming;
    const got = collect(rconn);
    await Promise.all(Array.from({ length: 50 }, (_, i) => conn.send(`m${i}`)));
    await tick(20);
    assert.deepEqual(got, Array.from({ length: 50 }, (_, i) => `m${i}`));
    a.close();
  });

  test(`${kind}: the receiver speaks first, right behind the start response`, async () => {
    const receiver = new PresentationReceiver();
    receiver.on("connection", (c) => void c.send("welcome"));
    const { controller, a } = setup(kind, receiver);
    const unknown: number[] = [];
    controller.on("unknownConnection", (id) => unknown.push(id));
    // Listen before start() resolves: the message may arrive in the same chunk.
    const got: PresentationMessage[] = [];
    const conn = await controller.start(URL1);
    conn.on("message", (m) => got.push(m));
    await tick(10);
    assert.deepEqual(unknown, [], "the connection was known when its first message arrived");
    a.close();
  });

  test(`${kind}: start failures`, async () => {
    const receiver = new PresentationReceiver({ start: ({ url }) => ({ result: url.endsWith("/404") ? Result.invalidUrl : Result.success, httpResponseCode: 404 }) });
    const { controller, a } = setup(kind, receiver);
    await assert.rejects(controller.start("https://example.com/404"), (e: unknown) => e instanceof PresentationError && e.result === Result.invalidUrl);
    await assert.rejects(controller.start("ftp://x/"), (e: unknown) => e instanceof PresentationError && e.result === Result.invalidUrl);
    await assert.rejects(controller.start(URL1, { presentationId: "short" }), /16 ASCII/);
    const conn = await controller.start(URL1);
    await assert.rejects(controller.start(URL1, { presentationId: conn.presentationId }), (e: unknown) => e instanceof PresentationError && e.result === Result.invalidPresentationId);
    a.close();
  });

  test(`${kind}: reconnect from a second controller, change events, close by the receiver`, async () => {
    const { controller, receiver, a } = setup(kind);
    const c1 = await controller.start(URL1);
    const c2ctl = second(kind, receiver);
    await assert.rejects(c2ctl.reconnect(c1.presentationId, "https://other/"), (e: unknown) => e instanceof PresentationError && e.result === Result.invalidUrl);
    await assert.rejects(c2ctl.reconnect("x".repeat(20), URL1), (e: unknown) => e instanceof PresentationError && e.result === Result.invalidPresentationId);

    const change = next<{ connectionCount: number }>((fn) => c1.on("change", fn));
    const rconns: PresentationConnection[] = [];
    receiver.on("connection", (c) => rconns.push(c));
    const c2 = await c2ctl.reconnect(c1.presentationId, URL1);
    assert.equal(c2.connectionCount, 2);
    assert.notEqual(c2.connectionId, c1.connectionId);
    assert.deepEqual(await change, { connectionCount: 2 });

    // The receiver closes the second controller's connection.
    const closed = next<{ reason: number; local: boolean }>((fn) => c2.on("close", fn));
    const change2 = next<{ connectionCount: number }>((fn) => c1.on("change", fn));
    await rconns[0]!.close(ConnectionCloseReason.connectionObjectDiscarded);
    assert.deepEqual(await closed, { reason: ConnectionCloseReason.connectionObjectDiscarded, local: false });
    assert.equal(c2.connectionCount, 1);
    assert.deepEqual(await change2, { connectionCount: 1 });
    a.close();
    c2ctl.session.close();
  });

  test(`${kind}: terminate by a controller notifies the other controllers`, async () => {
    const { controller, receiver, a } = setup(kind);
    const c1 = await controller.start(URL1);
    const c2ctl = second(kind, receiver);
    const c2 = await c2ctl.reconnect(c1.presentationId, URL1);
    const ev = next<{ presentationId: string; source: number; reason: number }>((fn) => c2ctl.on("terminated", fn));
    const t1 = next<{ source: number }>((fn) => c1.on("terminate", fn));
    const t2 = next<{ source: number }>((fn) => c2.on("terminate", fn));
    const tr = next<{ presentationId: string }>((fn) => receiver.on("terminated", fn));
    await c1.terminate(TerminationReason.userRequest);
    assert.deepEqual(await ev, { presentationId: c1.presentationId, source: TerminationSource.controller, reason: TerminationReason.userRequest });
    assert.equal((await t1).source, TerminationSource.controller);
    assert.equal((await t2).source, TerminationSource.controller);
    assert.equal((await tr).presentationId, c1.presentationId);
    assert.equal(c1.state, "terminated");
    assert.equal(receiver.presentations.size, 0);
    await assert.rejects(controller.terminate(c1.presentationId), (e: unknown) => e instanceof PresentationError && e.result === Result.invalidPresentationId);
    a.close();
    c2ctl.session.close();
  });

  test(`${kind}: terminate by the receiver`, async () => {
    const { controller, receiver, a } = setup(kind);
    const conn = await controller.start(URL1);
    const t = next<{ source: number; reason: number }>((fn) => conn.on("terminate", fn));
    await receiver.terminate(conn.presentationId, TerminationReason.receiverPoweringDown);
    assert.deepEqual(await t, { source: TerminationSource.receiver, reason: TerminationReason.receiverPoweringDown });
    a.close();
  });

  test(`${kind}: a closing transport closes the connections on both sides`, async () => {
    const { controller, receiver, a } = setup(kind);
    const incoming = next<PresentationConnection>((fn) => receiver.on("connection", fn));
    const conn = await controller.start(URL1);
    const rconn = await incoming;
    const c1 = next<{ reason: number }>((fn) => conn.on("close", fn));
    const c2 = next<{ reason: number }>((fn) => rconn.on("close", fn));
    a.close();
    assert.equal((await c1).reason, ConnectionCloseReason.unrecoverableErrorWhileSendingOrReceivingMessage);
    assert.equal((await c2).reason, ConnectionCloseReason.unrecoverableErrorWhileSendingOrReceivingMessage);
  });
}
