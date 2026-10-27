// The local peer-to-peer manager's state (spec "Local state"): the local
// trust store, the known peers map and the peer grants, plus the device's
// own identity and the receivers advertising on it.
//
// One LocalState stands for one device (browser profile). By default each
// Transport object gets its own, so two "devices" in one test or page don't
// share trust; pass ShimConfig.localState to share or inspect one. It lives
// in memory only: the shim does not persist anything.

import { createIdentity, type Identity } from "../protocol/identity.ts";
import type { Session } from "../protocol/session.ts";

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

export class LocalState {
  readonly anchors = new Map<string, TrustAnchor>();
  readonly knownPeers = new Map<string, KnownPeer>();
  readonly grants = new Map<string, Set<string>>();
  /** Receivers currently advertising on this device, in start order. */
  readonly receivers = new Set<ReceiverRegistration>();
  #identity: Promise<Identity> | null = null;

  /** The device's agent key pair, created on first use. */
  identity(): Promise<Identity> {
    return (this.#identity ??= createIdentity());
  }

  /** Whether identity chains to an anchor in the trust store (spec "authenticated"). Derived, never stored. */
  isAuthenticated(identity: string): boolean {
    for (const a of this.anchors.values()) {
      if (a.scope === "one-peer" && a.subject === identity && a.key === identity) return true;
    }
    return false;
  }

  /** A trust ceremony with a self-anchored peer adds a one-peer anchor on its key. */
  addSelfAnchor(identity: string): void {
    this.anchors.set(`one-peer:${identity}`, { key: identity, scope: "one-peer", subject: identity });
  }

  /** Adds a vouching anchor, which replaces the one-peer anchors with the same key. */
  addVouchingAnchor(key: string): void {
    for (const [id, a] of this.anchors) if (a.scope === "one-peer" && a.key === key) this.anchors.delete(id);
    this.anchors.set(`vouching:${key}`, { key, scope: "vouching", subject: "" });
  }

  /** Removes an anchor, and the grants of peers that are no longer authenticated. */
  removeAnchor(anchor: TrustAnchor): void {
    for (const [id, a] of this.anchors) {
      if (a.key === anchor.key && a.scope === anchor.scope && a.subject === anchor.subject) this.anchors.delete(id);
    }
    for (const peers of this.grants.values()) {
      for (const p of [...peers]) if (!this.isAuthenticated(p)) peers.delete(p);
    }
  }

  rememberPeer(p: KnownPeer): void {
    this.knownPeers.set(p.identity, p);
  }

  /** Removes a known peer, and it from all peer grants. */
  forgetPeer(identity: string): void {
    this.knownPeers.delete(identity);
    for (const peers of this.grants.values()) peers.delete(identity);
  }

  hasGrant(origin: string, identity: string): boolean {
    return this.grants.get(origin)?.has(identity) ?? false;
  }

  addGrant(origin: string, identity: string): void {
    let s = this.grants.get(origin);
    if (!s) this.grants.set(origin, (s = new Set()));
    s.add(identity);
  }

  /** Clearing an origin's site data or permissions removes its peer grants. */
  clearOrigin(origin: string): void {
    this.grants.delete(origin);
  }
}

const perTransport = new WeakMap<object, LocalState>();

/** The default local state of the device a transport object stands for. */
export function localStateFor(transport: object): LocalState {
  let s = perTransport.get(transport);
  if (!s) perTransport.set(transport, (s = new LocalState()));
  return s;
}

/**
 * Parses an origin as the spec's URL parser would and returns its ASCII
 * serialization, "null" for an opaque origin, or null if parsing fails.
 */
export function parseOrigin(value: string): string | null {
  try {
    return new URL(value).origin;
  } catch {
    return null;
  }
}
