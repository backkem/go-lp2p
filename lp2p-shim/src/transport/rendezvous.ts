// Discovery and addressed messaging over a Signaler, shared by the
// postMessage and WebRTC transports.
//
// Discovery is pull-based: a discovering peer broadcasts "query" every
// queryInterval and listeners answer with "announce"; peers not heard from
// within ttl are dropped. Pulling from the (visible) page that shows the
// picker keeps working when the advertising page is a background tab whose
// timers the browser throttles. Listeners also announce once when they
// start and say "bye" when they stop or the page goes away.

import type { Advertisement, Closer, DiscoveredPeer, DiscoveryEvent } from "./types.ts";
import type { SignalMessage, Signaler } from "./signaling.ts";

export interface RendezvousOptions {
  /** How often discovery queries are sent on a live medium, in ms. */
  queryInterval?: number;
  /** How long a silent peer stays listed on a live medium, in ms. */
  ttl?: number;
}

type Handler = (message: SignalMessage) => void;

export class Rendezvous {
  private readonly ads = new Map<string, Advertisement>();
  private readonly handlers = new Map<string, Handler>();
  private readonly discoverers = new Set<(m: SignalMessage) => void>();
  /**
   * Recent announcements, replayed to new discoveries: on a manual medium the
   * user may paste a peer's code before opening the picker.
   */
  private readonly recent = new Map<string, { message: SignalMessage; seen: number }>();
  private readonly queryInterval: number;
  private readonly ttl: number;
  private readonly onPageHide = (): void => {
    for (const id of this.ads.keys()) this.signaler.post({ t: "bye", from: id });
  };

  readonly signaler: Signaler;

  constructor(signaler: Signaler, options: RendezvousOptions = {}) {
    this.signaler = signaler;
    this.queryInterval = options.queryInterval ?? 1000;
    this.ttl = options.ttl ?? 3500;
    signaler.onmessage = (m) => this.receive(m);
    globalThis.addEventListener?.("pagehide", this.onPageHide);
  }

  /** Advertises ad until closed; addressed messages for ad.id go to onmessage. */
  advertise(ad: Advertisement, onmessage: Handler): Closer {
    this.ads.set(ad.id, ad);
    const reg = this.register(ad.id, onmessage);
    this.announce(ad);
    return {
      close: () => {
        if (!this.ads.delete(ad.id)) return;
        reg.close();
        this.signaler.post({ t: "bye", from: ad.id });
      },
    };
  }

  /** Receives messages addressed to id until closed. */
  register(id: string, onmessage: Handler): Closer {
    this.handlers.set(id, onmessage);
    return { close: () => void (this.handlers.get(id) === onmessage && this.handlers.delete(id)) };
  }

  send(message: SignalMessage, transfer?: Transferable[]): void {
    this.signaler.post(message, transfer);
  }

  discover(onevent: (event: DiscoveryEvent) => void): Closer {
    const peers = new Map<string, { peer: DiscoveredPeer; seen: number }>();
    const onmessage = (m: SignalMessage): void => {
      if (m.t === "announce") {
        const peer = { id: m.from, nickname: typeof m["nickname"] === "string" ? m["nickname"] : "" };
        const prev = peers.get(peer.id);
        peers.set(peer.id, { peer, seen: Date.now() });
        if (!prev) onevent({ kind: "added", peer });
        else if (prev.peer.nickname !== peer.nickname) onevent({ kind: "updated", peer });
      } else if (m.t === "bye") {
        const prev = peers.get(m.from);
        if (prev) {
          peers.delete(m.from);
          onevent({ kind: "removed", peer: prev.peer });
        }
      }
    };
    this.discoverers.add(onmessage);
    const fresh = Date.now() - this.ttl;
    for (const r of this.recent.values()) {
      if (!this.signaler.live || r.seen >= fresh) onmessage(r.message);
    }
    const query = (): void => this.signaler.post({ t: "query", from: "" });
    query();
    let timer: ReturnType<typeof setInterval> | undefined;
    if (this.signaler.live) {
      timer = setInterval(() => {
        const cutoff = Date.now() - this.ttl;
        for (const [id, p] of peers) {
          if (p.seen < cutoff) {
            peers.delete(id);
            onevent({ kind: "removed", peer: p.peer });
          }
        }
        query();
      }, this.queryInterval);
    }
    return {
      close: () => {
        this.discoverers.delete(onmessage);
        if (timer !== undefined) clearInterval(timer);
      },
    };
  }

  close(): void {
    for (const id of this.ads.keys()) this.signaler.post({ t: "bye", from: id });
    this.ads.clear();
    this.handlers.clear();
    this.discoverers.clear();
    globalThis.removeEventListener?.("pagehide", this.onPageHide);
    this.signaler.close();
  }

  private announce(ad: Advertisement): void {
    this.signaler.post({ t: "announce", from: ad.id, nickname: ad.nickname });
  }

  private receive(m: SignalMessage): void {
    if (m.to !== undefined) {
      this.handlers.get(m.to)?.(m);
      return;
    }
    switch (m.t) {
      case "query":
        for (const ad of this.ads.values()) this.announce(ad);
        break;
      case "announce":
      case "bye":
        if (m.t === "announce") this.recent.set(m.from, { message: m, seen: Date.now() });
        else this.recent.delete(m.from);
        for (const d of this.discoverers) d(m);
        break;
    }
  }
}
