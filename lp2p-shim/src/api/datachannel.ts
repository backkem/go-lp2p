// LP2PDataChannel, kept as close to RTCDataChannel as the LP2P spec intends.

import { DataEncoding } from "../protocol/messages.ts";
import type { DataChannelEndpoint } from "../protocol/session.ts";
import { getHandler, setHandler } from "./events.ts";

export interface LP2PDataChannelInit {
  protocol?: string;
  id?: number;
}

/** Not in the LP2P IDL (yet); same values as RTCDataChannelState. */
export type LP2PDataChannelState = "connecting" | "open" | "closing" | "closed";

const textDecoder = new TextDecoder();
const textEncoder = new TextEncoder();

let adopting: { endpoint: DataChannelEndpoint; incoming: boolean } | null = null;

/** Creates the API object for a session-layer channel. Internal. */
export function adoptChannel(endpoint: DataChannelEndpoint, incoming: boolean): LP2PDataChannel {
  adopting = { endpoint, incoming };
  try {
    const p = endpoint.params;
    return new LP2PDataChannel(p.label, { protocol: p.protocol, id: p.id });
  } finally {
    adopting = null;
  }
}

/** WebIDL [EnforceRange] unsigned short. */
export function enforceUnsignedShort(v: unknown, what: string): number {
  const n = Number(v);
  if (!Number.isFinite(n)) throw new TypeError(`${what} is not a finite number`);
  const t = Math.trunc(n);
  if (t < 0 || t > 0xffff) throw new TypeError(`${what} is outside the unsigned short range`);
  return t;
}

export class LP2PDataChannel extends EventTarget {
  readonly #label: string;
  readonly #protocol: string;
  readonly #id: number | null;
  #readyState: LP2PDataChannelState;
  #binaryType: BinaryType = "arraybuffer";
  readonly #endpoint: DataChannelEndpoint | null;
  #sendChain: Promise<void> | null = null;

  /**
   * The IDL gives LP2PDataChannel a public constructor, but says nothing
   * about what a channel made that way is attached to. The shim lets you
   * construct one, and it is permanently "closed": channels that carry data
   * come from LP2PConnection.createDataChannel() or the datachannel event.
   */
  constructor(label: string, dataChannelDict: LP2PDataChannelInit = {}) {
    super();
    if (arguments.length < 1) throw new TypeError("LP2PDataChannel: label is required");
    this.#label = String(label);
    this.#protocol = String(dataChannelDict.protocol ?? "");
    this.#id = dataChannelDict.id === undefined ? null : enforceUnsignedShort(dataChannelDict.id, "id");
    const a = adopting;
    adopting = null;
    if (!a) {
      this.#endpoint = null;
      this.#readyState = "closed";
      return;
    }
    const ep = a.endpoint;
    this.#endpoint = ep;
    this.#readyState = ep.state === "open" ? "open" : "connecting";
    ep.onopen = () => {
      if (this.#readyState !== "connecting") return;
      this.#readyState = "open";
      this.dispatchEvent(new Event("open"));
    };
    ep.onframe = (enc, payload) => this.#receive(enc, payload);
    ep.onerror = (err) => {
      const ev = new Event("error");
      Object.defineProperty(ev, "error", { value: err });
      this.dispatchEvent(ev);
    };
    ep.onclose = () => this.#closed();
    if (a.incoming) {
      // Like RTCDataChannel: open also fires on channels the peer opened,
      // right after the datachannel event that delivered them.
      queueMicrotask(() => {
        if (this.#readyState === "open") this.dispatchEvent(new Event("open"));
      });
    }
  }

  get label(): string {
    return this.#label;
  }
  get protocol(): string {
    return this.#protocol;
  }
  get id(): number | null {
    return this.#id;
  }
  /** Extension: not in the LP2P IDL, but needed to use the channel safely. */
  get readyState(): LP2PDataChannelState {
    return this.#readyState;
  }

  get binaryType(): BinaryType {
    return this.#binaryType;
  }
  set binaryType(v: BinaryType) {
    // An invalid enum value assigned to an IDL attribute is ignored.
    if (v === "blob" || v === "arraybuffer") this.#binaryType = v;
  }

  get onopen(): ((this: EventTarget, ev: Event) => unknown) | null {
    return getHandler(this, "open");
  }
  set onopen(fn) {
    setHandler(this, "open", fn);
  }
  get onerror(): ((this: EventTarget, ev: Event) => unknown) | null {
    return getHandler(this, "error");
  }
  set onerror(fn) {
    setHandler(this, "error", fn);
  }
  get onclosing(): ((this: EventTarget, ev: Event) => unknown) | null {
    return getHandler(this, "closing");
  }
  set onclosing(fn) {
    setHandler(this, "closing", fn);
  }
  get onclose(): ((this: EventTarget, ev: Event) => unknown) | null {
    return getHandler(this, "close");
  }
  set onclose(fn) {
    setHandler(this, "close", fn);
  }
  get onmessage(): ((this: EventTarget, ev: MessageEvent) => unknown) | null {
    return getHandler(this, "message") as ((this: EventTarget, ev: MessageEvent) => unknown) | null;
  }
  set onmessage(fn) {
    setHandler(this, "message", fn);
  }

  /**
   * Sends data. Strings go as encoding-id string, ArrayBuffer and views as
   * array-buffer, Blobs as blob. Blobs are read asynchronously; messages
   * sent after a Blob wait for it, so order is kept.
   */
  send(data: string | Blob | ArrayBuffer | ArrayBufferView): undefined {
    if (this.#readyState !== "open" || !this.#endpoint) {
      throw new DOMException(`LP2PDataChannel is ${this.#readyState}`, "InvalidStateError");
    }
    const ep = this.#endpoint;
    let frame: Promise<[DataEncoding, Uint8Array]> | [DataEncoding, Uint8Array];
    if (typeof data === "string") {
      frame = [DataEncoding.String, textEncoder.encode(data)];
    } else if (data instanceof ArrayBuffer) {
      frame = [DataEncoding.ArrayBuffer, new Uint8Array(data.slice(0))];
    } else if (ArrayBuffer.isView(data)) {
      frame = [DataEncoding.ArrayBuffer, new Uint8Array(data.buffer, data.byteOffset, data.byteLength).slice()];
    } else if (typeof Blob !== "undefined" && data instanceof Blob) {
      frame = data.arrayBuffer().then((b) => [DataEncoding.Blob, new Uint8Array(b)]);
    } else {
      frame = [DataEncoding.String, textEncoder.encode(String(data))];
    }
    if (!(frame instanceof Promise) && !this.#sendChain) {
      ep.send(frame[0], frame[1]);
      return;
    }
    const pending = Promise.resolve(frame);
    const chain = (this.#sendChain ?? Promise.resolve()).then(async () => {
      const [enc, payload] = await pending;
      if (ep.state === "open") ep.send(enc, payload);
    });
    this.#sendChain = chain;
    chain.then(
      () => {
        if (this.#sendChain === chain) this.#sendChain = null;
      },
      () => {
        if (this.#sendChain === chain) this.#sendChain = null;
        this.dispatchEvent(new Event("error"));
      },
    );
  }

  /**
   * Closes the channel. readyState becomes "closing" and closing fires (as
   * in go-lp2p); close fires once the channel is closed. Messages queued
   * behind a Blob are sent first.
   */
  close(): undefined {
    if (this.#readyState === "closing" || this.#readyState === "closed") return;
    this.#readyState = "closing";
    queueMicrotask(() => this.dispatchEvent(new Event("closing")));
    const ep = this.#endpoint;
    void (this.#sendChain ?? Promise.resolve()).finally(() => ep?.close());
  }

  #receive(enc: number, payload: Uint8Array): void {
    if (this.#readyState !== "open") return;
    let data: string | ArrayBuffer | Blob;
    if (enc === DataEncoding.String) data = textDecoder.decode(payload);
    else if (this.#binaryType === "blob") data = new Blob([payload as Uint8Array<ArrayBuffer>]);
    else data = payload.slice().buffer;
    this.dispatchEvent(new MessageEvent("message", { data }));
  }

  #closed(): void {
    if (this.#readyState === "closed") return;
    const wasClosing = this.#readyState === "closing";
    this.#readyState = "closing";
    // Queue, so that close() never dispatches synchronously and closing
    // always precedes close.
    queueMicrotask(() => {
      if (!wasClosing) this.dispatchEvent(new Event("closing"));
      this.#readyState = "closed";
      this.dispatchEvent(new Event("close"));
    });
  }
}
