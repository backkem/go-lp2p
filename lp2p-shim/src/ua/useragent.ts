// The user agent: the policy between the web API and the session layer.
// It asks the user for permission, runs the device picker and the trust
// ceremony, keeps the local state, and hands the API layer accepted
// sessions. Mirrors go-lp2p's lp2p-api/useragent.
//
// A session is established as the spec's OSP binding says ("Session
// extension"), so that the requesting origin only reaches a verified server
// and the answer only a verified client: after the connection's handshake
// both agents send auth-capabilities and agent-info. A client that hasn't
// verified the server runs the ceremony first. Only then does it send the
// requesting origin in lp2p-session-request. A server that hasn't verified
// the client answers a provisional authentication-required without looking
// at the origin, and the client runs the ceremony. Once the client is
// verified, the server picks a receiver that allows the origin, asks its
// user to grant the client (unless granted before) and answers success or
// permanent-error. Peers that trust each other run no ceremony.

import { AgentCapability, Result, type AgentInfo } from "../protocol/messages.ts";
import { AuthError, answerCeremony, sendAuthCapabilities, startCeremony, type Parties } from "../protocol/auth.ts";
import { handshake } from "../protocol/identity.ts";
import { Session, type SessionRequest } from "../protocol/session.ts";
import { randomId } from "../transport/base.ts";
import type { Closer, DiscoveryEvent, Transport, TransportConnection } from "../transport/types.ts";
import { localStateFor, parseOrigin, type LocalState, type ReceiverRegistration } from "./localstate.ts";
import type { PeerInfo, PeerPicker, UserAgentUI } from "./ui.ts";

export type Pairing = "pin" | "none";

export interface UserAgentConfig {
  transport: Transport;
  ui: UserAgentUI;
  /** "pin": the server shows a PIN the client must enter (toy, see protocol/auth.ts). Default "pin". */
  pairing?: Pairing;
  /** The page's origin: the requesting origin of its sessions, and shown in prompts. Default: location.origin. */
  origin?: string;
  /** agent-info display-name: the device's name, not the page's nickname. Default "lp2p-shim device". */
  deviceName?: string;
  /** agent-info model-name. Default "lp2p-shim". */
  modelName?: string;
  /** How long the handshake, agent-info and the session request may take, in ms. Default 10000. */
  handshakeTimeout?: number;
  /** The device's trust store, known peers and grants. Default: one per transport object. */
  localState?: LocalState;
  /** Diagnostics. */
  log?: (line: string) => void;
}

/** An accepted session plus what the user agent knows about the peer. */
export interface GrantedSession {
  session: Session;
  peer: PeerInfo;
}

/** A receiver as the page side registers it with advertise(). */
export interface ReceiverHandle extends ReceiverRegistration {
  readonly nickname: string;
}

class LivePicker implements PeerPicker {
  private readonly map = new Map<string, PeerInfo>();
  private sorted: PeerInfo[] = [];
  onchange: (() => void) | null = null;

  readonly origin: string;
  readonly nickname: string;

  constructor(origin: string, nickname: string) {
    this.origin = origin;
    this.nickname = nickname;
  }

  get peers(): readonly PeerInfo[] {
    return this.sorted;
  }

  update(e: DiscoveryEvent): void {
    if (e.kind === "removed") this.map.delete(e.peer.id);
    else this.map.set(e.peer.id, { id: e.peer.id, nickname: e.peer.nickname, verified: false });
    this.sorted = [...this.map.values()].sort(
      (a, b) => a.nickname.localeCompare(b.nickname) || a.id.localeCompare(b.id),
    );
    this.onchange?.();
  }
}

function notAllowed(message: string): DOMException {
  return new DOMException(message, "NotAllowedError");
}

export class UserAgent {
  readonly transport: Transport;
  readonly ui: UserAgentUI;
  readonly pairing: Pairing;
  readonly origin: string;
  readonly deviceName: string;
  readonly local: LocalState;
  private readonly modelName: string;
  private readonly handshakeTimeout: number;
  private readonly log: (line: string) => void;

  constructor(config: UserAgentConfig) {
    this.transport = config.transport;
    this.ui = config.ui;
    this.pairing = config.pairing ?? "pin";
    this.origin = config.origin ?? globalThis.location?.origin ?? "null";
    this.deviceName = config.deviceName ?? "lp2p-shim device";
    this.modelName = config.modelName ?? "lp2p-shim";
    this.handshakeTimeout = config.handshakeTimeout ?? 10_000;
    this.local = config.localState ?? localStateFor(config.transport);
    this.log = config.log ?? (() => {});
  }

  /**
   * LP2PReceiver.start(): asks to advertise as nickname and, if allowed,
   * advertises until closed. Accepted sessions go to a receiver of this
   * device that allows their requesting origin, preferring receiver.
   */
  async advertise(receiver: ReceiverHandle): Promise<Closer> {
    const allowed = await this.ui.requestPermission(
      { permission: "advertise", origin: receiver.origin, nickname: receiver.nickname },
      new AbortController().signal,
    );
    if (!allowed) throw notAllowed("The user did not allow advertising.");
    const id = randomId();
    this.log(`advertising "${receiver.nickname}" as ${id} over ${this.transport.name}`);
    this.local.receivers.add(receiver);
    const ad = this.transport.listen({ id, nickname: receiver.nickname }, (conn) => {
      this.admit(conn, receiver).catch((err: unknown) =>
        this.log(`incoming connection from ${conn.remoteId} failed: ${String(err)}`),
      );
    });
    return {
      close: () => {
        this.local.receivers.delete(receiver);
        ad.close();
      },
    };
  }

  /** The server side of a new connection. */
  private async admit(conn: TransportConnection, own: ReceiverHandle): Promise<void> {
    const session = new Session(conn, "server");
    const ac = new AbortController();
    session.onclose = (reason) => ac.abort(new DOMException(`connection closed: ${reason}`, "AbortError"));
    const refuse = (req: SessionRequest, why: string): void => {
      this.log(`refused session: ${why}`);
      // Close once the client ended its side of the stream, or after a timeout.
      void req.respond(Result.PermanentError).then(() => session.close(`refused: ${why}`));
    };
    try {
      const identity = await this.local.identity();
      const request = new Promise<SessionRequest>((resolve, reject) => {
        session.onsessionrequest = resolve;
        ac.signal.addEventListener("abort", () => reject(ac.signal.reason), { once: true });
      });
      request.catch(() => {});
      const { control, tls, remote } = await this.withTimeout(
        (async () => {
          const control = await session.controlChannel(ac.signal);
          const tls = await handshake(control, identity, "server", ac.signal);
          session.tls = tls;
          // As in OSP; it starts nothing.
          sendAuthCapabilities(control);
          const remote = await session.exchangeInfo(this.agentInfo(), ac.signal);
          return { control, tls, remote };
        })(),
        ac,
      );
      const client = tls.peerFingerprint;
      const parties: Parties = { client, server: identity.fingerprint };
      const peer: PeerInfo = { id: conn.remoteId, nickname: "", displayName: remote.displayName, verified: false };

      // Answer the ceremony whenever the client starts it, before or after
      // its session request; one that fails closes the connection.
      let verified = this.local.isAuthenticated(client);
      // The PIN is shown until the ceremony ends, or the connection does.
      const shown = new AbortController();
      session.whenClosed(() => shown.abort());
      const ceremony = answerCeremony(control, parties, this.pairing === "pin" ? this.pinPresenter(own, peer, shown.signal) : null, ac.signal).finally(() => shown.abort()).then(
        () => {
          this.local.addSelfAnchor(client);
          verified = true;
        },
        (err: unknown) => {
          if (err instanceof AuthError) session.close("authentication failed");
          throw err;
        },
      );
      ceremony.catch(() => {});

      const req = await request;
      if (!verified) {
        // Without looking at the origin.
        req.requireAuthentication();
        await ceremony;
      }
      peer.verified = true;

      const origin = parseOrigin(req.requestingOrigin) ?? "null";
      const receiver = this.receiverFor(origin, own);
      if (!receiver) {
        refuse(req, `no receiver allows ${origin}`);
        return;
      }
      this.local.rememberPeer({ identity: client, anchorKey: client, displayName: remote.displayName });
      if (!this.local.hasGrant(receiver.origin, client)) {
        const ok = await this.ui.requestPermission(
          { permission: "accept-connection", origin: receiver.origin, nickname: receiver.nickname, peer },
          ac.signal,
        );
        if (!ok) {
          refuse(req, "the user declined");
          return;
        }
        this.local.addGrant(receiver.origin, client);
      }
      void req.respond(Result.Success);
      session.onclose = null;
      // The ceremony responder keeps answering for as long as the session lives.
      // Delivered in a later task, as the spec queues the connection event.
      // A session the client already closed is still delivered, and its
      // connection then closes, as the spec's steps do.
      setTimeout(() => {
        if (!receiver.deliver(session)) session.close("receiver stopped");
      }, 0);
    } catch (err) {
      session.close(String(err));
      throw err;
    } finally {
      // The ceremony responder outlives admit() only through the session's own close.
      if (session.closed) ac.abort();
    }
  }

  /** A started receiver of this device that allows origin, preferring preferred. */
  private receiverFor(origin: string, preferred: ReceiverHandle): ReceiverHandle | null {
    if (this.local.receivers.has(preferred) && preferred.allows(origin)) return preferred;
    for (const r of this.local.receivers) if (r.allows(origin)) return r as ReceiverHandle;
    return null;
  }

  private pinPresenter(own: ReceiverHandle, peer: PeerInfo, signal: AbortSignal): (pin: string) => void {
    return (pin) => {
      const shown = new AbortController();
      signal.addEventListener("abort", () => shown.abort(), { once: true });
      this.ui.presentPIN({ pin, origin: own.origin, nickname: own.nickname, peer }, shown.signal);
    };
  }

  /**
   * LP2PRequest.start(): shows the picker, connects to the chosen peer,
   * verifies it (a ceremony if it isn't trusted), sends the requesting
   * origin and, if the server asks, runs the ceremony. Rejects with
   * NotAllowedError on every failure, as the spec requires. Aborting signal
   * takes the picker down or closes the session being set up, and rejects
   * with the abort reason.
   */
  async request(nickname: string, signal?: AbortSignal): Promise<GrantedSession> {
    signal?.throwIfAborted();
    const picker = new LivePicker(this.origin, nickname);
    const pickAbort = new AbortController();
    const onAbort = (): void => pickAbort.abort(signal?.reason);
    signal?.addEventListener("abort", onAbort, { once: true });
    const discovery = this.transport.discover((e) => picker.update(e));
    let chosen: PeerInfo | null;
    try {
      chosen = await this.ui.pickPeer(picker, pickAbort.signal);
    } finally {
      signal?.removeEventListener("abort", onAbort);
      pickAbort.abort();
      discovery.close();
    }
    signal?.throwIfAborted();
    if (!chosen) throw notAllowed("The user dismissed the device picker.");
    const peer = chosen;
    this.log(`connecting to ${peer.nickname} (${peer.id}) over ${this.transport.name}`);

    let conn: TransportConnection;
    try {
      conn = await this.transport.connect(peer.id, { id: randomId(), nickname }, signal ? { signal } : {});
    } catch (err) {
      signal?.throwIfAborted();
      throw notAllowed(`Could not connect to the peer: ${String(err)}`);
    }
    const session = new Session(conn, "client");
    const ac = new AbortController();
    session.onclose = (reason) => ac.abort(new DOMException(`connection closed: ${reason}`, "NetworkError"));
    const abortSetup = (): void => ac.abort(signal?.reason);
    signal?.addEventListener("abort", abortSetup, { once: true });
    try {
      const identity = await this.local.identity();
      const control = await session.controlChannel(ac.signal);
      const tls = await this.withTimeout(handshake(control, identity, "client", ac.signal), ac);
      session.tls = tls;
      // As in OSP; it starts nothing.
      sendAuthCapabilities(control);
      const remote = await this.withTimeout(session.exchangeInfo(this.agentInfo(), ac.signal), ac);
      const server = tls.peerFingerprint;
      const parties: Parties = { client: identity.fingerprint, server };
      const verified: PeerInfo = { ...peer, displayName: remote.displayName, verified: false };
      const ceremony = async (): Promise<void> => {
        await startCeremony(control, parties, () => this.ui.enterPIN({ origin: this.origin, nickname, peer: verified }, ac.signal), ac.signal);
        this.local.addSelfAnchor(server);
      };

      // Verify the server before it learns the origin.
      let ran = false;
      if (!this.local.isAuthenticated(server)) {
        await ceremony();
        ran = true;
      }
      verified.verified = true;

      let cue!: () => void;
      const required = new Promise<"required">((r) => (cue = () => r("required")));
      const result = session.requestSession(this.origin, cue);
      result.catch(() => {});
      const aborted = new Promise<never>((_, reject) => {
        if (ac.signal.aborted) reject(ac.signal.reason);
        ac.signal.addEventListener("abort", () => reject(ac.signal.reason), { once: true });
      });
      aborted.catch(() => {});
      const first = await Promise.race([required, result, aborted]);
      if (first === "required") {
        // The server hasn't verified us; it never saw a final answer for an unverified client.
        if (ran) throw notAllowed("The peer asked for a second ceremony.");
        await ceremony();
      }
      const final = await Promise.race([result, aborted]);
      if (final !== Result.Success) throw notAllowed("The peer refused the session.");
      // Accepted: success and a verified server. Streams held until now are routed.
      session.markAccepted();
      this.local.rememberPeer({ identity: server, anchorKey: server, displayName: remote.displayName });
      this.local.addGrant(this.origin, server);
      session.onclose = null;
      return { session, peer: verified };
    } catch (err) {
      session.close(String(err));
      signal?.throwIfAborted();
      if (err instanceof DOMException && err.name === "NotAllowedError") throw err;
      if (err instanceof AuthError) throw notAllowed(`Pairing failed: ${err.message}.`);
      throw notAllowed(`Connection setup failed: ${String(err instanceof Error ? err.message : err)}`);
    } finally {
      signal?.removeEventListener("abort", abortSetup);
      ac.abort();
    }
  }

  private agentInfo(): AgentInfo {
    return {
      displayName: this.deviceName,
      modelName: this.modelName,
      capabilities: [AgentCapability.DataChannels, AgentCapability.QuicTransport],
      stateToken: randomId(4),
      locales: globalThis.navigator?.languages ? [...globalThis.navigator.languages] : [],
    };
  }

  private withTimeout<T>(p: Promise<T>, ac: AbortController): Promise<T> {
    const t = setTimeout(
      () => ac.abort(new DOMException("timed out waiting for the peer", "TimeoutError")),
      this.handshakeTimeout,
    );
    return p.finally(() => clearTimeout(t));
  }
}
