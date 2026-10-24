import type { Advertisement, Closer, DiscoveryEvent } from "./types.js";
import type { SignalMessage, Signaler } from "./signaling.js";
export interface RendezvousOptions {
    /** How often discovery queries are sent on a live medium, in ms. */
    queryInterval?: number;
    /** How long a silent peer stays listed on a live medium, in ms. */
    ttl?: number;
}
type Handler = (message: SignalMessage) => void;
export declare class Rendezvous {
    private readonly ads;
    private readonly handlers;
    private readonly discoverers;
    /**
     * Recent announcements, replayed to new discoveries: on a manual medium the
     * user may paste a peer's code before opening the picker.
     */
    private readonly recent;
    private readonly queryInterval;
    private readonly ttl;
    private readonly onPageHide;
    readonly signaler: Signaler;
    constructor(signaler: Signaler, options?: RendezvousOptions);
    /** Advertises ad until closed; addressed messages for ad.id go to onmessage. */
    advertise(ad: Advertisement, onmessage: Handler): Closer;
    /** Receives messages addressed to id until closed. */
    register(id: string, onmessage: Handler): Closer;
    send(message: SignalMessage, transfer?: Transferable[]): void;
    discover(onevent: (event: DiscoveryEvent) => void): Closer;
    close(): void;
    private announce;
    private receive;
}
export {};
