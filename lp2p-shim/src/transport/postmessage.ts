// The postMessage transport: peers in the same browser - windows, iframes,
// workers - talking through structured-clone messaging. Zero setup, no
// network; ideal for demos and tests.
//
// Discovery and connection setup go over a Signaler (by default a
// BroadcastChannel, i.e. same-origin contexts). Each connection then gets
// its own port: a transferred MessageChannel port when the signaler can
// transfer ports (PortSignaler), else a BroadcastChannel named after the
// connection, which only the two ends use. Streams are multiplexed over
// that port; ordering per port is guaranteed by the platform.
//
// Datagrams go over the same port, so they are never lost or reordered:
// the connection reports datagramsReliable, and a WebTransport session on
// it has reliability "reliable-only".

import { BaseConnection, BaseStream, randomId } from "./base.ts";
import { Rendezvous, type RendezvousOptions } from "./rendezvous.ts";
import { BroadcastChannelSignaler, type SignalMessage, type Signaler } from "./signaling.ts";
import type { Advertisement, Closer, DiscoveryEvent, Transport, TransportConnection, TransportStream } from "./types.ts";

export interface PostMessageTransportOptions extends RendezvousOptions {
  /** Defaults to a BroadcastChannelSignaler on channelName. */
  signaler?: Signaler;
  /** BroadcastChannel name for the default signaler. Default "lp2p-shim". */
  channelName?: string;
  /** How long connect waits for the peer to accept, in ms. Default 5000. */
  connectTimeout?: number;
}

/** A per-connection port: MessagePort and BroadcastChannel both fit. */
interface Port {
  postMessage(message: unknown): void;
  onmessage: ((e: MessageEvent) => void) | null;
  close(): void;
}

type Frame =
  | { k: "accept" }
  | { k: "open"; s: number }
  | { k: "msg"; s: number; d: Uint8Array }
  | { k: "close"; s: number }
  | { k: "dgram"; d: Uint8Array }
  | { k: "bye"; reason: string };

class PortStream extends BaseStream {
  private readonly conn: PortConnection;

  constructor(id: number, conn: PortConnection) {
    super(id);
    this.conn = conn;
  }

  send(message: Uint8Array): void {
    if (this.closed) throw new DOMException("stream is closed", "InvalidStateError");
    this.conn.post({ k: "msg", s: this.id, d: message });
  }

  protected doClose(): void {
    this.conn.post({ k: "close", s: this.id });
    this.conn.forget(this.id);
  }

  receive(d: Uint8Array): void {
    this.deliver(d);
  }
}

class PortConnection extends BaseConnection {
  private nextStreamId: number;

  private readonly port: Port;

  constructor(remoteId: string, port: Port, dialer: boolean) {
    super(remoteId);
    this.port = port;
    // Dialer streams are even, listener streams odd, like QUIC.
    this.nextStreamId = dialer ? 0 : 1;
    port.onmessage = (e) => this.onFrame(e.data as Frame);
  }

  override get maxDatagramSize(): number {
    // What fits a QUIC packet on a typical path, so pages see the same limit everywhere.
    return 1200;
  }
  override get datagramsReliable(): boolean {
    return true;
  }

  protected override doSendDatagram(d: Uint8Array): void {
    this.post({ k: "dgram", d });
  }

  openStream(): TransportStream {
    if (this.closed) throw new DOMException("connection is closed", "InvalidStateError");
    const s = new PortStream(this.nextStreamId, this);
    this.nextStreamId += 2;
    this.streams.set(s.id, s);
    this.post({ k: "open", s: s.id });
    return s;
  }

  post(frame: Frame): void {
    if (!this.closed) this.port.postMessage(frame);
  }

  forget(id: number): void {
    this.streams.delete(id);
  }

  protected doClose(reason: string): void {
    this.port.postMessage({ k: "bye", reason } satisfies Frame);
    this.port.close();
  }

  private onFrame(f: Frame): void {
    switch (f.k) {
      case "open":
        this.accept(new PortStream(f.s, this));
        break;
      case "msg": {
        const s = this.streams.get(f.s);
        if (s instanceof PortStream) s.receive(f.d);
        break;
      }
      case "close": {
        const s = this.streams.get(f.s);
        this.streams.delete(f.s);
        if (s instanceof PortStream) s.remoteClosed();
        break;
      }
      case "dgram":
        this.deliverDatagram(f.d);
        break;
      case "bye":
        this.remoteClosed(`closed by peer: ${f.reason}`);
        this.port.close();
        break;
    }
  }
}

export class PostMessageTransport implements Transport {
  readonly name = "postmessage";
  private readonly rendezvous: Rendezvous;
  private readonly canTransfer: boolean;
  private readonly connectTimeout: number;

  constructor(options: PostMessageTransportOptions = {}) {
    const signaler = options.signaler ?? new BroadcastChannelSignaler(options.channelName ?? "lp2p-shim");
    this.canTransfer = signaler.canTransfer;
    this.rendezvous = new Rendezvous(signaler, options);
    this.connectTimeout = options.connectTimeout ?? 5000;
  }

  listen(ad: Advertisement, onconnection: (connection: TransportConnection) => void): Closer {
    return this.rendezvous.advertise(ad, (m: SignalMessage) => {
      if (m.t !== "connect" || typeof m["conn"] !== "string") return;
      const port = (m["port"] as Port | undefined) ?? new BroadcastChannel(connChannelName(m["conn"]));
      port.postMessage({ k: "accept" } satisfies Frame);
      onconnection(new PortConnection(m.from, port, false));
    });
  }

  discover(onevent: (event: DiscoveryEvent) => void): Closer {
    return this.rendezvous.discover(onevent);
  }

  connect(peerId: string, local: Advertisement, options: { signal?: AbortSignal } = {}): Promise<TransportConnection> {
    const conn = randomId();
    let port: Port;
    const msg: SignalMessage = { t: "connect", from: local.id, to: peerId, conn, nickname: local.nickname };
    const transfer: Transferable[] = [];
    if (this.canTransfer) {
      const mc = new MessageChannel();
      port = mc.port1;
      msg["port"] = mc.port2;
      transfer.push(mc.port2);
    } else {
      port = new BroadcastChannel(connChannelName(conn));
    }
    return new Promise<TransportConnection>((resolve, reject) => {
      const fail = (err: unknown): void => {
        clearTimeout(timer);
        options.signal?.removeEventListener("abort", onAbort);
        port.onmessage = null;
        port.close();
        reject(err);
      };
      const onAbort = (): void => fail(options.signal?.reason);
      const timer = setTimeout(
        () => fail(new DOMException(`peer ${peerId} did not answer`, "NetworkError")),
        this.connectTimeout,
      );
      options.signal?.addEventListener("abort", onAbort);
      port.onmessage = (e: MessageEvent) => {
        if ((e.data as Frame).k !== "accept") return;
        clearTimeout(timer);
        options.signal?.removeEventListener("abort", onAbort);
        resolve(new PortConnection(peerId, port, true));
      };
      this.rendezvous.send(msg, transfer);
    });
  }

  /** Stops all advertising and discovery and releases the signaler. */
  close(): void {
    this.rendezvous.close();
  }
}

function connChannelName(conn: string): string {
  return `lp2p-shim/conn/${conn}`;
}
