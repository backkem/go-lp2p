// End-to-end tests of the LP2P API over the postMessage transport, two
// "pages" in one process. configure() is global, but each object binds to
// the user agent when start() is called, so the tests switch it in between.

import { test } from "node:test";
import assert from "node:assert/strict";
import {
  LP2PConnection,
  LP2PConnectionEvent,
  LP2PDataChannel,
  LP2PDataChannelEvent,
  LP2PReceiver,
  LP2PRequest,
  configure,
  install,
  type ShimConfig,
} from "../src/index.ts";
import type { PostMessageTransport } from "../src/transport/postmessage.ts";
import { AutoUI, type AutoUIOptions } from "../src/ua/ui.ts";
import { closeAll, once, pairedUIs, transportPair } from "./helpers.ts";

interface Pages {
  receiverPage: ShimConfig;
  requesterPage: ShimConfig;
  transports: PostMessageTransport[];
}

function pages(opts: { receiver?: AutoUIOptions; requester?: AutoUIOptions; pairing?: "pin" | "none" } = {}): Pages {
  const [ta, tb] = transportPair();
  const [ua, ub] = pairedUIs(opts.receiver, opts.requester);
  const pairing = opts.pairing ?? "pin";
  return {
    receiverPage: { transport: ta, ui: ua, pairing, origin: "https://receiver.example" },
    requesterPage: { transport: tb, ui: ub, pairing, origin: "https://requester.example" },
    transports: [ta, tb],
  };
}

/** Starts a receiver on one page and a request on the other; returns both connections. */
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

test("api: the spec's data channel example works", async () => {
  const p = pages();
  try {
    const log: string[] = [];
    configure(p.receiverPage);
    const receiver = new LP2PReceiver({ nickname: "example-receiver" });
    const gotMessage = new Promise<void>((resolve) => {
      receiver.onconnection = (e) => {
        assert.ok(e instanceof LP2PConnectionEvent);
        const conn = e.connection;
        conn.ondatachannel = (e) => {
          assert.ok(e instanceof LP2PDataChannelEvent);
          const channel = e.channel;
          channel.onmessage = (e) => {
            log.push(`Receiver: ${e.data}`);
            resolve();
          };
          channel.send("Good day to you, requester!");
        };
      };
    });
    await receiver.start();

    configure(p.requesterPage);
    const request = new LP2PRequest({ nickname: "example-request" });
    const conn = await request.start();
    assert.ok(conn instanceof LP2PConnection);
    const channel = conn.createDataChannel("My Channel");
    assert.ok(channel instanceof LP2PDataChannel);
    assert.equal(channel.readyState, "connecting");
    const gotReply = new Promise<void>((resolve) => {
      channel.onopen = () => {
        channel.onmessage = (e) => {
          log.push(`Requester: ${e.data}`);
          resolve();
        };
        channel.send("Good day to you, receiver!");
      };
    });
    await Promise.all([gotMessage, gotReply]);
    assert.deepEqual(log.sort(), ["Receiver: Good day to you, receiver!", "Requester: Good day to you, requester!"]);
    conn.close();
    receiver.stop();
  } finally {
    closeAll(...p.transports);
  }
});

test("api: data types, binaryType, ordering behind Blobs, ids and protocol", async () => {
  const p = pages({ pairing: "none" });
  try {
    const { local, remote } = await connect(p);
    const incoming = once<LP2PDataChannelEvent>(remote, "datachannel");
    const ch = local.createDataChannel("data", { protocol: "test/1" });
    assert.equal(ch.id, 0);
    assert.equal(ch.protocol, "test/1");
    await once(ch, "open");
    const rch = (await incoming).channel;
    assert.equal(rch.label, "data");
    assert.equal(rch.protocol, "test/1");
    assert.equal(rch.id, 0);
    assert.equal(rch.binaryType, "arraybuffer");

    const received: unknown[] = [];
    const all = new Promise<void>((r) => {
      rch.onmessage = (e) => {
        received.push(e.data);
        if (received.length === 5) r();
      };
    });
    ch.send("text");
    ch.send(new Uint8Array([1, 2, 3]));
    ch.send(new Blob([new Uint8Array([4, 5])]));
    ch.send(new Uint16Array([0x0201]).buffer); // ArrayBuffer
    ch.send("after blob");
    await all;
    assert.equal(received[0], "text");
    assert.deepEqual(new Uint8Array(received[1] as ArrayBuffer), new Uint8Array([1, 2, 3]));
    assert.deepEqual(new Uint8Array(received[2] as ArrayBuffer), new Uint8Array([4, 5]));
    assert.deepEqual(new Uint8Array(received[3] as ArrayBuffer), new Uint8Array([1, 2]));
    assert.equal(received[4], "after blob");

    rch.binaryType = "blob";
    rch.binaryType = "bogus" as BinaryType; // ignored
    assert.equal(rch.binaryType, "blob");
    const blob = once<MessageEvent>(rch, "message");
    ch.send(new Uint8Array([9]));
    const data = (await blob).data as Blob;
    assert.ok(data instanceof Blob);
    assert.deepEqual(new Uint8Array(await data.arrayBuffer()), new Uint8Array([9]));

    // Receiver-created channels get odd ids; explicit ids are honored.
    const back = remote.createDataChannel("back");
    assert.equal(back.id, 1);
    const explicit = local.createDataChannel("explicit", { id: 42 });
    assert.equal(explicit.id, 42);
    assert.throws(() => local.createDataChannel("dup", { id: 42 }), { name: "OperationError" });
    assert.throws(() => local.createDataChannel("big", { id: 70000 }), TypeError);
    local.close();
  } finally {
    closeAll(...p.transports);
  }
});

test("api: closing a channel fires closing then close on both sides", async () => {
  const p = pages({ pairing: "none" });
  try {
    const { local, remote } = await connect(p);
    const incoming = once<LP2PDataChannelEvent>(remote, "datachannel");
    const ch = local.createDataChannel("c");
    await once(ch, "open");
    const rch = (await incoming).channel;
    const events: string[] = [];
    for (const [name, c] of [["local", ch], ["remote", rch]] as const) {
      c.onclosing = () => events.push(`${name} closing ${c.readyState}`);
      c.onclose = () => events.push(`${name} close ${c.readyState}`);
    }
    const done = Promise.all([once(ch, "close"), once(rch, "close")]);
    ch.close();
    assert.equal(ch.readyState, "closing");
    assert.throws(() => ch.send("x"), { name: "InvalidStateError" });
    await done;
    assert.deepEqual(events.filter((e) => e.startsWith("local")), ["local closing closing", "local close closed"]);
    assert.deepEqual(events.filter((e) => e.startsWith("remote")), ["remote closing closing", "remote close closed"]);

    // Closing the connection closes the peer's connection and channels.
    const ch2 = local.createDataChannel("c2");
    await once(ch2, "open");
    const peerClosed = once(remote, "close");
    const ch2Closed = once(ch2, "close");
    local.close();
    await Promise.all([peerClosed, ch2Closed]);
    assert.throws(() => remote.createDataChannel("late"), { name: "InvalidStateError" });
  } finally {
    closeAll(...p.transports);
  }
});

test("api: permission, picker and pairing failures reject like go-lp2p", async () => {
  // Advertising denied.
  {
    const p = pages({ receiver: { allow: (r) => r.permission !== "advertise" } });
    configure(p.receiverPage);
    await assert.rejects(new LP2PReceiver({ nickname: "x" }).start(), { name: "NotAllowedError" });
    closeAll(...p.transports);
  }
  // Connection declined by the receiver's user.
  {
    const p = pages({ receiver: { allow: (r) => r.permission === "advertise" } });
    configure(p.receiverPage);
    const receiver = new LP2PReceiver({ nickname: "x" });
    let fired = false;
    receiver.onconnection = () => (fired = true);
    await receiver.start();
    configure(p.requesterPage);
    await assert.rejects(new LP2PRequest().start(), { name: "NotAllowedError", message: /declined/ });
    assert.equal(fired, false);
    closeAll(...p.transports);
  }
  // Wrong PIN.
  {
    const p = pages({ requester: { enterPIN: () => "111-111-111" } });
    configure(p.receiverPage);
    await new LP2PReceiver({ nickname: "x" }).start();
    configure(p.requesterPage);
    await assert.rejects(new LP2PRequest().start(), { name: "NotAllowedError", message: /wrong PIN/ });
    closeAll(...p.transports);
  }
  // Picker dismissed.
  {
    const [t, unused] = transportPair();
    configure({
      transport: t,
      origin: "https://x.example",
      ui: Object.assign(new AutoUI(), { pickPeer: async () => null }),
    });
    await assert.rejects(new LP2PRequest().start(), { name: "NotAllowedError", message: /dismissed/ });
    closeAll(t, unused);
  }
});

test("api: picker sees peers come and go; start() is idempotent", async () => {
  const p = pages({ pairing: "none" });
  try {
    configure(p.receiverPage);
    const r1 = new LP2PReceiver({ nickname: "Bravo" });
    const r2 = new LP2PReceiver({ nickname: "Alpha" });
    assert.equal(r1.start(), r1.start());
    await Promise.all([r1.start(), r2.start()]);
    const seen: string[][] = [];
    let pickedFrom: string[] = [];
    configure({
      ...p.requesterPage,
      ui: new AutoUI({
        pick: (peers) => {
          seen.push(peers.map((x) => x.nickname));
          if (peers.length < 2) return undefined;
          pickedFrom = peers.map((x) => x.nickname);
          return peers[0];
        },
      }),
    });
    const connected = once<LP2PConnectionEvent>(r2, "connection");
    const conn = await new LP2PRequest().start();
    assert.deepEqual(pickedFrom, ["Alpha", "Bravo"]); // sorted
    await connected;
    conn.close();
    r1.stop();
    r2.stop();
  } finally {
    closeAll(...p.transports);
  }
});

test("api: interfaces behave like WebIDL ones", () => {
  assert.throws(() => new (LP2PConnection as unknown as new () => unknown)(), TypeError);
  const detached = new LP2PDataChannel("solo", { id: 7 });
  assert.equal(detached.label, "solo");
  assert.equal(detached.id, 7);
  assert.equal(detached.readyState, "closed");
  assert.throws(() => detached.send("x"), { name: "InvalidStateError" });
  assert.throws(() => new LP2PDataChannelEvent("datachannel", {} as never), TypeError);
  const ev = new LP2PDataChannelEvent("datachannel", { channel: detached });
  assert.equal(ev.channel, detached);

  // Event handler attributes: replacing keeps one listener; non-functions clear.
  let calls = 0;
  detached.onopen = () => calls++;
  detached.onopen = () => (calls += 10);
  detached.dispatchEvent(new Event("open"));
  assert.equal(calls, 10);
  detached.onopen = "nope" as never;
  assert.equal(detached.onopen, null);
  detached.dispatchEvent(new Event("open"));
  assert.equal(calls, 10);
});

test("install: defines missing globals only", () => {
  const g = globalThis as Record<string, unknown>;
  g["LP2PRequest"] = "native";
  try {
    const installed = install();
    assert.ok(installed.includes("LP2PReceiver"));
    assert.ok(!installed.includes("LP2PRequest"));
    assert.equal(g["LP2PRequest"], "native");
    assert.ok(install({ force: true }).includes("LP2PRequest"));
  } finally {
    for (const k of ["LP2PReceiver", "LP2PRequest", "LP2PConnection", "LP2PDataChannel", "LP2PConnectionEvent", "LP2PDataChannelEvent"]) {
      delete g[k];
    }
  }
});

