import { type LP2PConnection } from "./connection.js";
export interface LP2PRequestOptions {
    nickname?: string;
}
export declare class LP2PRequest {
    #private;
    constructor(options?: LP2PRequestOptions);
    /**
     * Shows the device picker, connects to the chosen peer and, if needed,
     * pairs with it. Rejects with NotAllowedError on every failure: dismissed
     * picker, unreachable peer, failed pairing or refused session, so script
     * can't tell them apart. While pending or after success, calling it again
     * returns the same promise; after a failure it starts over.
     */
    start(): Promise<LP2PConnection>;
}
