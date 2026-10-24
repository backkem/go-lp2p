import { type RendezvousOptions } from "./rendezvous.js";
import { type Signaler } from "./signaling.js";
import type { Advertisement, Closer, DiscoveryEvent, Transport, TransportConnection } from "./types.js";
export interface WebRTCTransportOptions extends RendezvousOptions {
    /** Defaults to a BroadcastChannelSignaler on channelName. */
    signaler?: Signaler;
    /** BroadcastChannel name for the default signaler. Default "lp2p-shim/webrtc". */
    channelName?: string;
    /** Passed to RTCPeerConnection. The default has no ICE servers: host candidates only, i.e. same LAN. */
    rtcConfiguration?: RTCConfiguration;
    /**
     * How long connect waits, in ms. Default 15000 on a live signaler and
     * 10 minutes on a manual one (a human is copying codes).
     */
    connectTimeout?: number;
}
export declare class WebRTCTransport implements Transport {
    readonly name = "webrtc";
    private readonly rendezvous;
    private readonly trickle;
    private readonly connectTimeout;
    private readonly rtcConfiguration;
    constructor(options?: WebRTCTransportOptions);
    listen(ad: Advertisement, onconnection: (connection: TransportConnection) => void): Closer;
    discover(onevent: (event: DiscoveryEvent) => void): Closer;
    connect(peerId: string, local: Advertisement, options?: {
        signal?: AbortSignal;
    }): Promise<TransportConnection>;
    close(): void;
}
