// The WebRTC transport: RTCPeerConnection with one RTCDataChannel per stream.
//
// Each transport stream is its own reliable, ordered RTCDataChannel. That
// mirrors OSP's QUIC stream per LP2P data channel: SCTP streams are
// independent, so a large transfer on one channel does not hold up another
// (no head-of-line blocking), and closing a stream is a native channel close.
// Multiplexing everything over one RTCDataChannel would need our own framing,
// flow control and close handshake for no gain.
//
// One extra pre-negotiated channel (id 0, "lp2p-shim/control") exists from
// the start: it gets SCTP into the offer, tells both sides when the
// connection is usable and carries the goodbye.
//
// Signaling (discovery, offer/answer, ICE candidates) is out of band and
// goes through a Signaler: BroadcastChannel for tabs of one browser, or
// ManualSignaler for copy/paste between devices. Finding peers on the LAN
// by itself - what LP2P's mDNS discovery does - is out of reach for a page.

import { BaseConnection, BaseStream, randomId } from "./base.ts";
import { Rendezvous, type RendezvousOptions } from "./rendezvous.ts";
import { BroadcastChannelSignaler, type SignalMessage, type Signaler } from "./signaling.ts";
import type { Advertisement, Closer, DiscoveryEvent, Transport, TransportConnection, TransportStream } from "./types.ts";

export interface WebRTCTransportOptions extends RendezvousOptions {
  /** Defaults to a BroadcastChannelSignaler on channelName. */
  signaler?: Signaler;
  /** BroadcastChannel name for the default signaler. Default "lp2p-shim/webrtc". */
  channelName?: string;
  /** Passed to RTCPeerConnection. The default has no ICE servers: host candidates only, i.e. same LAN. */
  rtcConfiguration?: RTCConfiguration;
  /**
   * How long connect waits, in ms. Default 15000 on a live signaler and
   * 10 minutes on a manual one (a human is copying codes).
   */
  connectTimeout?: number;
}

const CONTROL_LABEL = "lp2p-shim/control";
const DISCONNECT_GRACE_MS = 5000;

class ChannelStream extends BaseStream {
  private readonly pending: Uint8Array[] = [];

  private readonly dc: RTCDataChannel;
  private readonly conn: RTCConnection;

  constructor(
    id: number,
    dc: RTCDataChannel,
    conn: RTCConnection,
    received: ArrayBuffer[] = [],
  ) {
    super(id);
    this.dc = dc;
    this.conn = conn;
    dc.binaryType = "arraybuffer";
    for (const m of received) this.deliver(new Uint8Array(m));
    dc.onopen = () => {
      for (const m of this.pending.splice(0)) this.dc.send(m as Uint8Array<ArrayBuffer>);
      if (this.closed) this.dc.close();
    };
    dc.onmessage = (e: MessageEvent) => this.deliver(new Uint8Array(e.data as ArrayBuffer));
    dc.onclose = () => {
      this.conn.forget(this.id);
      this.remoteClosed();
    };
  }

  send(message: Uint8Array): void {
    if (this.closed) throw new DOMException("stream is closed", "InvalidStateError");
    if (this.dc.readyState === "connecting") this.pending.push(message);
    else this.dc.send(message as Uint8Array<ArrayBuffer>);
  }

  protected doClose(): void {
    this.conn.forget(this.id);
    // While connecting, closing now would drop the queued messages; onopen
    // closes the channel once they are out.
    if (this.dc.readyState !== "connecting") this.dc.close();
  }
}

class RTCConnection extends BaseConnection {
  private nextId = 0;

  private readonly pc: RTCPeerConnection;
  private readonly control: RTCDataChannel;

  constructor(
    remoteId: string,
    pc: RTCPeerConnection,
    control: RTCDataChannel,
    early: EarlyChannel[],
  ) {
    super(remoteId);
    this.pc = pc;
    this.control = control;
    pc.ondatachannel = (e) => this.accept(this.wrap(e.channel));
    for (const { dc, received } of early) this.accept(this.wrap(dc, received));
    control.onmessage = (e) => {
      if (e.data === "bye") this.shutdown("closed by peer");
    };
    control.onclose = () => this.shutdown("control channel closed");
    let grace: ReturnType<typeof setTimeout> | undefined;
    pc.onconnectionstatechange = () => {
      clearTimeout(grace);
      const state = pc.connectionState;
      if (state === "failed" || state === "closed") {
        this.shutdown(`peer connection ${state}`);
      } else if (state === "disconnected") {
        // Often transient (network change); give ICE a moment to recover.
        grace = setTimeout(() => this.shutdown("peer connection disconnected"), DISCONNECT_GRACE_MS);
      }
    };
  }

  openStream(): TransportStream {
    if (this.closed) throw new DOMException("connection is closed", "InvalidStateError");
    const s = this.wrap(this.pc.createDataChannel(`lp2p-shim/stream`, { ordered: true }));
    this.streams.set(s.id, s);
    return s;
  }

  forget(id: number): void {
    this.streams.delete(id);
  }

  protected doClose(): void {
    if (this.control.readyState === "open") this.control.send("bye");
    // Let the goodbye leave before tearing down.
    setTimeout(() => this.pc.close(), 100);
  }

  private wrap(dc: RTCDataChannel, received?: ArrayBuffer[]): ChannelStream {
    return new ChannelStream(this.nextId++, dc, this, received);
  }

  private shutdown(reason: string): void {
    if (this.closed) return;
    this.remoteClosed(reason);
    this.pc.close();
  }
}

/** Collects ICE candidates until the remote description is set. */
class CandidateQueue {
  private queue: RTCIceCandidateInit[] | null = [];
  private readonly pc: RTCPeerConnection;

  constructor(pc: RTCPeerConnection) {
    this.pc = pc;
  }

  add(c: RTCIceCandidateInit): void {
    if (this.queue) this.queue.push(c);
    else void this.pc.addIceCandidate(c).catch(() => {});
  }

  flush(): void {
    const q = this.queue ?? [];
    this.queue = null;
    for (const c of q) void this.pc.addIceCandidate(c).catch(() => {});
  }
}

export class WebRTCTransport implements Transport {
  readonly name = "webrtc";
  private readonly rendezvous: Rendezvous;
  private readonly trickle: boolean;
  private readonly connectTimeout: number;
  private readonly rtcConfiguration: RTCConfiguration;

  constructor(options: WebRTCTransportOptions = {}) {
    const signaler = options.signaler ?? new BroadcastChannelSignaler(options.channelName ?? "lp2p-shim/webrtc");
    this.rendezvous = new Rendezvous(signaler, options);
    // On a manual medium every message costs the user a copy/paste, so
    // gather all candidates first and send them inside the SDP.
    this.trickle = signaler.live;
    this.connectTimeout = options.connectTimeout ?? (signaler.live ? 15_000 : 600_000);
    this.rtcConfiguration = options.rtcConfiguration ?? { iceServers: [] };
  }

  listen(ad: Advertisement, onconnection: (connection: TransportConnection) => void): Closer {
    const sessions = new Map<string, { pc: RTCPeerConnection; candidates: CandidateQueue }>();
    return this.rendezvous.advertise(ad, (m: SignalMessage) => {
      if (m.t !== "signal" || typeof m["conn"] !== "string") return;
      const connId = m["conn"];
      const data = m["data"] as { sdp?: RTCSessionDescriptionInit; candidate?: RTCIceCandidateInit };
      if (data.sdp?.type === "offer" && !sessions.has(connId)) {
        const pc = new RTCPeerConnection(this.rtcConfiguration);
        const candidates = new CandidateQueue(pc);
        sessions.set(connId, { pc, candidates });
        const control = pc.createDataChannel(CONTROL_LABEL, { negotiated: true, id: 0 });
        const early = collectEarlyChannels(pc);
        const reply = (d: unknown): void => this.rendezvous.send({ t: "signal", from: ad.id, to: m.from, conn: connId, data: d });
        if (this.trickle) {
          pc.onicecandidate = (e) => e.candidate && reply({ candidate: e.candidate.toJSON() });
        }
        control.onopen = () => {
          sessions.delete(connId);
          onconnection(new RTCConnection(m.from, pc, control, early));
        };
        const fail = (): void => {
          sessions.delete(connId);
          pc.close();
        };
        const timer = setTimeout(fail, this.connectTimeout);
        control.addEventListener("open", () => clearTimeout(timer), { once: true });
        void (async () => {
          await pc.setRemoteDescription(data.sdp as RTCSessionDescriptionInit);
          candidates.flush();
          await pc.setLocalDescription(await pc.createAnswer());
          if (!this.trickle) await gathered(pc);
          reply({ sdp: pc.localDescription?.toJSON() });
        })().catch(fail);
      } else if (data.candidate) {
        sessions.get(connId)?.candidates.add(data.candidate);
      }
    });
  }

  discover(onevent: (event: DiscoveryEvent) => void): Closer {
    return this.rendezvous.discover(onevent);
  }

  async connect(peerId: string, local: Advertisement, options: { signal?: AbortSignal } = {}): Promise<TransportConnection> {
    const connId = randomId();
    const pc = new RTCPeerConnection(this.rtcConfiguration);
    const candidates = new CandidateQueue(pc);
    const control = pc.createDataChannel(CONTROL_LABEL, { negotiated: true, id: 0 });
    const early = collectEarlyChannels(pc);
    const send = (d: unknown): void =>
      this.rendezvous.send({ t: "signal", from: local.id, to: peerId, conn: connId, nickname: local.nickname, data: d });

    let registration: Closer | undefined;
    let timer: ReturnType<typeof setTimeout> | undefined;
    let onAbort: (() => void) | undefined;
    try {
      return await new Promise<TransportConnection>((resolve, reject) => {
        onAbort = () => reject(options.signal?.reason);
        options.signal?.addEventListener("abort", onAbort);
        timer = setTimeout(() => reject(new DOMException(`could not connect to ${peerId}`, "NetworkError")), this.connectTimeout);
        registration = this.rendezvous.register(local.id, (m) => {
          if (m.t !== "signal" || m["conn"] !== connId) return;
          const data = m["data"] as { sdp?: RTCSessionDescriptionInit; candidate?: RTCIceCandidateInit };
          if (data.sdp?.type === "answer") {
            pc.setRemoteDescription(data.sdp).then(() => candidates.flush(), reject);
          } else if (data.candidate) {
            candidates.add(data.candidate);
          }
        });
        if (this.trickle) {
          pc.onicecandidate = (e) => e.candidate && send({ candidate: e.candidate.toJSON() });
        }
        control.onopen = () => resolve(new RTCConnection(peerId, pc, control, early));
        void (async () => {
          await pc.setLocalDescription(await pc.createOffer());
          if (!this.trickle) await gathered(pc);
          send({ sdp: pc.localDescription?.toJSON() });
        })().catch(reject);
      });
    } catch (err) {
      pc.close();
      throw err;
    } finally {
      clearTimeout(timer);
      registration?.close();
      if (onAbort) options.signal?.removeEventListener("abort", onAbort);
    }
  }

  close(): void {
    this.rendezvous.close();
  }
}

interface EarlyChannel {
  dc: RTCDataChannel;
  received: ArrayBuffer[];
}

/** Holds channels (and their messages) the peer opens before the connection object exists. */
function collectEarlyChannels(pc: RTCPeerConnection): EarlyChannel[] {
  const early: EarlyChannel[] = [];
  pc.ondatachannel = (e) => {
    const ch: EarlyChannel = { dc: e.channel, received: [] };
    ch.dc.binaryType = "arraybuffer";
    ch.dc.onmessage = (m: MessageEvent) => ch.received.push(m.data as ArrayBuffer);
    early.push(ch);
  };
  return early;
}

function gathered(pc: RTCPeerConnection): Promise<void> {
  if (pc.iceGatheringState === "complete") return Promise.resolve();
  return new Promise((resolve) => {
    pc.addEventListener("icegatheringstatechange", () => {
      if (pc.iceGatheringState === "complete") resolve();
    });
  });
}

