import { type LP2PReceiverOptions } from "./receiver.js";
import { type LP2PRequestOptions } from "./request.js";
import { LP2PQuicTransport, type LP2PQuicTransportInit, type LP2PQuicTransportListenerInit } from "./quictransport.js";
export interface MakeDiscoverableOptions extends LP2PReceiverOptions, LP2PQuicTransportListenerInit {
}
export interface ConnectOptions extends LP2PRequestOptions, LP2PQuicTransportInit {
}
/**
 * Makes the device discoverable and returns one stream of the transports
 * peers open, across all connections. Transports in it are ready.
 * The shorthand owns the listener of each connection it creates; a page
 * reaches the connection as transport.connection, to open data channels
 * or close it. Cancelling the stream stops the receiver and closes the
 * listeners; connections already made stay open until closed.
 */
export declare function makeDiscoverable(options?: MakeDiscoverableOptions): ReadableStream<LP2PQuicTransport>;
/**
 * Asks the user to pick a peer and opens a WebTransport session with it.
 * Resolves with the transport once a peer is picked and the session is up;
 * rejects with NotAllowedError if the request fails, or with the
 * transport's error (and then closes the connection, which the page never
 * saw). The connection stays the page's to use and close
 * (transport.connection).
 */
export declare function connect(options?: ConnectOptions): Promise<LP2PQuicTransport>;
/** The shorthand namespace, as the explainer calls it. */
export declare const lp2p: Readonly<{
    makeDiscoverable: typeof makeDiscoverable;
    connect: typeof connect;
}>;
