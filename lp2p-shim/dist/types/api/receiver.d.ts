import type { Session } from "../protocol/session.js";
import { LP2PConnectionEvent } from "./events.js";
export interface LP2PReceiverOptions {
    nickname?: string;
}
/**
 * Calls fn for each connection the receiver gets from now on and, with
 * existing, for those it has. Returns a function that stops watching. Internal.
 */
export declare function watchSessions(receiver: LP2PReceiver, fn: (s: Session) => void, existing: boolean): () => void;
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
