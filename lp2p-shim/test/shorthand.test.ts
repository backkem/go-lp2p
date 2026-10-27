// The explainer's snippets run end to end, the shorthand API
// (lp2p.makeDiscoverable(), lp2p.connect()) against the full API in both
// directions, and both mixed on one page. Two "pages" in one process, as in
// api.test.ts; configure() switches between them.

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
  LP2PShorthandTransport,
  configure,
  lp2p,
  type LP2PBidirectionalStream,
  type ShimConfig,
} from "../src/index.ts";
import type { PostMessageTransport } from "../src/transport/postmessage.ts";
import { AutoUI, type PeerInfo } from "../src/ua/ui.ts";
import { closeAll, delay, once, transportPair } from "./helpers.ts";

const enc = new TextEncoder();
const dec = new TextDecoder();

interface Pages {
  /** Peer A: the discoverable side. */
  a: ShimConfig;
  /** Peer B: the side that connects. */
  b: (pick?: (peers: readonly PeerInfo[]) => PeerInfo | undefined) => ShimConfig;
  transports: PostMessageTransport[];
}

function pages(origins: { a?: string; b?: string } = {}, quirks: ShimConfig["quirks"] = {}): Pages {
  const [ta, tb] = transportPair();
  return {
    a: { transport: ta, ui: new AutoUI(), pairing: "none", origin: origins.a ?? "https://app.example", quirks },
    b: (pick) => ({
      transport: tb,
      ui: new AutoUI(pick ? { pick } : {}),
      pairing: "none",
      origin: origins.b ?? "https://app.example",
      quirks,
    }),
    transports: [ta, tb],
  };
}

const byNickname = (name: string) => (peers: readonly PeerInfo[]) => peers.find((p) => p.nickname === name);

async function echoOnce(transport: { incomingBidirectionalStreams: ReadableStream<LP2PBidirectionalStream> }): Promise<void> {
  const reader = transport.incomingBidirectionalStreams.getReader();
  const { value } = await reader.read();
  reader.releaseLock();
  await (value as LP2PBidirectionalStream).readable.pipeTo((value as LP2PBidirectionalStream).writable);
}

async function roundTrip(transport: { createBidirectionalStream(): Promise<LP2PBidirectionalStream> }, text: string): Promise<string> {
  const s = await transport.createBidirectionalStream();
  const w = s.writable.getWriter();
  await w.write(enc.encode(text));
  await w.close();
  let out = "";
  for await (const chunk of s.readable as unknown as AsyncIterable<Uint8Array>) out += dec.decode(chunk);
  return out;
}

// ------------------------------------------------------- explainer snippets

test("explainer: Shorthand APIs - makeDiscoverable() and connect()", async () => {
  const p = pages();
  try {
    const options = { nickname: "Peer A" };
    // Peer A
    configure(p.a);
    const transports = lp2p.makeDiscoverable(options);
    const served = (async () => {
      for await (const transport of transports as unknown as AsyncIterable<LP2PQuicTransport>) {
        // Incoming transports are already ready.
        assert.ok(transport instanceof LP2PQuicTransport);
        assert.equal(transport.reliability, "reliable-only");
        await echoOnce(transport);
        return transport;
      }
      throw new Error("stream ended");
    })();
    await delay(20);

    // Peer B
    configure(p.b());
    const transport = lp2p.connect({ nickname: "Peer B" });
    assert.ok(transport instanceof LP2PShorthandTransport);
    assert.equal(transport.reliability, "pending");
    await transport.ready;
    assert.equal(transport.reliability, "reliable-only");
    assert.equal(await roundTrip(transport, "hello shorthand"), "hello shorthand");

    const peer = await served;
    transport.close({ closeCode: 3, reason: "bye" });
    assert.deepEqual(await peer.closed, { closeCode: 3, reason: "bye" });
    await transports.cancel();
  } finally {
    closeAll(...p.transports);
  }
});

test("explainer: Peer discovery - allowedOrigins, start(), stop(), close() on both sides", async () => {
  const p = pages({ a: "https://a.example", b: "https://partner.example" });
  try {
    const log: string[] = [];
    // Peer A
    configure(p.a);
    const receiver = new LP2PReceiver({
      nickname: "Peer A",
      // Optional: also accept connections from pages of another origin.
      allowedOrigins: ["https://partner.example"],
    });
    const closed = new Promise<void>((resolve) => {
      receiver.onconnection = (e) => {
        const conn = e.connection;
        log.push("Receiver: Got a connection!");
        conn.onclose = () => {
          log.push("Receiver: Connection closed.");
          resolve();
        };
      };
    });
    // Asks the user for permission to advertise.
    await receiver.start();

    // Peer B
    configure(p.b());
    const request = new LP2PRequest({ nickname: "Peer B" });
    // Shows a peer picker. Rejects with NotAllowedError if the user dismisses
    // it, or if the chosen peer refuses the connection.
    const conn = await request.start();
    log.push("Requester: Got a connection!");
    // Later: close the connection on both sides.
    conn.close();
    await closed;

    // Later: stop advertising. Existing connections stay open.
    receiver.stop();
    // As specified, start() checks [[StartPromise]] before the stopped
    // state, so after a successful start() it returns the old, resolved
    // promise although the receiver no longer advertises (spec feedback).
    assert.equal(await receiver.start(), undefined);
    const fresh = new LP2PReceiver();
    fresh.stop();
    await assert.rejects(fresh.start(), { name: "InvalidStateError" });
    // Both sides learn of the connection in either order, then it closes.
    assert.deepEqual(log.slice(0, 2).sort(), ["Receiver: Got a connection!", "Requester: Got a connection!"]);
    assert.equal(log[2], "Receiver: Connection closed.");
  } finally {
    closeAll(...p.transports);
  }
});

test("explainer: Simple message exchange", async () => {
  const p = pages();
  try {
    configure(p.a);
    const receiver = new LP2PReceiver({ nickname: "Peer A" });
    const incoming = once<LP2PConnectionEvent>(receiver, "connection");
    await receiver.start();
    configure(p.b());
    const conn = await new LP2PRequest({ nickname: "Peer B" }).start();
    const remote = (await incoming).connection;

    // Peer A
    remote.ondatachannel = (e) => {
      const channel = e.channel;
      channel.send("Good day to you, requester!");
    };
    // Peer B
    const channel = conn.createDataChannel("My Channel");
    const got = new Promise<string>((resolve) => {
      channel.onmessage = (e) => resolve(`Receiver: Received message: ${e.data}`);
    });
    assert.equal(await got, "Receiver: Received message: Good day to you, requester!");
    conn.close();
    receiver.stop();
  } finally {
    closeAll(...p.transports);
  }
});

test("explainer: Advanced data exchange - listener in onconnection, new LP2PQuicTransport(conn)", async () => {
  const p = pages();
  try {
    configure(p.a);
    const receiver = new LP2PReceiver({ nickname: "Peer A" });
    // Peer A
    const gotStream = new Promise<LP2PBidirectionalStream>((resolve) => {
      receiver.onconnection = async (e) => {
        const listener = new LP2PQuicTransportListener(e.connection);
        for await (const transport of listener.incomingTransports as unknown as AsyncIterable<LP2PQuicTransport>) {
          const stream = await transport.createBidirectionalStream();
          resolve(stream);
        }
      };
    });
    await receiver.start();

    // Peer B
    configure(p.b());
    const conn = await new LP2PRequest({ nickname: "Peer B" }).start();
    const transport = new LP2PQuicTransport(conn);
    await transport.ready;

    // Either peer can open a stream: here the listening side did.
    const fromA = await gotStream;
    const w = fromA.writable.getWriter();
    await w.write(enc.encode("from A"));
    await w.close();
    const reader = transport.incomingBidirectionalStreams.getReader();
    const { value } = await reader.read();
    let text = "";
    for await (const c of (value as LP2PBidirectionalStream).readable as unknown as AsyncIterable<Uint8Array>) text += dec.decode(c);
    assert.equal(text, "from A");

    // Closing a transport leaves the connection open, while closing the
    // connection closes its transports.
    transport.close();
    await transport.closed;
    const t2 = new LP2PQuicTransport(conn);
    await t2.ready;
    conn.close();
    await assert.rejects(t2.closed, { name: "WebTransportError" });
    receiver.stop();
  } finally {
    closeAll(...p.transports);
  }
});

test("spec examples: request start() failures are NotAllowedError; a refused origin never prompts", async () => {
  const p = pages({ a: "https://a.example", b: "https://other.example" });
  try {
    const prompts: string[] = [];
    configure({ ...p.a, ui: new AutoUI({ allow: (r) => (prompts.push(r.permission), true) }) });
    const receiver = new LP2PReceiver({ nickname: "example-receiver" });
    await receiver.start();
    configure(p.b());
    const request = new LP2PRequest({ nickname: "example-request" });
    let error: unknown = null;
    try {
      // Blocks until the user picked a peer and it accepted the session.
      await request.start();
    } catch (e) {
      // NotAllowedError: dismissed, refused or failed; script can't tell which.
      error = e;
    }
    assert.equal((error as DOMException).name, "NotAllowedError");
    // Refused on the requesting origin hint: no accept-connection prompt.
    assert.deepEqual(prompts, ["advertise"]);
    receiver.stop();
  } finally {
    closeAll(...p.transports);
  }
});

// --------------------------------------------- shorthand <-> full API, both ways

test("coexistence: makeDiscoverable() peer <- full API peer (LP2PRequest + new LP2PQuicTransport)", async () => {
  const p = pages();
  try {
    configure(p.a);
    const transports = lp2p.makeDiscoverable({ nickname: "Shorthand A" });
    const served = (async () => {
      const reader = transports.getReader();
      const { value } = await reader.read();
      reader.releaseLock();
      await echoOnce(value as LP2PQuicTransport);
    })();
    await delay(20);

    configure(p.b());
    const conn = await new LP2PRequest({ nickname: "Full B" }).start();
    const t = new LP2PQuicTransport(conn);
    await t.ready;
    assert.equal(await roundTrip(t, "full -> shorthand"), "full -> shorthand");
    await served;
    // The full-API side still has its connection: data channels work, but
    // the shorthand side has no ondatachannel to receive them (it can't see
    // the connection).
    const ch = conn.createDataChannel("unheard");
    await new Promise<void>((r) => (ch.onopen = () => r()));
    conn.close();
    await transports.cancel();
  } finally {
    closeAll(...p.transports);
  }
});

test("coexistence: full API peer (LP2PReceiver + listener) <- lp2p.connect() peer", async () => {
  const p = pages();
  try {
    configure(p.a);
    const receiver = new LP2PReceiver({ nickname: "Full A" });
    let serverConn: LP2PConnection | null = null;
    const served = new Promise<void>((resolve) => {
      receiver.onconnection = async (e) => {
        serverConn = e.connection;
        const listener = new LP2PQuicTransportListener(e.connection);
        for await (const t of listener.incomingTransports as unknown as AsyncIterable<LP2PQuicTransport>) {
          await echoOnce(t);
          resolve();
        }
      };
    });
    await receiver.start();

    configure(p.b());
    const transport = lp2p.connect({ nickname: "Shorthand B" });
    await transport.ready;
    assert.equal(await roundTrip(transport, "shorthand -> full"), "shorthand -> full");
    await served;

    // The full-API side can open a transport toward the shorthand side, but
    // lp2p.connect() has no listener on its connection: refused.
    const back = new LP2PQuicTransport(serverConn as unknown as LP2PConnection);
    await assert.rejects(back.ready, (e: { source?: string }) => e.source === "session");

    // Closing the shorthand transport closes its hidden connection too.
    const connClosed = once(serverConn as unknown as LP2PConnection, "close");
    transport.close();
    await transport.closed;
    await connClosed;
    receiver.stop();
  } finally {
    closeAll(...p.transports);
  }
});

test("coexistence: lp2p.connect() closed before a peer is picked", async () => {
  const p = pages();
  try {
    configure(p.a);
    const receiver = new LP2PReceiver({ nickname: "A" });
    const conns: LP2PConnection[] = [];
    receiver.onconnection = (e) => conns.push(e.connection);
    await receiver.start();

    // The picker waits for "nobody" while the page gives up.
    let release!: () => void;
    const gate = new Promise<void>((r) => (release = r));
    configure({
      ...p.b(),
      ui: Object.assign(new AutoUI(), {
        pickPeer: async (picker: { peers: readonly PeerInfo[] }) => {
          await gate;
          return picker.peers[0] ?? null;
        },
      }),
    });
    const transport = lp2p.connect({ nickname: "B" });
    transport.close();
    await assert.rejects(transport.ready, { name: "WebTransportError" });
    // LP2PRequest.start() can't be aborted: the picker is still up. Once
    // the user picks, a session is established and closed right away, so
    // the receiver page sees nothing, or a connection that closes at once.
    release();
    await delay(100);
    assert.ok(conns.length <= 1);
    for (const c of conns) await once(c, "close");
    receiver.stop();
  } finally {
    closeAll(...p.transports);
  }
});

// ------------------------------------------------------- mixing on one page

test("coexistence: one page with makeDiscoverable() and its own LP2PReceiver", async () => {
  const p = pages({ a: "https://a.example", b: "https://b.example" });
  try {
    configure(p.a);
    // The shorthand allows https://b.example; the page's own receiver doesn't.
    const transports = lp2p.makeDiscoverable({ nickname: "Shorthand", allowedOrigins: ["https://b.example"] });
    const own = new LP2PReceiver({ nickname: "Own" });
    const ownConnections: LP2PConnection[] = [];
    own.onconnection = (e) => ownConnections.push(e.connection);
    await own.start();
    await delay(20);

    // Both advertise: the peer's picker lists two devices for one page.
    let listed: string[] = [];
    configure(p.b((peers) => ((listed = peers.map((x) => x.nickname)), byNickname("Own")(peers))));
    // B picks "Own", whose receiver doesn't allow B's origin. The spec lets
    // the user agent deliver to any advertising receiver that allows it: here
    // the shorthand's. The page's own receiver never sees the connection.
    const t = lp2p.connect({ nickname: "B" });
    await t.ready;
    assert.deepEqual([...listed].sort(), ["Own", "Shorthand"]);
    const reader = transports.getReader();
    const { value: incoming } = await reader.read();
    reader.releaseLock();
    assert.ok(incoming instanceof LP2PQuicTransport);
    assert.equal(ownConnections.length, 0);
    t.close();

    // Cancelling the shorthand stream stops its receiver only.
    await transports.cancel();
    configure(p.b(byNickname("Own")));
    await assert.rejects(new LP2PRequest({ nickname: "B2" }).start(), { name: "NotAllowedError" }); // refused: no receiver allows b.example now
    own.stop();
  } finally {
    closeAll(...p.transports);
  }
});

test("coexistence: the shorthand hides its connection, so the page can't listen on it (without the quirk)", async () => {
  const p = pages();
  try {
    configure(p.a);
    const transports = lp2p.makeDiscoverable({ nickname: "A" });
    await delay(20);
    configure(p.b());
    const t = lp2p.connect({ nickname: "B" });
    await t.ready;
    const reader = transports.getReader();
    const { value } = await reader.read();
    reader.releaseLock();
    assert.equal("connection" in (value as object), false);
    assert.equal("connection" in t, false);
    t.close();
    await transports.cancel();
  } finally {
    closeAll(...p.transports);
  }
});

test("coexistence: with quirks.exposeShorthandConnection, a page listener on a shorthand connection is InvalidStateError", async () => {
  const p = pages({}, { exposeShorthandConnection: true });
  try {
    configure(p.a);
    const transports = lp2p.makeDiscoverable({ nickname: "A" });
    await delay(20);
    configure(p.b());
    const t = lp2p.connect({ nickname: "B" });
    await t.ready;
    const reader = transports.getReader();
    const { value } = await reader.read();
    reader.releaseLock();
    const serverConn = (value as unknown as { connection: LP2PConnection }).connection;
    const clientConn = (t as unknown as { connection: LP2PConnection }).connection;
    assert.ok(serverConn instanceof LP2PConnection && clientConn instanceof LP2PConnection);

    // One listener per connection: the shorthand already has it.
    assert.throws(() => new LP2PQuicTransportListener(serverConn), { name: "InvalidStateError" });
    // The connect() side has none, so the page can add one, and then the
    // discoverable side's page can open a transport back.
    const back = new LP2PQuicTransportListener(clientConn);
    const fromA = new LP2PQuicTransport(serverConn);
    const r = back.incomingTransports.getReader();
    const { value: got } = await r.read();
    await Promise.all([fromA.ready, (got as LP2PQuicTransport).ready]);
    // Data channels work on the exposed connection.
    const dc = once<LP2PDataChannelEvent>(clientConn, "datachannel");
    serverConn.createDataChannel("side channel");
    assert.equal((await dc).channel.label, "side channel");

    // A transport the page opens on the exposed client connection lands in
    // the discoverable side's shorthand stream, like any other.
    const extra = new LP2PQuicTransport(clientConn);
    const again = transports.getReader();
    const { value: viaStream } = await again.read();
    again.releaseLock();
    await extra.ready;
    assert.ok(viaStream instanceof LP2PQuicTransport);
    t.close();
    await transports.cancel();
  } finally {
    closeAll(...p.transports);
  }
});
