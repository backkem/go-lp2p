// Shared plumbing for transport implementations: holding messages and
// streams until a handler is set, and idempotent closing.

import type { TransportConnection, TransportStream } from "./types.ts";

/** A held queue of items for a nullable callback. */
class Inbox<T> {
  private items: T[] = [];
  private fn: ((item: T) => void) | null = null;

  get handler(): ((item: T) => void) | null {
    return this.fn;
  }

  set handler(fn: ((item: T) => void) | null) {
    this.fn = fn;
    while (this.fn && this.items.length > 0) {
      this.fn(this.items.shift() as T);
    }
  }

  get empty(): boolean {
    return this.items.length === 0;
  }

  push(item: T): void {
    if (this.fn && this.items.length === 0) this.fn(item);
    else this.items.push(item);
  }

  clear(): void {
    this.items = [];
  }
}

export abstract class BaseStream implements TransportStream {
  private readonly inbox = new Inbox<Uint8Array>();
  private closeHandler: (() => void) | null = null;
  private closeFired = false;
  private isClosed = false;

  readonly id: number;

  constructor(id: number) {
    this.id = id;
  }

  get closed(): boolean {
    return this.isClosed;
  }

  get onmessage(): ((message: Uint8Array) => void) | null {
    return this.inbox.handler;
  }
  set onmessage(fn: ((message: Uint8Array) => void) | null) {
    this.inbox.handler = fn;
    this.maybeFireClose();
  }

  get onclose(): (() => void) | null {
    return this.closeHandler;
  }
  set onclose(fn: (() => void) | null) {
    this.closeHandler = fn;
    this.maybeFireClose();
  }

  abstract send(message: Uint8Array): void;
  protected abstract doClose(): void;

  close(): void {
    if (this.isClosed) return;
    this.isClosed = true;
    this.doClose();
    this.maybeFireClose();
  }

  /** For implementations: deliver a received message. */
  protected deliver(message: Uint8Array): void {
    if (!this.isClosed) this.inbox.push(message);
  }

  /**
   * For implementations: the peer closed the stream or the connection went
   * away. Public so connections can close their streams; not for API users.
   */
  remoteClosed(): void {
    if (this.isClosed) return;
    this.isClosed = true;
    this.maybeFireClose();
  }

  private maybeFireClose(): void {
    // Fire close only once held messages are delivered, so nothing the peer
    // sent before closing is lost.
    if (!this.isClosed || this.closeFired || !this.closeHandler) return;
    if (!this.inbox.empty) return;
    this.closeFired = true;
    this.closeHandler();
  }
}

// Connections a page still has open when it goes away are closed, so the
// peer learns at once instead of waiting for a timeout (or, on media with no
// liveness signal like BroadcastChannel, never).
const live = new Set<BaseConnection>();
let pageHideHooked = false;
function closeOnPageHide(c: BaseConnection): void {
  live.add(c);
  if (pageHideHooked) return;
  pageHideHooked = true;
  globalThis.addEventListener?.("pagehide", () => {
    for (const conn of [...live]) conn.close("page closed");
  });
}

export abstract class BaseConnection implements TransportConnection {
  private readonly incoming = new Inbox<TransportStream>();
  private closeHandler: ((reason: string) => void) | null = null;
  private closeReason: string | null = null;
  private closeFired = false;
  protected readonly streams = new Map<number, TransportStream>();

  readonly remoteId: string;

  constructor(remoteId: string) {
    this.remoteId = remoteId;
    closeOnPageHide(this);
  }

  get closed(): boolean {
    return this.closeReason !== null;
  }

  get onstream(): ((stream: TransportStream) => void) | null {
    return this.incoming.handler;
  }
  set onstream(fn: ((stream: TransportStream) => void) | null) {
    this.incoming.handler = fn;
  }

  get onclose(): ((reason: string) => void) | null {
    return this.closeHandler;
  }
  set onclose(fn: ((reason: string) => void) | null) {
    this.closeHandler = fn;
    this.maybeFireClose();
  }

  abstract openStream(): TransportStream;
  protected abstract doClose(reason: string): void;

  close(reason = "closed locally"): void {
    if (this.closed) return;
    this.closeReason = reason;
    this.doClose(reason);
    this.teardown();
  }

  /** For implementations: a stream the peer opened. */
  protected accept(stream: TransportStream): void {
    this.streams.set(stream.id, stream);
    this.incoming.push(stream);
  }

  /** For implementations: the peer or the medium closed the connection. */
  protected remoteClosed(reason: string): void {
    if (this.closed) return;
    this.closeReason = reason;
    this.teardown();
  }

  private teardown(): void {
    live.delete(this);
    this.incoming.clear();
    for (const s of this.streams.values()) {
      if (s instanceof BaseStream) s.remoteClosed();
      else s.close();
    }
    this.streams.clear();
    this.maybeFireClose();
  }

  private maybeFireClose(): void {
    if (this.closeReason === null || this.closeFired || !this.closeHandler) return;
    this.closeFired = true;
    this.closeHandler(this.closeReason);
  }
}

export function randomId(bytes = 8): string {
  const b = new Uint8Array(bytes);
  crypto.getRandomValues(b);
  return Array.from(b, (x) => x.toString(16).padStart(2, "0")).join("");
}
