// WebTransport sessions on the wire, as the spec's OSP binding carries them
// on the QUIC connection of the LP2P session ("WebTransport extension").
// The API objects (../api/quictransport.ts) drive these.
//
// - Control stream: a bidirectional stream whose first message is
//   data-transport-start-request (1201: request-id, exchange-id). The peer
//   answers data-transport-start-response (1202) once its user agent
//   accepts or refuses the session; a refusal ends the stream. Either side
//   closes the session with data-transport-close (1205: code, reason) and
//   then ends the stream; ending it without 1205 closes with code 0 and an
//   empty reason. data-transport-drain (1206) asks the peer to wind down.
//   The client uses even exchange-ids, the server odd ones. Sessions are
//   always pooled; the binding's dedicated "q2q" connections (optional in
//   the spec) are not implemented.
// - WebTransport stream: a stream of the same direction whose first message
//   is data-transport-stream (1203: exchange-id). No response; the stream's
//   data follows. The direction comes from the transport stream itself, as
//   from a QUIC stream ID. After the header each message is one frame: a tag
//   byte, then DATA bytes, FIN, RESET (varint code) or STOP_SENDING (varint
//   code), standing in for QUIC's own stream bytes, FIN bit, RESET_STREAM
//   and STOP_SENDING, which the shim's message streams don't have. Codes
//   from 2^32 are the binding's: session-gone (2^32) when the session is
//   closed or unknown.
// - Datagram: a connection datagram holding varint(exchange-id) and the
//   payload, like the binding's QUIC DATAGRAM frames.

import { decodeVarint, encodeVarint } from "../codec/varint.ts";
import type { TransportStream } from "../transport/types.ts";
import { Result, TypeKey, decodeMessage, encodeMessage, type Message } from "./messages.ts";
import { webTransportExporterContext } from "./identity.ts";

/** Stream frame tags after the 1203 header. */
export const StreamFrame = {
  Data: 0,
  Fin: 1,
  Reset: 2,
  StopSending: 3,
} as const;

/** QUIC stream error code: the stream's session is closed or unknown. Codes from 2^32 are reserved. */
export const SESSION_GONE = 2 ** 32;
/** How long a stream for a session that is not open yet is held. */
const HOLD_MS = 2000;
/** How long an agent that sent data-transport-close waits for the peer to end the control stream. */
const CLOSE_LINGER_MS = 2000;

/** What happens on a WebTransport stream, in order. */
export interface StreamSink {
  data(chunk: Uint8Array): void;
  fin(): void;
  reset(code: number): void;
  stopSending(code: number): void;
  /** The stream ended without a stream error code: its session went away, or the connection did. */
  abort(reason: string): void;
}

function frame(tag: number, body?: Uint8Array): Uint8Array {
  const out = new Uint8Array(1 + (body?.length ?? 0));
  out[0] = tag;
  if (body) out.set(body, 1);
  return out;
}

/** One WebTransport stream: a transport stream that began with 1203. */
export class WTStream {
  readonly unidirectional: boolean;
  /** True if this side opened it. */
  readonly outgoing: boolean;
  bytesSent = 0;
  bytesReceived = 0;
  onfinished: (() => void) | null = null;

  private sendDone: boolean;
  private recvDone: boolean;
  private sink: StreamSink | null = null;
  private readonly held: ((s: StreamSink) => void)[] = [];
  private readonly stream: TransportStream;

  constructor(stream: TransportStream, unidirectional: boolean, outgoing: boolean) {
    this.stream = stream;
    this.unidirectional = unidirectional;
    this.outgoing = outgoing;
    this.sendDone = unidirectional && !outgoing;
    this.recvDone = unidirectional && outgoing;
    stream.onmessage = (b) => this.receive(b);
    stream.onclose = () => this.closedUnderneath();
  }

  /** Delivers held events, then everything that follows, to sink. */
  attach(sink: StreamSink): void {
    this.sink = sink;
    for (const ev of this.held.splice(0)) ev(sink);
  }

  get sending(): boolean {
    return !this.sendDone;
  }

  send(chunk: Uint8Array): void {
    if (this.sendDone) throw new DOMException("stream is not writable", "InvalidStateError");
    this.stream.send(frame(StreamFrame.Data, chunk));
    this.bytesSent += chunk.length;
  }

  fin(): void {
    if (this.sendDone) return;
    this.sendDone = true;
    this.stream.send(frame(StreamFrame.Fin));
    this.maybeClose();
  }

  reset(code: number): void {
    if (this.sendDone) return;
    this.sendDone = true;
    this.stream.send(frame(StreamFrame.Reset, encodeVarint(code)));
    this.maybeClose();
  }

  stopSending(code: number): void {
    if (this.recvDone) return;
    this.recvDone = true;
    this.stream.send(frame(StreamFrame.StopSending, encodeVarint(code)));
    this.maybeClose();
  }

  /**
   * Refuses a stream nobody takes: STOP_SENDING and, for a bidirectional
   * stream, RESET_STREAM, with code (session-gone).
   */
  refuse(code: number): void {
    if (!this.stream.closed) {
      this.stream.send(frame(StreamFrame.StopSending, encodeVarint(code)));
      if (!this.unidirectional) this.stream.send(frame(StreamFrame.Reset, encodeVarint(code)));
      this.stream.close();
    }
    this.sendDone = this.recvDone = true;
  }

  /** The session closed: reset and stop the stream with session-gone, and tell the sink. */
  sessionGone(reason: string): void {
    if (this.sendDone && this.recvDone) return;
    if (!this.stream.closed) {
      if (!this.recvDone) this.stream.send(frame(StreamFrame.StopSending, encodeVarint(SESSION_GONE)));
      if (!this.sendDone) this.stream.send(frame(StreamFrame.Reset, encodeVarint(SESSION_GONE)));
      this.stream.close();
    }
    this.sendDone = this.recvDone = true;
    this.emit((s) => s.abort(reason));
    this.onfinished?.();
  }

  private emit(ev: (s: StreamSink) => void): void {
    if (this.sink) ev(this.sink);
    else this.held.push(ev);
  }

  private receive(b: Uint8Array): void {
    const tag = b[0];
    switch (tag) {
      case StreamFrame.Data:
        if (this.recvDone) return;
        this.bytesReceived += b.length - 1;
        this.emit((s) => s.data(b.subarray(1)));
        break;
      case StreamFrame.Fin:
        if (this.recvDone) return;
        this.recvDone = true;
        this.emit((s) => s.fin());
        this.maybeClose();
        break;
      case StreamFrame.Reset: {
        if (this.recvDone) return;
        this.recvDone = true;
        const [code] = decodeVarint(b, 1);
        // Codes from 2^32 are the binding's: reported without a stream error code.
        if (code >= SESSION_GONE) this.emit((s) => s.abort("the session is gone"));
        else this.emit((s) => s.reset(code));
        this.maybeClose();
        break;
      }
      case StreamFrame.StopSending: {
        if (this.sendDone) return;
        const [code] = decodeVarint(b, 1);
        if (code >= SESSION_GONE) {
          this.sendDone = true;
          this.emit((s) => s.abort("the session is gone"));
          this.maybeClose();
        } else {
          this.emit((s) => s.stopSending(code));
        }
        break;
      }
    }
  }

  private maybeClose(): void {
    if (!this.sendDone || !this.recvDone) return;
    this.stream.close();
    this.onfinished?.();
  }

  private closedUnderneath(): void {
    if (this.sendDone && this.recvDone) return;
    this.sendDone = this.recvDone = true;
    this.emit((s) => s.abort("stream closed"));
    this.onfinished?.();
  }
}

export interface CloseInfo {
  code: number;
  reason: string;
}

/** What the session layer (Session) provides to a WTSession. */
export interface WTHost {
  readonly maxDatagramSize: number;
  readonly datagramsReliable: boolean;
  /** True once the underlying connection is closed. */
  readonly connectionClosed: boolean;
  openStream(unidirectional: boolean): TransportStream;
  sendDatagram(d: Uint8Array): void;
  requestId(): number;
  forgetTransport(exchangeId: number): void;
  /** Promises of sessions that sent data-transport-close and wait for the peer's end of the control stream. */
  readonly closing: Set<Promise<void>>;
  exportKeyingMaterial(label: string, context: Uint8Array, length: number): Promise<Uint8Array>;
}

/**
 * "connecting": opened here, waiting for 1202. "pending": opened by the
 * peer, waiting for this user agent to accept or refuse.
 */
export type WTState = "connecting" | "pending" | "open" | "closed";

/** One WebTransport session: a control stream that began with 1201. */
export class WTSession {
  readonly exchangeId: number;
  readonly outgoing: boolean;
  state: WTState;
  bytesSent = 0;
  bytesReceived = 0;
  datagramsSent = 0;
  datagramsReceived = 0;
  /** Outgoing: the peer accepted. */
  onready: (() => void) | null = null;
  /** The peer sent data-transport-drain. */
  ondrain: (() => void) | null = null;
  /** Called once: info is null for an abrupt close (refused, or the connection went away). */
  onclose: ((info: CloseInfo | null, why: string) => void) | null = null;

  private readonly host: WTHost;
  private readonly control: TransportStream;
  private readonly requestIdValue: number;
  private readonly streams = new Set<WTStream>();
  private readonly heldStreams: WTStream[] = [];
  /** Incoming streams that arrived while this side was still waiting for 1202. */
  private readonly early: WTStream[] = [];
  private earlyTimer: ReturnType<typeof setTimeout> | null = null;
  private streamHandler: ((s: WTStream) => void) | null = null;
  private readonly heldDatagrams: Uint8Array[] = [];
  private datagramHandler: ((d: Uint8Array) => void) | null = null;
  private readonly prefix: Uint8Array;
  private peerClose: CloseInfo | null = null;
  private lingering = false;

  constructor(host: WTHost, exchangeId: number, control: TransportStream, outgoing: boolean, requestId: number) {
    this.host = host;
    this.exchangeId = exchangeId;
    this.control = control;
    this.outgoing = outgoing;
    this.requestIdValue = requestId;
    this.state = outgoing ? "connecting" : "pending";
    this.prefix = encodeVarint(exchangeId);
    control.onmessage = (b) => this.receiveControl(b);
    control.onclose = () => this.controlEnded();
    if (outgoing) {
      control.send(encodeMessage({ type: TypeKey.DataTransportStartRequest, requestId, exchangeId }));
    }
  }

  get maxDatagramSize(): number {
    return Math.max(0, this.host.maxDatagramSize - this.prefix.length);
  }

  get datagramsReliable(): boolean {
    return this.host.datagramsReliable;
  }

  /** Incoming streams. Streams that arrive while this is null are held. */
  set onstream(fn: ((s: WTStream) => void) | null) {
    this.streamHandler = fn;
    while (fn && this.heldStreams.length > 0) fn(this.heldStreams.shift() as WTStream);
  }

  /** Incoming datagrams. Up to 64 that arrive while this is null are held. */
  set ondatagram(fn: ((d: Uint8Array) => void) | null) {
    this.datagramHandler = fn;
    while (fn && this.heldDatagrams.length > 0) fn(this.heldDatagrams.shift() as Uint8Array);
  }

  /** Incoming, pending: this user agent accepts the session. */
  accept(): void {
    if (this.state !== "pending") return;
    this.state = "open";
    this.control.send(
      encodeMessage({ type: TypeKey.DataTransportStartResponse, requestId: this.requestIdValue, result: Result.Success }),
    );
  }

  /** Incoming, pending: refuses the session (permanent-error, then FIN). */
  refuse(): void {
    if (this.state !== "pending") return;
    this.control.send(
      encodeMessage({ type: TypeKey.DataTransportStartResponse, requestId: this.requestIdValue, result: Result.PermanentError }),
    );
    this.teardown(null, "refused");
  }

  openStream(unidirectional: boolean): WTStream {
    if (this.state !== "open") throw new DOMException("session is not open", "InvalidStateError");
    const t = this.host.openStream(unidirectional);
    t.send(encodeMessage({ type: TypeKey.DataTransportStream, exchangeId: this.exchangeId }));
    const s = new WTStream(t, unidirectional, true);
    this.track(s);
    return s;
  }

  /** From the session layer: the peer opened a stream for this session. */
  addIncomingStream(s: WTStream): void {
    if (this.state === "connecting") {
      // The accepting agent may open streams right after its 1202, which
      // can overtake it: hold them for a short time.
      this.early.push(s);
      this.earlyTimer ??= setTimeout(() => {
        for (const e of this.early.splice(0)) e.refuse(SESSION_GONE);
      }, HOLD_MS);
      return;
    }
    if (this.state !== "open") {
      s.refuse(SESSION_GONE);
      return;
    }
    this.track(s);
    if (this.streamHandler) this.streamHandler(s);
    else this.heldStreams.push(s);
  }

  sendDatagram(payload: Uint8Array): void {
    if (this.state !== "open") throw new DOMException("session is not open", "InvalidStateError");
    const d = new Uint8Array(this.prefix.length + payload.length);
    d.set(this.prefix);
    d.set(payload, this.prefix.length);
    this.host.sendDatagram(d);
    this.datagramsSent++;
    this.bytesSent += payload.length;
  }

  /** From the session layer: a datagram for this session, without its prefix. */
  receiveDatagram(payload: Uint8Array): void {
    if (this.state !== "open") return;
    this.datagramsReceived++;
    this.bytesReceived += payload.length;
    if (this.datagramHandler) {
      this.datagramHandler(payload);
      return;
    }
    this.heldDatagrams.push(payload);
    if (this.heldDatagrams.length > 64) this.heldDatagrams.shift();
  }

  /** Sends data-transport-drain: asks the peer to finish and close. */
  drain(): void {
    if (this.state === "open") this.control.send(encodeMessage({ type: TypeKey.DataTransportDrain }));
  }

  /** Keying material that only this session yields (spec "export keying material"). */
  exportKeyingMaterial(label: Uint8Array, context: Uint8Array, length: number): Promise<Uint8Array> {
    return this.host.exportKeyingMaterial(
      "EXPORTER-WebTransport",
      webTransportExporterContext(this.exchangeId, label, context),
      length,
    );
  }

  /**
   * Closes the session. When open, tells the peer code and reason (1205)
   * and ends the control stream; when connecting, abandons it; when
   * pending, refuses it.
   */
  close(code = 0, reason = ""): void {
    if (this.state === "closed") return;
    if (this.state === "pending") {
      this.refuse();
      return;
    }
    if (this.state === "open") {
      this.control.send(encodeMessage({ type: TypeKey.DataTransportClose, code, reason }));
      // Keep our side of the control stream until the peer ended its side
      // (or a timeout), so the close isn't lost to a connection close.
      this.lingering = true;
      const done = new Promise<void>((resolve) => {
        const t = setTimeout(() => {
          this.control.close();
          resolve();
        }, CLOSE_LINGER_MS);
        this.control.onclose = () => {
          clearTimeout(t);
          resolve();
        };
      });
      this.host.closing.add(done);
      void done.then(() => this.host.closing.delete(done));
    }
    this.teardown({ code, reason }, "closed locally");
  }

  /** Payload bytes sent and received so far, on streams and in datagrams. */
  totals(): { sent: number; received: number } {
    let sent = this.bytesSent;
    let received = this.bytesReceived;
    for (const s of this.streams) {
      sent += s.bytesSent;
      received += s.bytesReceived;
    }
    return { sent, received };
  }

  private track(s: WTStream): void {
    this.streams.add(s);
    s.onfinished = () => {
      this.streams.delete(s);
      this.bytesSent += s.bytesSent;
      this.bytesReceived += s.bytesReceived;
    };
  }

  private receiveControl(b: Uint8Array): void {
    let m: Message;
    try {
      m = decodeMessage(b);
    } catch {
      return;
    }
    if (m.type === TypeKey.DataTransportStartResponse && this.state === "connecting") {
      if (m.requestId !== this.requestIdValue || m.result !== Result.Success) {
        this.teardown(null, `the peer refused the session (result ${m.result})`);
        return;
      }
      this.state = "open";
      this.onready?.();
      if (this.earlyTimer) clearTimeout(this.earlyTimer);
      for (const s of this.early.splice(0)) this.addIncomingStream(s);
    } else if (m.type === TypeKey.DataTransportClose && this.state === "open") {
      this.peerClose = { code: m.code, reason: m.reason };
      this.teardown(this.peerClose, "closed by the peer");
    } else if (m.type === TypeKey.DataTransportDrain && this.state === "open") {
      this.ondrain?.();
    }
  }

  /** The control stream ended: FIN without 1205 is a clean close with code 0; a lost connection is abrupt. */
  private controlEnded(): void {
    if (this.state === "closed") return;
    if (this.state === "open" && !this.host.connectionClosed) {
      this.teardown({ code: 0, reason: "" }, "closed by the peer");
      return;
    }
    const why =
      this.state === "connecting" ? "the peer refused the session" : this.state === "pending" ? "the peer abandoned the session" : "connection closed";
    this.teardown(null, why);
  }

  private teardown(info: CloseInfo | null, why: string): void {
    if (this.state === "closed") return;
    this.state = "closed";
    if (this.earlyTimer) clearTimeout(this.earlyTimer);
    for (const s of this.early.splice(0)) s.refuse(SESSION_GONE);
    if (!this.lingering) this.control.close();
    for (const s of [...this.streams]) s.sessionGone(why);
    this.streams.clear();
    for (const s of this.heldStreams.splice(0)) s.refuse(SESSION_GONE);
    this.heldDatagrams.length = 0;
    this.host.forgetTransport(this.exchangeId);
    this.onclose?.(info, why);
  }
}
