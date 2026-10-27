import { LP2PConnection } from "./api/connection.js";
import { LP2PDataChannel } from "./api/datachannel.js";
import { LP2PConnectionEvent, LP2PDataChannelEvent } from "./api/events.js";
import { LP2PReceiver } from "./api/receiver.js";
import { LP2PRequest } from "./api/request.js";
import { LP2PQuicTransport, LP2PQuicTransportListener } from "./api/quictransport.js";
export declare const interfaces: {
    readonly LP2PReceiver: typeof LP2PReceiver;
    readonly LP2PRequest: typeof LP2PRequest;
    readonly LP2PConnection: typeof LP2PConnection;
    readonly LP2PDataChannel: typeof LP2PDataChannel;
    readonly LP2PConnectionEvent: typeof LP2PConnectionEvent;
    readonly LP2PDataChannelEvent: typeof LP2PDataChannelEvent;
    readonly LP2PQuicTransport: typeof LP2PQuicTransport;
    readonly LP2PQuicTransportListener: typeof LP2PQuicTransportListener;
    readonly lp2p: Readonly<{
        makeDiscoverable: typeof import("./index.js").makeDiscoverable;
        connect: typeof import("./index.js").connect;
    }>;
};
export interface InstallOptions {
    /** Replace interfaces the browser already has. Default false. */
    force?: boolean;
}
/**
 * Defines the LP2P interfaces on globalThis where missing (or everywhere
 * with force). Like the real API ([SecureContext]), nothing is installed in
 * an insecure context. Returns the names that were installed.
 */
export declare function install(options?: InstallOptions): string[];
