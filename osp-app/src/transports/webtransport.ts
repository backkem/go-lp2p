// OspTransport over anything shaped like WebTransport: WebTransport itself,
// LP2PQuicTransport, or a test fake. Nothing is imported; the shape below is
// all that is used.
//
// Mapping (network.bs / application.bs "Message delivery using CBOR"): OSP
// sends messages on unidirectional QUIC streams, each message framed as
// type-key varint + CBOR, several messages per stream when they must stay in
// order. Here a QUIC stream is a WebTransport unidirectional stream:
// - a message without an ordering group gets its own stream: open, write,
//   close (as openscreen-go does for every message);
// - an ordering group gets one stream, opened on its first message and
//   closed by endGroup();
// - every incoming unidirectional stream is read to its end and split into
//   messages;
// - OSP's "close the QUIC connection with application error code 404" is
//   WebTransport close({closeCode: 404, reason}).
// Bidirectional streams and datagrams are not used by the application
// protocol; incoming bidirectional streams are left alone.

import { FrameReader, encodeFrame, type Frame } from "../messages.ts";
import { TransportClosedError, type CloseInfo, type OspTransport, type SendOptions, type TransportSink } from "../transport.ts";

export interface StreamWriterLike {
  write(chunk: Uint8Array): Promise<void>;
  close(): Promise<void>;
  abort(reason?: unknown): Promise<void>;
  releaseLock(): void;
}

export interface WritableLike {
  getWriter(): StreamWriterLike;
}

export interface StreamReaderLike<T> {
  read(): Promise<{ done: true; value?: T | undefined } | { done: false; value: T }>;
  cancel(reason?: unknown): Promise<void>;
  releaseLock(): void;
}

export interface ReadableLike<T> {
  getReader(): StreamReaderLike<T>;
}

/** The subset of WebTransport / LP2PQuicTransport this transport uses. */
export interface WebTransportLike {
  readonly ready: Promise<unknown>;
  readonly closed: Promise<{ closeCode?: number; reason?: string } | undefined>;
  close(closeInfo?: { closeCode?: number; reason?: string }): void;
  createUnidirectionalStream(): Promise<WritableLike>;
  readonly incomingUnidirectionalStreams: ReadableLike<ReadableLike<Uint8Array>>;
}

interface Group {
  writer: Promise<StreamWriterLike>;
  /** The tail of the writes on this stream, to keep them in order. */
  tail: Promise<void>;
}

export class WebTransportTransport implements OspTransport {
  readonly ready: Promise<void>;
  readonly closed: Promise<CloseInfo>;
  readonly #wt: WebTransportLike;
  readonly #groups = new Map<string, Group>();
  #sink: TransportSink | null = null;
  #isClosed = false;

  constructor(wt: WebTransportLike) {
    this.#wt = wt;
    this.ready = Promise.resolve(wt.ready).then(() => undefined);
    this.ready.catch(() => {});
    this.closed = Promise.resolve(wt.closed).then(
      (info) => ({ code: info?.closeCode ?? 0, reason: info?.reason ?? "" }),
      (err: unknown) => ({ code: 0, reason: err instanceof Error ? err.message : String(err) }),
    );
    void this.closed.then(() => {
      this.#isClosed = true;
      this.#groups.clear();
    });
  }

  start(sink: TransportSink): void {
    if (this.#sink) throw new Error("transport already started");
    this.#sink = sink;
    void this.#acceptLoop();
  }

  async #acceptLoop(): Promise<void> {
    const reader = this.#wt.incomingUnidirectionalStreams.getReader();
    try {
      for (;;) {
        const { done, value } = await reader.read();
        if (done || !value) return;
        void this.#readStream(value);
      }
    } catch {
      // The session closed; closed reports it.
    } finally {
      reader.releaseLock();
    }
  }

  async #readStream(stream: ReadableLike<Uint8Array>): Promise<void> {
    const reader = stream.getReader();
    const frames = new FrameReader();
    try {
      for (;;) {
        const { done, value } = await reader.read();
        if (done) break;
        let out: Frame[];
        try {
          out = frames.push(value);
        } catch (err) {
          // Framing is lost for the rest of this stream: drop it.
          this.#sink?.malformed(err as Error);
          await reader.cancel(err).catch(() => {});
          return;
        }
        for (const f of out) this.#sink?.frame(f);
      }
      if (frames.pending) this.#sink?.malformed(new RangeError("stream ended inside a message"));
    } catch {
      // Stream reset or session closed.
    } finally {
      reader.releaseLock();
    }
  }

  async send(frame: Frame, options: SendOptions = {}): Promise<void> {
    await this.ready;
    if (this.#isClosed) throw new TransportClosedError("WebTransport session is closed");
    const bytes = encodeFrame(frame);
    if (options.group === undefined) {
      const writer = (await this.#wt.createUnidirectionalStream()).getWriter();
      await writer.write(bytes);
      await writer.close();
      return;
    }
    let g = this.#groups.get(options.group);
    if (!g) {
      const writer = this.#wt.createUnidirectionalStream().then((s) => s.getWriter());
      g = { writer, tail: writer.then(() => undefined) };
      this.#groups.set(options.group, g);
    }
    const group = g;
    const write = group.tail.then(async () => (await group.writer).write(bytes));
    group.tail = write.catch(() => {});
    return write;
  }

  async endGroup(group: string): Promise<void> {
    const g = this.#groups.get(group);
    if (!g) return;
    this.#groups.delete(group);
    await g.tail;
    await (await g.writer).close().catch(() => {});
  }

  close(info: Partial<CloseInfo> = {}): void {
    if (this.#isClosed) return;
    this.#isClosed = true;
    this.#wt.close({ closeCode: info.code ?? 0, reason: info.reason ?? "" });
  }
}
