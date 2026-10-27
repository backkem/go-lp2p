import type { Transport } from "./transport/types.js";
import { type UserAgentUI } from "./ua/ui.js";
import { UserAgent, type Pairing } from "./ua/useragent.js";
import type { LocalState } from "./ua/localstate.js";
/**
 * Behavior that departs from the LP2P spec, each off by default, like
 * go-lp2p's ospc.InteropQuirks. Turn them on to experiment, never to ship.
 */
export interface ShimQuirks {
    /**
     * Gives the transports of lp2p.makeDiscoverable() and lp2p.connect() a
     * non-standard `connection` property: the LP2PConnection the shorthand
     * made for them. The explainer's shorthand hides it, so a page can't
     * open data channels on it, close it, or listen on it itself.
     */
    exposeShorthandConnection?: boolean;
}
export interface ShimConfig {
    transport?: Transport;
    ui?: UserAgentUI;
    pairing?: Pairing;
    /** The page's origin: the requesting origin of its sessions. Default location.origin. */
    origin?: string;
    /** This device's name, sent as agent-info display-name. Not the page's nickname. */
    deviceName?: string;
    /** The device's trust store, known peers and grants. Default: one per transport object. */
    localState?: LocalState;
    quirks?: ShimQuirks;
    log?: (line: string) => void;
}
/** Merges c into the configuration. Objects started afterwards use it. */
export declare function configure(c: ShimConfig): void;
/** The current configuration. */
export declare function getConfig(): Readonly<ShimConfig>;
/** The user agent LP2P objects use when they start. */
export declare function getUserAgent(): UserAgent;
