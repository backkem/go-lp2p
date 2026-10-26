// WebTransport sessions on the wire, pooled on the LP2P session's
// connection. The API objects (../api/quictransport.ts) drive these.
//
// The LP2P spec's binding ("WebTransport extension") dials a separate QUIC
// connection with ALPN "q2q" per session, on which QUIC streams and
// datagrams are the WebTransport streams and datagrams. The shim cannot do
// that: "q2q" has no wire format, a requester is not advertising and so
// cannot be dialed by the receiver, and a fresh connection would need
// pairing again (or a binding to the session the spec doesn't define). So,
// like go-lp2p's pooled WebTransport, a session shares the connection:
//
// - Session stream: a transport stream whose first message is
//   data-transport-start-request (1201: request-id, exchange-id). The peer
//   answers with data-transport-start-response (1202) once its page takes
//   the session. Either side ends the session with data-transport-close
//   (1205: code, reason) and closes the stream; closing the stream without
//   1205 is an abrupt close. The exchange-id is even when the requester
//   opens the session and odd when the receiver does.
// - WebTransport stream: a transport stream whose first message is
//   data-transport-stream-request (1203: request-id, exchange-id,
//   unidirectional). The peer answers with data-transport-stream-response
//   (1204) on it; the opener does not wait for that. After the header, each
//   message is one stream frame: a tag byte, then DATA bytes, FIN, RESET
//   (varint code) or STOP_SENDING (varint code). Over QUIC these are the
//   stream's own bytes, FIN bit, RESET_STREAM and STOP_SENDING; the shim's
//   transport streams are message streams with only a full close, so it
//   frames them. The transport stream closes once both directions are done.
// - Datagram: a connection datagram holding varint(exchange-id) and the
//   payload, like HTTP/3 datagrams (RFC 9297) carry a quarter stream ID.

import { decodeVarint, encodeVarint } from "../codec/varint.ts";
import type { TransportStream } from "../transport/types.ts";
import { Result, TypeKey, decodeMessage, encodeMessage, type Message } from "./messages.ts";

/** Stream frame tags after the 1203 header. */
export const StreamFrame = {
  Data: 0,
  Fin: 1,
  Reset: 2,
  StopSending: 3,
} as const;

/** What happens on a WebTransport stream, in order. */
export interface StreamSink {
  data(chunk: Uint8Array): void;
  fin(): void;
  reset(code: number): void;
  stopSending(code: number): void;
  /** The stream ended without FIN or RESET: the peer refused it, or the session or connection went away. */
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
  private awaitingResponse: boolean;
  private sink: StreamSink | null = null;
  private readonly held: ((s: StreamSink) => void)[] = [];
  private readonly stream: TransportStream;
  private readonly requestId: number;

  constructor(stream: TransportStream, unidirectional: boolean, outgoing: boolean, requestId: number) {
    this.stream = stream;
    this.unidirectional = unidirectional;
    this.outgoing = outgoing;
    this.requestId = requestId;
    this.sendDone = unidirectional && !outgoing;
    this.recvDone = unidirectional && outgoing;
    this.awaitingResponse = outgoing;
    stream.onmessage = (b) => this.receive(b);
    stream.onclose = () => this.closedUnderneath();
  }

  /** Delivers held events, then everything that follows, to sink. */
  attach(sink: StreamSink): void {
    this.sink = sink;
    for (const ev of this.held.splice(0)) ev(sink);
  }

  /** Acceptor only: answers the 1203. */
  respond(result: number): void {
    this.stream.send(encodeMessage({ type: TypeKey.DataTransportStreamResponse, requestId: this.requestId, result }));
    if (result !== Result.Success) {
      this.sendDone = this.recvDone = true;
      this.stream.close();
    }
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

  /** The session ended: drop the stream, telling the sink why. */
  abort(reason: string): void {
    if (this.sendDone && this.recvDone) return;
    this.sendDone = this.recvDone = true;
    this.stream.close();
    this.emit((s) => s.abort(reason));
    this.onfinished?.();
  }

  private emit(ev: (s: StreamSink) => void): void {
    if (this.sink) ev(this.sink);
    else this.held.push(ev);
  }

  private receive(b: Uint8Array): void {
    if (this.awaitingResponse) {
      this.awaitingResponse = false;
      let m: Message | null = null;
      try {
        m = decodeMessage(b);
      } catch {
        // handled below
      }
      if (m?.type !== TypeKey.DataTransportStreamResponse || m.requestId !== this.requestId || m.result !== Result.Success) {
        this.abort("the peer refused the stream");
      }
      return;
    }
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
        this.emit((s) => s.reset(code));
        this.maybeClose();
        break;
      }
      case StreamFrame.StopSending: {
        if (this.sendDone) return;
        const [code] = decodeVarint(b, 1);
        this.emit((s) => s.stopSending(code));
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
  openStream(): TransportStream;
  sendDatagram(d: Uint8Array): void;
  requestId(): number;
  forgetTransport(exchangeId: number): void;
}

export type WTState = "connecting" | "pending" | "open" | "closed";

/** One WebTransport session: a session stream that began with 1201. */
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
  /** Called once: info is null for an abrupt close (the stream or the connection went away). */
  onclose: ((info: CloseInfo | null, why: string) => void) | null = null;

  private readonly host: WTHost;
  private readonly control: TransportStream;
  private readonly requestIdValue: number;
  private readonly streams = new Set<WTStream>();
  private readonly heldStreams: WTStream[] = [];
  private streamHandler: ((s: WTStream) => void) | null = null;
  private readonly heldDatagrams: Uint8Array[] = [];
  private datagramHandler: ((d: Uint8Array) => void) | null = null;
  private readonly prefix: Uint8Array;

  constructor(host: WTHost, exchangeId: number, control: TransportStream, outgoing: boolean, requestId: number) {
    this.host = host;
    this.exchangeId = exchangeId;
    this.control = control;
    this.outgoing = outgoing;
    this.requestIdValue = requestId;
    this.state = outgoing ? "connecting" : "pending";
    this.prefix = encodeVarint(exchangeId);
    control.onmessage = (b) => this.receiveControl(b);
    control.onclose = () => this.teardown(null, "session stream closed");
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

  /** Incoming, pending: the local page took the session. */
  accept(): void {
    if (this.state !== "pending") return;
    this.state = "open";
    this.control.send(
      encodeMessage({ type: TypeKey.DataTransportStartResponse, requestId: this.requestIdValue, result: Result.Success }),
    );
    for (const s of this.streams) s.respond(Result.Success);
  }

  openStream(unidirectional: boolean): WTStream {
    if (this.state !== "open") throw new DOMException("session is not open", "InvalidStateError");
    const t = this.host.openStream();
    const requestId = this.host.requestId();
    t.send(
      encodeMessage({ type: TypeKey.DataTransportStreamRequest, requestId, exchangeId: this.exchangeId, unidirectional }),
    );
    const s = new WTStream(t, unidirectional, true, requestId);
    this.track(s);
    return s;
  }

  /** From the session layer: the peer opened a stream for this session. */
  addIncomingStream(s: WTStream): void {
    if (this.state === "closed") {
      s.respond(Result.PermanentError);
      return;
    }
    this.track(s);
    // Answered at once if open; a pending session answers on accept().
    if (this.state === "open" || this.state === "connecting") s.respond(Result.Success);
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

  /**
   * Closes the session. When open, tells the peer code and reason (1205);
   * otherwise (connecting, or pending and refused) just drops it.
   */
  close(code = 0, reason = ""): void {
    if (this.state === "closed") return;
    if (this.state === "open") {
      this.control.send(encodeMessage({ type: TypeKey.DataTransportClose, code, reason }));
    } else if (this.state === "pending") {
      this.control.send(
        encodeMessage({ type: TypeKey.DataTransportStartResponse, requestId: this.requestIdValue, result: Result.PermanentError }),
      );
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
    } else if (m.type === TypeKey.DataTransportClose) {
      this.teardown({ code: m.code, reason: m.reason }, "closed by the peer");
    }
  }

  private teardown(info: CloseInfo | null, why: string): void {
    if (this.state === "closed") return;
    this.state = "closed";
    this.control.close();
    for (const s of [...this.streams]) s.abort(why);
    this.streams.clear();
    this.heldStreams.length = 0;
    this.heldDatagrams.length = 0;
    this.host.forgetTransport(this.exchangeId);
    this.onclose?.(info, why);
  }
}
