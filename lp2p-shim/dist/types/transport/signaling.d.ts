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
/**
 * Signals over a BroadcastChannel: every same-origin window, iframe and
 * worker in this browser profile that uses the same channel name.
 */
export declare class BroadcastChannelSignaler implements Signaler {
    readonly canTransfer = false;
    readonly live = true;
    onmessage: ((message: SignalMessage) => void) | null;
    private readonly channel;
    readonly name: string;
    constructor(name?: string);
    post(message: SignalMessage): void;
    close(): void;
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
export declare class PortSignaler implements Signaler {
    readonly canTransfer = true;
    readonly live = true;
    onmessage: ((message: SignalMessage) => void) | null;
    private readonly listener;
    private readonly port;
    constructor(port: PortLike);
    post(message: SignalMessage, transfer?: Transferable[]): void;
    close(): void;
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
export declare class ManualSignaler implements Signaler {
    readonly canTransfer = false;
    readonly live = false;
    onmessage: ((message: SignalMessage) => void) | null;
    private readonly shown;
    /** show is called with each code the user must copy to the other device. */
    private readonly show;
    constructor(show: (code: string, message: SignalMessage) => void);
    post(message: SignalMessage): void;
    /** Feeds a code copied from the other device. Throws on garbage. */
    paste(code: string): void;
    close(): void;
    static encode(message: SignalMessage): string;
    static decode(code: string): SignalMessage;
}
