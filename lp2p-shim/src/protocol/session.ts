// The session layer: Open Screen Protocol semantics over a transport
// connection. It knows the message formats and the order they come in, but
// nothing about users, permissions or the web API.
//
// Streams:
// - The requester (the side that connected) opens the first stream as the
//   control stream. It carries agent-info and the authentication messages.
// - Every other stream is a data channel. Its first message is a
//   data-channel-open-request (1101); the peer answers with a
//   data-channel-open-response (1102) on the same stream, after which both
//   sides send data-frames (24). Closing the stream closes the channel. This
//   is how go-lp2p maps LP2P data channels onto QUIC streams.

import type { TransportConnection, TransportStream } from "../transport/types.ts";
import { DataEncoding, Result, TypeKey, decodeMessage, encodeMessage, type AgentInfo, type Message } from "./messages.ts";

export type Role = "requester" | "receiver";

export class ProtocolError extends Error {
  override name = "ProtocolError";
}

/** The control stream, with a queue of messages the caller has not consumed yet. */
export class ControlChannel {
  private readonly queue: Message[] = [];
  private waiter: { types: readonly number[]; resolve: (m: Message) => void; reject: (e: Error) => void } | null = null;
  private closedWith: Error | null = null;
  private info: AgentInfo | null = null;
  private readonly unanswered: number[] = [];

  private readonly stream: TransportStream;

  constructor(stream: TransportStream) {
    this.stream = stream;
    stream.onmessage = (b) => this.feed(b);
    stream.onclose = () => this.fail(new ProtocolError("control stream closed"));
  }

  send(m: Message): void {
    this.stream.send(encodeMessage(m));
  }

  /** What to answer agent-info-request with. Requests that came earlier are answered now. */
  set localInfo(info: AgentInfo) {
    this.info = info;
    for (const requestId of this.unanswered.splice(0)) {
      this.send({ type: TypeKey.AgentInfoResponse, requestId, agentInfo: info });
    }
  }

  /** Resolves with the next message of one of types; earlier messages of other types are dropped. */
  next<T extends Message["type"]>(types: readonly T[], signal?: AbortSignal): Promise<Extract<Message, { type: T }>> {
    type R = Extract<Message, { type: T }>;
    while (this.queue.length > 0) {
      const m = this.queue.shift() as Message;
      if ((types as readonly number[]).includes(m.type)) return Promise.resolve(m as R);
    }
    if (this.closedWith) return Promise.reject(this.closedWith);
    if (this.waiter) return Promise.reject(new Error("ControlChannel.next: already waiting"));
    return new Promise<R>((resolve, reject) => {
      const onAbort = (): void => {
        this.waiter = null;
        reject(signal?.reason);
      };
      signal?.addEventListener("abort", onAbort, { once: true });
      this.waiter = {
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
    if (this.waiter && this.waiter.types.includes(m.type)) {
      const w = this.waiter;
      this.waiter = null;
      w.resolve(m);
    } else {
      this.queue.push(m);
    }
  }

  private fail(err: Error): void {
    if (this.closedWith) return;
    this.closedWith = err;
    const w = this.waiter;
    this.waiter = null;
    w?.reject(err);
  }
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

export class Session {
  readonly role: Role;
  remoteInfo: AgentInfo | null = null;
  private control: ControlChannel | null = null;
  private readonly controlWaiters: ((c: ControlChannel) => void)[] = [];
  private readonly usedIds = new Set<number>();
  private readonly channelIdParity: number;
  private nextRequestId = 1;
  private readonly incoming: DataChannelEndpoint[] = [];
  private channelHandler: ((ep: DataChannelEndpoint) => void) | null = null;
  private closeHandler: ((reason: string) => void) | null = null;
  private closeReason: string | null = null;

  readonly connection: TransportConnection;

  constructor(connection: TransportConnection, role: Role) {
    this.connection = connection;
    this.role = role;
    // The requester (QUIC client) allocates even IDs, the receiver odd ones.
    this.channelIdParity = role === "requester" ? 0 : 1;
    if (role === "requester") this.setControl(new ControlChannel(connection.openStream()));
    connection.onstream = (s) => this.classify(s);
    connection.onclose = (reason) => {
      this.closeReason = reason;
      this.closeHandler?.(reason);
    };
  }

  get closed(): boolean {
    return this.closeReason !== null;
  }

  set onclose(fn: ((reason: string) => void) | null) {
    this.closeHandler = fn;
    if (fn && this.closeReason !== null) fn(this.closeReason);
  }

  /** Incoming data channels. Channels that arrive while this is null are held. */
  set ondatachannel(fn: ((ep: DataChannelEndpoint) => void) | null) {
    this.channelHandler = fn;
    while (fn && this.incoming.length > 0) fn(this.incoming.shift() as DataChannelEndpoint);
  }

  requestId(): number {
    return this.nextRequestId++;
  }

  /** The control stream, once the requester has opened it. */
  controlChannel(signal?: AbortSignal): Promise<ControlChannel> {
    if (this.control) return Promise.resolve(this.control);
    return new Promise((resolve, reject) => {
      this.controlWaiters.push(resolve);
      signal?.addEventListener("abort", () => reject(signal.reason), { once: true });
    });
  }

  /** Exchanges agent-info: answers the peer's request and asks for the peer's. */
  async handshake(local: AgentInfo, signal?: AbortSignal): Promise<AgentInfo> {
    const control = await this.controlChannel(signal);
    control.localInfo = local;
    control.send({ type: TypeKey.AgentInfoRequest, requestId: this.requestId() });
    const res = await control.next([TypeKey.AgentInfoResponse], signal);
    this.remoteInfo = res.agentInfo;
    return res.agentInfo;
  }

  /**
   * Allocates the lowest free channel ID of the local parity, or validates
   * and reserves a requested one, which must be free and of that parity.
   */
  reserveChannelId(requested?: number): number {
    if (requested !== undefined) {
      if (requested % 2 !== this.channelIdParity) {
        throw new DOMException(`data channel id ${requested} has the peer's parity`, "OperationError");
      }
      if (this.usedIds.has(requested)) throw new DOMException(`data channel id ${requested} is in use`, "OperationError");
      this.usedIds.add(requested);
      return requested;
    }
    let id = this.channelIdParity;
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

  /** Looks at a new stream's first message to tell the control stream from data channels. */
  private classify(stream: TransportStream): void {
    stream.onmessage = (b) => {
      let m: Message;
      try {
        m = decodeMessage(b);
      } catch {
        stream.close();
        return;
      }
      if (m.type === TypeKey.DataChannelOpenRequest) {
        this.acceptDataChannel(stream, m);
      } else if (!this.control && this.role === "receiver") {
        const c = new ControlChannel(stream);
        this.setControl(c);
        c.feed(b);
      } else {
        stream.close();
      }
    };
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
