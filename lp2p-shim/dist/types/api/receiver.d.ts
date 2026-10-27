import { LP2PConnectionEvent } from "./events.js";
export interface LP2PReceiverOptions {
    nickname?: string;
    /** URLs whose origins, besides the receiver's own, may request sessions. */
    allowedOrigins?: string[];
}
export declare class LP2PReceiver extends EventTarget {
    #private;
    constructor(options?: LP2PReceiverOptions);
    /**
     * Asks the user for permission to advertise and starts advertising.
     * Resolves once advertising; rejects with NotAllowedError if denied, and
     * InvalidStateError after stop(). Calling it again returns the same
     * promise while pending or after success.
     */
    start(): Promise<undefined>;
    /** Stops advertising. Connections already delivered stay open. The receiver can't be started again. */
    stop(): void;
    get onconnection(): ((this: EventTarget, ev: LP2PConnectionEvent) => unknown) | null;
    set onconnection(fn: ((this: EventTarget, ev: LP2PConnectionEvent) => unknown) | null);
}
