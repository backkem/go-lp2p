import type { TransportConnection, TransportStream } from "../transport/types.js";
import { DataEncoding, type AgentInfo, type Message } from "./messages.js";
import { WTSession, type WTHost } from "./webtransport.js";
export type Role = "requester" | "receiver";
export declare class ProtocolError extends Error {
    name: string;
}
/** The control stream, with a queue of messages the caller has not consumed yet. */
export declare class ControlChannel {
    private readonly queue;
    private waiter;
    private closedWith;
    private info;
    private readonly unanswered;
    private readonly stream;
    constructor(stream: TransportStream);
    send(m: Message): void;
    /** What to answer agent-info-request with. Requests that came earlier are answered now. */
    set localInfo(info: AgentInfo);
    /** Resolves with the next message of one of types; earlier messages of other types are dropped. */
    next<T extends Message["type"]>(types: readonly T[], signal?: AbortSignal): Promise<Extract<Message, {
        type: T;
    }>>;
    close(): void;
    /** Handles one raw message from the control stream. */
    feed(b: Uint8Array): void;
    private fail;
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
    private control;
    private readonly controlWaiters;
    private readonly usedIds;
    private readonly channelIdParity;
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
    readonly connection: TransportConnection;
    constructor(connection: TransportConnection, role: Role);
    get closed(): boolean;
    set onclose(fn: ((reason: string) => void) | null);
    /** Like onclose, for any number of observers; called at once if already closed. */
    whenClosed(fn: (reason: string) => void): void;
    /** Incoming data channels. Channels that arrive while this is null are held. */
    set ondatachannel(fn: ((ep: DataChannelEndpoint) => void) | null);
    /** Incoming WebTransport sessions, pending. Sessions that arrive while this is null are held. */
    set ontransport(fn: ((t: WTSession) => void) | null);
    requestId(): number;
    get maxDatagramSize(): number;
    get datagramsReliable(): boolean;
    openStream(): TransportStream;
    sendDatagram(d: Uint8Array): void;
    forgetTransport(exchangeId: number): void;
    /** Opens a WebTransport session (1201). It is "connecting" until the peer's page takes it. */
    openTransport(): WTSession;
    private receiveDatagram;
    /** The control stream, once the requester has opened it. */
    controlChannel(signal?: AbortSignal): Promise<ControlChannel>;
    /** Exchanges agent-info: answers the peer's request and asks for the peer's. */
    handshake(local: AgentInfo, signal?: AbortSignal): Promise<AgentInfo>;
    /**
     * Allocates the lowest free channel ID of the local parity, or validates
     * and reserves a requested one, which must be free and of that parity.
     */
    reserveChannelId(requested?: number): number;
    releaseChannelId(id: number): void;
    /** Opens a data channel; params.id must come from reserveChannelId. */
    openDataChannel(params: ChannelParams): DataChannelEndpoint;
    close(reason?: string): void;
    private setControl;
    /** Looks at a new stream's first message to tell the control stream from data channels. */
    private classify;
    private acceptTransport;
    private acceptDataChannel;
}
