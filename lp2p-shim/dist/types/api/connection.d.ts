import type { Session } from "../protocol/session.js";
import type { WTSession } from "../protocol/webtransport.js";
import { type LP2PDataChannel, type LP2PDataChannelInit } from "./datachannel.js";
import { LP2PDataChannelEvent } from "./events.js";
/** What quictransport.ts needs of a connection: its [[Transports]] and [[Listener]] slots. Internal. */
export interface ConnectionSlots {
    readonly session: Session;
    /** [[Transports]]: failed when the connection closes. */
    readonly transports: Set<{
        failFromConnection(): void;
    }>;
    /** [[Listener]] */
    listener: {
        closeFromConnection(): void;
        incoming(t: WTSession): boolean;
    } | null;
    /** [[Closed]] */
    closed: boolean;
}
/** The internal slots of a connection. Internal. */
export declare function slotsOf(connection: LP2PConnection): ConnectionSlots;
/** The session behind a connection. Internal. */
export declare function sessionOf(connection: LP2PConnection): Session;
/** Creates the API object for an accepted session (spec "create an LP2PConnection"). Internal. */
export declare function adoptConnection(session: Session): LP2PConnection;
export declare class LP2PConnection extends EventTarget {
    #private;
    /** Not constructible from script: the IDL has no constructor. */
    constructor();
    createDataChannel(label: string, dataChannelDict?: LP2PDataChannelInit): LP2PDataChannel;
    get ondatachannel(): ((this: EventTarget, ev: LP2PDataChannelEvent) => unknown) | null;
    set ondatachannel(fn: ((this: EventTarget, ev: LP2PDataChannelEvent) => unknown) | null);
    /** Closes the session, its data channels and its transports, on both sides; then fires close. */
    close(): void;
    get onclose(): ((this: EventTarget, ev: Event) => unknown) | null;
    set onclose(fn: ((this: EventTarget, ev: Event) => unknown) | null);
}
