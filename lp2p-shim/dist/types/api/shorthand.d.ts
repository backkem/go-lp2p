import { type LP2PReceiverOptions } from "./receiver.js";
import { type LP2PRequestOptions } from "./request.js";
import { LP2PQuicTransport, type LP2PBidirectionalStream, type LP2PQuicTransportInit, type LP2PQuicTransportListenerInit, type LP2PReceiveStream, type LP2PSendGroup, type LP2PSendStream, type WebTransportCloseInfo, type WebTransportConnectionStats, type WebTransportSendOptions, type WebTransportSendStreamOptions } from "./quictransport.js";
export interface MakeDiscoverableOptions extends LP2PReceiverOptions, LP2PQuicTransportListenerInit {
}
export interface ConnectOptions extends LP2PRequestOptions, LP2PQuicTransportInit {
}
/**
 * Makes the device discoverable and returns one stream of the transports
 * peers open, across all connections. Transports in it are ready.
 * Cancelling the stream stops the receiver and closes the listeners;
 * connections already made stay open until the peer closes them.
 */
export declare function makeDiscoverable(options?: MakeDiscoverableOptions): ReadableStream<LP2PQuicTransport>;
/**
 * Asks the user to pick a peer and opens a WebTransport session with it.
 * Returns at once; ready resolves once a peer is picked and the session is
 * up, and rejects with NotAllowedError if the request fails.
 */
export declare function connect(options?: ConnectOptions): LP2PShorthandTransport;
/** A datagram duplex stream that forwards to the transport once there is one. */
declare class PendingDatagrams {
    #private;
    constructor(owner: LP2PShorthandTransport);
    /** Internal: the transport exists; apply what the page set before. */
    apply(t: LP2PQuicTransport): void;
    get readable(): ReadableStream<Uint8Array>;
    get writable(): WritableStream<Uint8Array>;
    createWritable(options?: WebTransportSendOptions): WritableStream<Uint8Array>;
    get maxDatagramSize(): number;
    get incomingMaxAge(): number | null;
    set incomingMaxAge(v: number | null);
    get outgoingMaxAge(): number | null;
    set outgoingMaxAge(v: number | null);
    get incomingMaxBufferedDatagrams(): number;
    set incomingMaxBufferedDatagrams(v: number);
    get outgoingMaxBufferedDatagrams(): number;
    set outgoingMaxBufferedDatagrams(v: number);
}
/**
 * What lp2p.connect() returns: the WebTransport members, forwarded to the
 * LP2PQuicTransport opened on the picked peer's connection. Closing it, or
 * the session ending, closes that connection too: the page has no other
 * handle on it.
 */
export declare class LP2PShorthandTransport {
    #private;
    constructor(options: ConnectOptions, key?: symbol);
    /** Internal: the LP2PQuicTransport, or null before the peer is picked. */
    get transport(): LP2PQuicTransport | null;
    /** Internal: resolves with the transport once ready. */
    transportWhenReady(): Promise<LP2PQuicTransport>;
    get ready(): Promise<undefined>;
    get closed(): Promise<WebTransportCloseInfo>;
    get draining(): Promise<undefined>;
    get reliability(): "pending" | "reliable-only" | "supports-unreliable";
    get congestionControl(): "default" | "throughput" | "low-latency";
    get protocol(): string;
    get responseHeaders(): null;
    get datagrams(): PendingDatagrams;
    get incomingBidirectionalStreams(): ReadableStream<LP2PBidirectionalStream>;
    get incomingUnidirectionalStreams(): ReadableStream<LP2PReceiveStream>;
    createBidirectionalStream(options?: WebTransportSendStreamOptions): Promise<LP2PBidirectionalStream>;
    createUnidirectionalStream(options?: WebTransportSendStreamOptions): Promise<LP2PSendStream>;
    /** Only once a peer is picked: a send group belongs to one LP2PQuicTransport. */
    createSendGroup(): LP2PSendGroup;
    getStats(): Promise<WebTransportConnectionStats>;
    exportKeyingMaterial(label: BufferSource, context?: BufferSource, outputLength?: number): Promise<ArrayBuffer>;
    /**
     * Closes the transport and its connection. Before a peer is picked, the
     * picker stays up (LP2PRequest.start() can't be aborted), and the
     * connection is closed as soon as it exists.
     */
    close(closeInfo?: WebTransportCloseInfo): undefined;
}
/** The shorthand namespace, as the explainer calls it. */
export declare const lp2p: Readonly<{
    makeDiscoverable: typeof makeDiscoverable;
    connect: typeof connect;
}>;
export {};
