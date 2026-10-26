// The user agent: the policy between the web API and the session layer.
// It asks the user for permission, runs the device picker and pairing, and
// hands the API layer authenticated sessions. Mirrors go-lp2p's
// lp2p-api/useragent (Advertise / connect with UI hooks).

import { AgentCapability, AuthResult, TypeKey, type AgentInfo } from "../protocol/messages.ts";
import { AuthError, awaitAuthentication, presentPIN, skipAuthentication } from "../protocol/auth.ts";
import { Session } from "../protocol/session.ts";
import { randomId } from "../transport/base.ts";
import type { Closer, DiscoveryEvent, Transport, TransportConnection } from "../transport/types.ts";
import type { PeerInfo, PeerPicker, UserAgentUI } from "./ui.ts";

export type Pairing = "pin" | "none";

export interface UserAgentConfig {
  transport: Transport;
  ui: UserAgentUI;
  /** "pin": the receiver shows a PIN the requester must enter (toy, see protocol/auth.ts). Default "pin". */
  pairing?: Pairing;
  /** Origin shown in prompts. Default: location.origin. */
  origin?: string;
  /** agent-info model-name. Default "lp2p-shim". */
  modelName?: string;
  /** How long the agent-info handshake may take, in ms. Default 10000. */
  handshakeTimeout?: number;
  /** Diagnostics. */
  log?: (line: string) => void;
}

/** An authenticated session plus what the user agent knows about the peer. */
export interface GrantedSession {
  session: Session;
  peer: PeerInfo;
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
  private readonly modelName: string;
  private readonly handshakeTimeout: number;
  private readonly log: (line: string) => void;

  constructor(config: UserAgentConfig) {
    this.transport = config.transport;
    this.ui = config.ui;
    this.pairing = config.pairing ?? "pin";
    this.origin = config.origin ?? globalThis.location?.origin ?? "null";
    this.modelName = config.modelName ?? "lp2p-shim";
    this.handshakeTimeout = config.handshakeTimeout ?? 10_000;
    this.log = config.log ?? (() => {});
  }

  /**
   * LP2PReceiver.start(): asks to advertise as nickname and, if allowed,
   * advertises until closed. Connections the user grants go to onsession.
   */
  async advertise(nickname: string, onsession: (g: GrantedSession) => void): Promise<Closer> {
    const allowed = await this.ui.requestPermission(
      { permission: "advertise", origin: this.origin, nickname },
      new AbortController().signal,
    );
    if (!allowed) throw notAllowed("The user did not allow advertising.");
    const id = randomId();
    this.log(`advertising "${nickname}" as ${id} over ${this.transport.name}`);
    return this.transport.listen({ id, nickname }, (conn) => {
      this.admit(conn, nickname).then(
        (g) => g && onsession(g),
        (err: unknown) => this.log(`incoming connection from ${conn.remoteId} failed: ${String(err)}`),
      );
    });
  }

  /** Receiver side of a new connection: agent-info, consent, pairing. */
  private async admit(conn: TransportConnection, nickname: string): Promise<GrantedSession | null> {
    const session = new Session(conn, "receiver");
    const ac = new AbortController();
    const cancel = (): void => ac.abort(new DOMException("connection closed", "AbortError"));
    session.onclose = cancel;
    try {
      const info = await this.withTimeout(session.handshake(this.agentInfo(nickname), ac.signal), ac);
      const peer: PeerInfo = { id: conn.remoteId, nickname: info.displayName, verified: false };
      const control = await session.controlChannel();
      const ok = await this.ui.requestPermission(
        { permission: "accept-connection", origin: this.origin, nickname, peer },
        ac.signal,
      );
      if (!ok) {
        this.log(`declined connection from ${peer.nickname}`);
        control.send({ type: TypeKey.AuthStatus, result: AuthResult.UnknownError });
        session.close("declined");
        return null;
      }
      if (this.pairing === "pin") {
        const shown = new AbortController();
        try {
          await presentPIN(
            control,
            (pin) => this.ui.presentPIN({ pin, origin: this.origin, nickname, peer }, shown.signal),
            ac.signal,
          );
        } finally {
          shown.abort();
        }
      } else {
        skipAuthentication(control);
      }
      session.onclose = null;
      return { session, peer };
    } catch (err) {
      session.close(String(err));
      throw err;
    } finally {
      ac.abort();
    }
  }

  /**
   * LP2PRequest.start(): shows the picker, connects to the chosen peer and
   * pairs with it.
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
      throw new DOMException(`Could not connect to the peer: ${String(err)}`, "NetworkError");
    }
    const session = new Session(conn, "requester");
    const ac = new AbortController();
    session.onclose = (reason) => ac.abort(new DOMException(`connection closed: ${reason}`, "NetworkError"));
    try {
      const info = await this.withTimeout(session.handshake(this.agentInfo(nickname), ac.signal), ac);
      const verified: PeerInfo = { ...peer, nickname: info.displayName };
      const control = await session.controlChannel(ac.signal);
      await awaitAuthentication(
        control,
        () => this.ui.enterPIN({ origin: this.origin, nickname, peer: verified }, ac.signal),
        ac.signal,
      );
      session.onclose = null;
      return { session, peer: verified };
    } catch (err) {
      session.close(String(err));
      if (err instanceof AuthError) {
        throw notAllowed(
          err.result === AuthResult.UnknownError && err.message.startsWith("authentication failed")
            ? "The peer declined the connection."
            : `Pairing failed: ${err.message}.`,
        );
      }
      if (err instanceof DOMException) throw err;
      throw new DOMException(`Connection setup failed: ${String(err)}`, "NetworkError");
    } finally {
      ac.abort();
    }
  }

  private agentInfo(nickname: string): AgentInfo {
    return {
      displayName: nickname,
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
