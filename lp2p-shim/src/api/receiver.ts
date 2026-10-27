// LP2PReceiver: advertise on the local communication medium and receive connections.

import { getUserAgent } from "../config.ts";
import type { Closer } from "../transport/types.ts";
import type { Session } from "../protocol/session.ts";
import { parseOrigin } from "../ua/localstate.ts";
import type { ReceiverHandle } from "../ua/useragent.ts";
import { adoptConnection } from "./connection.ts";
import { LP2PConnectionEvent, getHandler, setHandler } from "./events.ts";

export interface LP2PReceiverOptions {
  nickname?: string;
  /** URLs whose origins, besides the receiver's own, may request sessions. */
  allowedOrigins?: string[];
}

type State = "initial" | "advertising" | "stopped";

export class LP2PReceiver extends EventTarget {
  readonly #nickname: string;
  readonly #allowedOrigins: string[];
  #state: State = "initial";
  #startPromise: Promise<undefined> | null = null;
  #rejectStart: ((e: unknown) => void) | null = null;
  #advertisement: Closer | null = null;

  constructor(options: LP2PReceiverOptions = {}) {
    super();
    this.#nickname = options?.nickname === undefined ? "" : String(options.nickname);
    this.#allowedOrigins = [];
    for (const entry of options?.allowedOrigins ?? []) {
      const origin = parseOrigin(String(entry));
      if (origin === null) throw new TypeError(`allowedOrigins: "${String(entry)}" is not a valid URL`);
      // An opaque origin ("null") is kept but never matches, see #allows.
      this.#allowedOrigins.push(origin);
    }
  }

  /**
   * Asks the user for permission to advertise and starts advertising.
   * Resolves once advertising; rejects with NotAllowedError if denied, and
   * InvalidStateError after stop(). Calling it again returns the same
   * promise while pending or after success.
   */
  start(): Promise<undefined> {
    if (this.#startPromise) return this.#startPromise;
    if (this.#state === "stopped") {
      return Promise.reject(new DOMException("the receiver was stopped", "InvalidStateError"));
    }
    const ua = getUserAgent();
    if (ua.origin === "null") return Promise.reject(new DOMException("opaque origin", "NotAllowedError"));
    const handle: ReceiverHandle = {
      origin: ua.origin,
      nickname: this.#nickname,
      allows: (o) => this.#allows(o, ua.origin),
      deliver: (s) => this.#deliver(s),
    };
    const promise = new Promise<undefined>((resolve, reject) => {
      this.#rejectStart = reject;
      ua.advertise(handle).then(
        (ad) => {
          if (this.#state === "stopped") {
            ad.close();
            return;
          }
          this.#advertisement = ad;
          this.#state = "advertising";
          this.#rejectStart = null;
          resolve(undefined);
        },
        () => {
          if (this.#startPromise === promise) this.#startPromise = null;
          this.#rejectStart = null;
          reject(new DOMException("The user did not allow advertising.", "NotAllowedError"));
        },
      );
    });
    this.#startPromise = promise;
    return promise;
  }

  /** Stops advertising. Connections already delivered stay open. The receiver can't be started again. */
  stop(): void {
    if (this.#state === "stopped") return;
    this.#state = "stopped";
    this.#rejectStart?.(new DOMException("stopped", "AbortError"));
    this.#rejectStart = null;
    this.#advertisement?.close();
    this.#advertisement = null;
  }

  #allows(origin: string, own: string): boolean {
    if (origin === "null") return false;
    return origin === own || this.#allowedOrigins.includes(origin);
  }

  #deliver(session: Session): boolean {
    if (this.#state !== "advertising") return false;
    const connection = adoptConnection(session);
    this.dispatchEvent(new LP2PConnectionEvent("connection", { connection }));
    return true;
  }

  get onconnection(): ((this: EventTarget, ev: LP2PConnectionEvent) => unknown) | null {
    return getHandler(this, "connection") as ((this: EventTarget, ev: LP2PConnectionEvent) => unknown) | null;
  }
  set onconnection(fn) {
    setHandler(this, "connection", fn);
  }
}
