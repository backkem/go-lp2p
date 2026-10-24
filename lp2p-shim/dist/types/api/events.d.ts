import type { LP2PConnection } from "./connection.js";
import type { LP2PDataChannel } from "./datachannel.js";
export interface LP2PConnectionEventInit extends EventInit {
    connection: LP2PConnection;
}
export declare class LP2PConnectionEvent extends Event {
    constructor(type: string, connectionEventInitDict: LP2PConnectionEventInit);
    get connection(): LP2PConnection;
}
export interface LP2PDataChannelEventInit extends EventInit {
    channel: LP2PDataChannel;
}
export declare class LP2PDataChannelEvent extends Event {
    constructor(type: string, DataChannelEventInitDict: LP2PDataChannelEventInit);
    get channel(): LP2PDataChannel;
}
type Handler = ((this: EventTarget, ev: Event) => unknown) | null;
/** Getter half of an event handler IDL attribute. */
export declare function getHandler(target: EventTarget, type: string): Handler;
/**
 * Setter half of an event handler IDL attribute: like the platform, the
 * handler keeps its place among the listeners when replaced, and anything
 * that is not a function clears it.
 */
export declare function setHandler(target: EventTarget, type: string, fn: unknown): void;
export {};
