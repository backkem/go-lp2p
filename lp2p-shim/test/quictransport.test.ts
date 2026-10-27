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
import { sessionOf, slotsOf } from "../src/api/connection.ts";
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
    receiverPage: { transport: ta, ui: ua, pairing: "none", origin: "https://app.example" },
    requesterPage: { transport: tb, ui: ub, pairing: "none", origin: "https://app.example" },
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
  listener.close();
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

test("quic: the spec's example - the requester opens, the receiver listens; bidirectional echo", async () => {
  const p = pages();
  try {
    // Receiver
    configure(p.receiverPage);
    const receiver = new LP2PReceiver({ nickname: "example-receiver" });
    const echoed = new Promise<LP2PQuicTransport>((resolve) => {
      receiver.onconnection = async (e) => {
        const listener = new LP2PQuicTransportListener(e.connection);
        for await (const transport of listener.incomingTransports as unknown as AsyncIterable<LP2PQuicTransport>) {
          // Incoming transports are already ready.
          assert.equal(transport.reliability, "reliable-only");
          const stream = await first<LP2PBidirectionalStream>(transport.incomingBidirectionalStreams);
          await stream.readable.pipeTo(stream.writable);
          resolve(transport);
        }
      };
    });
    await receiver.start();

    // Requester
    configure(p.requesterPage);
    const request = new LP2PRequest({ nickname: "example-request" });
    const conn = await request.start();
    const transport = new LP2PQuicTransport(conn);
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
    // Closing a transport leaves the connection open.
    const ch = conn.createDataChannel("still open");
    assert.equal(ch.readyState, "connecting");
    conn.close();
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

test("quic: both sides constructing at once open two sessions; either side can open", async () => {
  const p = pages();
  try {
    const { local, remote } = await connect(p);
    const la = new LP2PQuicTransportListener(local);
    const lb = new LP2PQuicTransportListener(remote);
    const ta = new LP2PQuicTransport(local);
    const tb = new LP2PQuicTransport(remote);
    const [pa, pb] = await Promise.all([first<LP2PQuicTransport>(lb.incomingTransports), first<LP2PQuicTransport>(la.incomingTransports)]);
    await Promise.all([ta.ready, tb.ready]);
    // ta pairs with pa (on the receiver), tb with pb (on the requester).
    const dg = ta.datagrams.createWritable().getWriter();
    await dg.write(enc.encode("one"));
    assert.equal(dec.decode((await first(pa.datagrams.readable)) as Uint8Array), "one");
    const dg2 = tb.datagrams.createWritable().getWriter();
    await dg2.write(enc.encode("two"));
    assert.equal(dec.decode((await first(pb.datagrams.readable)) as Uint8Array), "two");
    local.close();
  } finally {
    closeAll(...p.transports);
  }
});

test("quic: listener rules - one per connection, refusal without one or when full, held until exposed", async () => {
  const p = pages();
  try {
    configure(p.receiverPage);
    const receiver = new LP2PReceiver({ nickname: "R" });
    // The requester opens a transport right away; the receiver's page only
    // constructs its listener in the connection event: the session is held.
    const gotTransport = new Promise<LP2PQuicTransport>((resolve) => {
      receiver.onconnection = (e) => {
        const l = new LP2PQuicTransportListener(e.connection);
        assert.throws(() => new LP2PQuicTransportListener(e.connection), { name: "InvalidStateError" });
        void first<LP2PQuicTransport>(l.incomingTransports).then(resolve);
      };
    });
    await receiver.start();
    configure(p.requesterPage);
    const conn = await new LP2PRequest({ nickname: "Q" }).start();
    const t = new LP2PQuicTransport(conn);
    await Promise.all([t.ready, gotTransport]);

    // On a second connection the requester has no listener: the receiver's session is refused.
    const { local: local2, remote: rconn } = await (async () => {
      configure(p.receiverPage);
      const r2 = new LP2PReceiver({ nickname: "R2" });
      const ev = once<LP2PConnectionEvent>(r2, "connection");
      await r2.start();
      configure({ ...p.requesterPage, ui: pairedUIs({}, { pick: (peers) => peers.find((x) => x.nickname === "R2") })[1] });
      const local = await new LP2PRequest({ nickname: "Q2" }).start();
      return { local, remote: (await ev).connection, r2 };
    })();
    const refused = new LP2PQuicTransport(rconn);
    await assert.rejects(refused.ready, (e: { name: string; source?: string }) => e.name === "WebTransportError" && e.source === "session");

    // A full queue refuses too: nobody reads this listener.
    const unread = new LP2PQuicTransportListener(local2);
    const opened: LP2PQuicTransport[] = [];
    for (let i = 0; i < 12; i++) opened.push(new LP2PQuicTransport(rconn));
    const results = await Promise.allSettled(opened.map((x) => x.ready));
    const ok = results.filter((r) => r.status === "fulfilled").length;
    assert.ok(ok >= 1 && ok < 12, `accepted ${ok} of 12`);

    // close() ends the stream, keeps what was queued, and frees the slot.
    unread.close();
    const reader = unread.incomingTransports.getReader();
    let queued = 0;
    for (;;) {
      const { done } = await reader.read();
      if (done) break;
      queued++;
    }
    assert.equal(queued, ok);
    const again = new LP2PQuicTransportListener(local2);
    again.close();
    local2.close();
    conn.close();
    receiver.stop();
  } finally {
    closeAll(...p.transports);
  }
});

test("quic: close codes, FIN without close, drain, session-gone on open streams", async () => {
  const p = pages();
  try {
    const { local, remote } = await connect(p);
    const [a, b] = await transportPairOver(local, remote);
    const s = await a.createBidirectionalStream();
    const w = s.writable.getWriter();
    await w.write(enc.encode("x"));
    const got = await first<LP2PBidirectionalStream>(b.incomingBidirectionalStreams);

    // No drain was asked for: draining is still pending.
    let drained = false;
    void a.draining.then(() => (drained = true));
    await new Promise((r) => setTimeout(r, 10));
    assert.equal(drained, false);

    // Closing with a code: streams of the session end with session-gone,
    // reported without a stream error code.
    b.close({ closeCode: 4242, reason: "é".repeat(600) }); // truncated to 1024 bytes
    const info = await a.closed;
    assert.equal(info.closeCode, 4242);
    assert.equal(new TextEncoder().encode(info.reason as string).length, 1024);
    await assert.rejects(w.closed, (e: { source?: string; streamErrorCode?: number | null }) => e.source === "session" && e.streamErrorCode === null);
    await assert.rejects(got.writable.getWriter().closed, { name: "WebTransportError" });
    local.close();
  } finally {
    closeAll(...p.transports);
  }
});

test("quic: closing the connection right after transport.close() still delivers the close", async () => {
  const p = pages();
  try {
    const { local, remote } = await connect(p);
    const [a, b] = await transportPairOver(local, remote);
    assert.equal(a.connection, local);
    assert.equal(b.connection, remote);
    a.close({ closeCode: 9, reason: "then the connection" });
    local.close();
    assert.deepEqual(await b.closed, { closeCode: 9, reason: "then the connection" });
    await once(remote, "close");
  } finally {
    closeAll(...p.transports);
  }
});

test("quic: data-transport-drain resolves draining; FIN without close is code 0", async () => {
  const p = pages();
  try {
    const { local, remote } = await connect(p);
    const listener = new LP2PQuicTransportListener(remote);
    const s = sessionOf(local);
    // Drive the wire directly: a page cannot ask for a drain.
    const ws = s.openTransport();
    const opened = new Promise<void>((r) => (ws.onready = r));
    const peer = await first<LP2PQuicTransport>(listener.incomingTransports);
    await opened;
    let drained = false;
    void peer.draining.then(() => (drained = true));
    ws.drain();
    await peer.draining;
    assert.equal(drained, true);
    assert.equal(peer.reliability, "reliable-only"); // still usable
    // End the control stream without data-transport-close.
    (ws as unknown as { control: { close(): void } }).control.close();
    assert.deepEqual(await peer.closed, { closeCode: 0, reason: "" });
    local.close();
  } finally {
    closeAll(...p.transports);
  }
});

test("quic: exportKeyingMaterial agrees on both sides and differs per session", async () => {
  const p = pages();
  try {
    const { local, remote } = await connect(p);
    const [a, b] = await transportPairOver(local, remote);
    const [c, d] = await transportPairOver(local, remote);
    const label = enc.encode("my-app proof");
    const ka = new Uint8Array(await a.exportKeyingMaterial(label));
    const kb = new Uint8Array(await b.exportKeyingMaterial(label));
    const kc = new Uint8Array(await c.exportKeyingMaterial(label));
    const kd = new Uint8Array(await d.exportKeyingMaterial(label));
    assert.equal(ka.length, 32);
    assert.deepEqual(ka, kb);
    assert.deepEqual(kc, kd);
    assert.notDeepEqual(ka, kc);
    const ctx = new Uint8Array(await a.exportKeyingMaterial(label, enc.encode("ctx")));
    assert.notDeepEqual(ctx, ka);
    assert.equal((await a.exportKeyingMaterial(label, undefined, 64)).byteLength, 64);
    await assert.rejects(a.exportKeyingMaterial(new Uint8Array(256)), TypeError);
    a.close();
    await assert.rejects(a.exportKeyingMaterial(label), { name: "InvalidStateError" });
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
    const listener = new LP2PQuicTransportListener(local);

    // Data channels and transports share the connection.
    const ch = local.createDataChannel("side by side");
    await once<LP2PDataChannelEvent>(remote, "datachannel");
    assert.equal(ch.label, "side by side");

    // Closing the LP2P connection fails its transports and closes its
    // listener before the close event.
    const seenAtClose: unknown[] = [];
    const closeEvent = new Promise<void>((resolve) => {
      local.onclose = () => {
        // Observable synchronously in the event: the transport has failed
        // (reliability is back to "pending") and the listener is gone.
        seenAtClose.push(a.reliability, slotsOf(local).listener);
        resolve();
      };
    });
    const listenerDone = listener.incomingTransports.getReader().read();
    local.close();
    await closeEvent;
    assert.deepEqual(seenAtClose, ["pending", null]);
    assert.equal((await listenerDone).done, true);
    for (const t of [a, b]) {
      await assert.rejects(t.closed, (err: { name: string; source?: string }) => err.name === "WebTransportError" && err.source === "session");
    }
    await assert.rejects(s.writable.getWriter().closed, { name: "WebTransportError" });
    await assert.rejects(a.createBidirectionalStream(), { name: "InvalidStateError" });
    assert.throws(() => new LP2PQuicTransport(local), { name: "InvalidStateError" });
    assert.throws(() => new LP2PQuicTransportListener(local), { name: "InvalidStateError" });

    // close() before ready.
    configure(p.requesterPage);
    const conn = await new LP2PRequest({ nickname: "Requester 2" }).start();
    const early = new LP2PQuicTransport(conn);
    early.close();
    await assert.rejects(early.ready, { name: "WebTransportError" });

    assert.throws(() => new LP2PQuicTransport({} as LP2PConnection), TypeError);
    assert.throws(() => new (LP2PQuicTransport as unknown as new (x: unknown) => unknown)(new LP2PRequest()), TypeError);
    assert.throws(() => new LP2PQuicTransport(conn, { congestionControl: "fast" as "default" }), TypeError);
    assert.throws(() => new (LP2PQuicTransportListener as unknown as new (x: unknown) => unknown)(receiver), TypeError);
    conn.close();
    receiver.stop();
  } finally {
    closeAll(...p.transports);
  }
});
