// The session layer: Open Screen Protocol semantics over a transport
// connection. It knows the message formats and the order they come in, but
// nothing about users, permissions or the web API.
//
// Roles: the client is the side that established the connection (the
// requester), the server the side that accepted it (the receiver). As in the
// spec's OSP binding, one connection carries one LP2P session.
//
// Streams:
// - The client opens the first stream as the control stream. It carries the
//   shim's TLS stand-in (identity.ts), agent-info and the authentication
//   messages. The server recognizes it by its first message.
// - The client opens a second stream for lp2p-session-request (1110), with
//   the requesting origin, right after the handshake. The server answers
//   lp2p-session-response (1111) on it once it accepts or refuses.
// - Every other stream is a data channel or WebTransport. Data channels
//   start with data-channel-open-request (1101); the peer answers
//   data-channel-open-response (1102) on the same stream, then both sides
//   send data-frames (24). Closing the stream closes the channel. Streams
//   that begin with 1201 or 1203 belong to WebTransport sessions, and
//   connection datagrams too; see webtransport.ts. None of them may come
//   before the session is accepted: the server closes them, the client
//   holds them until the 1111 arrives (it may come later, on another stream).

import type { TransportConnection, TransportStream } from "../transport/types.ts";
import { decodeVarint } from "../codec/varint.ts";
import { DataEncoding, Result, TypeKey, decodeMessage, encodeMessage, type AgentInfo, type Message } from "./messages.ts";
import { exportKeyingMaterial, type HandshakeResult } from "./identity.ts";
import { SESSION_GONE, WTSession, WTStream, type WTHost } from "./webtransport.ts";

export type Role = "client" | "server";

export class ProtocolError extends Error {
  override name = "ProtocolError";
}

const CONTROL_TYPES: readonly number[] = [
  TypeKey.ShimHello,
  TypeKey.AgentInfoRequest,
  TypeKey.AgentInfoResponse,
  TypeKey.AuthCapabilities,
  TypeKey.AuthSpake2Handshake,
];

type Waiter = { types: readonly number[]; resolve: (m: Message) => void; reject: (e: Error) => void };

/** The control stream, with a queue of messages no caller has taken yet. */
export class ControlChannel {
  private readonly queue: Message[] = [];
  private readonly waiters: Waiter[] = [];
  private closedWith: Error | null = null;
  private info: AgentInfo | null = null;
  private readonly unanswered: number[] = [];

  private readonly stream: TransportStream;

  /** first is a message already read from the stream, handled before any that follow. */
  constructor(stream: TransportStream, first?: Uint8Array) {
    this.stream = stream;
    if (first) this.feed(first);
    stream.onmessage = (b) => this.feed(b);
    stream.onclose = () => this.fail(new ProtocolError("control stream closed"));
  }

  send(m: Message): void {
    if (!this.stream.closed) this.stream.send(encodeMessage(m));
  }

  /** What to answer agent-info-request with. Requests that came earlier are answered now. */
  set localInfo(info: AgentInfo) {
    this.info = info;
    for (const requestId of this.unanswered.splice(0)) {
      this.send({ type: TypeKey.AgentInfoResponse, requestId, agentInfo: info });
    }
  }

  /**
   * Resolves with the oldest message of one of types, now or when it
   * arrives. Messages of other types stay queued for other callers.
   */
  next<T extends Message["type"]>(types: readonly T[], signal?: AbortSignal): Promise<Extract<Message, { type: T }>> {
    type R = Extract<Message, { type: T }>;
    const i = this.queue.findIndex((m) => (types as readonly number[]).includes(m.type));
    if (i >= 0) return Promise.resolve(this.queue.splice(i, 1)[0] as R);
    if (this.closedWith) return Promise.reject(this.closedWith);
    if (signal?.aborted) return Promise.reject(signal.reason);
    return new Promise<R>((resolve, reject) => {
      const w: Waiter = {
        types,
        resolve: (m) => {
          signal?.removeEventListener("abort", onAbort);
          resolve(m as R);
        },
        reject: (e) => {
          signal?.removeEventListener("abort", onAbort);
          reject(e);
        },
      };
      const onAbort = (): void => {
        const j = this.waiters.indexOf(w);
        if (j >= 0) this.waiters.splice(j, 1);
        reject(signal?.reason);
      };
      signal?.addEventListener("abort", onAbort, { once: true });
      this.waiters.push(w);
    });
  }

  close(): void {
    this.stream.close();
  }

  /** Handles one raw message from the control stream. */
  feed(b: Uint8Array): void {
    let m: Message;
    try {
      m = decodeMessage(b);
    } catch {
      return; // OSP: ignore what we do not understand on the control stream.
    }
    if (m.type === TypeKey.AgentInfoRequest) {
      if (this.info) this.send({ type: TypeKey.AgentInfoResponse, requestId: m.requestId, agentInfo: this.info });
      else this.unanswered.push(m.requestId);
      return;
    }
    const i = this.waiters.findIndex((w) => w.types.includes(m.type));
    if (i >= 0) (this.waiters.splice(i, 1)[0] as Waiter).resolve(m);
    else this.queue.push(m);
  }

  private fail(err: Error): void {
    if (this.closedWith) return;
    this.closedWith = err;
    for (const w of this.waiters.splice(0)) w.reject(err);
  }
}

/** The session stream, server side: the client's lp2p-session-request and how to answer it. */
export interface SessionRequest {
  /** The requesting origin hint, as sent. */
  requestingOrigin: string;
  /** Sends lp2p-session-response and ends the stream. */
  respond(result: number): void;
}

export type ChannelState = "connecting" | "open" | "closed";

export interface ChannelParams {
  id: number;
  label: string;
  protocol: string;
}

/** One LP2P data channel on the wire. The API object drives it. */
export class DataChannelEndpoint {
  state: ChannelState;
  onopen: (() => void) | null = null;
  onerror: ((err: Error) => void) | null = null;
  onclose: (() => void) | null = null;
  private requestId: number | null;
  private frameHandler: ((encodingId: number, payload: Uint8Array) => void) | null = null;
  private readonly heldFrames: [number, Uint8Array][] = [];

  readonly params: ChannelParams;
  private readonly stream: TransportStream;
  private readonly session: Session;

  constructor(
    params: ChannelParams,
    stream: TransportStream,
    session: Session,
    opener: { requestId: number } | null,
  ) {
    this.params = params;
    this.stream = stream;
    this.session = session;
    this.state = opener ? "connecting" : "open";
    this.requestId = opener ? opener.requestId : null;
    stream.onmessage = (b) => this.receive(b);
    stream.onclose = () => this.closed();
  }

  /** Received data-frames. Frames that arrive while this is null are held. */
  set onframe(fn: ((encodingId: number, payload: Uint8Array) => void) | null) {
    this.frameHandler = fn;
    while (fn && this.heldFrames.length > 0) fn(...(this.heldFrames.shift() as [number, Uint8Array]));
  }

  send(encodingId: DataEncoding, payload: Uint8Array): void {
    if (this.state !== "open") throw new DOMException("data channel is not open", "InvalidStateError");
    this.stream.send(encodeMessage({ type: TypeKey.DataFrame, encodingId, payload }));
  }

  close(): void {
    this.stream.close();
  }

  private receive(b: Uint8Array): void {
    let m: Message;
    try {
      m = decodeMessage(b);
    } catch (err) {
      this.onerror?.(err instanceof Error ? err : new ProtocolError(String(err)));
      this.stream.close();
      return;
    }
    if (this.state === "connecting") {
      if (m.type !== TypeKey.DataChannelOpenResponse || m.requestId !== this.requestId) {
        this.onerror?.(new ProtocolError(`expected data-channel-open-response, got type ${m.type}`));
        this.stream.close();
        return;
      }
      this.requestId = null;
      if (m.result !== Result.Success) {
        this.onerror?.(new ProtocolError(`peer refused the data channel (result ${m.result})`));
        this.stream.close();
        return;
      }
      this.state = "open";
      this.onopen?.();
      return;
    }
    if (m.type !== TypeKey.DataFrame) return;
    if (this.frameHandler) this.frameHandler(m.encodingId, m.payload);
    else this.heldFrames.push([m.encodingId, m.payload]);
  }

  private closed(): void {
    if (this.state === "closed") return;
    this.state = "closed";
    this.session.releaseChannelId(this.params.id);
    this.onclose?.();
  }
}

export class Session implements WTHost {
  readonly role: Role;
  remoteInfo: AgentInfo | null = null;
  /** Set by the handshake: the peer's proven identity and the exporter secret. */
  tls: HandshakeResult | null = null;
  private control: ControlChannel | null = null;
  private readonly controlWaiters: ((c: ControlChannel) => void)[] = [];
  private readonly usedIds = new Set<number>();
  private readonly parity: number;
  private nextRequestId = 1;
  private readonly incoming: DataChannelEndpoint[] = [];
  private channelHandler: ((ep: DataChannelEndpoint) => void) | null = null;
  private closeHandler: ((reason: string) => void) | null = null;
  private readonly closeListeners: ((reason: string) => void)[] = [];
  private closeReason: string | null = null;
  private readonly transports = new Map<number, WTSession>();
  private nextExchangeId: number;
  private readonly incomingTransports: WTSession[] = [];
  private transportHandler: ((t: WTSession) => void) | null = null;
  private sessionRequestHandler: ((r: SessionRequest) => void) | null = null;
  private heldSessionRequest: SessionRequest | null = null;
  private sawSessionRequest = false;
  private isAccepted = false;
  /** Client: streams the server opened before its 1111 arrived. */
  private readonly early: [TransportStream, Uint8Array][] = [];

  readonly connection: TransportConnection;

  constructor(connection: TransportConnection, role: Role) {
    this.connection = connection;
    this.role = role;
    // The client allocates even IDs, the server odd ones: data channel IDs
    // and exchange-ids alike.
    this.parity = role === "client" ? 0 : 1;
    this.nextExchangeId = this.parity;
    if (role === "client") this.setControl(new ControlChannel(connection.openStream()));
    connection.onstream = (s) => this.classify(s);
    connection.ondatagram = (d) => this.receiveDatagram(d);
    connection.onclose = (reason) => {
      this.closeReason = reason;
      this.early.length = 0;
      this.closeHandler?.(reason);
      for (const fn of this.closeListeners.splice(0)) fn(reason);
    };
  }

  get closed(): boolean {
    return this.closeReason !== null;
  }

  /** True once the server sent, or the client received, a success lp2p-session-response. */
  get accepted(): boolean {
    return this.isAccepted;
  }

  set onclose(fn: ((reason: string) => void) | null) {
    this.closeHandler = fn;
    if (fn && this.closeReason !== null) fn(this.closeReason);
  }

  /** Like onclose, for any number of observers; called at once if already closed. */
  whenClosed(fn: (reason: string) => void): void {
    if (this.closeReason !== null) fn(this.closeReason);
    else this.closeListeners.push(fn);
  }

  /** Incoming data channels. Channels that arrive while this is null are held. */
  set ondatachannel(fn: ((ep: DataChannelEndpoint) => void) | null) {
    this.channelHandler = fn;
    while (fn && this.incoming.length > 0) fn(this.incoming.shift() as DataChannelEndpoint);
  }

  /** Incoming WebTransport sessions, pending. Sessions that arrive while this is null are held. */
  set ontransport(fn: ((t: WTSession) => void) | null) {
    this.transportHandler = fn;
    while (fn && this.incomingTransports.length > 0) fn(this.incomingTransports.shift() as WTSession);
  }

  /** Server: the client's lp2p-session-request. One that arrives while this is null is held. */
  set onsessionrequest(fn: ((r: SessionRequest) => void) | null) {
    this.sessionRequestHandler = fn;
    const held = this.heldSessionRequest;
    if (fn && held) {
      this.heldSessionRequest = null;
      fn(held);
    }
  }

  requestId(): number {
    return this.nextRequestId++;
  }

  get maxDatagramSize(): number {
    return this.connection.maxDatagramSize;
  }

  get datagramsReliable(): boolean {
    return this.connection.datagramsReliable;
  }

  get connectionClosed(): boolean {
    return this.closed || this.connection.closed;
  }

  openStream(unidirectional: boolean): TransportStream {
    return this.connection.openStream({ unidirectional });
  }

  sendDatagram(d: Uint8Array): void {
    this.connection.sendDatagram(d);
  }

  forgetTransport(exchangeId: number): void {
    this.transports.delete(exchangeId);
    const i = this.incomingTransports.findIndex((t) => t.exchangeId === exchangeId);
    if (i >= 0) this.incomingTransports.splice(i, 1);
  }

  /** Keying material for one WebTransport session (exporter of the connection's handshake). */
  exportKeyingMaterial(label: string, context: Uint8Array, length: number): Promise<Uint8Array> {
    if (!this.tls) return Promise.reject(new DOMException("no handshake", "InvalidStateError"));
    return exportKeyingMaterial(this.tls, label, context, length);
  }

  /** Opens a WebTransport session (1201). It is "connecting" until the peer's user agent accepts it. */
  openTransport(): WTSession {
    if (this.closed) throw new DOMException("session is closed", "InvalidStateError");
    if (!this.isAccepted) throw new DOMException("session is not accepted yet", "InvalidStateError");
    const id = this.nextExchangeId;
    this.nextExchangeId += 2;
    const t = new WTSession(this, id, this.connection.openStream(), true, this.requestId());
    this.transports.set(id, t);
    return t;
  }

  private receiveDatagram(d: Uint8Array): void {
    let id: number;
    let n: number;
    try {
      [id, n] = decodeVarint(d);
    } catch {
      return;
    }
    // Datagrams for a session that is not open are dropped.
    this.transports.get(id)?.receiveDatagram(d.subarray(n));
  }

  /** The control stream, once the client has opened it. */
  controlChannel(signal?: AbortSignal): Promise<ControlChannel> {
    if (this.control) return Promise.resolve(this.control);
    return new Promise((resolve, reject) => {
      this.controlWaiters.push(resolve);
      signal?.addEventListener("abort", () => reject(signal.reason), { once: true });
    });
  }

  /** Exchanges agent-info: answers the peer's request and asks for the peer's. */
  async exchangeInfo(local: AgentInfo, signal?: AbortSignal): Promise<AgentInfo> {
    const control = await this.controlChannel(signal);
    control.localInfo = local;
    control.send({ type: TypeKey.AgentInfoRequest, requestId: this.requestId() });
    const res = await control.next([TypeKey.AgentInfoResponse], signal);
    this.remoteInfo = res.agentInfo;
    return res.agentInfo;
  }

  /**
   * Client: sends lp2p-session-request with origin on a new stream. The
   * promise resolves with the server's result, or rejects if the stream or
   * the connection closes first.
   */
  requestSession(origin: string): Promise<number> {
    const stream = this.connection.openStream();
    const requestId = this.requestId();
    stream.send(encodeMessage({ type: TypeKey.SessionRequest, requestId, requestingOrigin: origin }));
    return new Promise<number>((resolve, reject) => {
      stream.onmessage = (b) => {
        let m: Message;
        try {
          m = decodeMessage(b);
        } catch {
          return;
        }
        if (m.type !== TypeKey.SessionResponse || m.requestId !== requestId) return;
        if (m.result === Result.Success) this.markAccepted();
        resolve(m.result);
        stream.onclose = null;
        stream.close(); // both agents end their side of the stream
      };
      stream.onclose = () => reject(new ProtocolError("session stream closed without a response"));
    });
  }

  /**
   * Allocates the lowest free channel ID of the local parity, or validates
   * and reserves a requested one, which must be free and of that parity.
   */
  reserveChannelId(requested?: number): number {
    if (requested !== undefined) {
      if (requested % 2 !== this.parity) {
        throw new DOMException(`data channel id ${requested} has the peer's parity`, "OperationError");
      }
      if (this.usedIds.has(requested)) throw new DOMException(`data channel id ${requested} is in use`, "OperationError");
      this.usedIds.add(requested);
      return requested;
    }
    let id = this.parity;
    while (this.usedIds.has(id)) id += 2;
    if (id > 65534) throw new DOMException("out of data channel ids", "OperationError");
    this.usedIds.add(id);
    return id;
  }

  releaseChannelId(id: number): void {
    this.usedIds.delete(id);
  }

  /** Opens a data channel; params.id must come from reserveChannelId. */
  openDataChannel(params: ChannelParams): DataChannelEndpoint {
    const stream = this.connection.openStream();
    const requestId = this.requestId();
    const ep = new DataChannelEndpoint(params, stream, this, { requestId });
    stream.send(
      encodeMessage({
        type: TypeKey.DataChannelOpenRequest,
        requestId,
        channelId: params.id,
        label: params.label,
        protocol: params.protocol,
      }),
    );
    return ep;
  }

  close(reason?: string): void {
    this.connection.close(reason);
  }

  private setControl(c: ControlChannel): void {
    this.control = c;
    for (const w of this.controlWaiters.splice(0)) w(c);
  }

  private markAccepted(): void {
    if (this.isAccepted) return;
    this.isAccepted = true;
    for (const [stream, first] of this.early.splice(0)) this.route(stream, first);
  }

  /** Looks at a new stream's first message to tell what it is. */
  private classify(stream: TransportStream): void {
    stream.onmessage = (b) => {
      stream.onmessage = null;
      this.route(stream, b);
    };
  }

  private route(stream: TransportStream, b: Uint8Array): void {
    let m: Message;
    try {
      m = decodeMessage(b);
    } catch {
      stream.close();
      return;
    }
    if (this.role === "server" && !this.control && CONTROL_TYPES.includes(m.type)) {
      this.setControl(new ControlChannel(stream, b));
      return;
    }
    if (m.type === TypeKey.SessionRequest && this.role === "server") {
      this.receiveSessionRequest(stream, m.requestId, m.requestingOrigin);
      return;
    }
    const extension =
      m.type === TypeKey.DataChannelOpenRequest ||
      m.type === TypeKey.DataTransportStartRequest ||
      m.type === TypeKey.DataTransportStream;
    if (!extension) {
      stream.close();
      return;
    }
    if (!this.isAccepted) {
      // No extension messages before the success lp2p-session-response.
      if (this.role === "client") this.early.push([stream, b]);
      else stream.close();
      return;
    }
    if (m.type === TypeKey.DataChannelOpenRequest) {
      this.acceptDataChannel(stream, m);
    } else if (m.type === TypeKey.DataTransportStartRequest) {
      this.acceptTransport(stream, m.requestId, m.exchangeId);
    } else if (m.type === TypeKey.DataTransportStream) {
      const ws = new WTStream(stream, stream.unidirectional, false);
      const t = this.transports.get(m.exchangeId);
      if (t) t.addIncomingStream(ws);
      else ws.refuse(SESSION_GONE);
    }
  }

  private receiveSessionRequest(stream: TransportStream, requestId: number, requestingOrigin: string): void {
    if (this.sawSessionRequest) {
      // Only one per connection: a second one closes it.
      this.close("second lp2p-session-request");
      return;
    }
    this.sawSessionRequest = true;
    let answered = false;
    const req: SessionRequest = {
      requestingOrigin,
      respond: (result) => {
        if (answered || stream.closed) return;
        answered = true;
        stream.send(encodeMessage({ type: TypeKey.SessionResponse, requestId, result }));
        if (result === Result.Success) this.markAccepted();
        stream.close();
      },
    };
    if (this.sessionRequestHandler) this.sessionRequestHandler(req);
    else this.heldSessionRequest = req;
  }

  private acceptTransport(stream: TransportStream, requestId: number, exchangeId: number): void {
    // The peer allocates exchange-ids of its own parity, and never reuses one.
    if (this.transports.has(exchangeId) || exchangeId % 2 === this.parity) {
      stream.send(encodeMessage({ type: TypeKey.DataTransportStartResponse, requestId, result: Result.PermanentError }));
      stream.close();
      return;
    }
    const t = new WTSession(this, exchangeId, stream, false, requestId);
    this.transports.set(exchangeId, t);
    if (this.transportHandler) this.transportHandler(t);
    else this.incomingTransports.push(t);
  }

  private acceptDataChannel(stream: TransportStream, req: Extract<Message, { type: typeof TypeKey.DataChannelOpenRequest }>): void {
    const ok = !this.usedIds.has(req.channelId);
    stream.send(
      encodeMessage({
        type: TypeKey.DataChannelOpenResponse,
        requestId: req.requestId,
        result: ok ? Result.Success : Result.PermanentError,
      }),
    );
    if (!ok) {
      stream.close();
      return;
    }
    this.usedIds.add(req.channelId);
    const ep = new DataChannelEndpoint(
      { id: req.channelId, label: req.label, protocol: req.protocol },
      stream,
      this,
      null,
    );
    if (this.channelHandler) this.channelHandler(ep);
    else this.incoming.push(ep);
  }
}
