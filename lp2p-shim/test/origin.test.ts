// The requesting origin, allowedOrigins and the local state: trust anchors
// skip the ceremony, peer grants skip the prompt.

import { test } from "node:test";
import assert from "node:assert/strict";
import { LP2PDataChannelEvent, LP2PReceiver, LP2PRequest, LocalState, configure, type ShimConfig } from "../src/index.ts";
import type { PostMessageTransport } from "../src/transport/postmessage.ts";
import { AutoUI, type PermissionRequest } from "../src/ua/ui.ts";
import { closeAll, once, transportPair } from "./helpers.ts";

test("allowedOrigins: TypeError on an unparsable entry; opaque origins never match", async () => {
  assert.throws(() => new LP2PReceiver({ allowedOrigins: ["not a url"] }), TypeError);
  assert.throws(() => new LP2PReceiver({ allowedOrigins: ["https://ok.example", "//relative"] }), TypeError);
  new LP2PReceiver({ allowedOrigins: ["https://ok.example/path?q", "data:text/plain,x"] });

  // A requesting origin that parses to an opaque origin is refused even if
  // allowedOrigins holds one ("null" == "null" must not match).
  for (const [requester, allowed, ok] of [
    ["data:text/plain,x", ["data:text/plain,x"], false],
    ["https://b.example", ["https://b.example/some/path"], true],
    ["https://b.example:8443", ["https://b.example"], false],
  ] as const) {
    const [ta, tb] = transportPair();
    try {
      configure({ transport: ta, ui: new AutoUI(), pairing: "none", origin: "https://a.example" });
      const receiver = new LP2PReceiver({ nickname: "R", allowedOrigins: [...allowed] });
      await receiver.start();
      configure({ transport: tb, ui: new AutoUI(), pairing: "none", origin: requester });
      const started = new LP2PRequest().start();
      if (ok) (await started).close();
      else await assert.rejects(started, { name: "NotAllowedError" });
      receiver.stop();
    } finally {
      closeAll(ta, tb);
    }
  }
});

test("opaque page origins can't start receivers or requests", async () => {
  const [ta, tb] = transportPair();
  try {
    configure({ transport: ta, ui: new AutoUI(), origin: "null" });
    await assert.rejects(new LP2PReceiver().start(), { name: "NotAllowedError" });
    await assert.rejects(new LP2PRequest().start(), { name: "NotAllowedError" });
  } finally {
    closeAll(ta, tb);
  }
});

function pinPages(): { a: ShimConfig; b: ShimConfig; prompts: string[]; pins: string[]; transports: PostMessageTransport[] } {
  const [ta, tb] = transportPair();
  const prompts: string[] = [];
  const pins: string[] = [];
  let shown: ((pin: string) => void) | null = null;
  let next = new Promise<string>((r) => (shown = r));
  const a: ShimConfig = {
    transport: ta,
    pairing: "pin",
    origin: "https://app.example",
    ui: new AutoUI({
      allow: (r: PermissionRequest) => (prompts.push(r.permission), true),
      onPIN: (p) => (pins.push(p.pin), shown?.(p.pin)),
    }),
  };
  const b: ShimConfig = {
    transport: tb,
    pairing: "pin",
    origin: "https://app.example",
    ui: new AutoUI({
      enterPIN: async () => {
        const pin = await next;
        next = new Promise<string>((r) => (shown = r));
        return pin;
      },
    }),
  };
  return { a, b, prompts, pins, transports: [ta, tb] };
}

test("local state: the first session runs the ceremony and prompts; later ones need neither", async () => {
  const p = pinPages();
  try {
    configure(p.a);
    const receiver = new LP2PReceiver({ nickname: "R" });
    await receiver.start();
    configure(p.b);
    for (let i = 0; i < 3; i++) {
      const incoming = once(receiver, "connection");
      const conn = await new LP2PRequest({ nickname: `Q${i}` }).start();
      await incoming;
      conn.close();
    }
    // One PIN, one grant prompt, three sessions.
    assert.equal(p.pins.length, 1);
    assert.deepEqual(p.prompts, ["advertise", "accept-connection"]);
    receiver.stop();
  } finally {
    closeAll(...p.transports);
  }
});

test("local state: removing the trust anchor removes the grant; the ceremony runs again", async () => {
  const p = pinPages();
  const stateA = new LocalState();
  const stateB = new LocalState();
  try {
    configure({ ...p.a, localState: stateA });
    const receiver = new LP2PReceiver({ nickname: "R" });
    await receiver.start();
    configure({ ...p.b, localState: stateB });
    (await new LP2PRequest().start()).close();
    const clientId = (await stateB.identity()).fingerprint;
    assert.ok(stateA.isAuthenticated(clientId));
    assert.ok(stateA.hasGrant("https://app.example", clientId));
    assert.equal(stateA.knownPeers.get(clientId)?.displayName, "lp2p-shim device");

    const anchor = [...stateA.anchors.values()][0];
    assert.ok(anchor);
    stateA.removeAnchor(anchor);
    assert.equal(stateA.isAuthenticated(clientId), false);
    assert.equal(stateA.hasGrant("https://app.example", clientId), false);

    // The client still trusts the server, but the server asks for a
    // ceremony again (auth-capabilities), and prompts again.
    (await new LP2PRequest().start()).close();
    assert.equal(p.pins.length, 2);
    assert.deepEqual(p.prompts, ["advertise", "accept-connection", "accept-connection"]);

    // Removing a known peer removes its grants; clearing an origin too.
    stateA.forgetPeer(clientId);
    assert.equal(stateA.hasGrant("https://app.example", clientId), false);
    stateB.clearOrigin("https://app.example");
    assert.equal(stateB.grants.has("https://app.example"), false);
    receiver.stop();
  } finally {
    closeAll(...p.transports);
  }
});

test("local state: a server that trusts the client accepts without auth-capabilities; the client then starts the ceremony itself", async () => {
  const p = pinPages();
  const stateA = new LocalState();
  const stateB = new LocalState();
  try {
    configure({ ...p.a, localState: stateA });
    const receiver = new LP2PReceiver({ nickname: "R" });
    // The receiver opens a data channel as soon as it has the connection,
    // which is right after its success response: the client is still in
    // the ceremony then, and holds the stream until it accepts.
    receiver.onconnection = (e) => {
      try {
        e.connection.createDataChannel("early");
      } catch {
        // the first client closes at once
      }
    };
    await receiver.start();
    configure({ ...p.b, localState: stateB });
    (await new LP2PRequest().start()).close();
    assert.equal(p.pins.length, 1);
    // The client forgets the server; the server still trusts the client.
    for (const a of [...stateB.anchors.values()]) stateB.removeAnchor(a);
    const conn = await new LP2PRequest().start();
    const early = await once<LP2PDataChannelEvent>(conn, "datachannel");
    assert.equal(early.channel.label, "early");
    conn.close();
    // Success arrived while the client had not verified the server: it ran
    // the ceremony, so the server showed a PIN again, without a new prompt.
    assert.equal(p.pins.length, 2);
    assert.deepEqual(p.prompts, ["advertise", "accept-connection"]);
    receiver.stop();
  } finally {
    closeAll(...p.transports);
  }
});
