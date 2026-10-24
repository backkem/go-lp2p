// Signalers carry the small out-of-band messages transports need before a
// connection exists: announcements, discovery queries, connection requests
// and WebRTC offers/answers/ICE candidates.
//
// A Signaler is a broadcast medium: everything posted reaches every other
// participant, and receivers filter on the "to" field. This is the part of
// local peer-to-peer a web page cannot do by itself. A browser implementing
// LP2P discovers peers with mDNS/DNS-SD and connects to them directly; a page
// can only reach peers through something both sides already share - another
// tab of the same browser, a server, or a human copying text between devices.

/** A message on the signaling medium. Must survive structured clone (and JSON, for ManualSignaler). */
export interface SignalMessage {
  /** Message kind, e.g. "announce", "query", "bye", "connect", "signal". */
  t: string;
  /** Sender peer ID. */
  from: string;
  /** Recipient peer ID; absent for broadcasts. */
  to?: string;
  [key: string]: unknown;
}

export interface Signaler {
  /** Posts message to all other participants. */
  post(message: SignalMessage, transfer?: Transferable[]): void;
  /** Called for each message posted by another participant. */
  onmessage: ((message: SignalMessage) => void) | null;
  /**
   * Whether post can transfer MessagePorts. When true the postMessage
   * transport hands each connection its own MessageChannel port.
   */
  readonly canTransfer: boolean;
  /**
   * Whether the medium is live. Live media get periodic discovery queries
   * and peers expire when they stop answering; on a non-live medium
   * (ManualSignaler) every message is precious and peers never expire.
   */
  readonly live: boolean;
  close(): void;
}

function isSignalMessage(v: unknown): v is SignalMessage {
  return typeof v === "object" && v !== null && typeof (v as SignalMessage).t === "string" &&
    typeof (v as SignalMessage).from === "string";
}

/**
 * Signals over a BroadcastChannel: every same-origin window, iframe and
 * worker in this browser profile that uses the same channel name.
 */
export class BroadcastChannelSignaler implements Signaler {
  readonly canTransfer = false;
  readonly live = true;
  onmessage: ((message: SignalMessage) => void) | null = null;
  private readonly channel: BroadcastChannel;

  readonly name: string;

  constructor(name = "lp2p-shim") {
    this.name = name;
    this.channel = new BroadcastChannel(name);
    this.channel.onmessage = (e: MessageEvent) => {
      if (isSignalMessage(e.data)) this.onmessage?.(e.data);
    };
  }

  post(message: SignalMessage): void {
    this.channel.postMessage(message);
  }

  close(): void {
    this.channel.close();
  }
}

/** Anything with postMessage and message events: a MessagePort, a Worker, a DedicatedWorkerGlobalScope. */
export interface PortLike {
  postMessage(message: unknown, transfer: Transferable[]): void;
  addEventListener(type: "message", listener: (e: MessageEvent) => void): void;
  removeEventListener(type: "message", listener: (e: MessageEvent) => void): void;
  start?(): void;
}

/**
 * Signals over a point-to-point port: a page and its worker, or two frames
 * the application linked with a MessageChannel. Can transfer ports, so each
 * connection gets a dedicated MessageChannel, which also works across origins.
 */
export class PortSignaler implements Signaler {
  readonly canTransfer = true;
  readonly live = true;
  onmessage: ((message: SignalMessage) => void) | null = null;
  private readonly listener = (e: MessageEvent): void => {
    const d: unknown = e.data;
    if (typeof d === "object" && d !== null && "lp2pSignal" in d && isSignalMessage(d.lp2pSignal)) {
      const msg = d.lp2pSignal;
      if (e.ports.length > 0) msg["port"] = e.ports[0];
      this.onmessage?.(msg);
    }
  };

  private readonly port: PortLike;

  constructor(port: PortLike) {
    this.port = port;
    port.addEventListener("message", this.listener);
    port.start?.();
  }

  post(message: SignalMessage, transfer: Transferable[] = []): void {
    // Ports must be transferred, not cloned inside the message.
    const { port, ...rest } = message;
    const ports = port ? [port as Transferable] : [];
    this.port.postMessage({ lp2pSignal: rest }, [...ports, ...transfer.filter((t) => t !== port)]);
  }

  close(): void {
    this.port.removeEventListener("message", this.listener);
  }
}

/**
 * Signals through the user: every message is turned into a code the user
 * copies to the other device (chat, email, QR...), where it is pasted with
 * paste(). This is enough to connect two devices on a LAN with WebRTC
 * without any server, at the price of three copy/pastes: the receiver's
 * announcement, the requester's offer and the receiver's answer.
 *
 * Only use it with the WebRTC transport.
 */
export class ManualSignaler implements Signaler {
  readonly canTransfer = false;
  readonly live = false;
  onmessage: ((message: SignalMessage) => void) | null = null;
  private readonly shown = new Set<string>();

  /** show is called with each code the user must copy to the other device. */
  private readonly show: (code: string, message: SignalMessage) => void;

  constructor(show: (code: string, message: SignalMessage) => void) {
    this.show = show;
  }

  post(message: SignalMessage): void {
    if (message.t === "query" || message.t === "bye") return; // meaningless offline
    const code = ManualSignaler.encode(message);
    if (this.shown.has(code)) return;
    this.shown.add(code);
    this.show(code, message);
  }

  /** Feeds a code copied from the other device. Throws on garbage. */
  paste(code: string): void {
    this.onmessage?.(ManualSignaler.decode(code));
  }

  close(): void {}

  static encode(message: SignalMessage): string {
    const bytes = new TextEncoder().encode(JSON.stringify(message));
    let bin = "";
    for (const b of bytes) bin += String.fromCharCode(b);
    return "LP2P:" + btoa(bin);
  }

  static decode(code: string): SignalMessage {
    const trimmed = code.trim().replace(/^LP2P:/, "").replace(/\s+/g, "");
    const bin = atob(trimmed);
    const bytes = Uint8Array.from(bin, (c) => c.charCodeAt(0));
    const msg: unknown = JSON.parse(new TextDecoder().decode(bytes));
    if (!isSignalMessage(msg)) throw new TypeError("not an LP2P signaling code");
    return msg;
  }
}
