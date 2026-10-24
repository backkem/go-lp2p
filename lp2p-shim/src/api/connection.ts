// LP2PConnection: an authenticated connection to one peer.

import type { Session } from "../protocol/session.ts";
import { adoptChannel, enforceUnsignedShort, type LP2PDataChannel, type LP2PDataChannelInit } from "./datachannel.ts";
import { LP2PDataChannelEvent, getHandler, setHandler } from "./events.ts";

let adopting: Session | null = null;

/** Creates the API object for an authenticated session. Internal. */
export function adoptConnection(session: Session): LP2PConnection {
  adopting = session;
  try {
    return new LP2PConnection();
  } finally {
    adopting = null;
  }
}

export class LP2PConnection extends EventTarget {
  readonly #session: Session;
  #closed = false;

  /** Not constructible from script: the IDL has no constructor. */
  constructor() {
    super();
    const s = adopting;
    adopting = null;
    if (!s) throw new TypeError("Illegal constructor");
    this.#session = s;
    s.ondatachannel = (ep) => {
      const channel = adoptChannel(ep, true);
      this.dispatchEvent(new LP2PDataChannelEvent("datachannel", { channel }));
    };
    s.onclose = () => {
      if (this.#closed) return;
      this.#closed = true;
      this.dispatchEvent(new Event("close"));
    };
  }

  createDataChannel(label: string, dataChannelDict: LP2PDataChannelInit = {}): LP2PDataChannel {
    if (arguments.length < 1) throw new TypeError("createDataChannel: label is required");
    if (this.#closed) throw new DOMException("LP2PConnection is closed", "InvalidStateError");
    const requested = dataChannelDict.id === undefined ? undefined : enforceUnsignedShort(dataChannelDict.id, "id");
    if (requested === 65535) throw new TypeError("id 65535 is reserved");
    const id = this.#session.reserveChannelId(requested);
    const ep = this.#session.openDataChannel({
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

  /** Extension: not in the LP2P IDL. Closes the connection and all its channels; fires close. */
  close(): void {
    this.#session.close("closed by page");
  }

  /** Extension: fired when the connection closes, locally or by the peer. */
  get onclose(): ((this: EventTarget, ev: Event) => unknown) | null {
    return getHandler(this, "close");
  }
  set onclose(fn) {
    setHandler(this, "close", fn);
  }
}
