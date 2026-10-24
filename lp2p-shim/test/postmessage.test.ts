import { test } from "node:test";
import assert from "node:assert/strict";
import { PostMessageTransport } from "../src/transport/postmessage.ts";
import { PortSignaler } from "../src/transport/signaling.ts";
import type { DiscoveryEvent, TransportConnection, TransportStream } from "../src/transport/types.ts";
import { closeAll, delay, transportPair } from "./helpers.ts";

function nextStream(conn: TransportConnection): Promise<TransportStream> {
  return new Promise((r) => (conn.onstream = r));
}
function messages(s: TransportStream, count: number): Promise<string[]> {
  const out: string[] = [];
  return new Promise((r) => {
    s.onmessage = (m) => {
      out.push(new TextDecoder().decode(m));
      if (out.length === count) r(out);
    };
  });
}
const enc = (s: string): Uint8Array => new TextEncoder().encode(s);

test("postMessage: discovery reports peers appearing and leaving", async () => {
  const [a, b] = transportPair();
  try {
    const events: DiscoveryEvent[] = [];
    const disc = b.discover((e) => events.push(e));
    const ad = a.listen({ id: "peer-a", nickname: "Alice" }, () => {});
    await delay(100);
    assert.deepEqual(events, [{ kind: "added", peer: { id: "peer-a", nickname: "Alice" } }]);
    ad.close();
    await delay(100);
    assert.deepEqual(events.at(-1), { kind: "removed", peer: { id: "peer-a", nickname: "Alice" } });
    disc.close();
  } finally {
    closeAll(a, b);
  }
});

test("postMessage: discovery expires silent peers", async () => {
  const [a, b] = transportPair();
  try {
    const events: DiscoveryEvent[] = [];
    const disc = b.discover((e) => events.push(e));
    a.listen({ id: "peer-a", nickname: "Alice" }, () => {});
    await delay(100);
    // Silence a without a goodbye: stop answering queries.
    (a as unknown as { rendezvous: { signaler: { onmessage: unknown } } }).rendezvous.signaler.onmessage = null;
    const kinds = events.map((e) => e.kind);
    assert.deepEqual(kinds.slice(0, 1), ["added"]);
    await delay(400);
    assert.equal(events.at(-1)?.kind, "removed");
    disc.close();
  } finally {
    closeAll(a, b);
  }
});

async function exerciseConnection(a: PostMessageTransport, b: PostMessageTransport): Promise<void> {
  const incoming = new Promise<TransportConnection>((resolve) => a.listen({ id: "A", nickname: "Alice" }, resolve));
  await delay(20);
  const dialed = await b.connect("A", { id: "B", nickname: "Bob" });
  const accepted = await incoming;
  assert.equal(dialed.remoteId, "A");
  assert.equal(accepted.remoteId, "B");

  // Requester-opened stream; messages sent before the peer saw it arrive in order.
  const s1 = dialed.openStream();
  s1.send(enc("one"));
  s1.send(enc("two"));
  const r1 = await nextStream(accepted);
  assert.deepEqual(await messages(r1, 2), ["one", "two"]);
  assert.equal(s1.id % 2, 0);

  // Receiver-opened stream, other direction.
  const s2 = accepted.openStream();
  assert.equal(s2.id % 2, 1);
  s2.send(enc("three"));
  const r2 = await nextStream(dialed);
  assert.deepEqual(await messages(r2, 1), ["three"]);

  // Stream close reaches the peer after the data.
  r1.send(enc("last"));
  const got = messages(s1, 1);
  r1.close();
  const closed = new Promise<void>((r) => (s1.onclose = r));
  assert.deepEqual(await got, ["last"]);
  await closed;
  assert.equal(s1.closed, true);

  // Connection close reaches the peer and closes its streams.
  const connClosed = new Promise<string>((r) => (accepted.onclose = r));
  const streamClosed = new Promise<void>((r) => (r2.onclose = r));
  dialed.close();
  assert.match(await connClosed, /closed by peer/);
  await streamClosed;
  assert.throws(() => accepted.openStream(), /closed/);
}

test("postMessage: connections and streams over BroadcastChannel", async () => {
  const [a, b] = transportPair();
  try {
    await exerciseConnection(a, b);
  } finally {
    closeAll(a, b);
  }
});

test("postMessage: connections over a transferred MessageChannel (PortSignaler)", async () => {
  const link = new MessageChannel();
  const a = new PostMessageTransport({ signaler: new PortSignaler(link.port1) });
  const b = new PostMessageTransport({ signaler: new PortSignaler(link.port2) });
  try {
    await exerciseConnection(a, b);
  } finally {
    closeAll(a, b);
    link.port1.close();
    link.port2.close();
  }
});

test("postMessage: connect to a missing peer times out", async () => {
  const [a, b] = transportPair({ connectTimeout: 100 });
  try {
    await assert.rejects(b.connect("nobody", { id: "B", nickname: "Bob" }), { name: "NetworkError" });
  } finally {
    closeAll(a, b);
  }
});
