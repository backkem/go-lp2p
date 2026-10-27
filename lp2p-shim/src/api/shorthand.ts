// The explainer's shorthand APIs ("Shorthand APIs"):
//
//   const transports = lp2p.makeDiscoverable(options);
//   for await (const transport of transports) { ... }   // already ready
//
//   const transport = await lp2p.connect(options);
//   await transport.ready;
//
// Both are thin compositions over the full API and use only what a page
// could use: makeDiscoverable() is an LP2PReceiver plus an
// LP2PQuicTransportListener on each of its connections, merged into one
// stream; connect() is an LP2PRequest plus new LP2PQuicTransport(connection),
// and creates no listener, so a page can construct one on
// transport.connection.

import type { LP2PConnection } from "./connection.ts";
import { LP2PReceiver, type LP2PReceiverOptions } from "./receiver.ts";
import { LP2PRequest, type LP2PRequestOptions } from "./request.ts";
import {
  LP2PQuicTransport,
  LP2PQuicTransportListener,
  type LP2PQuicTransportInit,
  type LP2PQuicTransportListenerInit,
} from "./quictransport.ts";

export interface MakeDiscoverableOptions extends LP2PReceiverOptions, LP2PQuicTransportListenerInit {}
export interface ConnectOptions extends LP2PRequestOptions, LP2PQuicTransportInit {}

function receiverOptions(o: MakeDiscoverableOptions): LP2PReceiverOptions {
  const r: LP2PReceiverOptions = {};
  if (o.nickname !== undefined) r.nickname = o.nickname;
  if (o.allowedOrigins !== undefined) r.allowedOrigins = o.allowedOrigins;
  return r;
}

function transportOptions(o: LP2PQuicTransportInit): LP2PQuicTransportInit {
  const { requireUnreliable, congestionControl, datagramsReadableType } = o;
  const t: LP2PQuicTransportInit = {};
  if (requireUnreliable !== undefined) t.requireUnreliable = requireUnreliable;
  if (congestionControl !== undefined) t.congestionControl = congestionControl;
  if (datagramsReadableType !== undefined) t.datagramsReadableType = datagramsReadableType;
  if (o.anticipatedConcurrentIncomingBidirectionalStreams !== undefined) {
    t.anticipatedConcurrentIncomingBidirectionalStreams = o.anticipatedConcurrentIncomingBidirectionalStreams;
  }
  if (o.anticipatedConcurrentIncomingUnidirectionalStreams !== undefined) {
    t.anticipatedConcurrentIncomingUnidirectionalStreams = o.anticipatedConcurrentIncomingUnidirectionalStreams;
  }
  return t;
}

const MERGED_HIGH_WATER_MARK = 8;

/**
 * Makes the device discoverable and returns one stream of the transports
 * peers open, across all connections. Transports in it are ready.
 * The shorthand owns the listener of each connection it creates; a page
 * reaches the connection as transport.connection, to open data channels
 * or close it. Cancelling the stream stops the receiver and closes the
 * listeners; connections already made stay open until closed.
 */
export function makeDiscoverable(options: MakeDiscoverableOptions = {}): ReadableStream<LP2PQuicTransport> {
  const opts = options ?? {};
  const receiver = new LP2PReceiver(receiverOptions(opts)); // a bad allowedOrigins entry throws here
  const listenerInit = transportOptions(opts);
  const listeners = new Set<LP2PQuicTransportListener>();
  let controller!: ReadableStreamDefaultController<LP2PQuicTransport>;
  let done = false;
  let wake: (() => void) | null = null;
  const pulled = (): Promise<void> => new Promise((r) => (wake = r));

  const merged = new ReadableStream<LP2PQuicTransport>(
    {
      start: (c) => void (controller = c),
      pull: () => {
        wake?.();
        wake = null;
      },
      cancel: () => {
        done = true;
        receiver.stop();
        for (const l of listeners) l.close();
        wake?.();
      },
    },
    { highWaterMark: MERGED_HIGH_WATER_MARK },
  );

  // Moves one connection's transports into the merged stream, as fast as
  // the page reads: a listener whose queue fills refuses further sessions.
  const pump = async (listener: LP2PQuicTransportListener): Promise<void> => {
    const reader = listener.incomingTransports.getReader();
    for (;;) {
      while (!done && (controller.desiredSize ?? 0) <= 0) await pulled();
      if (done) break;
      const { value, done: end } = await reader.read();
      if (end) break;
      if (done) {
        value.close();
        break;
      }
      controller.enqueue(value);
    }
    reader.releaseLock();
  };

  receiver.addEventListener("connection", (e) => {
    const connection = (e as Event & { connection: LP2PConnection }).connection;
    if (done) {
      connection.close();
      return;
    }
    const listener = new LP2PQuicTransportListener(connection, listenerInit);
    listeners.add(listener);
    connection.addEventListener("close", () => listeners.delete(listener));
    void pump(listener);
  });
  receiver.start().catch((err: unknown) => {
    if (done) return;
    done = true;
    controller.error(err);
  });
  return merged;
}

/**
 * Asks the user to pick a peer and opens a WebTransport session with it.
 * Resolves with the transport once a peer is picked and the session is up;
 * rejects with NotAllowedError if the request fails, or with the
 * transport's error (and then closes the connection, which the page never
 * saw). The connection stays the page's to use and close
 * (transport.connection).
 */
export async function connect(options: ConnectOptions = {}): Promise<LP2PQuicTransport> {
  const opts = options ?? {};
  const connection = await new LP2PRequest(opts).start();
  try {
    const transport = new LP2PQuicTransport(connection, transportOptions(opts));
    await transport.ready;
    return transport;
  } catch (err) {
    // The page never got the connection: don't leave it open.
    connection.close();
    throw err;
  }
}

/** The shorthand namespace, as the explainer calls it. */
export const lp2p = Object.freeze({ makeDiscoverable, connect });
