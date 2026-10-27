import { test } from "node:test";
import assert from "node:assert/strict";
import { Session, type DataChannelEndpoint, type SessionRequest } from "../src/protocol/session.ts";
import { AuthError, answerCeremony, sendAuthCapabilities, startCeremony } from "../src/protocol/auth.ts";
import { createIdentity, handshake, HandshakeError } from "../src/protocol/identity.ts";
import { Result, TypeKey, decodeMessage, encodeMessage, type AgentInfo } from "../src/protocol/messages.ts";
import type { TransportConnection } from "../src/transport/types.ts";
import { closeAll, delay, transportPair } from "./helpers.ts";

const info = (name: string): AgentInfo => ({
  displayName: name,
  modelName: "test",
  capabilities: [1100, 1200],
  stateToken: "x",
  locales: [],
});

async function sessions(): Promise<{ cli: Session; srv: Session; done: () => void }> {
  const [a, b] = transportPair();
  const incoming = new Promise<TransportConnection>((r) => a.listen({ id: "R", nickname: "R" }, r));
  await delay(10);
  const cli = new Session(await b.connect("R", { id: "Q", nickname: "Q" }), "client");
  const srv = new Session(await incoming, "server");
  return { cli, srv, done: () => closeAll(a, b) };
}

/** Handshake, agent-info and an accepted session request, as the user agent does it. */
async function accepted(): Promise<{ cli: Session; srv: Session; done: () => void }> {
  const s = await sessions();
  const [ic, is] = await Promise.all([createIdentity(), createIdentity()]);
  const [tc, ts] = await Promise.all([
    s.cli.controlChannel().then((c) => handshake(c, ic, "client")),
    s.srv.controlChannel().then((c) => handshake(c, is, "server")),
  ]);
  s.cli.tls = tc;
  s.srv.tls = ts;
  await Promise.all([s.cli.exchangeInfo(info("C")), s.srv.exchangeInfo(info("S"))]);
  const req = new Promise<SessionRequest>((r) => (s.srv.onsessionrequest = r));
  const result = s.cli.requestSession("https://app.example");
  void (await req).respond(Result.Success);
  assert.equal(await result, Result.Success);
  s.cli.markAccepted(); // the user agent does this once it verified the server
  assert.ok(s.cli.accepted && s.srv.accepted);
  return s;
}

test("session: the handshake proves both identities and agrees on a secret", async () => {
  const { cli, srv, done } = await sessions();
  try {
    const [ic, is] = await Promise.all([createIdentity(), createIdentity()]);
    const [tc, ts] = await Promise.all([
      cli.controlChannel().then((c) => handshake(c, ic, "client")),
      srv.controlChannel().then((c) => handshake(c, is, "server")),
    ]);
    assert.equal(tc.peerFingerprint, is.fingerprint);
    assert.equal(ts.peerFingerprint, ic.fingerprint);
    assert.deepEqual(tc.secret, ts.secret);
    assert.deepEqual(tc.transcriptHash, ts.transcriptHash);
    const [r1, r2] = await Promise.all([cli.exchangeInfo(info("Client")), srv.exchangeInfo(info("Server"))]);
    assert.equal(r1.displayName, "Server");
    assert.equal(r2.displayName, "Client");
  } finally {
    cli.close();
    done();
  }
});

test("session: a forged signature fails the handshake", async () => {
  const { cli, srv, done } = await sessions();
  try {
    // The server side claims someone else's key but can't sign for it.
    const [ic, victim, impostor] = await Promise.all([createIdentity(), createIdentity(), createIdentity()]);
    const client = handshake(await cli.controlChannel(), ic, "client");
    const sc = await srv.controlChannel();
    await sc.next([TypeKey.ShimHello]);
    const eph = await crypto.subtle.generateKey({ name: "ECDH", namedCurve: "P-256" }, true, ["deriveBits"]);
    const raw = new Uint8Array(await crypto.subtle.exportKey("raw", eph.publicKey));
    sc.send({ type: TypeKey.ShimHello, signingKey: victim.spki, exchangeKey: raw, nonce: new Uint8Array(32) });
    const sig = new Uint8Array(
      await crypto.subtle.sign({ name: "ECDSA", hash: "SHA-256" }, impostor.keys.privateKey, new Uint8Array(32)),
    );
    sc.send({ type: TypeKey.ShimFinished, signature: sig });
    await assert.rejects(client, HandshakeError);
  } finally {
    cli.close();
    done();
  }
});

test("session: lp2p-session-request carries the origin; refusal and a second request close", async () => {
  {
    const { cli, srv, done } = await sessions();
    try {
      const req = new Promise<SessionRequest>((r) => (srv.onsessionrequest = r));
      const result = cli.requestSession("https://app.example");
      const r = await req;
      assert.equal(r.requestingOrigin, "https://app.example");
      r.respond(Result.PermanentError);
      assert.equal(await result, Result.PermanentError);
      assert.equal(cli.accepted, false);
    } finally {
      cli.close();
      done();
    }
  }
  {
    const { cli, srv, done } = await sessions();
    try {
      const closed = new Promise<string>((r) => srv.whenClosed(r));
      srv.onsessionrequest = () => {};
      cli.requestSession("https://a.example").catch(() => {});
      cli.requestSession("https://b.example").catch(() => {});
      assert.match(await closed, /second lp2p-session-request|closed/);
    } finally {
      done();
    }
  }
});

test("session: no extension messages before the session is accepted", async () => {
  const { cli, srv, done } = await sessions();
  try {
    let channels = 0;
    srv.ondatachannel = () => channels++;
    // A client that opens a channel too early: the server closes the stream.
    const s = cli.connection.openStream();
    const closed = new Promise<void>((r) => (s.onclose = r));
    s.send(encodeMessage({ type: TypeKey.DataChannelOpenRequest, requestId: 1, channelId: 0, label: "x", protocol: "" }));
    await closed;
    assert.equal(channels, 0);
  } finally {
    cli.close();
    done();
  }
});

test("session: the client starts the ceremony; right PIN, wrong PIN, pairing none", async () => {
  for (const mode of ["right", "wrong", "none"] as const) {
    const { cli, srv, done } = await sessions();
    try {
      const parties = { client: "C", server: "S" };
      // The server knows the control stream from the client's first message on it.
      await Promise.all([cli.exchangeInfo(info("C")), srv.exchangeInfo(info("S"))]);
      const [cc, sc] = await Promise.all([cli.controlChannel(), srv.controlChannel()]);
      let shown = "";
      sendAuthCapabilities(sc);
      await cc.next([TypeKey.AuthCapabilities]);
      const server = answerCeremony(sc, parties, mode === "none" ? null : (pin) => (shown = pin));
      const client = startCeremony(cc, parties, async () => (mode === "right" ? shown : "000-000-000"));
      if (mode === "wrong") {
        await assert.rejects(server, AuthError);
        await assert.rejects(client, (e: AuthError) => e.result === 5);
      } else {
        await Promise.all([server, client]);
        if (mode === "right") assert.match(shown, /^\d{3}-\d{3}-\d{3}$/);
        else assert.equal(shown, "");
      }
    } finally {
      cli.close();
      done();
    }
  }
});

test("session: data channels open with 1101/1102 and carry data-frames", async () => {
  const { cli, srv, done } = await accepted();
  try {
    const incoming = new Promise<DataChannelEndpoint>((r) => (srv.ondatachannel = r));
    const id = cli.reserveChannelId();
    assert.equal(id, 0); // client: even
    const ep = cli.openDataChannel({ id, label: "chat", protocol: "p" });
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

    // Server-allocated IDs are odd; explicit IDs must be free and of the
    // local parity.
    assert.equal(srv.reserveChannelId(), 1);
    assert.throws(() => cli.reserveChannelId(0), { name: "OperationError" });
    assert.throws(() => cli.reserveChannelId(3), { name: "OperationError" });
    assert.equal(cli.reserveChannelId(4), 4);
    assert.equal(cli.reserveChannelId(), 2); // lowest free even ID
    cli.releaseChannelId(2);
    cli.releaseChannelId(4);

    // Closing on one side closes the other.
    const closed = new Promise<void>((r) => (remote.onclose = r));
    ep.close();
    await closed;
    assert.equal(remote.state, "closed");
    assert.equal(cli.reserveChannelId(0), 0); // released
  } finally {
    cli.close();
    done();
  }
});

test("session: a channel ID the peer already uses is refused", async () => {
  const { cli, srv, done } = await accepted();
  try {
    assert.equal(srv.reserveChannelId(), 1);
    const ep = cli.openDataChannel({ id: 1, label: "x", protocol: "" });
    const err = new Promise<Error>((r) => (ep.onerror = r));
    const closed = new Promise<void>((r) => (ep.onclose = r));
    assert.match((await err).message, /refused/);
    await closed;
  } finally {
    cli.close();
    done();
  }
});

test("session: first message on a data channel stream is the open request", async () => {
  const { cli, srv, done } = await accepted();
  try {
    const raw: number[] = [];
    srv.connection.onstream = (s) => (s.onmessage = (b) => raw.push(decodeMessage(b).type));
    cli.openDataChannel({ id: 2, label: "a", protocol: "" });
    await delay(20);
    assert.deepEqual(raw, [TypeKey.DataChannelOpenRequest]);
  } finally {
    cli.close();
    done();
  }
});

export { accepted, info };
