/** A remote peer as the UI shows it. */
export interface PeerInfo {
    /** Transport-level ID, stable while the peer advertises. */
    id: string;
    /** Nickname the peer's page chose. Never verified in the shim. */
    nickname: string;
    /**
     * Whether the name is verified. A browser can verify names against the
     * peer's certificate after pairing; the shim never can, so this is
     * always false.
     */
    verified: boolean;
}
export type Permission = "advertise" | "accept-connection";
export interface PermissionRequest {
    permission: Permission;
    /** Origin of the page asking. */
    origin: string;
    /** Nickname the page uses for itself. */
    nickname: string;
    /** The remote peer, for "accept-connection". */
    peer?: PeerInfo;
}
/** The live list the device picker shows. */
export interface PeerPicker {
    readonly origin: string;
    readonly nickname: string;
    /** Current peers, sorted by nickname. */
    readonly peers: readonly PeerInfo[];
    /** Called whenever peers changes. */
    onchange: (() => void) | null;
}
export interface PINPresentation {
    pin: string;
    origin: string;
    nickname: string;
    /** The peer that must enter the PIN. */
    peer: PeerInfo;
}
export interface PINPrompt {
    origin: string;
    nickname: string;
    /** The peer showing the PIN. */
    peer: PeerInfo;
}
export interface UserAgentUI {
    /** Asks the user to allow req. */
    requestPermission(req: PermissionRequest, signal: AbortSignal): Promise<boolean>;
    /** Shows the device picker; resolves with the chosen peer, or null if dismissed. */
    pickPeer(picker: PeerPicker, signal: AbortSignal): Promise<PeerInfo | null>;
    /** Shows a PIN until signal aborts. */
    presentPIN(p: PINPresentation, signal: AbortSignal): void;
    /** Asks the user for the PIN shown on the other device; null if canceled. */
    enterPIN(p: PINPrompt, signal: AbortSignal): Promise<string | null>;
}
/** Display label for a peer, marking it unverified like go-lp2p's Peer.Label. */
export declare function peerLabel(p: PeerInfo): string;
export interface AutoUIOptions {
    /** Answer to permission prompts. Default: allow everything. */
    allow?: boolean | ((req: PermissionRequest) => boolean | Promise<boolean>);
    /**
     * Picks a peer from the live list; return undefined to keep waiting.
     * Default: the first peer to appear.
     */
    pick?: (peers: readonly PeerInfo[]) => PeerInfo | undefined;
    /** Called with PINs this side presents. */
    onPIN?: (p: PINPresentation) => void;
    /** Supplies PINs this side must enter. Default: cancel. */
    enterPIN?: (p: PINPrompt, signal: AbortSignal) => Promise<string | null> | string | null;
    /** Logs every decision. */
    log?: (line: string) => void;
}
/**
 * A scripted, headless UI for tests, workers and pages that bring their own
 * UI. With pairing "pin", wire onPIN on one side to enterPIN on the other.
 */
export declare class AutoUI implements UserAgentUI {
    private readonly options;
    constructor(options?: AutoUIOptions);
    requestPermission(req: PermissionRequest): Promise<boolean>;
    pickPeer(picker: PeerPicker, signal: AbortSignal): Promise<PeerInfo | null>;
    presentPIN(p: PINPresentation): void;
    enterPIN(p: PINPrompt, signal: AbortSignal): Promise<string | null>;
}
