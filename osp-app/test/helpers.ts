// In-memory fakes of the two transport shapes, built only from web platform
// globals that Node has (EventTarget, MessageEvent, streams).

import { DataChannelTransport, OspSession, WebTransportTransport, type OspTransport, type SessionOptions } from "../src/index.ts";

/** One end of an in-memory RTCDataChannel pair. */
export class FakeDataChannel extends EventTarget {
  binaryType = "blob";
  readyState: "connecting" | "open" | "closing" | "closed" = "connecting";
  peer: FakeDataChannel | null = null;
  sent = 0;

  send(data: ArrayBuffer | string): void {
    if (this.readyState !== "open") throw new Error("InvalidStateError");
    this.sent++;
    const peer = this.peer;
    const copy = typeof data === "string" ? data : data.slice(0);
    setTimeout(() => {
      if (peer?.readyState === "open") peer.dispatchEvent(new MessageEvent("message", { data: copy }));
    }, 0);
  }

  close(): void {
    if (this.readyState === "closed" || this.readyState === "closing") return;
    for (const ch of [this, this.peer]) {
      if (!ch || ch.readyState === "closed") continue;
      ch.readyState = "closing";
      setTimeout(() => {
        ch.readyState = "closed";
        ch.dispatchEvent(new Event("close"));
      }, 0);
    }
  }

  static pair(): [FakeDataChannel, FakeDataChannel] {
    const a = new FakeDataChannel();
    const b = new FakeDataChannel();
    a.peer = b;
    b.peer = a;
    setTimeout(() => {
      for (const ch of [a, b]) {
        ch.readyState = "open";
        ch.dispatchEvent(new Event("open"));
      }
    }, 0);
    return [a, b];
  }
}

/** One end of an in-memory WebTransport pair: unidirectional streams only. */
export class FakeWebTransport {
  readonly ready: Promise<undefined> = Promise.resolve(undefined);
  readonly closed: Promise<{ closeCode: number; reason: string }>;
  readonly incomingUnidirectionalStreams: ReadableStream<ReadableStream<Uint8Array>>;
  peer: FakeWebTransport | null = null;
  /** Unidirectional streams opened by this end. */
  opened = 0;
  /** Splits every write into chunks of this many bytes on the way (0: no split). */
  chunkSize = 0;
  #incoming!: ReadableStreamDefaultController<ReadableStream<Uint8Array>>;
  #resolveClosed!: (info: { closeCode: number; reason: string }) => void;
  #isClosed = false;
  readonly #streams = new Set<ReadableStreamDefaultController<Uint8Array>>();

  constructor() {
    this.incomingUnidirectionalStreams = new ReadableStream({ start: (c) => void (this.#incoming = c) });
    this.closed = new Promise((r) => (this.#resolveClosed = r));
  }

  async createUnidirectionalStream(): Promise<WritableStream<Uint8Array>> {
    if (this.#isClosed) throw new Error("session closed");
    const peer = this.peer as FakeWebTransport;
    this.opened++;
    let ctrl!: ReadableStreamDefaultController<Uint8Array>;
    const readable = new ReadableStream<Uint8Array>({ start: (c) => void (ctrl = c) });
    peer.#streams.add(ctrl);
    peer.#incoming.enqueue(readable);
    const size = this.chunkSize;
    return new WritableStream<Uint8Array>({
      write(chunk) {
        try {
          if (size <= 0) ctrl.enqueue(chunk.slice());
          else for (let i = 0; i < chunk.length; i += size) ctrl.enqueue(chunk.slice(i, i + size));
        } catch {
          throw new Error("stream reset by peer");
        }
      },
      close() {
        peer.#streams.delete(ctrl);
        try {
          ctrl.close();
        } catch {
          // Cancelled by the reader or errored by close: nothing to end.
        }
      },
    });
  }

  close(info: { closeCode?: number; reason?: string } = {}): void {
    const full = { closeCode: info.closeCode ?? 0, reason: info.reason ?? "" };
    for (const t of [this, this.peer]) if (t) t.#shutdown(full);
  }

  #shutdown(info: { closeCode: number; reason: string }): void {
    if (this.#isClosed) return;
    this.#isClosed = true;
    for (const s of this.#streams) {
      try {
        s.error(new Error("session closed"));
      } catch {
        // Already closed.
      }
    }
    this.#incoming.close();
    this.#resolveClosed(info);
  }

  static pair(): [FakeWebTransport, FakeWebTransport] {
    const a = new FakeWebTransport();
    const b = new FakeWebTransport();
    a.peer = b;
    b.peer = a;
    return [a, b];
  }
}

export type Kind = "datachannel" | "webtransport";
export const kinds: Kind[] = ["datachannel", "webtransport"];

export interface Pair {
  a: OspTransport;
  b: OspTransport;
  rawA: FakeDataChannel | FakeWebTransport;
  rawB: FakeDataChannel | FakeWebTransport;
}

export function transportPair(kind: Kind): Pair {
  if (kind === "datachannel") {
    const [x, y] = FakeDataChannel.pair();
    return { a: new DataChannelTransport(x), b: new DataChannelTransport(y), rawA: x, rawB: y };
  }
  const [x, y] = FakeWebTransport.pair();
  return { a: new WebTransportTransport(x), b: new WebTransportTransport(y), rawA: x, rawB: y };
}

export function sessionPair(kind: Kind, a: SessionOptions = {}, b: SessionOptions = {}): { a: OspSession; b: OspSession; pair: Pair } {
  const pair = transportPair(kind);
  return { a: new OspSession(pair.a, a), b: new OspSession(pair.b, b), pair };
}

export const tick = (ms = 0): Promise<void> => new Promise((r) => setTimeout(r, ms));

export { hex, unhex } from "./hex.ts";

/** Resolves with the next value an emitter-style `on` produces. */
export function next<T>(on: (fn: (v: T) => void) => () => void): Promise<T> {
  return new Promise((resolve) => {
    const off = on((v) => {
      off();
      resolve(v);
    });
  });
}
