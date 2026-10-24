// LP2PRequest: ask the user to pick a peer and connect to it.

import { getUserAgent } from "../config.ts";
import { adoptConnection, type LP2PConnection } from "./connection.ts";

export interface LP2PRequestOptions {
  nickname?: string;
}

export class LP2PRequest {
  readonly #nickname: string;
  #started: Promise<LP2PConnection> | null = null;

  constructor(options: LP2PRequestOptions = {}) {
    this.#nickname = options.nickname === undefined ? "" : String(options.nickname);
  }

  /**
   * Shows the device picker, connects to the chosen peer and pairs with it.
   * Rejects with NotAllowedError if the user dismisses the picker, the peer
   * declines or pairing fails, and NetworkError if the peer is unreachable.
   * While pending or after success, calling it again returns the same
   * promise; after a failure it starts over.
   */
  start(): Promise<LP2PConnection> {
    if (this.#started) return this.#started;
    const p = getUserAgent()
      .request(this.#nickname)
      .then(({ session }) => adoptConnection(session));
    this.#started = p;
    p.catch(() => {
      if (this.#started === p) this.#started = null;
    });
    return p;
  }
}
