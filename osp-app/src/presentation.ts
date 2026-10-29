// The presentation protocol (application.bs "Presentation Protocol"): a
// controller that checks URL availability, starts, reconnects to, terminates
// and talks to presentations, and a receiver that serves them to any number
// of controllers, each on its own OspSession.
//
// Ordering: every message about one presentation (start, connection
// messages, close, change, termination) is sent in the ordering group
// "presentation:<id>", so a connection message never overtakes the response
// that announced its connection-id. Availability messages are ungrouped.

import { Emitter } from "./emitter.ts";
import {
  ConnectionCloseReason,
  Result,
  TerminationReason,
  TerminationSource,
  UrlAvailability,
} from "./messages.ts";
import type { OspSession } from "./session.ts";

export type PresentationMessage = string | Uint8Array;

/** A start, reconnect or terminate request failed with this result. */
export class PresentationError extends Error {
  override name = "PresentationError";
  readonly result: Result;
  constructor(message: string, result: Result) {
    super(message);
    this.result = result;
  }
}

const groupOf = (presentationId: string): string => `presentation:${presentationId}`;

const ID_CHARS = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789";

/** A random presentation identifier: 24 ASCII alphanumerics (the Presentation API asks for at least 16). */
export function generatePresentationId(): string {
  const b = crypto.getRandomValues(new Uint8Array(24));
  return Array.from(b, (x) => ID_CHARS[x % ID_CHARS.length]).join("");
}

/** Presentation API 6.1: at least 16 ASCII characters. */
export function isValidPresentationId(id: string): boolean {
  return id.length >= 16 && /^[\x20-\x7e]+$/.test(id);
}

export type ConnectionState = "connected" | "closed" | "terminated";

export interface ConnectionCloseInfo {
  reason: ConnectionCloseReason;
  errorMessage?: string;
  /** Whether this side closed the connection. */
  local: boolean;
}

export interface ConnectionEvents {
  message: PresentationMessage;
  close: ConnectionCloseInfo;
  terminate: { source: TerminationSource; reason: TerminationReason };
  /** presentation-change-event: another connection to the presentation opened or closed. */
  change: { connectionCount: number };
}

interface ConnectionSide {
  send(m: PresentationMessage): Promise<void>;
  close(reason: ConnectionCloseReason, errorMessage?: string): Promise<void>;
  terminate(reason: TerminationReason): Promise<void>;
}

/** One presentation connection, on either side. */
export class PresentationConnection extends Emitter<ConnectionEvents> {
  readonly presentationId: string;
  readonly url: string;
  readonly connectionId: number;
  /** The number of open connections to the presentation, as last reported. */
  connectionCount: number;
  #state: ConnectionState = "connected";
  readonly #side: ConnectionSide;

  /** @internal */
  constructor(presentationId: string, url: string, connectionId: number, connectionCount: number, side: ConnectionSide) {
    super();
    this.presentationId = presentationId;
    this.url = url;
    this.connectionId = connectionId;
    this.connectionCount = connectionCount;
    this.#side = side;
  }

  get state(): ConnectionState {
    return this.#state;
  }

  /** Sends a text (string) or binary (Uint8Array) presentation message. */
  send(message: PresentationMessage): Promise<void> {
    if (this.#state !== "connected") return Promise.reject(new Error(`connection is ${this.#state}`));
    return this.#side.send(message);
  }

  /** Closes this connection without terminating the presentation. */
  async close(reason: ConnectionCloseReason = ConnectionCloseReason.closeMethodCalled, errorMessage?: string): Promise<void> {
    if (this.#state !== "connected") return;
    this.#state = "closed";
    try {
      await this.#side.close(reason, errorMessage);
    } finally {
      this.emit("close", { reason, ...(errorMessage === undefined ? {} : { errorMessage }), local: true });
    }
  }

  /** Terminates the presentation (controller: termination request; receiver: termination event). */
  terminate(reason?: TerminationReason): Promise<void> {
    return this.#side.terminate(reason ?? TerminationReason.applicationRequest);
  }

  /** @internal */
  _message(m: PresentationMessage): void {
    if (this.#state === "connected") this.emit("message", m);
  }

  /** @internal */
  _closed(info: ConnectionCloseInfo): void {
    if (this.#state !== "connected") return;
    this.#state = "closed";
    this.emit("close", info);
  }

  /** @internal */
  _terminated(source: TerminationSource, reason: TerminationReason): void {
    if (this.#state === "terminated") return;
    this.#state = "terminated";
    this.emit("terminate", { source, reason });
  }

  /** @internal */
  _change(connectionCount: number): void {
    this.connectionCount = connectionCount;
    this.emit("change", { connectionCount });
  }
}

// ---- Controller ---------------------------------------------------------

export interface AvailabilityResult {
  watchId: number;
  availabilities: UrlAvailability[];
}

export interface ControllerEvents {
  /** presentation-url-availability-event for a watch of checkAvailability(). */
  availability: { watchId: number; urls: string[]; availabilities: UrlAvailability[] };
  /** presentation-termination-event. */
  terminated: { presentationId: string; source: TerminationSource; reason: TerminationReason };
  /** A connection message or close event for a connection-id this controller doesn't know. */
  unknownConnection: number;
}

export interface StartOptions {
  /** Defaults to generatePresentationId(). */
  presentationId?: string;
  headers?: [key: string, value: string][];
  /** Milliseconds to wait for the response. */
  timeout?: number;
}

/** The controlling side of the presentation protocol, over one session. */
export class PresentationController extends Emitter<ControllerEvents> {
  readonly session: OspSession;
  readonly #connections = new Map<number, PresentationConnection>();
  readonly #watches = new Map<number, string[]>();
  #nextWatchId = 1;

  constructor(session: OspSession) {
    super();
    this.session = session;
    session.handle("presentation-url-availability-event", (b) => {
      this.emit("availability", { watchId: b.watchId, urls: this.#watches.get(b.watchId) ?? [], availabilities: b.urlAvailabilities });
    });
    session.handle("presentation-connection-message", (b) => {
      const c = this.#connections.get(b.connectionId);
      if (c) c._message(b.message);
      else this.emit("unknownConnection", b.connectionId);
    });
    session.handle("presentation-connection-close-event", (b) => {
      const c = this.#connections.get(b.connectionId);
      if (!c) return void this.emit("unknownConnection", b.connectionId);
      this.#connections.delete(b.connectionId);
      c.connectionCount = b.connectionCount;
      c._closed({ reason: b.reason, ...(b.errorMessage === undefined ? {} : { errorMessage: b.errorMessage }), local: false });
      this.#maybeEndGroup(c.presentationId);
    });
    session.handle("presentation-change-event", (b) => {
      for (const c of this.#connections.values()) if (c.presentationId === b.presentationId) c._change(b.connectionCount);
    });
    session.handle("presentation-termination-event", (b) => {
      this.#terminated(b.presentationId, b.source, b.reason);
      this.emit("terminated", { presentationId: b.presentationId, source: b.source, reason: b.reason });
    });
    void session.closed.then(() => {
      for (const c of this.#connections.values()) c._closed({ reason: ConnectionCloseReason.unrecoverableErrorWhileSendingOrReceivingMessage, local: false });
      this.#connections.clear();
    });
  }

  /** Open connections, by connection-id. */
  get connections(): ReadonlyMap<number, PresentationConnection> {
    return this.#connections;
  }

  /**
   * Asks which of urls the receiver can present. With watchDuration (in
   * microseconds, as on the wire) the receiver sends "availability" events
   * for changes until it expires.
   */
  async checkAvailability(urls: string[], options: { watchDuration?: number; timeout?: number } = {}): Promise<AvailabilityResult> {
    const watchId = this.#nextWatchId++;
    this.#watches.set(watchId, urls);
    const resp = await this.session.request(
      "presentation-url-availability-request",
      { urls, watchDuration: options.watchDuration ?? 0, watchId },
      options.timeout === undefined ? {} : { timeout: options.timeout },
    );
    if (resp.urlAvailabilities.length !== urls.length) throw new RangeError("availability response does not match the requested urls");
    return { watchId, availabilities: resp.urlAvailabilities };
  }

  /** Starts a presentation of url and returns its first connection. */
  start(url: string, options: StartOptions = {}): Promise<PresentationConnection> {
    const presentationId = options.presentationId ?? generatePresentationId();
    if (!isValidPresentationId(presentationId)) return Promise.reject(new TypeError("presentation id must be at least 16 ASCII characters"));
    return this.#open(presentationId, url, "presentation-start-request", options.headers ?? [], options.timeout);
  }

  /** Opens another connection to a running presentation (Presentation API reconnect). */
  reconnect(presentationId: string, url: string, options: { timeout?: number } = {}): Promise<PresentationConnection> {
    return this.#open(presentationId, url, "presentation-connection-open-request", [], options.timeout);
  }

  async #open(
    presentationId: string,
    url: string,
    kind: "presentation-start-request" | "presentation-connection-open-request",
    headers: [string, string][],
    timeout: number | undefined,
  ): Promise<PresentationConnection> {
    let conn: PresentationConnection | undefined;
    // The connection is registered synchronously when the response arrives,
    // so messages right behind it in the same group find it.
    const onResponse = (b: { result: Result; connectionId: number; connectionCount?: number }): void => {
      if (b.result !== Result.success) return;
      conn = this.#newConnection(presentationId, url, b.connectionId, b.connectionCount ?? 1);
    };
    const opts = { group: groupOf(presentationId), onResponse, ...(timeout === undefined ? {} : { timeout }) };
    const resp =
      kind === "presentation-start-request"
        ? await this.session.request(kind, { presentationId, url, headers }, opts)
        : await this.session.request(kind, { presentationId, url }, opts);
    if (resp.result !== Result.success || !conn) {
      throw new PresentationError(`${kind} failed with result ${resp.result}`, resp.result);
    }
    return conn;
  }

  #newConnection(presentationId: string, url: string, connectionId: number, count: number): PresentationConnection {
    const group = groupOf(presentationId);
    const conn: PresentationConnection = new PresentationConnection(presentationId, url, connectionId, count, {
      send: (message) => this.session.send("presentation-connection-message", { connectionId, message }, { group }),
      close: async (reason, errorMessage) => {
        this.#connections.delete(connectionId);
        // The CDDL requires connection-count; the prose gives the controller
        // no value to send. Send the count as the controller knows it.
        await this.session.send(
          "presentation-connection-close-event",
          { connectionId, reason, connectionCount: Math.max(0, conn.connectionCount - 1), ...(errorMessage === undefined ? {} : { errorMessage }) },
          { group },
        );
        this.#maybeEndGroup(presentationId);
      },
      terminate: (reason) => this.terminate(presentationId, reason),
    });
    this.#connections.set(connectionId, conn);
    return conn;
  }

  /** Terminates a presentation. Resolves once the receiver confirmed it. */
  async terminate(presentationId: string, reason: TerminationReason = TerminationReason.applicationRequest, options: { timeout?: number } = {}): Promise<void> {
    const resp = await this.session.request(
      "presentation-termination-request",
      { presentationId, reason },
      { group: groupOf(presentationId), ...(options.timeout === undefined ? {} : { timeout: options.timeout }) },
    );
    if (resp.result !== Result.success) throw new PresentationError(`termination failed with result ${resp.result}`, resp.result);
    this.#terminated(presentationId, TerminationSource.controller, reason);
  }

  #terminated(presentationId: string, source: TerminationSource, reason: TerminationReason): void {
    for (const [id, c] of this.#connections) {
      if (c.presentationId !== presentationId) continue;
      this.#connections.delete(id);
      c._terminated(source, reason);
    }
    void this.session.transport.endGroup(groupOf(presentationId)).catch(() => {});
  }

  #maybeEndGroup(presentationId: string): void {
    for (const c of this.#connections.values()) if (c.presentationId === presentationId) return;
    void this.session.transport.endGroup(groupOf(presentationId)).catch(() => {});
  }
}

// ---- Receiver -----------------------------------------------------------

export interface StartRequest {
  presentationId: string;
  url: string;
  headers: [key: string, value: string][];
  session: OspSession;
}

export interface ReceiverOptions {
  /** Availability of a URL. Default: setAvailability() values, else "available" for http(s) URLs, "invalid" otherwise. */
  availabilityOf?: (url: string) => UrlAvailability;
  /**
   * Loads a presentation (application.bs: respond "after either the
   * presentation URL has been fetched and loaded, or the receiver has failed
   * to do so"). Default: success.
   */
  start?: (req: StartRequest) => { result: Result; httpResponseCode?: number } | Promise<{ result: Result; httpResponseCode?: number }>;
  /** Decides a termination request. Default: success. */
  terminate?: (presentationId: string, reason: TerminationReason, session: OspSession) => Result | Promise<Result>;
}

export interface ReceiverEvents {
  /** A presentation started. */
  presentation: { presentationId: string; url: string };
  /** A connection opened (by start or reconnect). */
  connection: PresentationConnection;
  /** A presentation was terminated, by a controller or by terminate(). */
  terminated: { presentationId: string; source: TerminationSource; reason: TerminationReason };
}

interface Presentation {
  url: string;
  connections: Map<PresentationConnection, OspSession>;
}

interface Watch {
  watchId: number;
  urls: string[];
  expires: number;
  last: UrlAvailability[];
}

/** The receiving side of the presentation protocol, serving any number of controller sessions. */
export class PresentationReceiver extends Emitter<ReceiverEvents> {
  readonly #options: ReceiverOptions;
  readonly #presentations = new Map<string, Presentation>();
  readonly #availability = new Map<string, UrlAvailability>();
  readonly #watches = new Map<OspSession, Watch[]>();
  /** connection-ids are unique across all sessions of this receiver. */
  #nextConnectionId = 1;

  constructor(options: ReceiverOptions = {}) {
    super();
    this.#options = options;
  }

  /** Running presentations: id to URL. */
  get presentations(): ReadonlyMap<string, string> {
    return new Map([...this.#presentations].map(([id, p]) => [id, p.url]));
  }

  /** Serves a controller on session. Returns a function that detaches it. */
  attach(session: OspSession): () => void {
    const offs = [
      session.handle("presentation-url-availability-request", (b) => {
        const last = b.urls.map((u) => this.#availabilityOf(u));
        const watches = this.#watches.get(session) ?? [];
        if (b.watchDuration > 0) watches.push({ watchId: b.watchId, urls: b.urls, expires: Date.now() + b.watchDuration / 1000, last });
        this.#watches.set(session, watches);
        void session.send("presentation-url-availability-response", { requestId: b.requestId, urlAvailabilities: last }).catch(() => {});
      }),
      session.handle("presentation-start-request", (b) => void this.#onStart(session, b)),
      session.handle("presentation-connection-open-request", (b) => {
        const group = groupOf(b.presentationId);
        const p = this.#presentations.get(b.presentationId);
        const result = !p ? Result.invalidPresentationId : p.url !== b.url ? Result.invalidUrl : Result.success;
        if (!p || result !== Result.success) {
          void session.send("presentation-connection-open-response", { requestId: b.requestId, result, connectionId: 0, connectionCount: p?.connections.size ?? 0 }, { group }).catch(() => {});
          return;
        }
        const conn = this.#newConnection(session, b.presentationId, p);
        void session
          .send("presentation-connection-open-response", { requestId: b.requestId, result, connectionId: conn.connectionId, connectionCount: p.connections.size }, { group })
          .catch(() => {});
        this.#announceChange(b.presentationId, p, conn);
        this.emit("connection", conn);
      }),
      session.handle("presentation-connection-message", (b) => this.#find(session, b.connectionId)?._message(b.message)),
      session.handle("presentation-connection-close-event", (b) => {
        const conn = this.#find(session, b.connectionId);
        if (!conn) return;
        const p = this.#presentations.get(conn.presentationId);
        p?.connections.delete(conn);
        conn._closed({ reason: b.reason, ...(b.errorMessage === undefined ? {} : { errorMessage: b.errorMessage }), local: false });
        if (p) this.#announceChange(conn.presentationId, p, conn);
      }),
      session.handle("presentation-termination-request", (b) => void this.#onTerminate(session, b)),
    ];
    const detach = (): void => {
      for (const off of offs) off();
      this.#watches.delete(session);
      for (const p of this.#presentations.values()) {
        for (const [c, s] of p.connections) {
          if (s !== session) continue;
          p.connections.delete(c);
          c._closed({ reason: ConnectionCloseReason.unrecoverableErrorWhileSendingOrReceivingMessage, local: false });
        }
      }
    };
    void session.closed.then(detach);
    return detach;
  }

  /** Sets a URL's availability and notifies the controllers watching it. */
  setAvailability(url: string, availability: UrlAvailability): void {
    this.#availability.set(url, availability);
    const now = Date.now();
    for (const [session, watches] of this.#watches) {
      const live = watches.filter((w) => w.expires > now);
      this.#watches.set(session, live);
      for (const w of live) {
        if (!w.urls.includes(url)) continue;
        const next = w.urls.map((u) => this.#availabilityOf(u));
        if (next.every((a, i) => a === w.last[i])) continue;
        w.last = next;
        void session.send("presentation-url-availability-event", { watchId: w.watchId, urlAvailabilities: next }).catch(() => {});
      }
    }
  }

  /** Terminates a presentation from the receiver side (termination event to every connected controller). */
  async terminate(presentationId: string, reason: TerminationReason = TerminationReason.unknown): Promise<void> {
    await this.#terminate(presentationId, TerminationSource.receiver, reason, null);
  }

  #availabilityOf(url: string): UrlAvailability {
    const set = this.#availability.get(url);
    if (set !== undefined) return set;
    if (this.#options.availabilityOf) return this.#options.availabilityOf(url);
    return /^https?:\/\//.test(url) ? UrlAvailability.available : UrlAvailability.invalid;
  }

  async #onStart(session: OspSession, b: { requestId: number; presentationId: string; url: string; headers: [string, string][] }): Promise<void> {
    const group = groupOf(b.presentationId);
    let result: Result = Result.success;
    let httpResponseCode: number | undefined;
    if (!isValidPresentationId(b.presentationId) || this.#presentations.has(b.presentationId)) result = Result.invalidPresentationId;
    else if (this.#availabilityOf(b.url) !== UrlAvailability.available) result = Result.invalidUrl;
    else if (this.#options.start) {
      try {
        ({ result, httpResponseCode } = await this.#options.start({ presentationId: b.presentationId, url: b.url, headers: b.headers, session }));
      } catch {
        result = Result.unknownError;
      }
    }
    if (result !== Result.success || this.#presentations.has(b.presentationId)) {
      const r = result === Result.success ? Result.invalidPresentationId : result;
      await session.send("presentation-start-response", { requestId: b.requestId, result: r, connectionId: 0, ...(httpResponseCode === undefined ? {} : { httpResponseCode }) }, { group }).catch(() => {});
      return;
    }
    const p: Presentation = { url: b.url, connections: new Map() };
    this.#presentations.set(b.presentationId, p);
    const conn = this.#newConnection(session, b.presentationId, p);
    this.emit("presentation", { presentationId: b.presentationId, url: b.url });
    const sent = session.send(
      "presentation-start-response",
      { requestId: b.requestId, result, connectionId: conn.connectionId, ...(httpResponseCode === undefined ? {} : { httpResponseCode }) },
      { group },
    );
    this.emit("connection", conn);
    await sent.catch(() => {});
  }

  async #onTerminate(session: OspSession, b: { requestId: number; presentationId: string; reason: TerminationReason }): Promise<void> {
    const group = groupOf(b.presentationId);
    let result: Result = this.#presentations.has(b.presentationId) ? Result.success : Result.invalidPresentationId;
    if (result === Result.success && this.#options.terminate) {
      try {
        result = await this.#options.terminate(b.presentationId, b.reason, session);
      } catch {
        result = Result.unknownError;
      }
    }
    await session.send("presentation-termination-response", { requestId: b.requestId, result }, { group }).catch(() => {});
    if (result === Result.success) await this.#terminate(b.presentationId, TerminationSource.controller, b.reason, session);
  }

  /** Ends a presentation: termination event to every controller except `except`, then local cleanup. */
  async #terminate(presentationId: string, source: TerminationSource, reason: TerminationReason, except: OspSession | null): Promise<void> {
    const p = this.#presentations.get(presentationId);
    if (!p) return;
    this.#presentations.delete(presentationId);
    const group = groupOf(presentationId);
    const sessions = new Set(p.connections.values());
    if (except) sessions.add(except);
    const sends: Promise<void>[] = [];
    for (const s of sessions) {
      if (s !== except) sends.push(s.send("presentation-termination-event", { presentationId, source, reason }, { group }).catch(() => {}));
    }
    await Promise.all(sends);
    for (const s of sessions) void s.transport.endGroup(group).catch(() => {});
    for (const c of p.connections.keys()) c._terminated(source, reason);
    this.emit("terminated", { presentationId, source, reason });
  }

  #newConnection(session: OspSession, presentationId: string, p: Presentation): PresentationConnection {
    const connectionId = this.#nextConnectionId++;
    const group = groupOf(presentationId);
    const conn: PresentationConnection = new PresentationConnection(presentationId, p.url, connectionId, p.connections.size + 1, {
      send: (message) => session.send("presentation-connection-message", { connectionId, message }, { group }),
      close: async (reason, errorMessage) => {
        p.connections.delete(conn);
        await session.send(
          "presentation-connection-close-event",
          { connectionId, reason, connectionCount: p.connections.size, ...(errorMessage === undefined ? {} : { errorMessage }) },
          { group },
        );
        this.#announceChange(presentationId, p, conn);
      },
      terminate: (reason) => this.terminate(presentationId, reason),
    });
    p.connections.set(conn, session);
    return conn;
  }

  /** presentation-change-event to the other controllers with a connection to the presentation. */
  #announceChange(presentationId: string, p: Presentation, changed: PresentationConnection): void {
    const count = p.connections.size;
    const notified = new Set<OspSession>();
    for (const [c, s] of p.connections) {
      c.connectionCount = count;
      if (c === changed || notified.has(s)) continue;
      notified.add(s);
      void s.send("presentation-change-event", { presentationId, connectionCount: count }, { group: groupOf(presentationId) }).catch(() => {});
    }
  }

  #find(session: OspSession, connectionId: number): PresentationConnection | undefined {
    for (const p of this.#presentations.values()) for (const [c, s] of p.connections) if (s === session && c.connectionId === connectionId) return c;
    return undefined;
  }
}
