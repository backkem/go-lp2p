// LP2PConnection: an accepted session with one peer.

import type { Session } from "../protocol/session.ts";
import type { WTSession } from "../protocol/webtransport.ts";
import { adoptChannel, enforceUnsignedShort, type LP2PDataChannel, type LP2PDataChannelInit } from "./datachannel.ts";
import { LP2PDataChannelEvent, getHandler, setHandler } from "./events.ts";

/** What quictransport.ts needs of a connection: its [[Transports]] and [[Listener]] slots. Internal. */
export interface ConnectionSlots {
  readonly session: Session;
  /** [[Transports]]: failed when the connection closes. */
  readonly transports: Set<{ failFromConnection(): void }>;
  /** [[Listener]] */
  listener: { closeFromConnection(): void; incoming(t: WTSession): boolean } | null;
  /** [[Closed]] */
  closed: boolean;
}

let adopting: Session | null = null;
const slots = new WeakMap<LP2PConnection, ConnectionSlots>();

/** The internal slots of a connection. Internal. */
export function slotsOf(connection: LP2PConnection): ConnectionSlots {
  const s = slots.get(connection);
  if (!s) throw new TypeError("not an LP2PConnection");
  return s;
}

/** The session behind a connection. Internal. */
export function sessionOf(connection: LP2PConnection): Session {
  return slotsOf(connection).session;
}

/** Creates the API object for an accepted session (spec "create an LP2PConnection"). Internal. */
export function adoptConnection(session: Session): LP2PConnection {
  adopting = session;
  try {
    return new LP2PConnection();
  } finally {
    adopting = null;
  }
}

export class LP2PConnection extends EventTarget {
  readonly #slots: ConnectionSlots;

  /** Not constructible from script: the IDL has no constructor. */
  constructor() {
    super();
    const s = adopting;
    adopting = null;
    if (!s) throw new TypeError("Illegal constructor");
    this.#slots = { session: s, transports: new Set(), listener: null, closed: false };
    slots.set(this, this.#slots);
    s.ondatachannel = (ep) => {
      const channel = adoptChannel(ep, true);
      this.dispatchEvent(new LP2PDataChannelEvent("datachannel", { channel }));
    };
    s.whenClosed(() => setTimeout(() => this.#finalize(), 0));
    // Incoming WebTransport sessions reported before the connection is
    // exposed to script are held until then, so script can construct a
    // listener first. The connection is exposed when start() resolves or
    // the connection event is dispatched, both right after this; script
    // reacting to them runs before the next task.
    setTimeout(() => {
      s.ontransport = (t) => {
        const listener = this.#slots.listener;
        if (this.#slots.closed || !listener || !listener.incoming(t)) t.refuse();
      };
    }, 0);
  }

  createDataChannel(label: string, dataChannelDict: LP2PDataChannelInit = {}): LP2PDataChannel {
    if (arguments.length < 1) throw new TypeError("createDataChannel: label is required");
    if (this.#slots.closed) throw new DOMException("LP2PConnection is closed", "InvalidStateError");
    const requested = dataChannelDict.id === undefined ? undefined : enforceUnsignedShort(dataChannelDict.id, "id");
    if (requested === 65535) throw new TypeError("id 65535 is reserved");
    const session = this.#slots.session;
    const id = session.reserveChannelId(requested);
    const ep = session.openDataChannel({
      id,
      label: String(label),
      protocol: String(dataChannelDict.protocol ?? ""),
    });
    return adoptChannel(ep, false);
  }

  get ondatachannel(): ((this: EventTarget, ev: LP2PDataChannelEvent) => unknown) | null {
    return getHandler(this, "datachannel") as ((this: EventTarget, ev: LP2PDataChannelEvent) => unknown) | null;
  }
  set ondatachannel(fn) {
    setHandler(this, "datachannel", fn);
  }

  /** Closes the session, its data channels and its transports, on both sides; then fires close. */
  close(): void {
    if (this.#slots.closed) return;
    this.#slots.session.close("closed by page");
    setTimeout(() => this.#finalize(), 0);
  }

  /** Spec "finalize": fail the transports, close the listener, fire close. At most once. */
  #finalize(): void {
    if (this.#slots.closed) return;
    this.#slots.closed = true;
    for (const t of [...this.#slots.transports]) t.failFromConnection();
    this.#slots.listener?.closeFromConnection();
    this.dispatchEvent(new Event("close"));
  }

  get onclose(): ((this: EventTarget, ev: Event) => unknown) | null {
    return getHandler(this, "close");
  }
  set onclose(fn) {
    setHandler(this, "close", fn);
  }
}
