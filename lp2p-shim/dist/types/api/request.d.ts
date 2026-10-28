import { type LP2PConnection } from "./connection.js";
export interface LP2PRequestOptions {
    nickname?: string;
}
export interface LP2PRequestStartOptions {
    signal?: AbortSignal;
}
export declare class LP2PRequest {
    #private;
    constructor(options?: LP2PRequestOptions);
    /**
     * Shows the device picker, connects to the chosen peer and, if needed,
     * pairs with it. Rejects with NotAllowedError on every failure: dismissed
     * picker, unreachable peer, failed pairing or refused session, so script
     * can't tell them apart. options.signal aborts it: the promise rejects with
     * the abort reason and the picker or the session being set up is taken
     * down. While pending or after success, calling it again returns the same
     * promise and ignores the new options; aborting after success does
     * nothing. After a failure it starts over.
     */
    start(options?: LP2PRequestStartOptions): Promise<LP2PConnection>;
}
