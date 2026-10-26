// End-to-end tests of LP2PQuicTransport and LP2PQuicTransportListener over
// the postMessage transport: two "pages" in one process, as in api.test.ts.

import { test } from "node:test";
import assert from "node:assert/strict";
import {
  LP2PConnection,
  LP2PConnectionEvent,
  LP2PDataChannelEvent,
  LP2PQuicTransport,
  LP2PQuicTransportListener,
  LP2PReceiver,
  LP2PRequest,
  WebTransportError,
  configure,
  type LP2PBidirectionalStream,
  type LP2PReceiveStream,
  type ShimConfig,
} from "../src/index.ts";
import type { PostMessageTransport } from "../src/transport/postmessage.ts";
import { closeAll, once, pairedUIs, transportPair } from "./helpers.ts";

interface Pages {
  receiverPage: ShimConfig;
  requesterPage: ShimConfig;
  transports: PostMessageTransport[];
}

function pages(): Pages {
  const [ta, tb] = transportPair();
  const [ua, ub] = pairedUIs();
  return {
    receiverPage: { transport: ta, ui: ua, pairing: "none", origin: "https://receiver.example" },
    requesterPage: { transport: tb, ui: ub, pairing: "none", origin: "https://requester.example" },
    transports: [ta, tb],
  };
}

async function connect(p: Pages): Promise<{ receiver: LP2PReceiver; local: LP2PConnection; remote: LP2PConnection }> {
  configure(p.receiverPage);
  const receiver = new LP2PReceiver({ nickname: "Receiver" });
  const incoming = once<LP2PConnectionEvent>(receiver, "connection");
  await receiver.start();
  configure(p.requesterPage);
  const local = await new LP2PRequest({ nickname: "Requester" }).start();
  const remote = (await incoming).connection;
  return { receiver, local, remote };
}

/** A transport on each side of a connection: the requester opens, the receiver listens. */
async function transportPairOver(local: LP2PConnection, remote: LP2PConnection): Promise<[LP2PQuicTransport, LP2PQuicTransport]> {
  const listener = new LP2PQuicTransportListener(remote);
  const reader = listener.incomingTransports.getReader();
  const t = new LP2PQuicTransport(local);
  const { value: peer } = await reader.read();
  reader.releaseLock();
  assert.ok(peer instanceof LP2PQuicTransport);
  await Promise.all([t.ready, peer.ready]);
  return [t, peer];
}

const enc = new TextEncoder();
const dec = new TextDecoder();

async function readAll(readable: ReadableStream<Uint8Array>): Promise<Uint8Array> {
  const parts: Uint8Array[] = [];
  for await (const chunk of readable as unknown as AsyncIterable<Uint8Array>) parts.push(chunk);
  const out = new Uint8Array(parts.reduce((n, p) => n + p.length, 0));
  let off = 0;
  for (const p of parts) {
    out.set(p, off);
    off += p.length;
  }
  return out;
}

async function first<T>(stream: ReadableStream<T>): Promise<T> {
  const reader = stream.getReader();
  const { value, done } = await reader.read();
  reader.releaseLock();
  assert.equal(done, false);
  return value as T;
}

test("quic: the spec's request/listener example, with a bidirectional echo", async () => {
  const p = pages();
  try {
    configure(p.receiverPage);
    const receiver = new LP2PReceiver({ nickname: "example-receiver" });
    const listener = new LP2PQuicTransportListener(receiver);
    await listener.ready;
    const echoed = (async () => {
      for await (const transport of listener.incomingTransports as unknown as AsyncIterable<LP2PQuicTransport>) {
        await transport.ready;
        const stream = await first<LP2PBidirectionalStream>(transport.incomingBidirectionalStreams);
        await stream.readable.pipeTo(stream.writable);
        return transport;
      }
      throw new Error("no transport");
    })();

    configure(p.requesterPage);
    const request = new LP2PRequest({ nickname: "example-request" });
    const transport = new LP2PQuicTransport(request);
    assert.equal(transport.reliability, "pending");
    await transport.ready;
    assert.equal(transport.reliability, "reliable-only");
    assert.equal(transport.protocol, "");
    assert.equal(transport.responseHeaders, null);
    assert.equal(transport.congestionControl, "default");

    const stream = await transport.createBidirectionalStream();
    const writer = stream.writable.getWriter();
    const big = new Uint8Array(100_000).map((_, i) => i % 251);
    await writer.write(enc.encode("hello "));
    await writer.write(big);
    await writer.close();
    const back = await readAll(stream.readable);
    assert.equal(back.length, 6 + big.length);
    assert.equal(dec.decode(back.subarray(0, 6)), "hello ");
    assert.deepEqual(back.subarray(6), big);
    const stats = await transport.getStats();
    assert.equal(stats.bytesSent, 6 + big.length);
    assert.equal(stats.bytesReceived, 6 + big.length);

    const peer = await echoed;
    transport.close({ closeCode: 7, reason: "done" });
    assert.deepEqual(await transport.closed, { closeCode: 7, reason: "done" });
    assert.deepEqual(await peer.closed, { closeCode: 7, reason: "done" });
    await peer.draining;
    (await request.start()).close();
    receiver.stop();
  } finally {
    closeAll(...p.transports);
  }
});

test("quic: streams both ways, unidirectional streams, BYOB reads", async () => {
  const p = pages();
  try {
    const { local, remote } = await connect(p);
    const [a, b] = await transportPairOver(local, remote);

    // The listener's side opens a bidirectional stream to the opener.
    const bs = await b.createBidirectionalStream();
    const incoming = first<LP2PBidirectionalStream>(a.incomingBidirectionalStreams);
    const w = bs.writable.getWriter();
    await w.write(enc.encode("ping"));
    const as = await incoming;
    const r = as.readable.getReader();
    assert.equal(dec.decode((await r.read()).value), "ping");
    const aw = as.writable.getWriter();
    await aw.write(enc.encode("pong"));
    await aw.close();
    assert.equal(dec.decode(await readAll(bs.readable)), "pong");
    await w.close();
    assert.equal((await r.read()).done, true);

    // Unidirectional, both directions; the receiving side uses a BYOB reader.
    for (const [from, to] of [[a, b], [b, a]] as const) {
      const uni = await from.createUnidirectionalStream({ sendOrder: 3 });
      assert.equal(uni.sendOrder, 3);
      const got = first<LP2PReceiveStream>(to.incomingUnidirectionalStreams);
      const writer = uni.getWriter();
      await writer.atomicWrite(enc.encode("one way"));
      writer.commit();
      await writer.close();
      const rs = await got;
      const byob = rs.getReader({ mode: "byob" });
      let out = "";
      for (;;) {
        const { value, done } = await byob.read(new Uint8Array(4));
        if (done) break;
        out += dec.decode(value);
      }
      assert.equal(out, "one way");
      assert.deepEqual(await rs.getStats(), { bytesReceived: 7, bytesRead: 7 });
      assert.deepEqual(await uni.getStats(), { bytesWritten: 7, bytesSent: 7, bytesAcknowledged: 7 });
    }
    local.close();
  } finally {
    closeAll(...p.transports);
  }
});

test("quic: reset and STOP_SENDING carry stream error codes", async () => {
  const p = pages();
  try {
    const { local, remote } = await connect(p);
    const [a, b] = await transportPairOver(local, remote);

    // Writer aborts with a code: the peer's read rejects with it.
    const s1 = await a.createUnidirectionalStream();
    const got1 = first<LP2PReceiveStream>(b.incomingUnidirectionalStreams);
    const w1 = s1.getWriter();
    await w1.write(enc.encode("partial"));
    await w1.abort(new WebTransportError("give up", { streamErrorCode: 42 }));
    const r1 = (await got1).getReader();
    await assert.rejects(
      (async () => {
        for (;;) if ((await r1.read()).done) return;
      })(),
      (err: { source?: string; streamErrorCode?: number }) => err.source === "stream" && err.streamErrorCode === 42,
    );

    // Reader cancels with a code: the peer's writer errors with it.
    const s2 = await a.createBidirectionalStream();
    const w2 = s2.writable.getWriter();
    await w2.write(enc.encode("x"));
    const got2 = await first<LP2PBidirectionalStream>(b.incomingBidirectionalStreams);
    await got2.readable.cancel(new WebTransportError("no thanks", { streamErrorCode: 9 }));
    await assert.rejects(w2.closed, (err: { streamErrorCode?: number }) => err.streamErrorCode === 9);
    // The other direction still works.
    const w2b = got2.writable.getWriter();
    await w2b.write(enc.encode("still here"));
    await w2b.close();
    assert.equal(dec.decode(await readAll(s2.readable)), "still here");
    local.close();
  } finally {
    closeAll(...p.transports);
  }
});

test("quic: datagrams, maxDatagramSize and requireUnreliable", async () => {
  const p = pages();
  try {
    const { local, remote } = await connect(p);
    const [a, b] = await transportPairOver(local, remote);
    // postMessage cannot lose anything: reliable-only, like WebTransport over HTTP/2.
    assert.equal(a.reliability, "reliable-only");
    const max = a.datagrams.maxDatagramSize;
    assert.ok(max > 1000 && max < 1200, `maxDatagramSize ${max}`);

    const w = a.datagrams.createWritable();
    const writer = w.getWriter();
    const reader = b.datagrams.readable.getReader();
    await writer.write(enc.encode("d1"));
    await writer.write(new Uint8Array(max + 1)); // dropped silently
    await writer.write(enc.encode("d2"));
    assert.equal(dec.decode((await reader.read()).value), "d1");
    assert.equal(dec.decode((await reader.read()).value), "d2");

    // And back, through the legacy writable.
    const back = b.datagrams.writable.getWriter();
    await back.write(enc.encode("d3"));
    assert.equal(dec.decode((await first(a.datagrams.readable)) as Uint8Array), "d3");

    assert.throws(() => (a.datagrams.incomingMaxAge = -1), RangeError);
    a.datagrams.outgoingMaxAge = 0;
    assert.equal(a.datagrams.outgoingMaxAge, null);

    // A page that needs lossy delivery finds out at ready.
    const strict = new LP2PQuicTransport(local, { requireUnreliable: true });
    await assert.rejects(strict.ready, { name: "WebTransportError" });
    await assert.rejects(strict.closed, { name: "WebTransportError" });
    assert.equal(a.reliability, "reliable-only"); // the other session is unaffected
    a.close();
    assert.deepEqual(await b.closed, { closeCode: 0, reason: "" });
    assert.equal((await reader.read()).done, true);
    local.close();
  } finally {
    closeAll(...p.transports);
  }
});

test("quic: the spec's connection examples, both sides constructing (glare)", async () => {
  const p = pages();
  try {
    const { local, remote } = await connect(p);
    // Both pages do `new LP2PQuicTransport(conn)` at the same time; they get one session.
    const ta = new LP2PQuicTransport(local);
    const tb = new LP2PQuicTransport(remote);
    await Promise.all([ta.ready, tb.ready]);
    const s = await ta.createBidirectionalStream();
    const w = s.writable.getWriter();
    await w.write(enc.encode("same session"));
    await w.close();
    const got = await first<LP2PBidirectionalStream>(tb.incomingBidirectionalStreams);
    assert.equal(dec.decode(await readAll(got.readable)), "same session");

    // One after the other: the later constructor joins the session the peer opened.
    const tc = new LP2PQuicTransport(remote);
    await new Promise((r) => setTimeout(r, 20));
    const td = new LP2PQuicTransport(local);
    await Promise.all([tc.ready, td.ready]);
    const dg = td.datagrams.createWritable().getWriter();
    await dg.write(enc.encode("joined"));
    assert.equal(dec.decode((await first(tc.datagrams.readable)) as Uint8Array), "joined");
    local.close();
  } finally {
    closeAll(...p.transports);
  }
});

test("quic: lifecycle - closing the connection, closing while connecting, bad arguments", async () => {
  const p = pages();
  try {
    const { local, remote, receiver } = await connect(p);
    const [a, b] = await transportPairOver(local, remote);
    const s = await a.createBidirectionalStream();

    // Data channels and transports share the connection.
    const ch = local.createDataChannel("side by side");
    await once<LP2PDataChannelEvent>(remote, "datachannel");
    assert.equal(ch.label, "side by side");

    // Closing the LP2P connection closes its WebTransport sessions abruptly.
    local.close();
    for (const t of [a, b]) {
      await assert.rejects(t.closed, (err: { name: string; source?: string }) => err.name === "WebTransportError" && err.source === "session");
    }
    await assert.rejects(s.writable.getWriter().closed, { name: "WebTransportError" });
    await assert.rejects(a.createBidirectionalStream(), { name: "InvalidStateError" });
    await assert.rejects(new LP2PQuicTransport(local).ready, { name: "WebTransportError" });

    // A transport on a receiver uses its next connection.
    configure(p.receiverPage);
    const onNext = new LP2PQuicTransport(receiver);
    configure(p.requesterPage);
    const second = new LP2PRequest({ nickname: "Requester 2" });
    const listener2 = new LP2PQuicTransportListener(second);
    await listener2.ready;
    const peer = await first<LP2PQuicTransport>(listener2.incomingTransports);
    await Promise.all([onNext.ready, peer.ready]);

    // close() before ready.
    const early = new LP2PQuicTransport(await second.start());
    early.close();
    await assert.rejects(early.ready, { name: "WebTransportError" });

    assert.throws(() => new LP2PQuicTransport({} as LP2PConnection), TypeError);
    assert.throws(() => new LP2PQuicTransport(remote, { congestionControl: "fast" as "default" }), TypeError);
    assert.throws(() => new LP2PQuicTransportListener({} as LP2PRequest), TypeError);
    (await second.start()).close();
    receiver.stop();
  } finally {
    closeAll(...p.transports);
  }
});
