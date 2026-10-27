// The user agent's UI hooks: everything a browser implementing LP2P would
// show to, or ask of, the user. Mirrors go-lp2p's lp2p-api/useragent UI
// interface (RequestPermission, PickPeer, PresentPIN, EnterPIN).
//
// Every call gets an AbortSignal; when it aborts (the flow finished, failed
// or was canceled) the UI must take down whatever it shows.

/** A remote peer as the UI shows it. */
export interface PeerInfo {
  /** Transport-level ID, stable while the peer advertises. */
  id: string;
  /** Nickname the peer's page chose, from discovery. A hint, never verified. Empty if unknown. */
  nickname: string;
  /** The device's name (agent-info display-name), once known. Not the page's nickname. */
  displayName?: string;
  /**
   * Whether displayName is trusted: it was obtained on a session that is
   * authenticated. Nicknames never are.
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

/**
 * Display label for a peer, like go-lp2p's Peer.Label: the page's nickname
 * (a hint) and, once trusted, the device's name.
 */
export function peerLabel(p: PeerInfo): string {
  const device = p.displayName ? (p.verified ? p.displayName : `${p.displayName}?`) : "";
  if (!p.nickname) return device ? `${device}${p.verified ? "" : " (unverified)"}` : "Unnamed device";
  if (p.verified && device) return `${p.nickname} on ${device}`;
  return `${p.nickname} (unverified)`;
}

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
export class AutoUI implements UserAgentUI {
  private readonly options: AutoUIOptions;

  constructor(options: AutoUIOptions = {}) {
    this.options = options;
  }

  async requestPermission(req: PermissionRequest): Promise<boolean> {
    const a = this.options.allow ?? true;
    const ok = typeof a === "function" ? await a(req) : a;
    this.options.log?.(`${req.origin} asks for ${req.permission}${req.peer ? ` with ${peerLabel(req.peer)}` : ""}: ${ok ? "allowed" : "denied"}`);
    return ok;
  }

  pickPeer(picker: PeerPicker, signal: AbortSignal): Promise<PeerInfo | null> {
    const pick = this.options.pick ?? ((peers) => peers[0]);
    return new Promise((resolve) => {
      const check = (): void => {
        const p = pick(picker.peers);
        if (!p) return;
        picker.onchange = null;
        this.options.log?.(`picked ${peerLabel(p)}`);
        resolve(p);
      };
      signal.addEventListener("abort", () => resolve(null), { once: true });
      picker.onchange = check;
      check();
    });
  }

  presentPIN(p: PINPresentation): void {
    this.options.log?.(`shows PIN ${p.pin} to pair with ${peerLabel(p.peer)}`);
    this.options.onPIN?.(p);
  }

  async enterPIN(p: PINPrompt, signal: AbortSignal): Promise<string | null> {
    const pin = this.options.enterPIN ? await this.options.enterPIN(p, signal) : null;
    this.options.log?.(pin === null ? "cancels PIN entry" : `enters PIN ${pin}`);
    return pin;
  }
}
