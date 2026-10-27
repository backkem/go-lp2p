import { type Identity } from "../protocol/identity.js";
import type { Session } from "../protocol/session.js";
/** A key the device trusts to authenticate peers. */
export interface TrustAnchor {
    /** The key: an agent fingerprint in the shim. */
    key: string;
    /** "one-peer": only the peer with that key. "vouching" anchors are modeled, never added by the shim. */
    scope: "one-peer" | "vouching";
    /** For one-peer: the identity the anchor covers (its own key for a self-anchored peer). */
    subject: string;
}
/** A peer the device has had a session with. */
export interface KnownPeer {
    /** Peer identity: the agent fingerprint. */
    identity: string;
    /** The anchor key its credentials chained to when last seen, so anchor removal can be applied offline. */
    anchorKey: string;
    /** agent-info display-name, obtained on a session that was then authenticated. */
    displayName: string;
}
/** A started LP2PReceiver, as the user agent routes incoming sessions to it. */
export interface ReceiverRegistration {
    /** The receiver page's origin. */
    readonly origin: string;
    /** Whether the receiver allows origin (spec "allows an origin"). */
    allows(origin: string): boolean;
    /** Hands an accepted session to the receiver; false if it is no longer advertising. */
    deliver(session: Session): boolean;
}
export declare class LocalState {
    #private;
    readonly anchors: Map<string, TrustAnchor>;
    readonly knownPeers: Map<string, KnownPeer>;
    readonly grants: Map<string, Set<string>>;
    /** Receivers currently advertising on this device, in start order. */
    readonly receivers: Set<ReceiverRegistration>;
    /** The device's agent key pair, created on first use. */
    identity(): Promise<Identity>;
    /** Whether identity chains to an anchor in the trust store (spec "authenticated"). Derived, never stored. */
    isAuthenticated(identity: string): boolean;
    /** A trust ceremony with a self-anchored peer adds a one-peer anchor on its key. */
    addSelfAnchor(identity: string): void;
    /** Adds a vouching anchor, which replaces the one-peer anchors with the same key. */
    addVouchingAnchor(key: string): void;
    /** Removes an anchor, and the grants of peers that are no longer authenticated. */
    removeAnchor(anchor: TrustAnchor): void;
    rememberPeer(p: KnownPeer): void;
    /** Removes a known peer, and it from all peer grants. */
    forgetPeer(identity: string): void;
    hasGrant(origin: string, identity: string): boolean;
    addGrant(origin: string, identity: string): void;
    /** Clearing an origin's site data or permissions removes its peer grants. */
    clearOrigin(origin: string): void;
}
/** The default local state of the device a transport object stands for. */
export declare function localStateFor(transport: object): LocalState;
/**
 * Parses an origin as the spec's URL parser would and returns its ASCII
 * serialization, "null" for an opaque origin, or null if parsing fails.
 */
export declare function parseOrigin(value: string): string | null;
