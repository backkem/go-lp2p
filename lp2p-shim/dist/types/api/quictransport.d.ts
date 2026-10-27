import type { StreamSink, WTStream } from "../protocol/webtransport.js";
import { LP2PConnection } from "./connection.js";
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
    /** Fail unless datagrams can be lost, i.e. the session is not "reliable-only". */
    requireUnreliable?: boolean;
    /** Accepted and reflected; the shim has no congestion controller to choose. */
    congestionControl?: "default" | "throughput" | "low-latency";
    anticipatedConcurrentIncomingUnidirectionalStreams?: number | null;
    anticipatedConcurrentIncomingBidirectionalStreams?: number | null;
    /** Extension (from WebTransportOptions): "bytes" makes datagrams.readable a byte stream. */
    datagramsReadableType?: "bytes";
}
/** The options of a listener apply to each transport it creates. */
export interface LP2PQuicTransportListenerInit extends LP2PQuicTransportInit {
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
    /** Always true: some shim transports only give reliable-only sessions. */
    static get supportsReliableOnly(): boolean;
    /**
     * Opens a new WebTransport session on connection. Each construction opens
     * its own session; the peer receives it through its
     * LP2PQuicTransportListener on the connection.
     */
    constructor(connection: LP2PConnection, options?: LP2PQuicTransportInit);
    get ready(): Promise<undefined>;
    get closed(): Promise<WebTransportCloseInfo>;
    /** Resolves when the peer drains the session (data-transport-drain), or when it ends. */
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
    /**
     * Keying material of this session only, derived as the binding says (the
     * connection's exporter with label "EXPORTER-WebTransport" and the
     * WebTransport Exporter Context, with the exchange-id as session ID). In
     * the shim the connection's exporter is the handshake stand-in of
     * protocol/identity.ts, not TLS. Extension: outputLength, default 32.
     */
    exportKeyingMaterial(label: BufferSource, context?: BufferSource, outputLength?: number): Promise<ArrayBuffer>;
    close(closeInfo?: WebTransportCloseInfo): undefined;
}
export declare class LP2PQuicTransportListener {
    #private;
    /**
     * Receives the incoming WebTransport sessions of connection. There is at
     * most one listener per connection at a time; with none, or with a full
     * queue, incoming sessions are refused.
     */
    constructor(connection: LP2PConnection, options?: LP2PQuicTransportListenerInit);
    get incomingTransports(): ReadableStream<LP2PQuicTransport>;
    /** Stops listening. Transports already queued can still be read; a new listener can then be constructed. */
    close(): undefined;
}
export {};
