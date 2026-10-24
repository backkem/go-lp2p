import { type LP2PConnection } from "./connection.js";
export interface LP2PRequestOptions {
    nickname?: string;
}
export declare class LP2PRequest {
    #private;
    constructor(options?: LP2PRequestOptions);
    /**
     * Shows the device picker, connects to the chosen peer and pairs with it.
     * Rejects with NotAllowedError if the user dismisses the picker, the peer
     * declines or pairing fails, and NetworkError if the peer is unreachable.
     * While pending or after success, calling it again returns the same
     * promise; after a failure it starts over.
     */
    start(): Promise<LP2PConnection>;
}
