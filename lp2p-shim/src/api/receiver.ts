// LP2PReceiver: advertise on the local communication medium and receive connections.

import { getUserAgent } from "../config.ts";
import type { Closer } from "../transport/types.ts";
import { adoptConnection } from "./connection.ts";
import { LP2PConnectionEvent, getHandler, setHandler } from "./events.ts";

export interface LP2PReceiverOptions {
  nickname?: string;
}

export class LP2PReceiver extends EventTarget {
  readonly #nickname: string;
  #started: Promise<undefined> | null = null;
  #advertisement: Closer | null = null;
  #stopped = false;

  constructor(options: LP2PReceiverOptions = {}) {
    super();
    this.#nickname = options.nickname === undefined ? "" : String(options.nickname);
  }

  /**
   * Asks the user for permission to advertise and starts advertising.
   * Resolves once advertising; rejects with NotAllowedError if denied.
   * Calling it again returns the same promise (the spec does not say).
   */
  start(): Promise<undefined> {
    if (this.#started) return this.#started;
    this.#started = getUserAgent()
      .advertise(this.#nickname, ({ session }) => {
        if (this.#stopped) {
          session.close("receiver stopped");
          return;
        }
        this.dispatchEvent(new LP2PConnectionEvent("connection", { connection: adoptConnection(session) }));
      })
      .then((ad) => {
        this.#advertisement = ad;
        if (this.#stopped) ad.close();
        return undefined;
      });
    this.#started.catch(() => {
      this.#started = null; // allow another attempt
    });
    return this.#started;
  }

  /** Extension: not in the LP2P IDL. Stops advertising; existing connections stay open. */
  stop(): void {
    this.#stopped = true;
    this.#advertisement?.close();
  }

  get onconnection(): ((this: EventTarget, ev: LP2PConnectionEvent) => unknown) | null {
    return getHandler(this, "connection") as ((this: EventTarget, ev: LP2PConnectionEvent) => unknown) | null;
  }
  set onconnection(fn) {
    setHandler(this, "connection", fn);
  }
}
