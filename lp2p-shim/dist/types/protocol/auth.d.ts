import type { ControlChannel } from "./session.js";
export declare class AuthError extends Error {
    name: string;
    readonly result: number;
    constructor(message: string, result: number);
}
/** A 9-digit PIN, about 27 bits, shown as "123-456-789" (OSP Appendix B grouping). */
export declare function generatePIN(): string;
/** Receiver side without pairing. */
export declare function skipAuthentication(control: ControlChannel): void;
/**
 * Receiver side: shows a PIN via showPIN and checks the requester's proof.
 * Throws AuthError if the requester gets it wrong.
 */
export declare function presentPIN(control: ControlChannel, showPIN: (pin: string) => void, signal?: AbortSignal): Promise<void>;
/**
 * Requester side: waits for the receiver to accept and, if it asks for a
 * PIN, gets one from enterPIN (null means the user canceled). Resolves when
 * the receiver reports authenticated.
 */
export declare function awaitAuthentication(control: ControlChannel, enterPIN: () => Promise<string | null>, signal?: AbortSignal): Promise<void>;
