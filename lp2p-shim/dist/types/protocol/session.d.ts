import type { TransportConnection, TransportStream } from "../transport/types.js";
import { DataEncoding, type AgentInfo, type Message } from "./messages.js";
import { type HandshakeResult } from "./identity.js";
import { WTSession, type WTHost } from "./webtransport.js";
export type Role = "client" | "server";
export declare class ProtocolError extends Error {
    name: string;
}
/** How long a refusing server, or an agent that sent data-transport-close, waits for the peer to end its side. */
export declare const REFUSAL_LINGER_MS = 2000;
/** The control stream, with a queue of messages no caller has taken yet. */
export declare class ControlChannel {
    private readonly queue;
    private readonly waiters;
    private closedWith;
    private info;
    private readonly unanswered;
    private readonly stream;
    /** first is a message already read from the stream, handled before any that follow. */
    constructor(stream: TransportStream, first?: Uint8Array);
    send(m: Message): void;
    /** What to answer agent-info-request with. Requests that came earlier are answered now. */
    set localInfo(info: AgentInfo);
    /**
     * Resolves with the oldest message of one of types, now or when it
     * arrives. Messages of other types stay queued for other callers.
     */
    next<T extends Message["type"]>(types: readonly T[], signal?: AbortSignal): Promise<Extract<Message, {
        type: T;
    }>>;
    close(): void;
    /** Handles one raw message from the control stream. */
    feed(b: Uint8Array): void;
    private fail;
}
/** The session stream, server side: the client's lp2p-session-request and how to answer it. */
export interface SessionRequest {
    /** The requesting origin hint, as sent. */
    requestingOrigin: string;
    /** Sends the provisional lp2p-session-response authentication-required: the cue for the client's ceremony. */
    requireAuthentication(): void;
    /**
     * Sends the final lp2p-session-response. On success both sides end the stream; on a
     * refusal the promise resolves once the client ended its side, or after a
     * timeout, and the caller then closes the connection.
     */
    respond(result: number): Promise<void>;
}
export type ChannelState = "connecting" | "open" | "closed";
export interface ChannelParams {
    id: number;
    label: string;
    protocol: string;
}
/** One LP2P data channel on the wire. The API object drives it. */
export declare class DataChannelEndpoint {
    state: ChannelState;
    onopen: (() => void) | null;
    onerror: ((err: Error) => void) | null;
    onclose: (() => void) | null;
    private requestId;
    private frameHandler;
    private readonly heldFrames;
    readonly params: ChannelParams;
    private readonly stream;
    private readonly session;
    constructor(params: ChannelParams, stream: TransportStream, session: Session, opener: {
        requestId: number;
    } | null);
    /** Received data-frames. Frames that arrive while this is null are held. */
    set onframe(fn: ((encodingId: number, payload: Uint8Array) => void) | null);
    send(encodingId: DataEncoding, payload: Uint8Array): void;
    close(): void;
    private receive;
    private closed;
}
export declare class Session implements WTHost {
    readonly role: Role;
    remoteInfo: AgentInfo | null;
    /** Set by the handshake: the peer's proven identity and the exporter secret. */
    tls: HandshakeResult | null;
    private control;
    private readonly controlWaiters;
    private readonly usedIds;
    private readonly parity;
    private nextRequestId;
    private readonly incoming;
    private channelHandler;
    private closeHandler;
    private readonly closeListeners;
    private closeReason;
    private readonly transports;
    private nextExchangeId;
    private readonly incomingTransports;
    private transportHandler;
    private sessionRequestHandler;
    private heldSessionRequest;
    private sawSessionRequest;
    private isAccepted;
    /** Client: streams the server opened before its 1111 arrived. */
    private readonly early;
    readonly connection: TransportConnection;
    constructor(connection: TransportConnection, role: Role);
    get closed(): boolean;
    /** True once the server sent, or the client received, a success lp2p-session-response. */
    get accepted(): boolean;
    set onclose(fn: ((reason: string) => void) | null);
    /** Like onclose, for any number of observers; called at once if already closed. */
    whenClosed(fn: (reason: string) => void): void;
    /** Incoming data channels. Channels that arrive while this is null are held. */
    set ondatachannel(fn: ((ep: DataChannelEndpoint) => void) | null);
    /** Incoming WebTransport sessions, pending. Sessions that arrive while this is null are held. */
    set ontransport(fn: ((t: WTSession) => void) | null);
    /** Server: the client's lp2p-session-request. One that arrives while this is null is held. */
    set onsessionrequest(fn: ((r: SessionRequest) => void) | null);
    requestId(): number;
    get maxDatagramSize(): number;
    get datagramsReliable(): boolean;
    get connectionClosed(): boolean;
    openStream(unidirectional: boolean): TransportStream;
    sendDatagram(d: Uint8Array): void;
    forgetTransport(exchangeId: number): void;
    /** Keying material for one WebTransport session (exporter of the connection's handshake). */
    exportKeyingMaterial(label: string, context: Uint8Array, length: number): Promise<Uint8Array>;
    /** Opens a WebTransport session (1201). It is "connecting" until the peer's user agent accepts it. */
    openTransport(): WTSession;
    private receiveDatagram;
    /** The control stream, once the client has opened it. */
    controlChannel(signal?: AbortSignal): Promise<ControlChannel>;
    /** Exchanges agent-info: answers the peer's request and asks for the peer's. */
    exchangeInfo(local: AgentInfo, signal?: AbortSignal): Promise<AgentInfo>;
    /**
     * Client: sends lp2p-session-request with origin on a new stream. The
     * promise resolves with the server's final result, or rejects if the
     * stream or the connection closes first. A provisional
     * authentication-required calls onAuthenticationRequired.
     */
    requestSession(origin: string, onAuthenticationRequired?: () => void): Promise<number>;
    /**
     * Allocates the lowest free channel ID of the local parity, or validates
     * and reserves a requested one, which must be free and of that parity.
     */
    reserveChannelId(requested?: number): number;
    releaseChannelId(id: number): void;
    /** Opens a data channel; params.id must come from reserveChannelId. */
    openDataChannel(params: ChannelParams): DataChannelEndpoint;
    /**
     * Closes the connection. WebTransport sessions that sent
     * data-transport-close get until their peer ended the control stream, or
     * a timeout, so the close isn't lost.
     */
    close(reason?: string): void;
    /** WebTransport sessions waiting for the peer to end their control stream. Internal. */
    readonly closing: Set<Promise<void>>;
    private setControl;
    /**
     * The session is accepted: the server sent success, or the client got
     * success and verified the server. Streams the client held are routed now.
     */
    markAccepted(): void;
    /** Looks at a new stream's first message to tell what it is. */
    private classify;
    private route;
    private receiveSessionRequest;
    private acceptTransport;
    private acceptDataChannel;
}
