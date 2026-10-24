import type { Session } from "../protocol/session.js";
import { type LP2PDataChannel, type LP2PDataChannelInit } from "./datachannel.js";
import { LP2PDataChannelEvent } from "./events.js";
/** Creates the API object for an authenticated session. Internal. */
export declare function adoptConnection(session: Session): LP2PConnection;
export declare class LP2PConnection extends EventTarget {
    #private;
    /** Not constructible from script: the IDL has no constructor. */
    constructor();
    createDataChannel(label: string, dataChannelDict?: LP2PDataChannelInit): LP2PDataChannel;
    get ondatachannel(): ((this: EventTarget, ev: LP2PDataChannelEvent) => unknown) | null;
    set ondatachannel(fn: ((this: EventTarget, ev: LP2PDataChannelEvent) => unknown) | null);
    /** Extension: not in the LP2P IDL. Closes the connection and all its channels; fires close. */
    close(): void;
    /** Extension: fired when the connection closes, locally or by the peer. */
    get onclose(): ((this: EventTarget, ev: Event) => unknown) | null;
    set onclose(fn: ((this: EventTarget, ev: Event) => unknown) | null);
}
