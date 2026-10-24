import { LP2PConnectionEvent } from "./events.js";
export interface LP2PReceiverOptions {
    nickname?: string;
}
export declare class LP2PReceiver extends EventTarget {
    #private;
    constructor(options?: LP2PReceiverOptions);
    /**
     * Asks the user for permission to advertise and starts advertising.
     * Resolves once advertising; rejects with NotAllowedError if denied.
     * Calling it again returns the same promise (the spec does not say).
     */
    start(): Promise<undefined>;
    /** Extension: not in the LP2P IDL. Stops advertising; existing connections stay open. */
    stop(): void;
    get onconnection(): ((this: EventTarget, ev: LP2PConnectionEvent) => unknown) | null;
    set onconnection(fn: ((this: EventTarget, ev: LP2PConnectionEvent) => unknown) | null);
}
