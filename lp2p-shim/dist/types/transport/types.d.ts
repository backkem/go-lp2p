/** What a listening peer makes discoverable. */
export interface Advertisement {
    /** Transport-level peer ID; unique per listener. */
    id: string;
    /** The page-provided nickname (LP2PReceiverOptions.nickname). Unverified. */
    nickname: string;
}
/** A peer seen by discovery. */
export interface DiscoveredPeer {
    id: string;
    nickname: string;
}
export interface DiscoveryEvent {
    kind: "added" | "updated" | "removed";
    peer: DiscoveredPeer;
}
/** Something that can be stopped: an advertisement or a discovery session. */
export interface Closer {
    close(): void;
}
export interface Transport {
    /** Short name for logs and UI, e.g. "postmessage" or "webrtc". */
    readonly name: string;
    /**
     * Starts advertising ad. Every incoming connection is passed to
     * onconnection; the transport does no policy, the caller decides whether
     * to keep or close it.
     */
    listen(ad: Advertisement, onconnection: (connection: TransportConnection) => void): Closer;
    /** Reports peers as they appear, change and disappear, until closed. */
    discover(onevent: (event: DiscoveryEvent) => void): Closer;
    /**
     * Connects to the discovered peer peerId, identifying as local. Resolves
     * once the connection can carry streams.
     */
    connect(peerId: string, local: Advertisement, options?: {
        signal?: AbortSignal;
    }): Promise<TransportConnection>;
}
export interface TransportConnection {
    /** The remote peer's transport-level ID. */
    readonly remoteId: string;
    /** True once the connection is closed, locally or by the peer. */
    readonly closed: boolean;
    /**
     * Opens a stream. It is usable at once; messages sent before the peer has
     * seen the stream are queued. A unidirectional stream is one the opener
     * only sends on, like a QUIC unidirectional stream: the peer learns its
     * direction from the stream itself, not from a message on it.
     */
    openStream(options?: StreamOptions): TransportStream;
    /**
     * Called for every stream the peer opens. Streams that arrive while this
     * is null are held until it is set.
     */
    onstream: ((stream: TransportStream) => void) | null;
    /** Called once when the connection closes, for whatever reason. */
    onclose: ((reason: string) => void) | null;
    /** Closes the connection and all its streams. */
    close(reason?: string): void;
    /** The largest datagram sendDatagram takes, in bytes; 0 if the connection has no datagrams. */
    readonly maxDatagramSize: number;
    /** True if datagrams are in fact delivered reliably and in order (no loss is possible). */
    readonly datagramsReliable: boolean;
    /**
     * Sends one datagram, at most maxDatagramSize bytes. It may be lost,
     * duplicated or reordered. Throws if the connection is closed.
     */
    sendDatagram(datagram: Uint8Array): void;
    /** Called for each datagram. A few that arrive while this is null are held; older ones are dropped. */
    ondatagram: ((datagram: Uint8Array) => void) | null;
}
export interface StreamOptions {
    /** Only the opener sends. Default false. */
    unidirectional?: boolean;
}
export interface TransportStream {
    /** Unique within its connection. */
    readonly id: number;
    /** True for a stream only its opener sends on. The transport doesn't enforce it. */
    readonly unidirectional: boolean;
    readonly closed: boolean;
    /** Sends one message. Messages arrive whole, reliably and in order. */
    send(message: Uint8Array): void;
    /** Called for each message. Messages that arrive while this is null are held. */
    onmessage: ((message: Uint8Array) => void) | null;
    /**
     * Called once when the stream closes: after the peer closed it (and every
     * message it sent before that was delivered), after close(), or when the
     * connection closed.
     */
    onclose: (() => void) | null;
    /** Closes the stream in both directions. */
    close(): void;
}
