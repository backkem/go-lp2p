// LP2PQuicTransport and LP2PQuicTransportListener: the WebTransport side of
// the LP2P API, on the wire as in ../protocol/webtransport.ts.
//
// LP2PQuicTransport has the WebTransport API surface (the W3C WebTransport
// IDL), but it is not a subclass of the browser's WebTransport: that
// constructor takes a URL and connects over HTTP/3, and a script cannot set
// up its internal slots any other way. Streams are WHATWG streams, as in
// WebTransport: receive streams are byte streams.

import { AgentCapability } from "../protocol/messages.ts";
import type { CloseInfo, StreamSink, WTSession, WTStream } from "../protocol/webtransport.ts";
import { LP2PConnection, slotsOf, type ConnectionSlots } from "./connection.ts";

// ---------------------------------------------------------------- errors

export interface WebTransportErrorInit {
  source?: "stream" | "session";
  streamErrorCode?: number | null;
}

class WebTransportErrorPolyfill extends DOMException {
  readonly #source: "stream" | "session";
  readonly #code: number | null;

  constructor(message = "", options: WebTransportErrorInit = {}) {
    super(message, "WebTransportError");
    this.#source = options.source ?? "stream";
    const c = options.streamErrorCode;
    this.#code = c === undefined || c === null ? null : Math.min(Math.max(Math.round(Number(c)) || 0, 0), 0xffffffff);
  }
  get source(): "stream" | "session" {
    return this.#source;
  }
  get streamErrorCode(): number | null {
    return this.#code;
  }
}

type WTErrorCtor = new (message?: string, options?: WebTransportErrorInit) => DOMException & {
  readonly source: "stream" | "session";
  readonly streamErrorCode: number | null;
};

// Chromium's WebTransportError still takes one dictionary ({message,
// streamErrorCode}), not the spec's (message, options), so the shim always
// uses its own class, made to inherit from the browser's where there is one
// so that instanceof WebTransportError holds.
const native = (globalThis as { WebTransportError?: { prototype: object } }).WebTransportError;
if (native) Object.setPrototypeOf(WebTransportErrorPolyfill.prototype, native.prototype);

/** WebTransportError, as the WebTransport spec defines it. */
export const WebTransportError: WTErrorCtor = WebTransportErrorPolyfill;

function sessionError(message: string): Error {
  return new WebTransportError(message, { source: "session" });
}

/** The application error code a page passed to abort() or cancel(), as WebTransport reads it. */
function codeOf(reason: unknown): number {
  const c = (reason as { streamErrorCode?: unknown } | null)?.streamErrorCode;
  return reason instanceof DOMException && typeof c === "number" ? c : 0;
}

function toBytes(chunk: unknown): Uint8Array {
  if (chunk instanceof ArrayBuffer) return new Uint8Array(chunk.slice(0));
  if (ArrayBuffer.isView(chunk)) return new Uint8Array(chunk.buffer, chunk.byteOffset, chunk.byteLength).slice();
  throw new TypeError("chunk must be an ArrayBuffer or an ArrayBufferView");
}

function handled<T>(p: Promise<T>): Promise<T> {
  p.catch(() => {});
  return p;
}

// ------------------------------------------------------------------ streams

const CHUNK = 16 * 1024;

export interface WebTransportSendStreamStats {
  bytesWritten: number;
  bytesSent: number;
  bytesAcknowledged: number;
}

export interface WebTransportReceiveStreamStats {
  bytesReceived: number;
  bytesRead: number;
}

export interface WebTransportSendOptions {
  sendGroup?: LP2PSendGroup | null;
  sendOrder?: number;
}

export interface WebTransportSendStreamOptions extends WebTransportSendOptions {
  waitUntilAvailable?: boolean;
}

/** WebTransportSendGroup. Groups streams for stats; the shim has no prioritization to apply. */
export class LP2PSendGroup {
  readonly #streams = new Set<WTStream>();
  readonly #owner: LP2PQuicTransport;

  constructor() {
    const o = groupOwner;
    groupOwner = null;
    if (!o) throw new TypeError("Illegal constructor");
    this.#owner = o;
  }

  /** Internal. */
  static ownerOf(g: LP2PSendGroup): LP2PQuicTransport {
    return g.#owner;
  }
  /** Internal. */
  static add(g: LP2PSendGroup, s: WTStream): void {
    g.#streams.add(s);
  }

  getStats(): Promise<WebTransportSendStreamStats> {
    let n = 0;
    for (const s of this.#streams) n += s.bytesSent;
    return Promise.resolve({ bytesWritten: n, bytesSent: n, bytesAcknowledged: n });
  }
}
let groupOwner: LP2PQuicTransport | null = null;

/** Wire state shared by the two halves of a stream. */
class StreamHalves implements StreamSink {
  readable: ReadableByteStreamController | null = null;
  writable: WritableStreamDefaultController | null = null;
  readableDone = false;
  writableDone = false;
  readonly ws: WTStream;

  constructor(ws: WTStream) {
    this.ws = ws;
  }

  data(chunk: Uint8Array): void {
    if (this.readableDone || !this.readable) return;
    this.readable.enqueue(chunk.slice());
  }
  fin(): void {
    if (this.readableDone || !this.readable) return;
    this.readableDone = true;
    this.readable.close();
    // A pending BYOB read completes (done) only once answered with 0 bytes.
    try {
      this.readable.byobRequest?.respond(0);
    } catch {
      // no pending read
    }
  }
  reset(code: number): void {
    this.errorReadable(new WebTransportError("the peer reset the stream", { source: "stream", streamErrorCode: code }));
  }
  stopSending(code: number): void {
    // As QUIC: the sender answers STOP_SENDING with RESET_STREAM.
    this.errorWritable(new WebTransportError("the peer stopped reading", { source: "stream", streamErrorCode: code }));
    this.ws.reset(code);
  }
  abort(reason: string): void {
    const err = sessionError(reason);
    this.errorReadable(err);
    this.errorWritable(err);
  }

  errorReadable(err: unknown): void {
    if (this.readableDone || !this.readable) return;
    this.readableDone = true;
    this.readable.error(err);
  }
  errorWritable(err: unknown): void {
    if (this.writableDone || !this.writable) return;
    this.writableDone = true;
    this.writable.error(err);
  }
}

// TypeScript cannot pick the byte source overload when extending ReadableStream<R>.
const ByteStream = ReadableStream as unknown as new (source: UnderlyingByteSource) => ReadableStream<Uint8Array>;

/** WebTransportReceiveStream: a readable byte stream. */
export class LP2PReceiveStream extends ByteStream {
  readonly #halves: StreamHalves;

  constructor(halves: StreamHalves) {
    if (!(halves instanceof StreamHalves)) throw new TypeError("Illegal constructor");
    super({
      type: "bytes",
      start: (c: ReadableByteStreamController) => {
        halves.readable = c;
      },
      cancel: (reason) => {
        halves.readableDone = true;
        halves.ws.stopSending(codeOf(reason));
      },
    });
    this.#halves = halves;
  }

  getStats(): Promise<WebTransportReceiveStreamStats> {
    const h = this.#halves;
    const queued = h.readableDone || !h.readable ? 0 : Math.max(0, -(h.readable.desiredSize ?? 0));
    return Promise.resolve({ bytesReceived: h.ws.bytesReceived, bytesRead: h.ws.bytesReceived - queued });
  }
}

/** WebTransportSendStream: a writable stream of BufferSource chunks. */
export class LP2PSendStream extends WritableStream<Uint8Array> {
  readonly #halves: StreamHalves;
  readonly #transport: LP2PQuicTransport;
  #sendGroup: LP2PSendGroup | null = null;
  #sendOrder = 0;

  constructor(halves: StreamHalves, transport: LP2PQuicTransport, options: WebTransportSendOptions) {
    if (!(halves instanceof StreamHalves)) throw new TypeError("Illegal constructor");
    super({
      start: (c) => {
        halves.writable = c;
      },
      write: (chunk) => {
        const bytes = toBytes(chunk);
        if (!halves.ws.sending) throw new DOMException("stream is not writable", "InvalidStateError");
        for (let off = 0; off < bytes.length; off += CHUNK) halves.ws.send(bytes.subarray(off, off + CHUNK));
      },
      close: () => {
        halves.writableDone = true;
        halves.ws.fin();
      },
      abort: (reason) => {
        halves.writableDone = true;
        halves.ws.reset(codeOf(reason));
      },
    });
    this.#halves = halves;
    this.#transport = transport;
    this.sendGroup = options.sendGroup ?? null;
    this.sendOrder = options.sendOrder ?? 0;
  }

  get sendGroup(): LP2PSendGroup | null {
    return this.#sendGroup;
  }
  set sendGroup(g: LP2PSendGroup | null) {
    if (g !== null && (!(g instanceof LP2PSendGroup) || LP2PSendGroup.ownerOf(g) !== this.#transport)) {
      throw new DOMException("sendGroup belongs to another transport", "InvalidStateError");
    }
    this.#sendGroup = g;
    if (g) LP2PSendGroup.add(g, this.#halves.ws);
  }

  /** Stored, not applied: the shim's transports have no send scheduler to prioritize with. */
  get sendOrder(): number {
    return this.#sendOrder;
  }
  set sendOrder(v: number) {
    this.#sendOrder = Math.trunc(Number(v)) || 0;
  }

  getStats(): Promise<WebTransportSendStreamStats> {
    const n = this.#halves.ws.bytesSent;
    // Transport streams are reliable and have no acknowledgements to observe.
    return Promise.resolve({ bytesWritten: n, bytesSent: n, bytesAcknowledged: n });
  }

  /** A WebTransportWriter. Without flow control, atomicWrite() is write() and commit() does nothing. */
  override getWriter(): WritableStreamDefaultWriter<Uint8Array> & {
    atomicWrite(chunk?: unknown): Promise<undefined>;
    commit(): undefined;
  } {
    const w = super.getWriter();
    return Object.assign(w, {
      atomicWrite: (chunk?: unknown) => w.write(chunk as Uint8Array).then(() => undefined),
      commit: () => undefined,
    });
  }
}

/** WebTransportBidirectionalStream. */
export class LP2PBidirectionalStream {
  readonly #readable: LP2PReceiveStream;
  readonly #writable: LP2PSendStream;

  constructor(readable: LP2PReceiveStream, writable: LP2PSendStream) {
    if (!(readable instanceof LP2PReceiveStream) || !(writable instanceof LP2PSendStream)) {
      throw new TypeError("Illegal constructor");
    }
    this.#readable = readable;
    this.#writable = writable;
  }
  get readable(): LP2PReceiveStream {
    return this.#readable;
  }
  get writable(): LP2PSendStream {
    return this.#writable;
  }
}

// ---------------------------------------------------------------- datagrams

const DEFAULT_MAX_BUFFERED_DATAGRAMS = 1024;

/** WebTransportDatagramsWritable. */
export class LP2PDatagramsWritable extends WritableStream<Uint8Array> {
  #sendGroup: LP2PSendGroup | null;
  #sendOrder: number;

  constructor(sink: UnderlyingSink<Uint8Array>, options: WebTransportSendOptions) {
    super(sink, { highWaterMark: 1 });
    this.#sendGroup = options.sendGroup ?? null;
    this.#sendOrder = options.sendOrder ?? 0;
  }
  get sendGroup(): LP2PSendGroup | null {
    return this.#sendGroup;
  }
  set sendGroup(g: LP2PSendGroup | null) {
    this.#sendGroup = g;
  }
  get sendOrder(): number {
    return this.#sendOrder;
  }
  set sendOrder(v: number) {
    this.#sendOrder = Math.trunc(Number(v)) || 0;
  }
}

function maxAge(v: number | null): number | null {
  if (v === null) return null;
  const n = Number(v);
  if (Number.isNaN(n) || n < 0) throw new RangeError("max age must be a non-negative number or null");
  return n === 0 ? null : n;
}

/** WebTransportDatagramDuplexStream. */
export class LP2PDatagramDuplexStream {
  readonly #transport: LP2PQuicTransport;
  readonly #readable: ReadableStream<Uint8Array>;
  #controller: ReadableStreamDefaultController<Uint8Array> | ReadableByteStreamController | null = null;
  #writable: LP2PDatagramsWritable | null = null;
  #incomingMaxAge: number | null = null;
  #outgoingMaxAge: number | null = null;
  #incomingMaxBuffered = DEFAULT_MAX_BUFFERED_DATAGRAMS;
  #outgoingMaxBuffered = DEFAULT_MAX_BUFFERED_DATAGRAMS;
  #done = false;
  droppedIncoming = 0;

  constructor(transport: LP2PQuicTransport, readableType: "bytes" | undefined) {
    if (!(transport instanceof LP2PQuicTransport)) throw new TypeError("Illegal constructor");
    this.#transport = transport;
    const start = (c: ReadableStreamDefaultController<Uint8Array> | ReadableByteStreamController): void => {
      this.#controller = c;
    };
    this.#readable =
      readableType === "bytes"
        ? new ReadableStream({ type: "bytes", start })
        : new ReadableStream<Uint8Array>({ start }, { highWaterMark: 0 });
  }

  get readable(): ReadableStream<Uint8Array> {
    return this.#readable;
  }

  /** Legacy (still shipped by Chromium): one shared writable. */
  get writable(): LP2PDatagramsWritable {
    return (this.#writable ??= this.createWritable());
  }

  createWritable(options: WebTransportSendOptions = {}): LP2PDatagramsWritable {
    const t = this.#transport;
    return new LP2PDatagramsWritable(
      {
        write: async (chunk) => {
          const bytes = toBytes(chunk);
          await internalsOf(t).ready;
          const ws = internalsOf(t).session;
          if (!ws || ws.state !== "open") throw new DOMException("transport is closed", "InvalidStateError");
          // As WebTransport: a datagram larger than maxDatagramSize is dropped silently.
          if (bytes.length > ws.maxDatagramSize) return;
          ws.sendDatagram(bytes);
        },
      },
      options,
    );
  }

  /** 0 until the transport is ready, then what fits one datagram of the underlying connection. */
  get maxDatagramSize(): number {
    return internalsOf(this.#transport).session?.maxDatagramSize ?? 0;
  }

  /** Stored; the shim does not expire datagrams. */
  get incomingMaxAge(): number | null {
    return this.#incomingMaxAge;
  }
  set incomingMaxAge(v: number | null) {
    this.#incomingMaxAge = maxAge(v);
  }
  get outgoingMaxAge(): number | null {
    return this.#outgoingMaxAge;
  }
  set outgoingMaxAge(v: number | null) {
    this.#outgoingMaxAge = maxAge(v);
  }
  get incomingMaxBufferedDatagrams(): number {
    return this.#incomingMaxBuffered;
  }
  set incomingMaxBufferedDatagrams(v: number) {
    this.#incomingMaxBuffered = Math.max(0, Math.trunc(Number(v)) || 0);
  }
  get outgoingMaxBufferedDatagrams(): number {
    return this.#outgoingMaxBuffered;
  }
  set outgoingMaxBufferedDatagrams(v: number) {
    this.#outgoingMaxBuffered = Math.max(0, Math.trunc(Number(v)) || 0);
  }
  /** Legacy names. */
  get incomingHighWaterMark(): number {
    return this.incomingMaxBufferedDatagrams;
  }
  set incomingHighWaterMark(v: number) {
    this.incomingMaxBufferedDatagrams = v;
  }
  get outgoingHighWaterMark(): number {
    return this.outgoingMaxBufferedDatagrams;
  }
  set outgoingHighWaterMark(v: number) {
    this.outgoingMaxBufferedDatagrams = v;
  }

  /** Internal: a datagram arrived. */
  receive(d: Uint8Array): void {
    const c = this.#controller;
    if (this.#done || !c) return;
    // Like WebTransport, drop when the page is not keeping up.
    if (-(c.desiredSize ?? 0) >= this.#incomingMaxBuffered) {
      this.droppedIncoming++;
      return;
    }
    c.enqueue(d.slice());
  }

  /** Internal: the session ended. */
  finish(err: unknown | null): void {
    if (this.#done || !this.#controller) return;
    this.#done = true;
    if (err === null) this.#controller.close();
    else this.#controller.error(err);
  }
}

// ---------------------------------------------------------------- transport

export interface LP2PQuicTransportInit {
  /** Fail unless datagrams can be lost, i.e. the session is not "reliable-only". */
  requireUnreliable?: boolean;
  /** Accepted and reflected; the shim has no congestion controller to choose. */
  congestionControl?: "default" | "throughput" | "low-latency";
  anticipatedConcurrentIncomingUnidirectionalStreams?: number | null;
  anticipatedConcurrentIncomingBidirectionalStreams?: number | null;
  /** Extension (from WebTransportOptions): "bytes" makes datagrams.readable a byte stream. */
  datagramsReadableType?: "bytes";
}

/** The options of a listener apply to each transport it creates. */
export interface LP2PQuicTransportListenerInit extends LP2PQuicTransportInit {}

export interface WebTransportCloseInfo {
  closeCode?: number;
  reason?: string;
}

export interface WebTransportConnectionStats {
  bytesSent: number;
  bytesReceived: number;
  datagrams: { droppedIncoming: number; expiredIncoming: number; expiredOutgoing: number; lostOutgoing: number };
  estimatedSendRate: number | null;
  atSendCapacity: boolean;
}

interface TransportInternals {
  session: WTSession | null;
  ready: Promise<undefined>;
}
const transportInternals = new WeakMap<LP2PQuicTransport, TransportInternals>();
function internalsOf(t: LP2PQuicTransport): TransportInternals {
  return transportInternals.get(t) as TransportInternals;
}

function enforceUnsignedShortOrNull(v: unknown, what: string): number | null {
  if (v === null || v === undefined) return null;
  const n = Math.trunc(Number(v));
  if (!Number.isFinite(Number(v)) || n < 0 || n > 0xffff) throw new TypeError(`${what} is outside the unsigned short range`);
  return n;
}

function bufferSource(v: unknown, what: string): Uint8Array {
  if (v instanceof ArrayBuffer) return new Uint8Array(v);
  if (ArrayBuffer.isView(v)) return new Uint8Array(v.buffer, v.byteOffset, v.byteLength);
  throw new TypeError(`${what} must be a BufferSource`);
}

const CONGESTION_CONTROL = ["default", "throughput", "low-latency"];
const MAX_REASON_BYTES = 1024;
const EXPORTER_LENGTH = 32;

let adopting: { session: WTSession; init: LP2PQuicTransportInit; connection: LP2PConnection } | null = null;

/** Wraps an accepted incoming session. Internal. */
function adoptTransport(session: WTSession, init: LP2PQuicTransportInit, connection: LP2PConnection): LP2PQuicTransport {
  adopting = { session, init, connection };
  try {
    return new LP2PQuicTransport(connection, init);
  } finally {
    adopting = null;
  }
}

type State = "connecting" | "connected" | "closed" | "failed";

export class LP2PQuicTransport {
  #state: State = "connecting";
  #session: WTSession | null = null;
  #slots: ConnectionSlots;
  readonly #connection: LP2PConnection;
  #reliability: "pending" | "reliable-only" | "supports-unreliable" = "pending";
  readonly #congestionControl: "default" | "throughput" | "low-latency";
  readonly #requireUnreliable: boolean;
  #anticipatedUni: number | null;
  #anticipatedBidi: number | null;
  readonly #ready: Promise<undefined>;
  #resolveReady!: () => void;
  #rejectReady!: (e: unknown) => void;
  readonly #closed: Promise<WebTransportCloseInfo>;
  #resolveClosed!: (i: WebTransportCloseInfo) => void;
  #rejectClosed!: (e: unknown) => void;
  readonly #draining: Promise<undefined>;
  #resolveDraining!: () => void;
  readonly #datagrams: LP2PDatagramDuplexStream;
  #incomingBidiController!: ReadableStreamDefaultController<LP2PBidirectionalStream>;
  #incomingUniController!: ReadableStreamDefaultController<LP2PReceiveStream>;
  readonly #incomingBidi: ReadableStream<LP2PBidirectionalStream>;
  readonly #incomingUni: ReadableStream<LP2PReceiveStream>;
  readonly #member = { failFromConnection: () => this.#fail(sessionError("the LP2P connection closed")) };

  /** Always true: some shim transports only give reliable-only sessions. */
  static get supportsReliableOnly(): boolean {
    return true;
  }

  /**
   * Opens a new WebTransport session on connection. Each construction opens
   * its own session; the peer receives it through its
   * LP2PQuicTransportListener on the connection.
   */
  constructor(connection: LP2PConnection, options: LP2PQuicTransportInit = {}) {
    const a = adopting;
    adopting = null;
    if (!a) {
      if (arguments.length < 1) throw new TypeError("LP2PQuicTransport: connection is required");
      if (!(connection instanceof LP2PConnection)) throw new TypeError("LP2PQuicTransport: connection must be an LP2PConnection");
    }
    const slots = slotsOf(connection);
    if (!a && slots.closed) throw new DOMException("the LP2PConnection is closed", "InvalidStateError");
    this.#slots = slots;
    this.#connection = connection;
    const init = options ?? {};
    const cc = init.congestionControl ?? "default";
    if (!CONGESTION_CONTROL.includes(cc)) throw new TypeError(`invalid congestionControl "${String(cc)}"`);
    if (init.datagramsReadableType !== undefined && init.datagramsReadableType !== "bytes") {
      throw new TypeError(`invalid datagramsReadableType "${String(init.datagramsReadableType)}"`);
    }
    this.#congestionControl = cc;
    this.#requireUnreliable = Boolean(init.requireUnreliable);
    this.#anticipatedUni = enforceUnsignedShortOrNull(
      init.anticipatedConcurrentIncomingUnidirectionalStreams,
      "anticipatedConcurrentIncomingUnidirectionalStreams",
    );
    this.#anticipatedBidi = enforceUnsignedShortOrNull(
      init.anticipatedConcurrentIncomingBidirectionalStreams,
      "anticipatedConcurrentIncomingBidirectionalStreams",
    );

    this.#ready = handled(
      new Promise<undefined>((resolve, reject) => {
        this.#resolveReady = () => resolve(undefined);
        this.#rejectReady = reject;
      }),
    );
    this.#closed = handled(
      new Promise<WebTransportCloseInfo>((resolve, reject) => {
        this.#resolveClosed = resolve;
        this.#rejectClosed = reject;
      }),
    );
    this.#draining = new Promise<undefined>((resolve) => {
      this.#resolveDraining = () => resolve(undefined);
    });
    this.#datagrams = new LP2PDatagramDuplexStream(this, init.datagramsReadableType);
    this.#incomingBidi = new ReadableStream<LP2PBidirectionalStream>(
      { start: (c) => void (this.#incomingBidiController = c) },
      { highWaterMark: 0 },
    );
    this.#incomingUni = new ReadableStream<LP2PReceiveStream>(
      { start: (c) => void (this.#incomingUniController = c) },
      { highWaterMark: 0 },
    );
    transportInternals.set(this, { session: null, ready: this.#ready });
    slots.transports.add(this.#member);

    if (a) {
      this.#attach(a.session);
      return;
    }
    const session = slots.session;
    // An unknown type key would close the whole connection: only open
    // sessions with agents that list quic-transport.
    if (!session.remoteInfo?.capabilities.includes(AgentCapability.QuicTransport)) {
      queueMicrotask(() => this.#fail(sessionError("the peer does not support quic-transport")));
      return;
    }
    let out: WTSession;
    try {
      out = session.openTransport();
    } catch (err) {
      queueMicrotask(() => this.#fail(sessionError(String(err))));
      return;
    }
    this.#session = out; // so close() while connecting can abandon it
    out.onready = () => this.#attach(out);
    out.onclose = (_info, why) => this.#fail(sessionError(why));
  }

  #attach(ws: WTSession): void {
    if (this.#state !== "connecting") {
      ws.onclose = null;
      ws.close();
      return;
    }
    this.#session = ws;
    internalsOf(this).session = ws;
    const reliability = ws.datagramsReliable || ws.maxDatagramSize === 0 ? "reliable-only" : "supports-unreliable";
    if (this.#requireUnreliable && reliability !== "supports-unreliable") {
      this.#fail(new WebTransportError("requireUnreliable: this session is reliable-only", { source: "session" }));
      return;
    }
    this.#reliability = reliability;
    this.#state = "connected";
    ws.onclose = (info, why) => this.#cleanup(info, why);
    ws.ondrain = () => this.#resolveDraining();
    ws.onstream = (s) => this.#incoming(s);
    ws.ondatagram = (d) => this.#datagrams.receive(d);
    this.#resolveReady();
  }

  #incoming(s: WTStream): void {
    const halves = new StreamHalves(s);
    const readable = new LP2PReceiveStream(halves);
    if (s.unidirectional) {
      s.attach(halves);
      this.#incomingUniController.enqueue(readable);
      return;
    }
    const writable = new LP2PSendStream(halves, this, {});
    s.attach(halves);
    this.#incomingBidiController.enqueue(new LP2PBidirectionalStream(readable, writable));
  }

  #fail(err: unknown): void {
    if (this.#state === "closed" || this.#state === "failed") return;
    const ws = this.#session;
    this.#state = "failed";
    this.#reliability = "pending";
    this.#slots.transports.delete(this.#member);
    if (ws) {
      ws.onclose = null;
      ws.close();
    }
    this.#rejectReady(err);
    this.#rejectClosed(err);
    this.#incomingBidiController.error(err);
    this.#incomingUniController.error(err);
    this.#datagrams.finish(err);
    this.#resolveDraining();
  }

  #cleanup(info: CloseInfo | null, why: string): void {
    if (this.#state === "closed" || this.#state === "failed") return;
    if (info === null) {
      this.#fail(sessionError(why));
      return;
    }
    this.#state = "closed";
    this.#slots.transports.delete(this.#member);
    this.#resolveClosed({ closeCode: info.code, reason: info.reason });
    this.#incomingBidiController.close();
    this.#incomingUniController.close();
    this.#datagrams.finish(null);
    this.#resolveDraining();
  }

  /** [[Connection]]: the LP2PConnection this transport's session is on. */
  get connection(): LP2PConnection {
    return this.#connection;
  }
  get ready(): Promise<undefined> {
    return this.#ready;
  }
  get closed(): Promise<WebTransportCloseInfo> {
    return this.#closed;
  }
  /** Resolves when the peer drains the session (data-transport-drain), or when it ends. */
  get draining(): Promise<undefined> {
    return this.#draining;
  }
  get reliability(): "pending" | "reliable-only" | "supports-unreliable" {
    return this.#reliability;
  }
  get congestionControl(): "default" | "throughput" | "low-latency" {
    return this.#congestionControl;
  }
  get anticipatedConcurrentIncomingUnidirectionalStreams(): number | null {
    return this.#anticipatedUni;
  }
  set anticipatedConcurrentIncomingUnidirectionalStreams(v: number | null) {
    this.#anticipatedUni = enforceUnsignedShortOrNull(v, "anticipatedConcurrentIncomingUnidirectionalStreams");
  }
  get anticipatedConcurrentIncomingBidirectionalStreams(): number | null {
    return this.#anticipatedBidi;
  }
  set anticipatedConcurrentIncomingBidirectionalStreams(v: number | null) {
    this.#anticipatedBidi = enforceUnsignedShortOrNull(v, "anticipatedConcurrentIncomingBidirectionalStreams");
  }
  /** Always null: there is no HTTP exchange. */
  get responseHeaders(): null {
    return null;
  }
  /** Always "": there is no subprotocol negotiation. */
  get protocol(): string {
    return "";
  }
  get datagrams(): LP2PDatagramDuplexStream {
    return this.#datagrams;
  }
  get incomingBidirectionalStreams(): ReadableStream<LP2PBidirectionalStream> {
    return this.#incomingBidi;
  }
  get incomingUnidirectionalStreams(): ReadableStream<LP2PReceiveStream> {
    return this.#incomingUni;
  }

  async #openStream(unidirectional: boolean): Promise<StreamHalves> {
    if (this.#state === "connecting") await this.#ready;
    const ws = this.#session;
    if (this.#state !== "connected" || !ws) throw new DOMException("transport is closed", "InvalidStateError");
    return new StreamHalves(ws.openStream(unidirectional));
  }

  async createBidirectionalStream(options: WebTransportSendStreamOptions = {}): Promise<LP2PBidirectionalStream> {
    const halves = await this.#openStream(false);
    const readable = new LP2PReceiveStream(halves);
    const writable = new LP2PSendStream(halves, this, options ?? {});
    halves.ws.attach(halves);
    return new LP2PBidirectionalStream(readable, writable);
  }

  async createUnidirectionalStream(options: WebTransportSendStreamOptions = {}): Promise<LP2PSendStream> {
    const halves = await this.#openStream(true);
    const writable = new LP2PSendStream(halves, this, options ?? {});
    halves.ws.attach(halves);
    return writable;
  }

  createSendGroup(): LP2PSendGroup {
    groupOwner = this;
    return new LP2PSendGroup();
  }

  /** Counts payload bytes; RTT and packet counts are not observable through the shim's transports. */
  getStats(): Promise<WebTransportConnectionStats> {
    const t = this.#session?.totals() ?? { sent: 0, received: 0 };
    return Promise.resolve({
      bytesSent: t.sent,
      bytesReceived: t.received,
      datagrams: { droppedIncoming: this.#datagrams.droppedIncoming, expiredIncoming: 0, expiredOutgoing: 0, lostOutgoing: 0 },
      estimatedSendRate: null,
      atSendCapacity: false,
    });
  }

  /**
   * Keying material of this session only, derived as the binding says (the
   * connection's exporter with label "EXPORTER-WebTransport" and the
   * WebTransport Exporter Context, with the exchange-id as session ID). In
   * the shim the connection's exporter is the handshake stand-in of
   * protocol/identity.ts, not TLS. Extension: outputLength, default 32.
   */
  async exportKeyingMaterial(label: BufferSource, context?: BufferSource, outputLength = EXPORTER_LENGTH): Promise<ArrayBuffer> {
    const l = bufferSource(label, "label");
    const c = context === undefined ? new Uint8Array(0) : bufferSource(context, "context");
    if (l.length > 255 || c.length > 255) throw new TypeError("label and context are at most 255 bytes");
    const n = Math.trunc(Number(outputLength));
    if (!(n > 0 && n <= 255 * 32)) throw new RangeError("outputLength out of range");
    if (this.#state === "connecting") await this.#ready;
    const ws = this.#session;
    if (this.#state !== "connected" || !ws) throw new DOMException("transport is closed", "InvalidStateError");
    const out = await ws.exportKeyingMaterial(l, c, n);
    return out.buffer.slice(out.byteOffset, out.byteOffset + out.byteLength) as ArrayBuffer;
  }

  close(closeInfo: WebTransportCloseInfo = {}): undefined {
    if (this.#state === "closed" || this.#state === "failed") return;
    if (this.#state === "connecting") {
      this.#fail(new WebTransportError("closed before the session was established", { source: "session" }));
      return;
    }
    const code = Math.min(Math.max(Math.trunc(Number(closeInfo?.closeCode ?? 0)) || 0, 0), 0xffffffff);
    // WebTransport truncates the reason to 1024 bytes of UTF-8.
    let reason = String(closeInfo?.reason ?? "");
    const enc = new TextEncoder();
    while (enc.encode(reason).length > MAX_REASON_BYTES) reason = [...reason].slice(0, -1).join("");
    this.#session?.close(code, reason);
    return;
  }
}

// ----------------------------------------------------------------- listener

/** How many incoming transports a listener queues before refusing more. */
const LISTENER_HIGH_WATER_MARK = 8;

export class LP2PQuicTransportListener {
  readonly #incoming: ReadableStream<LP2PQuicTransport>;
  #controller!: ReadableStreamDefaultController<LP2PQuicTransport>;
  #closed = false;
  #streamDone = false;
  readonly #slots: ConnectionSlots;

  /**
   * Receives the incoming WebTransport sessions of connection. There is at
   * most one listener per connection at a time; with none, or with a full
   * queue, incoming sessions are refused.
   */
  constructor(connection: LP2PConnection, options: LP2PQuicTransportListenerInit = {}) {
    if (arguments.length < 1) throw new TypeError("LP2PQuicTransportListener: connection is required");
    if (!(connection instanceof LP2PConnection)) {
      throw new TypeError("LP2PQuicTransportListener: connection must be an LP2PConnection");
    }
    const slots = slotsOf(connection);
    if (slots.closed) throw new DOMException("the LP2PConnection is closed", "InvalidStateError");
    if (slots.listener) throw new DOMException("the LP2PConnection already has a listener", "InvalidStateError");
    const init: LP2PQuicTransportListenerInit = { ...(options ?? {}) };
    // Validates the options as the transports will.
    if (init.congestionControl !== undefined && !CONGESTION_CONTROL.includes(init.congestionControl)) {
      throw new TypeError(`invalid congestionControl "${String(init.congestionControl)}"`);
    }
    this.#slots = slots;
    this.#incoming = new ReadableStream<LP2PQuicTransport>(
      {
        start: (c) => void (this.#controller = c),
        cancel: () => {
          this.#streamDone = true;
          this.close();
        },
      },
      { highWaterMark: LISTENER_HIGH_WATER_MARK },
    );
    slots.listener = {
      closeFromConnection: () => this.close(),
      incoming: (t) => {
        if (this.#closed || (this.#controller.desiredSize ?? 0) <= 0) return false;
        if (init.requireUnreliable && (t.datagramsReliable || t.maxDatagramSize === 0)) return false;
        t.accept();
        this.#controller.enqueue(adoptTransport(t, init, connection));
        return true;
      },
    };
  }

  get incomingTransports(): ReadableStream<LP2PQuicTransport> {
    return this.#incoming;
  }

  /** Stops listening. Transports already queued can still be read; a new listener can then be constructed. */
  close(): undefined {
    if (this.#closed) return;
    this.#closed = true;
    this.#slots.listener = null;
    if (!this.#streamDone) {
      this.#streamDone = true;
      try {
        this.#controller.close();
      } catch {
        // already closed or errored
      }
    }
    return;
  }
}
