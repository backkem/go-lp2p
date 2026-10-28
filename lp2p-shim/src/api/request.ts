// LP2PRequest: ask the user to pick a peer and connect to it.

import { getUserAgent } from "../config.ts";
import { adoptConnection, type LP2PConnection } from "./connection.ts";

export interface LP2PRequestOptions {
  nickname?: string;
}

export interface LP2PRequestStartOptions {
  signal?: AbortSignal;
}

export class LP2PRequest {
  readonly #nickname: string;
  #started: Promise<LP2PConnection> | null = null;

  constructor(options: LP2PRequestOptions = {}) {
    this.#nickname = options?.nickname === undefined ? "" : String(options.nickname);
  }

  /**
   * Shows the device picker, connects to the chosen peer and, if needed,
   * pairs with it. Rejects with NotAllowedError on every failure: dismissed
   * picker, unreachable peer, failed pairing or refused session, so script
   * can't tell them apart. options.signal aborts it: the promise rejects with
   * the abort reason and the picker or the session being set up is taken
   * down. While pending or after success, calling it again returns the same
   * promise and ignores the new options; aborting after success does
   * nothing. After a failure it starts over.
   */
  start(options: LP2PRequestStartOptions = {}): Promise<LP2PConnection> {
    const signal = options?.signal ?? null;
    if (signal?.aborted) return Promise.reject(signal.reason);
    if (this.#started) return this.#started;
    const ua = getUserAgent();
    if (ua.origin === "null") return Promise.reject(new DOMException("opaque origin", "NotAllowedError"));
    const internal = new AbortController();
    let settled = false;
    let aborted = false;
    const p = new Promise<LP2PConnection>((resolve, reject) => {
      signal?.addEventListener(
        "abort",
        () => {
          if (settled) return;
          aborted = true;
          if (this.#started === p) this.#started = null;
          reject(signal.reason);
          internal.abort(signal.reason);
        },
        { once: true },
      );
      ua.request(this.#nickname, internal.signal).then(
        ({ session }) => {
          if (aborted) {
            session.close("start() was aborted");
            return;
          }
          settled = true;
          resolve(adoptConnection(session));
        },
        (err: unknown) => {
          if (aborted) return;
          settled = true;
          if (this.#started === p) this.#started = null;
          reject(err);
        },
      );
    });
    this.#started = p;
    p.catch(() => {});
    return p;
  }
}
