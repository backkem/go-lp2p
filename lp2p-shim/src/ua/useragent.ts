// The user agent: the policy between the web API and the session layer.
// It asks the user for permission, runs the device picker and the trust
// ceremony, keeps the local state, and hands the API layer accepted
// sessions. Mirrors go-lp2p's lp2p-api/useragent.
//
// A session is established as the spec's OSP binding says ("Session
// extension"): after the connection's handshake, the client sends the
// requesting origin in lp2p-session-request; the server refuses it right
// away if no advertising receiver allows that origin, before any ceremony
// or prompt. Otherwise, unless it already trusts the client's key, it sends
// auth-capabilities as the cue for the client to start the ceremony. Once
// the client is authenticated, the server asks its user to grant the client
// to the receiver's origin (unless granted before) and answers
// lp2p-session-response.

import { AgentCapability, Result, type AgentInfo } from "../protocol/messages.ts";
import { AuthError, answerCeremony, sendAuthCapabilities, startCeremony, type Parties } from "../protocol/auth.ts";
import { handshake } from "../protocol/identity.ts";
import { Session, type SessionRequest } from "../protocol/session.ts";
import { TypeKey } from "../protocol/messages.ts";
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
      let info!: Promise<AgentInfo>;
      const { control, tls, req } = await this.withTimeout(
        (async () => {
          const control = await session.controlChannel(ac.signal);
          const tls = await handshake(control, identity, "server", ac.signal);
          session.tls = tls;
          // The client sends its session request once it has our agent-info.
          info = session.exchangeInfo(this.agentInfo(), ac.signal);
          info.catch(() => {});
          return { control, tls, req: await request };
        })(),
        ac,
      );

      // The hint: refuse before any ceremony or prompt if no receiver allows it.
      const hint = parseOrigin(req.requestingOrigin) ?? "null";
      if (!this.receiverFor(hint, own)) {
        refuse(req, `no receiver allows ${hint}`);
        return;
      }

      const parties: Parties = { client: tls.peerFingerprint, server: identity.fingerprint };
      const remote = await this.withTimeout(info, ac);
      const peer: PeerInfo = { id: conn.remoteId, nickname: "", displayName: remote.displayName, verified: false };
      const showPIN = this.pairing === "pin" ? this.pinPresenter(own, peer, ac.signal) : null;
      if (this.local.isAuthenticated(tls.peerFingerprint)) {
        // A key verified before is bound by the handshake: no ceremony. The
        // client may still start one if it doesn't trust this side; answer
        // it for as long as the session lives.
        const live = new AbortController();
        session.whenClosed(() => live.abort(new DOMException("connection closed", "AbortError")));
        const show = this.pairing === "pin" ? this.pinPresenter(own, peer, live.signal) : null;
        // If that ceremony fails, the server closes the connection.
        answerCeremony(control, parties, show, live.signal).catch((err: unknown) => {
          if (err instanceof AuthError) session.close("authentication after success failed");
        });
      } else {
        sendAuthCapabilities(control);
        await answerCeremony(control, parties, showPIN, ac.signal);
        this.local.addSelfAnchor(tls.peerFingerprint);
      }
      peer.verified = true;

      // In this binding the requesting origin is the hint, bound by the handshake.
      const receiver = this.receiverFor(hint, own);
      if (!receiver) {
        refuse(req, `no receiver allows ${hint} anymore`);
        return;
      }
      this.local.rememberPeer({ identity: tls.peerFingerprint, anchorKey: tls.peerFingerprint, displayName: remote.displayName });
      if (!this.local.hasGrant(receiver.origin, tls.peerFingerprint)) {
        const ok = await this.ui.requestPermission(
          { permission: "accept-connection", origin: receiver.origin, nickname: receiver.nickname, peer },
          ac.signal,
        );
        if (!ok) {
          refuse(req, "the user declined");
          return;
        }
        this.local.addGrant(receiver.origin, tls.peerFingerprint);
      }
      req.respond(Result.Success);
      session.onclose = null;
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
      ac.abort();
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
   * runs the session request and, when needed, the trust ceremony. Rejects
   * with NotAllowedError on every failure, as the spec requires.
   */
  async request(nickname: string): Promise<GrantedSession> {
    const picker = new LivePicker(this.origin, nickname);
    const pickAbort = new AbortController();
    const discovery = this.transport.discover((e) => picker.update(e));
    let chosen: PeerInfo | null;
    try {
      chosen = await this.ui.pickPeer(picker, pickAbort.signal);
    } finally {
      pickAbort.abort();
      discovery.close();
    }
    if (!chosen) throw notAllowed("The user dismissed the device picker.");
    const peer = chosen;
    this.log(`connecting to ${peer.nickname} (${peer.id}) over ${this.transport.name}`);

    let conn: TransportConnection;
    try {
      conn = await this.transport.connect(peer.id, { id: randomId(), nickname });
    } catch (err) {
      throw notAllowed(`Could not connect to the peer: ${String(err)}`);
    }
    const session = new Session(conn, "client");
    const ac = new AbortController();
    session.onclose = (reason) => ac.abort(new DOMException(`connection closed: ${reason}`, "NetworkError"));
    try {
      const identity = await this.local.identity();
      const control = await session.controlChannel(ac.signal);
      const tls = await this.withTimeout(handshake(control, identity, "client", ac.signal), ac);
      session.tls = tls;
      // The session request follows the agent-info exchange.
      const remote = await this.withTimeout(session.exchangeInfo(this.agentInfo(), ac.signal), ac);
      const result = session.requestSession(this.origin);
      result.catch(() => {});
      const cue = control.next([TypeKey.AuthCapabilities], ac.signal).then(() => "cue" as const);
      cue.catch(() => {});

      const parties: Parties = { client: identity.fingerprint, server: tls.peerFingerprint };
      const trusted = this.local.isAuthenticated(tls.peerFingerprint);
      const first = await Promise.race([cue, result]);
      if (first !== "cue" && first !== Result.Success) throw notAllowed("The peer refused the session.");
      const verified: PeerInfo = { ...peer, displayName: remote.displayName, verified: false };
      if (first === "cue" || !trusted) {
        await startCeremony(control, parties, () => this.ui.enterPIN({ origin: this.origin, nickname, peer: verified }, ac.signal), ac.signal);
        this.local.addSelfAnchor(tls.peerFingerprint);
      }
      verified.verified = true;
      if ((await result) !== Result.Success) throw notAllowed("The peer refused the session.");
      // Accepted: success and a verified server. Streams held until now are routed.
      session.markAccepted();
      this.local.rememberPeer({ identity: tls.peerFingerprint, anchorKey: tls.peerFingerprint, displayName: remote.displayName });
      this.local.addGrant(this.origin, tls.peerFingerprint);
      session.onclose = null;
      return { session, peer: verified };
    } catch (err) {
      session.close(String(err));
      if (err instanceof DOMException && err.name === "NotAllowedError") throw err;
      if (err instanceof AuthError) throw notAllowed(`Pairing failed: ${err.message}.`);
      throw notAllowed(`Connection setup failed: ${String(err instanceof Error ? err.message : err)}`);
    } finally {
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
