import type { TransportStream } from "../transport/types.js";
/** Stream frame tags after the 1203 header. */
export declare const StreamFrame: {
    readonly Data: 0;
    readonly Fin: 1;
    readonly Reset: 2;
    readonly StopSending: 3;
};
/** What happens on a WebTransport stream, in order. */
export interface StreamSink {
    data(chunk: Uint8Array): void;
    fin(): void;
    reset(code: number): void;
    stopSending(code: number): void;
    /** The stream ended without FIN or RESET: the peer refused it, or the session or connection went away. */
    abort(reason: string): void;
}
/** One WebTransport stream: a transport stream that began with 1203. */
export declare class WTStream {
    readonly unidirectional: boolean;
    /** True if this side opened it. */
    readonly outgoing: boolean;
    bytesSent: number;
    bytesReceived: number;
    onfinished: (() => void) | null;
    private sendDone;
    private recvDone;
    private awaitingResponse;
    private sink;
    private readonly held;
    private readonly stream;
    private readonly requestId;
    constructor(stream: TransportStream, unidirectional: boolean, outgoing: boolean, requestId: number);
    /** Delivers held events, then everything that follows, to sink. */
    attach(sink: StreamSink): void;
    /** Acceptor only: answers the 1203. */
    respond(result: number): void;
    get sending(): boolean;
    send(chunk: Uint8Array): void;
    fin(): void;
    reset(code: number): void;
    stopSending(code: number): void;
    /** The session ended: drop the stream, telling the sink why. */
    abort(reason: string): void;
    private emit;
    private receive;
    private maybeClose;
    private closedUnderneath;
}
export interface CloseInfo {
    code: number;
    reason: string;
}
/** What the session layer (Session) provides to a WTSession. */
export interface WTHost {
    readonly maxDatagramSize: number;
    readonly datagramsReliable: boolean;
    openStream(): TransportStream;
    sendDatagram(d: Uint8Array): void;
    requestId(): number;
    forgetTransport(exchangeId: number): void;
}
export type WTState = "connecting" | "pending" | "open" | "closed";
/** One WebTransport session: a session stream that began with 1201. */
export declare class WTSession {
    readonly exchangeId: number;
    readonly outgoing: boolean;
    state: WTState;
    bytesSent: number;
    bytesReceived: number;
    datagramsSent: number;
    datagramsReceived: number;
    /** Outgoing: the peer accepted. */
    onready: (() => void) | null;
    /** Called once: info is null for an abrupt close (the stream or the connection went away). */
    onclose: ((info: CloseInfo | null, why: string) => void) | null;
    private readonly host;
    private readonly control;
    private readonly requestIdValue;
    private readonly streams;
    private readonly heldStreams;
    private streamHandler;
    private readonly heldDatagrams;
    private datagramHandler;
    private readonly prefix;
    constructor(host: WTHost, exchangeId: number, control: TransportStream, outgoing: boolean, requestId: number);
    get maxDatagramSize(): number;
    get datagramsReliable(): boolean;
    /** Incoming streams. Streams that arrive while this is null are held. */
    set onstream(fn: ((s: WTStream) => void) | null);
    /** Incoming datagrams. Up to 64 that arrive while this is null are held. */
    set ondatagram(fn: ((d: Uint8Array) => void) | null);
    /** Incoming, pending: the local page took the session. */
    accept(): void;
    openStream(unidirectional: boolean): WTStream;
    /** From the session layer: the peer opened a stream for this session. */
    addIncomingStream(s: WTStream): void;
    sendDatagram(payload: Uint8Array): void;
    /** From the session layer: a datagram for this session, without its prefix. */
    receiveDatagram(payload: Uint8Array): void;
    /**
     * Closes the session. When open, tells the peer code and reason (1205);
     * otherwise (connecting, or pending and refused) just drops it.
     */
    close(code?: number, reason?: string): void;
    /** Payload bytes sent and received so far, on streams and in datagrams. */
    totals(): {
        sent: number;
        received: number;
    };
    private track;
    private receiveControl;
    private teardown;
}
