import type { ControlChannel } from "./session.js";
export declare class AuthError extends Error {
    name: string;
    readonly result: number;
    constructor(message: string, result: number);
}
/** The fingerprints the ceremony binds. */
export interface Parties {
    client: string;
    server: string;
}
/** A 9-digit PIN, about 27 bits, shown as "123-456-789" (OSP Appendix B grouping). */
export declare function generatePIN(): string;
/** Server: the cue that it checked the requesting origin hint and needs a ceremony. */
export declare function sendAuthCapabilities(control: ControlChannel): void;
/**
 * Server: answers the ceremony the client starts. With showPIN null
 * (pairing "none") it reports success at once. Throws AuthError if the
 * client proves the wrong PIN.
 */
export declare function answerCeremony(control: ControlChannel, parties: Parties, showPIN: ((pin: string) => void) | null, signal?: AbortSignal): Promise<void>;
/**
 * Client: starts the ceremony and, if the server shows a PIN, gets it from
 * enterPIN (null means the user canceled). Resolves when the server reports
 * authenticated.
 */
export declare function startCeremony(control: ControlChannel, parties: Parties, enterPIN: () => Promise<string | null>, signal?: AbortSignal): Promise<void>;
