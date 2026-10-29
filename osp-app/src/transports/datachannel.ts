// OspTransport over anything shaped like RTCDataChannel: RTCDataChannel
// itself, LP2PDataChannel, or a test fake. Nothing is imported; the shape
// below is all that is used.
//
// Mapping: one OSP message per channel message, framed as type-key varint +
// CBOR, sent as an ArrayBuffer. The channel must be reliable and ordered (the
// default), so every ordering group is trivially kept and groups are ignored.
// A channel has no close code: close() just closes the channel, and the peer
// sees code 0.

import { decodeFrame, encodeFrame, type Frame } from "../messages.ts";
import { TransportClosedError, type CloseInfo, type OspTransport, type SendOptions, type TransportSink } from "../transport.ts";

/** The subset of RTCDataChannel / LP2PDataChannel this transport uses. */
export interface DataChannelLike {
  binaryType: string;
  readonly readyState: string;
  send(data: ArrayBuffer): void;
  close(): void;
  /** "open", "close" and "message" (a MessageEvent) are used. */
  addEventListener(type: string, listener: (ev: Event) => void): void;
  removeEventListener(type: string, listener: (ev: Event) => void): void;
}

export class DataChannelTransport implements OspTransport {
  readonly ready: Promise<void>;
  readonly closed: Promise<CloseInfo>;
  readonly #channel: DataChannelLike;
  #sink: TransportSink | null = null;
  #queue: Frame[] = [];
  #isClosed = false;
  #closeInfo: CloseInfo = { code: 0, reason: "" };

  constructor(channel: DataChannelLike) {
    this.#channel = channel;
    channel.binaryType = "arraybuffer";
    this.ready = new Promise<void>((resolve, reject) => {
      if (channel.readyState === "open") return resolve();
      if (channel.readyState === "closing" || channel.readyState === "closed") return reject(new TransportClosedError("data channel is closed"));
      const onOpen = (): void => {
        channel.removeEventListener("open", onOpen);
        resolve();
      };
      channel.addEventListener("open", onOpen);
      channel.addEventListener("close", () => reject(new TransportClosedError("data channel closed before opening")));
    });
    this.ready.catch(() => {});
    this.closed = new Promise<CloseInfo>((resolve) => {
      const done = (): void => {
        this.#isClosed = true;
        resolve(this.#closeInfo);
      };
      if (channel.readyState === "closed") done();
      else channel.addEventListener("close", done);
    });
    channel.addEventListener("message", this.#onMessage);
  }

  #onMessage = (ev: Event): void => {
    const data: unknown = (ev as MessageEvent).data;
    let buf: Uint8Array;
    if (data instanceof ArrayBuffer) buf = new Uint8Array(data);
    else if (ArrayBuffer.isView(data)) buf = new Uint8Array(data.buffer, data.byteOffset, data.byteLength);
    else {
      this.#sink?.malformed(new TypeError(`data channel: expected a binary message, got ${typeof data}`));
      return;
    }
    let frame: Frame;
    try {
      frame = decodeFrame(buf);
    } catch (err) {
      this.#sink?.malformed(err as Error);
      return;
    }
    if (this.#sink) this.#sink.frame(frame);
    else this.#queue.push(frame);
  };

  start(sink: TransportSink): void {
    // Messages that arrived earlier are delivered after the current task, so
    // the caller can register handlers right after creating the session.
    queueMicrotask(() => {
      const queued = this.#queue;
      this.#queue = [];
      this.#sink = sink;
      for (const f of queued) sink.frame(f);
    });
  }

  async send(frame: Frame, _options?: SendOptions): Promise<void> {
    await this.ready;
    if (this.#isClosed || this.#channel.readyState !== "open") throw new TransportClosedError("data channel is closed");
    const bytes = encodeFrame(frame);
    this.#channel.send(bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.byteLength) as ArrayBuffer);
  }

  async endGroup(_group: string): Promise<void> {}

  close(info: Partial<CloseInfo> = {}): void {
    if (this.#isClosed) return;
    this.#closeInfo = { code: info.code ?? 0, reason: info.reason ?? "" };
    this.#channel.removeEventListener("message", this.#onMessage);
    this.#channel.close();
  }
}
