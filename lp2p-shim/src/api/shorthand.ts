// The explainer's shorthand APIs ("Shorthand APIs"):
//
//   const transports = lp2p.makeDiscoverable(options);
//   for await (const transport of transports) { ... }   // already ready
//
//   const transport = lp2p.connect(options);
//   await transport.ready;
//
// Both are thin compositions over the full API and use only what a page
// could use: makeDiscoverable() is an LP2PReceiver plus an
// LP2PQuicTransportListener on each of its connections, merged into one
// stream; connect() is an LP2PRequest plus new LP2PQuicTransport(connection).
//
// connect() returns a transport right away, like the WebTransport
// constructor, but an LP2PQuicTransport needs a connection to be
// constructed, which only exists once the user picked a peer. So connect()
// returns an LP2PShorthandTransport: the WebTransport members, forwarded to
// the LP2PQuicTransport once there is one. It is not an LP2PQuicTransport
// (instanceof fails). See README.md, "Shorthand APIs", for what this means
// for pages that mix the shorthand and the full API.

import { getConfig } from "../config.ts";
import type { LP2PConnection } from "./connection.ts";
import { LP2PReceiver, type LP2PReceiverOptions } from "./receiver.ts";
import { LP2PRequest, type LP2PRequestOptions } from "./request.ts";
import {
  LP2PQuicTransport,
  LP2PQuicTransportListener,
  WebTransportError,
  type LP2PBidirectionalStream,
  type LP2PDatagramsWritable,
  type LP2PQuicTransportInit,
  type LP2PQuicTransportListenerInit,
  type LP2PReceiveStream,
  type LP2PSendGroup,
  type LP2PSendStream,
  type WebTransportCloseInfo,
  type WebTransportConnectionStats,
  type WebTransportSendOptions,
  type WebTransportSendStreamOptions,
} from "./quictransport.ts";

export interface MakeDiscoverableOptions extends LP2PReceiverOptions, LP2PQuicTransportListenerInit {}
export interface ConnectOptions extends LP2PRequestOptions, LP2PQuicTransportInit {}

function receiverOptions(o: MakeDiscoverableOptions): LP2PReceiverOptions {
  const r: LP2PReceiverOptions = {};
  if (o.nickname !== undefined) r.nickname = o.nickname;
  if (o.allowedOrigins !== undefined) r.allowedOrigins = o.allowedOrigins;
  return r;
}

function transportOptions(o: LP2PQuicTransportInit): LP2PQuicTransportInit {
  const { requireUnreliable, congestionControl, datagramsReadableType } = o;
  const t: LP2PQuicTransportInit = {};
  if (requireUnreliable !== undefined) t.requireUnreliable = requireUnreliable;
  if (congestionControl !== undefined) t.congestionControl = congestionControl;
  if (datagramsReadableType !== undefined) t.datagramsReadableType = datagramsReadableType;
  if (o.anticipatedConcurrentIncomingBidirectionalStreams !== undefined) {
    t.anticipatedConcurrentIncomingBidirectionalStreams = o.anticipatedConcurrentIncomingBidirectionalStreams;
  }
  if (o.anticipatedConcurrentIncomingUnidirectionalStreams !== undefined) {
    t.anticipatedConcurrentIncomingUnidirectionalStreams = o.anticipatedConcurrentIncomingUnidirectionalStreams;
  }
  return t;
}

/** Extension, behind ShimConfig.quirks.exposeShorthandConnection: the hidden connection, as a property. */
function expose(target: object, connection: LP2PConnection): void {
  if (getConfig().quirks?.exposeShorthandConnection) {
    Object.defineProperty(target, "connection", { value: connection, configurable: true, enumerable: false });
  }
}

const MERGED_HIGH_WATER_MARK = 8;

/**
 * Makes the device discoverable and returns one stream of the transports
 * peers open, across all connections. Transports in it are ready.
 * Cancelling the stream stops the receiver and closes the listeners;
 * connections already made stay open until the peer closes them.
 */
export function makeDiscoverable(options: MakeDiscoverableOptions = {}): ReadableStream<LP2PQuicTransport> {
  const opts = options ?? {};
  const receiver = new LP2PReceiver(receiverOptions(opts)); // a bad allowedOrigins entry throws here
  const listenerInit = transportOptions(opts);
  const listeners = new Set<LP2PQuicTransportListener>();
  let controller!: ReadableStreamDefaultController<LP2PQuicTransport>;
  let done = false;
  let wake: (() => void) | null = null;
  const pulled = (): Promise<void> => new Promise((r) => (wake = r));

  const merged = new ReadableStream<LP2PQuicTransport>(
    {
      start: (c) => void (controller = c),
      pull: () => {
        wake?.();
        wake = null;
      },
      cancel: () => {
        done = true;
        receiver.stop();
        for (const l of listeners) l.close();
        wake?.();
      },
    },
    { highWaterMark: MERGED_HIGH_WATER_MARK },
  );

  // Moves one connection's transports into the merged stream, as fast as
  // the page reads: a listener whose queue fills refuses further sessions.
  const pump = async (listener: LP2PQuicTransportListener, connection: LP2PConnection): Promise<void> => {
    const reader = listener.incomingTransports.getReader();
    for (;;) {
      while (!done && (controller.desiredSize ?? 0) <= 0) await pulled();
      if (done) break;
      const { value, done: end } = await reader.read();
      if (end) break;
      if (done) {
        value.close();
        break;
      }
      expose(value, connection);
      controller.enqueue(value);
    }
    reader.releaseLock();
  };

  receiver.addEventListener("connection", (e) => {
    const connection = (e as Event & { connection: LP2PConnection }).connection;
    if (done) {
      connection.close();
      return;
    }
    const listener = new LP2PQuicTransportListener(connection, listenerInit);
    listeners.add(listener);
    connection.addEventListener("close", () => listeners.delete(listener));
    void pump(listener, connection);
  });
  receiver.start().catch((err: unknown) => {
    if (done) return;
    done = true;
    controller.error(err);
  });
  return merged;
}

/**
 * Asks the user to pick a peer and opens a WebTransport session with it.
 * Returns at once; ready resolves once a peer is picked and the session is
 * up, and rejects with NotAllowedError if the request fails.
 */
export function connect(options: ConnectOptions = {}): LP2PShorthandTransport {
  return new LP2PShorthandTransport(options ?? {}, token);
}

const token = Symbol("LP2PShorthandTransport");

type DatagramSetting = "incomingMaxAge" | "outgoingMaxAge" | "incomingMaxBufferedDatagrams" | "outgoingMaxBufferedDatagrams";

/** A datagram duplex stream that forwards to the transport once there is one. */
class PendingDatagrams {
  readonly #owner: LP2PShorthandTransport;
  readonly #readable: ReadableStream<Uint8Array>;
  #writable: WritableStream<Uint8Array> | null = null;
  #settings: Partial<Record<DatagramSetting, number | null>> = {};

  constructor(owner: LP2PShorthandTransport) {
    this.#owner = owner;
    let reader: ReadableStreamDefaultReader<Uint8Array> | null = null;
    this.#readable = new ReadableStream<Uint8Array>(
      {
        pull: async (c) => {
          const t = await owner.transportWhenReady();
          reader ??= t.datagrams.readable.getReader();
          const { value, done } = await reader.read();
          if (done) c.close();
          else c.enqueue(value);
        },
        cancel: (r) => reader?.cancel(r),
      },
      { highWaterMark: 0 },
    );
  }

  /** Internal: the transport exists; apply what the page set before. */
  apply(t: LP2PQuicTransport): void {
    Object.assign(t.datagrams, this.#settings);
  }

  get readable(): ReadableStream<Uint8Array> {
    return this.#readable;
  }
  get writable(): WritableStream<Uint8Array> {
    return (this.#writable ??= this.createWritable());
  }
  createWritable(options: WebTransportSendOptions = {}): WritableStream<Uint8Array> {
    let writer: WritableStreamDefaultWriter<Uint8Array> | null = null;
    const get = async (): Promise<WritableStreamDefaultWriter<Uint8Array>> => {
      const t = await this.#owner.transportWhenReady();
      return (writer ??= (t.datagrams.createWritable(options) as LP2PDatagramsWritable).getWriter());
    };
    return new WritableStream<Uint8Array>(
      {
        write: async (chunk) => (await get()).write(chunk),
        close: async () => (await get()).close(),
        abort: async (r) => writer?.abort(r),
      },
      { highWaterMark: 1 },
    );
  }
  get maxDatagramSize(): number {
    return this.#owner.transport?.datagrams.maxDatagramSize ?? 0;
  }
  #get(k: DatagramSetting): number | null {
    const t = this.#owner.transport;
    return t ? (t.datagrams[k] as number | null) : (this.#settings[k] ?? (k.endsWith("Age") ? null : 1024));
  }
  #set(k: DatagramSetting, v: number | null): void {
    const t = this.#owner.transport;
    if (t) (t.datagrams as unknown as Record<string, number | null>)[k] = v;
    else this.#settings[k] = v;
  }
  get incomingMaxAge(): number | null {
    return this.#get("incomingMaxAge");
  }
  set incomingMaxAge(v: number | null) {
    this.#set("incomingMaxAge", v);
  }
  get outgoingMaxAge(): number | null {
    return this.#get("outgoingMaxAge");
  }
  set outgoingMaxAge(v: number | null) {
    this.#set("outgoingMaxAge", v);
  }
  get incomingMaxBufferedDatagrams(): number {
    return this.#get("incomingMaxBufferedDatagrams") as number;
  }
  set incomingMaxBufferedDatagrams(v: number) {
    this.#set("incomingMaxBufferedDatagrams", v);
  }
  get outgoingMaxBufferedDatagrams(): number {
    return this.#get("outgoingMaxBufferedDatagrams") as number;
  }
  set outgoingMaxBufferedDatagrams(v: number) {
    this.#set("outgoingMaxBufferedDatagrams", v);
  }
}

function forwardStream<T>(get: () => Promise<ReadableStream<T>>): ReadableStream<T> {
  let reader: ReadableStreamDefaultReader<T> | null = null;
  return new ReadableStream<T>(
    {
      pull: async (c) => {
        reader ??= (await get()).getReader();
        const { value, done } = await reader.read();
        if (done) c.close();
        else c.enqueue(value);
      },
      cancel: (r) => reader?.cancel(r),
    },
    { highWaterMark: 0 },
  );
}

/**
 * What lp2p.connect() returns: the WebTransport members, forwarded to the
 * LP2PQuicTransport opened on the picked peer's connection. Closing it, or
 * the session ending, closes that connection too: the page has no other
 * handle on it.
 */
export class LP2PShorthandTransport {
  #transport: LP2PQuicTransport | null = null;

  #aborted: unknown = null;
  readonly #options: LP2PQuicTransportInit;
  readonly #ready: Promise<undefined>;
  readonly #closed: Promise<WebTransportCloseInfo>;
  readonly #draining: Promise<undefined>;
  readonly #transportReady: Promise<LP2PQuicTransport>;
  #fail!: (err: unknown) => void;
  readonly #datagrams: PendingDatagrams;
  readonly #incomingBidi: ReadableStream<LP2PBidirectionalStream>;
  readonly #incomingUni: ReadableStream<LP2PReceiveStream>;

  constructor(options: ConnectOptions, key?: symbol) {
    if (key !== token) throw new TypeError("Illegal constructor: use lp2p.connect()");
    this.#options = transportOptions(options);
    let attach!: (t: LP2PQuicTransport) => void;
    const transport = new Promise<LP2PQuicTransport>((resolve, reject) => {
      attach = resolve;
      this.#fail = reject;
    });
    transport.catch(() => {});
    this.#ready = transport.then((t) => t.ready);
    this.#ready.catch(() => {});
    this.#closed = transport.then((t) => t.closed);
    this.#closed.catch(() => {});
    this.#draining = transport.then(
      (t) => t.draining,
      () => undefined,
    );
    this.#transportReady = this.#ready.then(() => this.#transport as LP2PQuicTransport);
    this.#transportReady.catch(() => {});
    this.#datagrams = new PendingDatagrams(this);
    this.#incomingBidi = forwardStream(async () => (await this.transportWhenReady()).incomingBidirectionalStreams);
    this.#incomingUni = forwardStream(async () => (await this.transportWhenReady()).incomingUnidirectionalStreams);

    new LP2PRequest(options).start().then(
      (connection) => {
        if (this.#aborted !== null) {
          connection.close();
          return;
        }
        let t: LP2PQuicTransport;
        try {
          t = new LP2PQuicTransport(connection, this.#options);
        } catch (err) {
          connection.close();
          this.#fail(err);
          return;
        }
        this.#transport = t;

        expose(this, connection);
        this.#datagrams.apply(t);
        // The connection exists only for this transport.
        t.closed.then(
          () => connection.close(),
          () => connection.close(),
        );
        attach(t);
      },
      (err: unknown) => this.#fail(err),
    );
  }

  /** Internal: the LP2PQuicTransport, or null before the peer is picked. */
  get transport(): LP2PQuicTransport | null {
    return this.#transport;
  }

  /** Internal: resolves with the transport once ready. */
  transportWhenReady(): Promise<LP2PQuicTransport> {
    return this.#transportReady;
  }

  get ready(): Promise<undefined> {
    return this.#ready;
  }
  get closed(): Promise<WebTransportCloseInfo> {
    return this.#closed;
  }
  get draining(): Promise<undefined> {
    return this.#draining;
  }
  get reliability(): "pending" | "reliable-only" | "supports-unreliable" {
    return this.#transport?.reliability ?? "pending";
  }
  get congestionControl(): "default" | "throughput" | "low-latency" {
    return this.#transport?.congestionControl ?? this.#options.congestionControl ?? "default";
  }
  get protocol(): string {
    return "";
  }
  get responseHeaders(): null {
    return null;
  }
  get datagrams(): PendingDatagrams {
    return this.#datagrams;
  }
  get incomingBidirectionalStreams(): ReadableStream<LP2PBidirectionalStream> {
    return this.#incomingBidi;
  }
  get incomingUnidirectionalStreams(): ReadableStream<LP2PReceiveStream> {
    return this.#incomingUni;
  }
  async createBidirectionalStream(options?: WebTransportSendStreamOptions): Promise<LP2PBidirectionalStream> {
    return (await this.transportWhenReady()).createBidirectionalStream(options);
  }
  async createUnidirectionalStream(options?: WebTransportSendStreamOptions): Promise<LP2PSendStream> {
    return (await this.transportWhenReady()).createUnidirectionalStream(options);
  }
  /** Only once a peer is picked: a send group belongs to one LP2PQuicTransport. */
  createSendGroup(): LP2PSendGroup {
    if (!this.#transport) throw new DOMException("no peer picked yet", "InvalidStateError");
    return this.#transport.createSendGroup();
  }
  async getStats(): Promise<WebTransportConnectionStats> {
    return (await this.transportWhenReady()).getStats();
  }
  async exportKeyingMaterial(label: BufferSource, context?: BufferSource, outputLength?: number): Promise<ArrayBuffer> {
    return (await this.transportWhenReady()).exportKeyingMaterial(label, context, outputLength);
  }

  /**
   * Closes the transport and its connection. Before a peer is picked, the
   * picker stays up (LP2PRequest.start() can't be aborted), and the
   * connection is closed as soon as it exists.
   */
  close(closeInfo: WebTransportCloseInfo = {}): undefined {
    if (this.#transport) {
      this.#transport.close(closeInfo);
      return;
    }
    if (this.#aborted !== null) return;
    this.#aborted = new WebTransportError("closed before the session was established", { source: "session" });
    this.#fail(this.#aborted);
    return;
  }
}

/** The shorthand namespace, as the explainer calls it. */
export const lp2p = Object.freeze({ makeDiscoverable, connect });
