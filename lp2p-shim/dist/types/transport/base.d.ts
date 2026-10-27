import type { StreamOptions, TransportConnection, TransportStream } from "./types.js";
export declare abstract class BaseStream implements TransportStream {
    private readonly inbox;
    private closeHandler;
    private closeFired;
    private isClosed;
    readonly id: number;
    readonly unidirectional: boolean;
    constructor(id: number, unidirectional?: boolean);
    get closed(): boolean;
    get onmessage(): ((message: Uint8Array) => void) | null;
    set onmessage(fn: ((message: Uint8Array) => void) | null);
    get onclose(): (() => void) | null;
    set onclose(fn: (() => void) | null);
    abstract send(message: Uint8Array): void;
    protected abstract doClose(): void;
    close(): void;
    /** For implementations: deliver a received message. */
    protected deliver(message: Uint8Array): void;
    /**
     * For implementations: the peer closed the stream or the connection went
     * away. Public so connections can close their streams; not for API users.
     */
    remoteClosed(): void;
    private maybeFireClose;
}
export declare abstract class BaseConnection implements TransportConnection {
    private readonly incoming;
    private closeHandler;
    private closeReason;
    private closeFired;
    protected readonly streams: Map<number, TransportStream>;
    private datagramHandler;
    private readonly heldDatagrams;
    readonly remoteId: string;
    /** Overridden by transports that carry datagrams. */
    get maxDatagramSize(): number;
    get datagramsReliable(): boolean;
    constructor(remoteId: string);
    get closed(): boolean;
    get onstream(): ((stream: TransportStream) => void) | null;
    set onstream(fn: ((stream: TransportStream) => void) | null);
    get onclose(): ((reason: string) => void) | null;
    set onclose(fn: ((reason: string) => void) | null);
    get ondatagram(): ((datagram: Uint8Array) => void) | null;
    set ondatagram(fn: ((datagram: Uint8Array) => void) | null);
    abstract openStream(options?: StreamOptions): TransportStream;
    protected abstract doClose(reason: string): void;
    sendDatagram(datagram: Uint8Array): void;
    /** For transports with datagrams: send one. */
    protected doSendDatagram(_datagram: Uint8Array): void;
    /** For implementations: deliver a received datagram. */
    protected deliverDatagram(datagram: Uint8Array): void;
    close(reason?: string): void;
    /** For implementations: a stream the peer opened. */
    protected accept(stream: TransportStream): void;
    /** For implementations: the peer or the medium closed the connection. */
    protected remoteClosed(reason: string): void;
    private teardown;
    private maybeFireClose;
}
export declare function randomId(bytes?: number): string;
