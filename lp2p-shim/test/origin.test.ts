// The requesting origin, allowedOrigins and the local state: trust anchors
// skip the ceremony, peer grants skip the prompt.

import { test } from "node:test";
import assert from "node:assert/strict";
import { LP2PDataChannelEvent, LP2PReceiver, LP2PRequest, LocalState, configure, lp2p, type ShimConfig } from "../src/index.ts";
import { SessionResult, TypeKey, decodeMessage } from "../src/protocol/messages.ts";
import type { Transport, TransportStream } from "../src/transport/types.ts";
import type { PostMessageTransport } from "../src/transport/postmessage.ts";
import { AutoUI, type PermissionRequest } from "../src/ua/ui.ts";
import { closeAll, delay, once, transportPair } from "./helpers.ts";

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

test("local state: the server trusts the client but the client forgot the server; an early data channel is held", async () => {
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

// ------------------------------------------------- the wire, as the client sees it

/** Wraps a transport so every OSP message on the client's connection is logged as "send:type" / "recv:type[:result]". */
function tapped(t: PostMessageTransport, log: string[]): Transport {
  const name = (b: Uint8Array): string | null => {
    try {
      const m = decodeMessage(b);
      if (m.type === TypeKey.SessionResponse) return `${m.type}:${m.result}`;
      if (m.type === TypeKey.AuthSpake2Handshake) return `${m.type}:${m.pskStatus}:${m.publicValue ? "pv" : "nopv"}`;
      return String(m.type);
    } catch {
      return null;
    }
  };
  const wrapStream = (s: TransportStream): TransportStream => {
    const send = s.send.bind(s);
    s.send = (b) => {
      const n = name(b);
      if (n) log.push(`send:${n}`);
      send(b);
    };
    const proto = Object.getPrototypeOf(s) as object;
    const desc = Object.getOwnPropertyDescriptor(Object.getPrototypeOf(proto), "onmessage") as PropertyDescriptor;
    Object.defineProperty(s, "onmessage", {
      get: () => desc.get?.call(s),
      set: (fn: ((b: Uint8Array) => void) | null) =>
        desc.set?.call(
          s,
          fn &&
            ((b: Uint8Array) => {
              const n = name(b);
              if (n) log.push(`recv:${n}`);
              fn(b);
            }),
        ),
    });
    return s;
  };
  return {
    name: t.name,
    listen: t.listen.bind(t),
    discover: t.discover.bind(t),
    async connect(id, local, opts) {
      const c = await t.connect(id, local, opts);
      const open = c.openStream.bind(c);
      c.openStream = (o) => wrapStream(open(o));
      const desc = Object.getOwnPropertyDescriptor(Object.getPrototypeOf(Object.getPrototypeOf(c)), "onstream") as PropertyDescriptor;
      Object.defineProperty(c, "onstream", {
        get: () => desc.get?.call(c),
        set: (fn: ((s: TransportStream) => void) | null) => desc.set?.call(c, fn && ((s: TransportStream) => fn(wrapStream(s)))),
      });
      return c;
    },
  };
}

const REQ = `send:${TypeKey.SessionRequest}`;
const AUTHED = `recv:${TypeKey.AuthStatus}`;
const CUE = `recv:${TypeKey.SessionResponse}:${SessionResult.AuthenticationRequired}`;
const SUCCESS = `recv:${TypeKey.SessionResponse}:1`;
const REFUSED = `recv:${TypeKey.SessionResponse}:102`;

test("wire: neither side knows the other - the origin is sent only after the server is verified", async () => {
  const p = pinPages();
  const log: string[] = [];
  try {
    configure(p.a);
    const receiver = new LP2PReceiver({ nickname: "R" });
    await receiver.start();
    configure({ ...p.b, transport: tapped(p.transports[1] as PostMessageTransport, log) });
    (await new LP2PRequest().start()).close();
    assert.ok(log.includes(AUTHED) && log.includes(REQ));
    assert.ok(log.indexOf(AUTHED) < log.indexOf(REQ), log.join(" "));
    assert.ok(!log.includes(CUE)); // the ceremony already verified the client too
    // The client, the PSK consumer, asks for presentation without a
    // public-value; the server presents (psk-shown) and the client answers.
    const hs = log.filter((x) => x.includes(`:${TypeKey.AuthSpake2Handshake}:`));
    assert.deepEqual(hs, [
      `send:${TypeKey.AuthSpake2Handshake}:0:nopv`,
      `recv:${TypeKey.AuthSpake2Handshake}:1:pv`,
      `send:${TypeKey.AuthSpake2Handshake}:2:pv`,
    ]);
    assert.ok(log.includes(SUCCESS));
    assert.equal(p.pins.length, 1);
    receiver.stop();
  } finally {
    closeAll(...p.transports);
  }
});

test("wire: no ceremony and no cue on repeat sessions; the server forgot the client - the cue path", async () => {
  const p = pinPages();
  const stateA = new LocalState();
  const log: string[] = [];
  try {
    configure({ ...p.a, localState: stateA });
    const receiver = new LP2PReceiver({ nickname: "R" });
    await receiver.start();
    const client = { ...p.b, transport: tapped(p.transports[1] as PostMessageTransport, log), localState: new LocalState() };
    configure(client);
    (await new LP2PRequest().start()).close();
    log.length = 0;
    (await new LP2PRequest().start()).close();
    // Both trust each other: the request goes out at once and success comes back.
    assert.deepEqual(
      log.filter((x) => x === REQ || x.startsWith(`recv:${TypeKey.SessionResponse}`) || x === AUTHED),
      [REQ, SUCCESS],
    );
    assert.equal(p.pins.length, 1);

    // The server forgets the client: it answers the request with the cue,
    // the client runs the ceremony, then gets success.
    for (const a of [...stateA.anchors.values()]) stateA.removeAnchor(a);
    log.length = 0;
    (await new LP2PRequest().start()).close();
    const seq = log.filter((x) => x === REQ || x === CUE || x === AUTHED || x === SUCCESS);
    assert.deepEqual(seq, [REQ, CUE, AUTHED, SUCCESS]);
    assert.equal(p.pins.length, 2);
    receiver.stop();
  } finally {
    closeAll(...p.transports);
  }
});

test("wire: an unverified client never learns whether its origin is accepted", async () => {
  const [ta, tb] = transportPair();
  const log: string[] = [];
  const stateB = new LocalState();
  try {
    // The receiver doesn't allow the client's origin, and the client trusts
    // the server from an earlier session but types a wrong PIN on the cue.
    let pin: (p: string) => void = () => {};
    const shown = new Promise<string>((r) => (pin = r));
    // configure() merges, so name the local state of each device.
    const stateA = new LocalState();
    configure({ transport: ta, pairing: "pin", origin: "https://a.example", localState: stateA, ui: new AutoUI({ onPIN: (x) => pin(x.pin) }) });
    const receiver = new LP2PReceiver({ nickname: "R", allowedOrigins: ["https://b.example"] });
    await receiver.start();
    configure({
      transport: tb,
      pairing: "pin",
      origin: "https://b.example",
      localState: stateB,
      ui: new AutoUI({ enterPIN: () => shown }),
    });
    (await new LP2PRequest().start()).close();
    // Now the server forgets the client and the client's page origin changes.
    assert.ok(stateA.anchors.size > 0);
    for (const a of [...stateA.anchors.values()]) stateA.removeAnchor(a);
    configure({
      transport: tapped(tb, log),
      pairing: "pin",
      origin: "https://c.example",
      localState: stateB,
      ui: new AutoUI({ enterPIN: () => "000-000-000" }),
    });
    await assert.rejects(new LP2PRequest().start(), { name: "NotAllowedError" });
    assert.ok(log.includes(CUE), log.join(" "));
    assert.ok(!log.includes(SUCCESS) && !log.includes(REFUSED), log.join(" "));
    receiver.stop();
  } finally {
    closeAll(ta, tb);
  }
});

test("abort: during the picker, during the session setup, and after success", async () => {
  const [ta, tb] = transportPair();
  try {
    let lastPIN = "";
    const stateB = new LocalState();
    configure({ transport: ta, localState: new LocalState(), ui: new AutoUI({ onPIN: (x) => (lastPIN = x.pin) }), pairing: "pin", origin: "https://app.example" });
    const receiver = new LP2PReceiver({ nickname: "R" });
    await receiver.start();

    // During the picker: the picker is taken down.
    let pickerSignal: AbortSignal | null = null;
    configure({
      transport: tb,
      localState: stateB,
      origin: "https://app.example",
      ui: Object.assign(new AutoUI(), {
        pickPeer: (_p: unknown, signal: AbortSignal) => {
          pickerSignal = signal;
          return new Promise<null>((r) => signal.addEventListener("abort", () => r(null)));
        },
      }),
    });
    const ac1 = new AbortController();
    const request = new LP2PRequest();
    const p1 = request.start({ signal: ac1.signal });
    // A second start() while pending returns the same promise.
    assert.equal(request.start(), p1);
    await delay(20);
    ac1.abort(new DOMException("stop", "AbortError"));
    await assert.rejects(p1, { name: "AbortError", message: "stop" });
    assert.equal((pickerSignal as AbortSignal | null)?.aborted, true);
    // An aborted signal rejects at once.
    await assert.rejects(new LP2PRequest().start({ signal: AbortSignal.abort() }), { name: "AbortError" });

    // During the session setup: while the user is asked for the PIN.
    const conns: string[] = [];
    receiver.onconnection = () => conns.push("connection");
    const ac2 = new AbortController();
    let pinAsked!: () => void;
    const asked = new Promise<void>((r) => (pinAsked = r));
    configure({
      transport: tb,
      localState: stateB,
      origin: "https://app.example",
      ui: new AutoUI({
        enterPIN: (_p, signal) => {
          pinAsked();
          return new Promise<string | null>((r) => signal.addEventListener("abort", () => r(null)));
        },
      }),
    });
    const p2 = lp2p.connect({ signal: ac2.signal });
    await asked;
    ac2.abort();
    await assert.rejects(p2, { name: "AbortError" });
    await delay(50);
    assert.deepEqual(conns, []);

    // After success: nothing happens.
    configure({
      transport: tb,
      localState: stateB,
      origin: "https://app.example",
      ui: new AutoUI({ enterPIN: async () => (await delay(10), lastPIN) }),
    });
    const ac3 = new AbortController();
    const conn = await new LP2PRequest().start({ signal: ac3.signal });
    let closed = false;
    conn.onclose = () => (closed = true);
    ac3.abort();
    await delay(30);
    assert.equal(closed, false);
    conn.close();
    receiver.stop();
  } finally {
    closeAll(ta, tb);
  }
});
