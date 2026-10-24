import { test } from "node:test";
import assert from "node:assert/strict";
import { Session, type DataChannelEndpoint } from "../src/protocol/session.ts";
import { AuthError, awaitAuthentication, presentPIN } from "../src/protocol/auth.ts";
import { TypeKey, decodeMessage, type AgentInfo } from "../src/protocol/messages.ts";
import type { TransportConnection } from "../src/transport/types.ts";
import { closeAll, delay, transportPair } from "./helpers.ts";

const info = (name: string): AgentInfo => ({
  displayName: name,
  modelName: "test",
  capabilities: [1100],
  stateToken: "x",
  locales: [],
});

async function sessions(): Promise<{ req: Session; rcv: Session; done: () => void }> {
  const [a, b] = transportPair();
  const incoming = new Promise<TransportConnection>((r) => a.listen({ id: "R", nickname: "R" }, r));
  await delay(10);
  const req = new Session(await b.connect("R", { id: "Q", nickname: "Q" }), "requester");
  const rcv = new Session(await incoming, "receiver");
  return { req, rcv, done: () => closeAll(a, b) };
}

test("session: agent-info handshake in both directions", async () => {
  const { req, rcv, done } = await sessions();
  try {
    const [r1, r2] = await Promise.all([req.handshake(info("Requester")), rcv.handshake(info("Receiver"))]);
    assert.equal(r1.displayName, "Receiver");
    assert.equal(r2.displayName, "Requester");
    assert.deepEqual(r1.capabilities, [1100]);
  } finally {
    req.close();
    done();
  }
});

test("session: toy PIN pairing accepts the right PIN and rejects a wrong one", async () => {
  for (const right of [true, false]) {
    const { req, rcv, done } = await sessions();
    try {
      await Promise.all([req.handshake(info("Q")), rcv.handshake(info("R"))]);
      let shown = "";
      const presenter = presentPIN(await rcv.controlChannel(), (pin) => (shown = pin));
      const consumer = awaitAuthentication(await req.controlChannel(), async () => (right ? shown : "000-000-000"));
      if (right) {
        await Promise.all([presenter, consumer]);
        assert.match(shown, /^\d{3}-\d{3}-\d{3}$/);
      } else {
        await assert.rejects(presenter, AuthError);
        await assert.rejects(consumer, (e: AuthError) => e.result === 5);
      }
    } finally {
      req.close();
      done();
    }
  }
});

test("session: data channels open with 1101/1102 and carry data-frames", async () => {
  const { req, rcv, done } = await sessions();
  try {
    const incoming = new Promise<DataChannelEndpoint>((r) => (rcv.ondatachannel = r));
    const id = req.reserveChannelId();
    assert.equal(id, 0); // requester: even
    const ep = req.openDataChannel({ id, label: "chat", protocol: "p" });
    assert.equal(ep.state, "connecting");
    assert.throws(() => ep.send(1, new Uint8Array()), { name: "InvalidStateError" });
    const opened = new Promise<void>((r) => (ep.onopen = r));
    const remote = await incoming;
    assert.deepEqual(remote.params, { id: 0, label: "chat", protocol: "p" });
    assert.equal(remote.state, "open");
    await opened;

    const got = new Promise<[number, string]>((r) => (remote.onframe = (e, p) => r([e, new TextDecoder().decode(p)])));
    ep.send(1, new TextEncoder().encode("hello"));
    assert.deepEqual(await got, [1, "hello"]);

    // Receiver-allocated IDs are odd; explicit IDs must be free.
    assert.equal(rcv.reserveChannelId(), 1);
    assert.throws(() => req.reserveChannelId(0), { name: "OperationError" });

    // Closing on one side closes the other.
    const closed = new Promise<void>((r) => (remote.onclose = r));
    ep.close();
    await closed;
    assert.equal(remote.state, "closed");
    assert.equal(req.reserveChannelId(0), 0); // released
  } finally {
    req.close();
    done();
  }
});

test("session: a channel ID the peer already uses is refused", async () => {
  const { req, rcv, done } = await sessions();
  try {
    rcv.reserveChannelId(4);
    const ep = req.openDataChannel({ id: req.reserveChannelId(4), label: "x", protocol: "" });
    const err = new Promise<Error>((r) => (ep.onerror = r));
    const closed = new Promise<void>((r) => (ep.onclose = r));
    assert.match((await err).message, /refused/);
    await closed;
  } finally {
    req.close();
    done();
  }
});

test("session: first message on a data channel stream is the open request", async () => {
  const { req, rcv, done } = await sessions();
  try {
    const raw: number[] = [];
    rcv.connection.onstream = (s) => (s.onmessage = (b) => raw.push(decodeMessage(b).type));
    req.openDataChannel({ id: 2, label: "a", protocol: "" });
    await delay(20);
    assert.deepEqual(raw, [TypeKey.DataChannelOpenRequest]);
  } finally {
    req.close();
    done();
  }
});
