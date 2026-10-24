import { type RendezvousOptions } from "./rendezvous.js";
import { type Signaler } from "./signaling.js";
import type { Advertisement, Closer, DiscoveryEvent, Transport, TransportConnection } from "./types.js";
export interface PostMessageTransportOptions extends RendezvousOptions {
    /** Defaults to a BroadcastChannelSignaler on channelName. */
    signaler?: Signaler;
    /** BroadcastChannel name for the default signaler. Default "lp2p-shim". */
    channelName?: string;
    /** How long connect waits for the peer to accept, in ms. Default 5000. */
    connectTimeout?: number;
}
export declare class PostMessageTransport implements Transport {
    readonly name = "postmessage";
    private readonly rendezvous;
    private readonly canTransfer;
    private readonly connectTimeout;
    constructor(options?: PostMessageTransportOptions);
    listen(ad: Advertisement, onconnection: (connection: TransportConnection) => void): Closer;
    discover(onevent: (event: DiscoveryEvent) => void): Closer;
    connect(peerId: string, local: Advertisement, options?: {
        signal?: AbortSignal;
    }): Promise<TransportConnection>;
    /** Stops all advertising and discovery and releases the signaler. */
    close(): void;
}
