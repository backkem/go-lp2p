import type { StreamSink, WTStream } from "../protocol/webtransport.js";
import { LP2PConnection } from "./connection.js";
import { LP2PReceiver } from "./receiver.js";
import { LP2PRequest } from "./request.js";
export interface WebTransportErrorInit {
    source?: "stream" | "session";
    streamErrorCode?: number | null;
}
type WTErrorCtor = new (message?: string, options?: WebTransportErrorInit) => DOMException & {
    readonly source: "stream" | "session";
    readonly streamErrorCode: number | null;
};
/** WebTransportError, as the WebTransport spec defines it. */
export declare const WebTransportError: WTErrorCtor;
export interface WebTransportSendStreamStats {
    bytesWritten: number;
    bytesSent: number;
    bytesAcknowledged: number;
}
export interface WebTransportReceiveStreamStats {
    bytesReceived: number;
    bytesRead: number;
}
export interface WebTransportSendOptions {
    sendGroup?: LP2PSendGroup | null;
    sendOrder?: number;
}
export interface WebTransportSendStreamOptions extends WebTransportSendOptions {
    waitUntilAvailable?: boolean;
}
/** WebTransportSendGroup. Groups streams for stats; the shim has no prioritization to apply. */
export declare class LP2PSendGroup {
    #private;
    constructor();
    /** Internal. */
    static ownerOf(g: LP2PSendGroup): LP2PQuicTransport;
    /** Internal. */
    static add(g: LP2PSendGroup, s: WTStream): void;
    getStats(): Promise<WebTransportSendStreamStats>;
}
/** Wire state shared by the two halves of a stream. */
declare class StreamHalves implements StreamSink {
    readable: ReadableByteStreamController | null;
    writable: WritableStreamDefaultController | null;
    readableDone: boolean;
    writableDone: boolean;
    readonly ws: WTStream;
    constructor(ws: WTStream);
    data(chunk: Uint8Array): void;
    fin(): void;
    reset(code: number): void;
    stopSending(code: number): void;
    abort(reason: string): void;
    errorReadable(err: unknown): void;
    errorWritable(err: unknown): void;
}
declare const ByteStream: new (source: UnderlyingByteSource) => ReadableStream<Uint8Array>;
/** WebTransportReceiveStream: a readable byte stream. */
export declare class LP2PReceiveStream extends ByteStream {
    #private;
    constructor(halves: StreamHalves);
    getStats(): Promise<WebTransportReceiveStreamStats>;
}
/** WebTransportSendStream: a writable stream of BufferSource chunks. */
export declare class LP2PSendStream extends WritableStream<Uint8Array> {
    #private;
    constructor(halves: StreamHalves, transport: LP2PQuicTransport, options: WebTransportSendOptions);
    get sendGroup(): LP2PSendGroup | null;
    set sendGroup(g: LP2PSendGroup | null);
    /** Stored, not applied: the shim's transports have no send scheduler to prioritize with. */
    get sendOrder(): number;
    set sendOrder(v: number);
    getStats(): Promise<WebTransportSendStreamStats>;
    /** A WebTransportWriter. Without flow control, atomicWrite() is write() and commit() does nothing. */
    getWriter(): WritableStreamDefaultWriter<Uint8Array> & {
        atomicWrite(chunk?: unknown): Promise<undefined>;
        commit(): undefined;
    };
}
/** WebTransportBidirectionalStream. */
export declare class LP2PBidirectionalStream {
    #private;
    constructor(readable: LP2PReceiveStream, writable: LP2PSendStream);
    get readable(): LP2PReceiveStream;
    get writable(): LP2PSendStream;
}
/** WebTransportDatagramsWritable. */
export declare class LP2PDatagramsWritable extends WritableStream<Uint8Array> {
    #private;
    constructor(sink: UnderlyingSink<Uint8Array>, options: WebTransportSendOptions);
    get sendGroup(): LP2PSendGroup | null;
    set sendGroup(g: LP2PSendGroup | null);
    get sendOrder(): number;
    set sendOrder(v: number);
}
/** WebTransportDatagramDuplexStream. */
export declare class LP2PDatagramDuplexStream {
    #private;
    droppedIncoming: number;
    constructor(transport: LP2PQuicTransport, readableType: "bytes" | undefined);
    get readable(): ReadableStream<Uint8Array>;
    /** Legacy (still shipped by Chromium): one shared writable. */
    get writable(): LP2PDatagramsWritable;
    createWritable(options?: WebTransportSendOptions): LP2PDatagramsWritable;
    /** 0 until the transport is ready, then what fits one datagram of the underlying connection. */
    get maxDatagramSize(): number;
    /** Stored; the shim does not expire datagrams. */
    get incomingMaxAge(): number | null;
    set incomingMaxAge(v: number | null);
    get outgoingMaxAge(): number | null;
    set outgoingMaxAge(v: number | null);
    get incomingMaxBufferedDatagrams(): number;
    set incomingMaxBufferedDatagrams(v: number);
    get outgoingMaxBufferedDatagrams(): number;
    set outgoingMaxBufferedDatagrams(v: number);
    /** Legacy names. */
    get incomingHighWaterMark(): number;
    set incomingHighWaterMark(v: number);
    get outgoingHighWaterMark(): number;
    set outgoingHighWaterMark(v: number);
    /** Internal: a datagram arrived. */
    receive(d: Uint8Array): void;
    /** Internal: the session ended. */
    finish(err: unknown | null): void;
}
export interface LP2PQuicTransportInit {
    /** Fail (ready rejects) unless datagrams can be lost, i.e. the transport is not reliable-only. */
    requireUnreliable?: boolean;
    /** Accepted and reflected; the shim has no congestion controller to choose. */
    congestionControl?: "default" | "throughput" | "low-latency";
    anticipatedConcurrentIncomingUnidirectionalStreams?: number | null;
    anticipatedConcurrentIncomingBidirectionalStreams?: number | null;
    /** "bytes" makes datagrams.readable a byte stream. */
    datagramsReadableType?: "bytes";
}
export interface LP2PQuicTransportListenerInit {
}
export interface WebTransportCloseInfo {
    closeCode?: number;
    reason?: string;
}
export interface WebTransportConnectionStats {
    bytesSent: number;
    bytesReceived: number;
    datagrams: {
        droppedIncoming: number;
        expiredIncoming: number;
        expiredOutgoing: number;
        lostOutgoing: number;
    };
    estimatedSendRate: number | null;
    atSendCapacity: boolean;
}
export declare class LP2PQuicTransport {
    #private;
    /** Always true: the postMessage transport gives reliable-only sessions. */
    static get supportsReliableOnly(): boolean;
    /**
     * Opens a WebTransport session with the peer of source. An LP2PRequest or
     * LP2PReceiver that is not started yet is started, as the spec says. For
     * an LP2PReceiver, the session is opened on its next connection.
     */
    constructor(source: LP2PConnection | LP2PRequest | LP2PReceiver, quicTransportDict?: LP2PQuicTransportInit);
    get ready(): Promise<undefined>;
    get closed(): Promise<WebTransportCloseInfo>;
    /** Resolves when the session closes: no peer page can ask for a drain (there is no GOAWAY). */
    get draining(): Promise<undefined>;
    get reliability(): "pending" | "reliable-only" | "supports-unreliable";
    get congestionControl(): "default" | "throughput" | "low-latency";
    get anticipatedConcurrentIncomingUnidirectionalStreams(): number | null;
    set anticipatedConcurrentIncomingUnidirectionalStreams(v: number | null);
    get anticipatedConcurrentIncomingBidirectionalStreams(): number | null;
    set anticipatedConcurrentIncomingBidirectionalStreams(v: number | null);
    /** Always null: there is no HTTP exchange. */
    get responseHeaders(): null;
    /** Always "": there is no subprotocol negotiation. */
    get protocol(): string;
    get datagrams(): LP2PDatagramDuplexStream;
    get incomingBidirectionalStreams(): ReadableStream<LP2PBidirectionalStream>;
    get incomingUnidirectionalStreams(): ReadableStream<LP2PReceiveStream>;
    createBidirectionalStream(options?: WebTransportSendStreamOptions): Promise<LP2PBidirectionalStream>;
    createUnidirectionalStream(options?: WebTransportSendStreamOptions): Promise<LP2PSendStream>;
    createSendGroup(): LP2PSendGroup;
    /** Counts payload bytes; RTT and packet counts are not observable through the shim's transports. */
    getStats(): Promise<WebTransportConnectionStats>;
    /** Not supported: the shim has no TLS session to export keying material from. */
    exportKeyingMaterial(_label: BufferSource, _context: BufferSource, _outputLength: number): Promise<Uint8Array>;
    close(closeInfo?: WebTransportCloseInfo): undefined;
}
export declare class LP2PQuicTransportListener {
    #private;
    /**
     * Hands out the WebTransport sessions peers open on source's connections:
     * the one connection of an LP2PRequest, or every connection of an
     * LP2PReceiver (current and future). Starts source if needed. Extension:
     * source may also be an LP2PConnection.
     */
    constructor(source: LP2PRequest | LP2PReceiver | LP2PConnection, quicTransportListenerDict?: LP2PQuicTransportListenerInit);
    get ready(): Promise<undefined>;
    get incomingTransports(): ReadableStream<LP2PQuicTransport>;
}
export {};
