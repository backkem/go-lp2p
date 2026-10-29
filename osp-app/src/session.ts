// OspSession: the message layer of the application protocol over one
// OspTransport. It encodes and decodes messages, allocates request IDs and
// matches responses to requests, answers agent-info and agent-status
// requests, and applies the protocol's rules for unknown type keys.
// Presentation and remote playback build on it.

import {
  decodeBody,
  encodeBody,
  isSupported,
  messageName,
  typeKeyOf,
  SchemaError,
  type AgentInfo,
  type Frame,
  type Message,
  type MessageBody,
  type MessageName,
  type RequestName,
} from "./messages.ts";
import { Emitter } from "./emitter.ts";
import { TransportClosedError, type CloseInfo, type OspTransport, type SendOptions } from "./transport.ts";

/** application.bs: an unknown type key closes the connection with this code. */
export const UNKNOWN_TYPE_KEY_CODE = 404;

export interface SessionOptions {
  /**
   * The local agent-info. When set, agent-info-request and
   * agent-status-request are answered automatically.
   */
  agentInfo?: AgentInfo;
  /**
   * Allocates request IDs. The default counts from 1 per session. OSP
   * implementations (openscreen-go) keep one counter per remote agent across
   * connections; pass a shared allocator to do the same.
   */
  nextRequestId?: () => number;
  /**
   * What to do with an unknown type key: "close" (application.bs: close with
   * error code 404 and the type key in the reason) or "ignore". Default "close".
   */
  onUnknownTypeKey?: "close" | "ignore";
}

export interface SessionEvents {
  /** A message with a supported type that no handler took, or a response to no pending request. */
  unhandled: Message;
  /** A recognized message of an out-of-scope protocol (streaming). Dropped. */
  unsupported: Message;
  /** A message with an unknown type key. */
  unknown: Frame;
  /** A message that could not be decoded. Dropped. */
  malformed: Error;
  /** The transport closed. */
  close: CloseInfo;
}

type Body<N extends MessageName> = MessageBody<N>;
type WithoutId<T> = T extends { requestId: number } ? Omit<T, "requestId"> : T;

/** A response type for each request type. */
export const responseOf = {
  "agent-info-request": "agent-info-response",
  "agent-status-request": "agent-status-response",
  "presentation-url-availability-request": "presentation-url-availability-response",
  "presentation-start-request": "presentation-start-response",
  "presentation-termination-request": "presentation-termination-response",
  "presentation-connection-open-request": "presentation-connection-open-response",
  "remote-playback-availability-request": "remote-playback-availability-response",
  "remote-playback-start-request": "remote-playback-start-response",
  "remote-playback-termination-request": "remote-playback-termination-response",
  "remote-playback-modify-request": "remote-playback-modify-response",
} as const satisfies Partial<Record<RequestName, RequestName>>;
export type OutgoingRequest = keyof typeof responseOf;
export type ResponseTo<N extends OutgoingRequest> = (typeof responseOf)[N];

export interface RequestOptions<R extends MessageName> extends SendOptions {
  /** Runs synchronously when the response arrives, before later messages are dispatched. */
  onResponse?: (body: Body<R>) => void;
  /** Rejects with a TimeoutError after this many milliseconds. */
  timeout?: number;
}

interface Pending {
  resolve(v: unknown): void;
  reject(err: unknown): void;
  onResponse: ((body: never) => void) | undefined;
}

export class OspSession extends Emitter<SessionEvents> {
  readonly transport: OspTransport;
  /** The peer's agent-info, once requested with requestAgentInfo() or announced with agent-info-event. */
  remoteAgentInfo: AgentInfo | null = null;
  readonly closed: Promise<CloseInfo>;

  readonly #options: SessionOptions;
  readonly #handlers = new Map<MessageName, Set<(body: never, msg: Message) => void>>();
  readonly #pending = new Map<string, Pending>();
  #nextId = 1;
  #isClosed = false;

  constructor(transport: OspTransport, options: SessionOptions = {}) {
    super();
    this.transport = transport;
    this.#options = options;
    this.closed = transport.closed.then((info) => {
      this.#isClosed = true;
      const err = new TransportClosedError(`transport closed (${info.code}${info.reason ? `: ${info.reason}` : ""})`);
      for (const p of this.#pending.values()) p.reject(err);
      this.#pending.clear();
      this.emit("close", info);
      return info;
    });

    this.handle("agent-info-event", (b) => {
      this.remoteAgentInfo = b.agentInfo;
    });
    if (options.agentInfo) {
      const info = options.agentInfo;
      this.handle("agent-info-request", (b) => void this.send("agent-info-response", { requestId: b.requestId, agentInfo: info }).catch(() => {}));
      this.handle("agent-status-request", (b) => void this.send("agent-status-response", { requestId: b.requestId }).catch(() => {}));
    }

    transport.start({ frame: (f) => this.#receive(f), malformed: (err) => this.emit("malformed", err) });
  }

  get isClosed(): boolean {
    return this.#isClosed;
  }

  /** Allocates a request ID. */
  nextRequestId(): number {
    return this.#options.nextRequestId ? this.#options.nextRequestId() : this.#nextId++;
  }

  /** Sends a message. */
  send<N extends MessageName>(name: N, body: Body<N>, options?: SendOptions): Promise<void> {
    if (!isSupported(name)) return Promise.reject(new TypeError(`${name} is not supported`));
    let encoded: Uint8Array;
    try {
      encoded = encodeBody(name, body);
    } catch (err) {
      return Promise.reject(err);
    }
    return this.transport.send({ typeKey: typeKeyOf(name), body: encoded }, options);
  }

  /** Sends a request with a fresh request ID and resolves with the matching response. */
  request<N extends OutgoingRequest>(
    name: N,
    body: WithoutId<Body<N>>,
    options: RequestOptions<ResponseTo<N>> = {},
  ): Promise<Body<ResponseTo<N>>> {
    if (this.#isClosed) return Promise.reject(new TransportClosedError("session is closed"));
    const requestId = this.nextRequestId();
    const respName = responseOf[name];
    const key = `${typeKeyOf(respName)}:${requestId}`;
    return new Promise<Body<ResponseTo<N>>>((resolve, reject) => {
      let timer: ReturnType<typeof setTimeout> | undefined;
      const done = (): void => {
        if (timer !== undefined) clearTimeout(timer);
        this.#pending.delete(key);
      };
      this.#pending.set(key, {
        resolve: (v) => (done(), resolve(v as Body<ResponseTo<N>>)),
        reject: (e) => (done(), reject(e)),
        onResponse: options.onResponse as ((body: never) => void) | undefined,
      });
      if (options.timeout !== undefined) {
        timer = setTimeout(() => this.#pending.get(key)?.reject(new DOMException(`${name} timed out`, "TimeoutError")), options.timeout);
      }
      const full = { ...body, requestId } as Body<N>;
      this.send(name, full, options).catch((err) => this.#pending.get(key)?.reject(err));
    });
  }

  /** Handles every received message of a type. Returns a function that removes the handler. */
  handle<N extends MessageName>(name: N, fn: (body: Body<N>, msg: Message) => void): () => void {
    let set = this.#handlers.get(name);
    if (!set) this.#handlers.set(name, (set = new Set()));
    const h = fn as (body: never, msg: Message) => void;
    set.add(h);
    return () => set.delete(h);
  }

  /** Asks the peer for its agent-info. */
  async requestAgentInfo(options?: SendOptions): Promise<AgentInfo> {
    const resp = await this.request("agent-info-request", {}, options);
    this.remoteAgentInfo = resp.agentInfo;
    return resp.agentInfo;
  }

  /** Closes the transport. */
  close(info?: Partial<CloseInfo>): void {
    this.transport.close(info);
  }

  #receive(frame: Frame): void {
    if (this.#isClosed) return;
    const name = messageName(frame.typeKey);
    if (name === undefined) {
      this.emit("unknown", frame);
      if ((this.#options.onUnknownTypeKey ?? "close") === "close") {
        this.transport.close({ code: UNKNOWN_TYPE_KEY_CODE, reason: `unknown type key ${frame.typeKey}` });
      }
      return;
    }
    let msg: Message;
    try {
      msg = decodeBody(frame.typeKey, frame.body);
    } catch (err) {
      this.emit("malformed", err instanceof SchemaError || err instanceof RangeError ? err : new Error(String(err)));
      return;
    }
    if (!isSupported(msg.name)) {
      this.emit("unsupported", msg);
      return;
    }
    const body = msg.body as { requestId?: unknown };
    if (msg.name.endsWith("-response") && typeof body.requestId === "number") {
      const p = this.#pending.get(`${frame.typeKey}:${body.requestId}`);
      if (p) {
        try {
          p.onResponse?.(msg.body as never);
        } catch (err) {
          p.reject(err);
          return;
        }
        p.resolve(msg.body);
        return;
      }
      this.emit("unhandled", msg);
      return;
    }
    const set = this.#handlers.get(msg.name);
    if (!set || set.size === 0) {
      this.emit("unhandled", msg);
      return;
    }
    for (const fn of [...set]) {
      try {
        fn(msg.body as never, msg);
      } catch (err) {
        queueMicrotask(() => {
          throw err;
        });
      }
    }
  }
}
