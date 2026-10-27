import type { TransportStream } from "../transport/types.js";
/** Stream frame tags after the 1203 header. */
export declare const StreamFrame: {
    readonly Data: 0;
    readonly Fin: 1;
    readonly Reset: 2;
    readonly StopSending: 3;
};
/** QUIC stream error code: the stream's session is closed or unknown. Codes from 2^32 are reserved. */
export declare const SESSION_GONE: number;
/** What happens on a WebTransport stream, in order. */
export interface StreamSink {
    data(chunk: Uint8Array): void;
    fin(): void;
    reset(code: number): void;
    stopSending(code: number): void;
    /** The stream ended without a stream error code: its session went away, or the connection did. */
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
    private sink;
    private readonly held;
    private readonly stream;
    constructor(stream: TransportStream, unidirectional: boolean, outgoing: boolean);
    /** Delivers held events, then everything that follows, to sink. */
    attach(sink: StreamSink): void;
    get sending(): boolean;
    send(chunk: Uint8Array): void;
    fin(): void;
    reset(code: number): void;
    stopSending(code: number): void;
    /**
     * Refuses a stream nobody takes: STOP_SENDING and, for a bidirectional
     * stream, RESET_STREAM, with code (session-gone).
     */
    refuse(code: number): void;
    /** The session closed: reset and stop the stream with session-gone, and tell the sink. */
    sessionGone(reason: string): void;
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
    /** True once the underlying connection is closed. */
    readonly connectionClosed: boolean;
    openStream(unidirectional: boolean): TransportStream;
    sendDatagram(d: Uint8Array): void;
    requestId(): number;
    forgetTransport(exchangeId: number): void;
    /** Promises of sessions that sent data-transport-close and wait for the peer's end of the control stream. */
    readonly closing: Set<Promise<void>>;
    exportKeyingMaterial(label: string, context: Uint8Array, length: number): Promise<Uint8Array>;
}
/**
 * "connecting": opened here, waiting for 1202. "pending": opened by the
 * peer, waiting for this user agent to accept or refuse.
 */
export type WTState = "connecting" | "pending" | "open" | "closed";
/** One WebTransport session: a control stream that began with 1201. */
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
    /** The peer sent data-transport-drain. */
    ondrain: (() => void) | null;
    /** Called once: info is null for an abrupt close (refused, or the connection went away). */
    onclose: ((info: CloseInfo | null, why: string) => void) | null;
    private readonly host;
    private readonly control;
    private readonly requestIdValue;
    private readonly streams;
    private readonly heldStreams;
    /** Incoming streams that arrived while this side was still waiting for 1202. */
    private readonly early;
    private earlyTimer;
    private streamHandler;
    private readonly heldDatagrams;
    private datagramHandler;
    private readonly prefix;
    private peerClose;
    private lingering;
    constructor(host: WTHost, exchangeId: number, control: TransportStream, outgoing: boolean, requestId: number);
    get maxDatagramSize(): number;
    get datagramsReliable(): boolean;
    /** Incoming streams. Streams that arrive while this is null are held. */
    set onstream(fn: ((s: WTStream) => void) | null);
    /** Incoming datagrams. Up to 64 that arrive while this is null are held. */
    set ondatagram(fn: ((d: Uint8Array) => void) | null);
    /** Incoming, pending: this user agent accepts the session. */
    accept(): void;
    /** Incoming, pending: refuses the session (permanent-error, then FIN). */
    refuse(): void;
    openStream(unidirectional: boolean): WTStream;
    /** From the session layer: the peer opened a stream for this session. */
    addIncomingStream(s: WTStream): void;
    sendDatagram(payload: Uint8Array): void;
    /** From the session layer: a datagram for this session, without its prefix. */
    receiveDatagram(payload: Uint8Array): void;
    /** Sends data-transport-drain: asks the peer to finish and close. */
    drain(): void;
    /** Keying material that only this session yields (spec "export keying material"). */
    exportKeyingMaterial(label: Uint8Array, context: Uint8Array, length: number): Promise<Uint8Array>;
    /**
     * Closes the session. When open, tells the peer code and reason (1205)
     * and ends the control stream; when connecting, abandons it; when
     * pending, refuses it.
     */
    close(code?: number, reason?: string): void;
    /** Payload bytes sent and received so far, on streams and in datagrams. */
    totals(): {
        sent: number;
        received: number;
    };
    private track;
    private receiveControl;
    /** The control stream ended: FIN without 1205 is a clean close with code 0; a lost connection is abrupt. */
    private controlEnded;
    private teardown;
}
