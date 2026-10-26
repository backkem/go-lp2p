// LP2PReceiver: advertise on the local communication medium and receive connections.

import { getUserAgent } from "../config.ts";
import type { Closer } from "../transport/types.ts";
import type { Session } from "../protocol/session.ts";
import { adoptConnection } from "./connection.ts";
import { LP2PConnectionEvent, getHandler, setHandler } from "./events.ts";

export interface LP2PReceiverOptions {
  nickname?: string;
}

interface ReceiverInternals {
  /** Open sessions of the connections this receiver handed out. */
  sessions: Set<Session>;
  watchers: Set<(s: Session) => void>;
}
const internals = new WeakMap<LP2PReceiver, ReceiverInternals>();

/**
 * Calls fn for each connection the receiver gets from now on and, with
 * existing, for those it has. Returns a function that stops watching. Internal.
 */
export function watchSessions(receiver: LP2PReceiver, fn: (s: Session) => void, existing: boolean): () => void {
  const i = internals.get(receiver);
  if (!i) throw new TypeError("not an LP2PReceiver");
  if (existing) for (const s of i.sessions) fn(s);
  i.watchers.add(fn);
  return () => i.watchers.delete(fn);
}

export class LP2PReceiver extends EventTarget {
  readonly #nickname: string;
  #started: Promise<undefined> | null = null;
  #advertisement: Closer | null = null;
  #stopped = false;

  constructor(options: LP2PReceiverOptions = {}) {
    super();
    this.#nickname = options.nickname === undefined ? "" : String(options.nickname);
    internals.set(this, { sessions: new Set(), watchers: new Set() });
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
        const connection = adoptConnection(session);
        const i = internals.get(this) as ReceiverInternals;
        i.sessions.add(session);
        session.whenClosed(() => i.sessions.delete(session));
        this.dispatchEvent(new LP2PConnectionEvent("connection", { connection }));
        for (const fn of [...i.watchers]) fn(session);
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
