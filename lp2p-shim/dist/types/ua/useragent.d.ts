import { Session } from "../protocol/session.js";
import type { Closer, Transport } from "../transport/types.js";
import { type LocalState, type ReceiverRegistration } from "./localstate.js";
import type { PeerInfo, UserAgentUI } from "./ui.js";
export type Pairing = "pin" | "none";
export interface UserAgentConfig {
    transport: Transport;
    ui: UserAgentUI;
    /** "pin": the server shows a PIN the client must enter (toy, see protocol/auth.ts). Default "pin". */
    pairing?: Pairing;
    /** The page's origin: the requesting origin of its sessions, and shown in prompts. Default: location.origin. */
    origin?: string;
    /** agent-info display-name: the device's name, not the page's nickname. Default "lp2p-shim device". */
    deviceName?: string;
    /** agent-info model-name. Default "lp2p-shim". */
    modelName?: string;
    /** How long the handshake, agent-info and the session request may take, in ms. Default 10000. */
    handshakeTimeout?: number;
    /** The device's trust store, known peers and grants. Default: one per transport object. */
    localState?: LocalState;
    /** Diagnostics. */
    log?: (line: string) => void;
}
/** An accepted session plus what the user agent knows about the peer. */
export interface GrantedSession {
    session: Session;
    peer: PeerInfo;
}
/** A receiver as the page side registers it with advertise(). */
export interface ReceiverHandle extends ReceiverRegistration {
    readonly nickname: string;
}
export declare class UserAgent {
    readonly transport: Transport;
    readonly ui: UserAgentUI;
    readonly pairing: Pairing;
    readonly origin: string;
    readonly deviceName: string;
    readonly local: LocalState;
    private readonly modelName;
    private readonly handshakeTimeout;
    private readonly log;
    constructor(config: UserAgentConfig);
    /**
     * LP2PReceiver.start(): asks to advertise as nickname and, if allowed,
     * advertises until closed. Accepted sessions go to a receiver of this
     * device that allows their requesting origin, preferring receiver.
     */
    advertise(receiver: ReceiverHandle): Promise<Closer>;
    /** The server side of a new connection. */
    private admit;
    /** A started receiver of this device that allows origin, preferring preferred. */
    private receiverFor;
    private pinPresenter;
    /**
     * LP2PRequest.start(): shows the picker, connects to the chosen peer,
     * verifies it (a ceremony if it isn't trusted), sends the requesting
     * origin and, if the server asks, runs the ceremony. Rejects with
     * NotAllowedError on every failure, as the spec requires. Aborting signal
     * takes the picker down or closes the session being set up, and rejects
     * with the abort reason.
     */
    request(nickname: string, signal?: AbortSignal): Promise<GrantedSession>;
    private agentInfo;
    private withTimeout;
}
