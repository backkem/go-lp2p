import { Session } from "../protocol/session.js";
import type { Closer, Transport } from "../transport/types.js";
import type { PeerInfo, UserAgentUI } from "./ui.js";
export type Pairing = "pin" | "none";
export interface UserAgentConfig {
    transport: Transport;
    ui: UserAgentUI;
    /** "pin": the receiver shows a PIN the requester must enter (toy, see protocol/auth.ts). Default "pin". */
    pairing?: Pairing;
    /** Origin shown in prompts. Default: location.origin. */
    origin?: string;
    /** agent-info model-name. Default "lp2p-shim". */
    modelName?: string;
    /** How long the agent-info handshake may take, in ms. Default 10000. */
    handshakeTimeout?: number;
    /** Diagnostics. */
    log?: (line: string) => void;
}
/** An authenticated session plus what the user agent knows about the peer. */
export interface GrantedSession {
    session: Session;
    peer: PeerInfo;
}
export declare class UserAgent {
    readonly transport: Transport;
    readonly ui: UserAgentUI;
    readonly pairing: Pairing;
    readonly origin: string;
    private readonly modelName;
    private readonly handshakeTimeout;
    private readonly log;
    constructor(config: UserAgentConfig);
    /**
     * LP2PReceiver.start(): asks to advertise as nickname and, if allowed,
     * advertises until closed. Connections the user grants go to onsession.
     */
    advertise(nickname: string, onsession: (g: GrantedSession) => void): Promise<Closer>;
    /** Receiver side of a new connection: agent-info, consent, pairing. */
    private admit;
    /**
     * LP2PRequest.start(): shows the picker, connects to the chosen peer and
     * pairs with it.
     */
    request(nickname: string): Promise<GrantedSession>;
    private agentInfo;
    private withTimeout;
}
