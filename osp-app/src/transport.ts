// The transport interface: what the OSP application protocol needs from
// whatever carries it. OSP's network protocol (network.bs) provides this with
// QUIC; here it is pluggable.
//
// A transport moves framed messages (type key + one CBOR item) between two
// authenticated agents, keeps the order of messages within an ordering group
// and reports how it closed. Authentication, discovery and agent identity are
// the transport's job, not this package's.

import type { Frame } from "./messages.ts";

export interface CloseInfo {
  /** Application close code; 404 is OSP's "unknown type key". 0 when unknown. */
  code: number;
  reason: string;
}

export interface SendOptions {
  /**
   * The ordering group (application.bs: "To send a group of messages in
   * order, that group of messages must be sent in one QUIC stream").
   * Messages with the same group arrive in the order sent. Messages without
   * a group, or in different groups, may be reordered relative to each other.
   */
  group?: string;
}

export interface TransportSink {
  /** A complete framed message arrived. */
  frame(frame: Frame): void;
  /** Bytes arrived that are not a framed message; they were dropped. */
  malformed(error: Error): void;
}

export interface OspTransport {
  /** Resolves when messages can be sent; rejects if the transport fails to open. */
  readonly ready: Promise<void>;
  /** Resolves once the transport is closed, with the close info if known. */
  readonly closed: Promise<CloseInfo>;
  /** Starts delivering received frames to sink. Called once, by the session. */
  start(sink: TransportSink): void;
  /** Sends one framed message. Rejects if the transport is closed. */
  send(frame: Frame, options?: SendOptions): Promise<void>;
  /** The group has no more messages: release what carries it (e.g. end its stream). */
  endGroup(group: string): Promise<void>;
  /** Closes the transport. The code and reason reach the peer only if the transport can carry them. */
  close(info?: Partial<CloseInfo>): void;
}

/** Thrown by send() once the transport is closed. */
export class TransportClosedError extends Error {
  override name = "TransportClosedError";
}
