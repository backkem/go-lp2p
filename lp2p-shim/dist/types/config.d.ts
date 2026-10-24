import type { Transport } from "./transport/types.js";
import { type UserAgentUI } from "./ua/ui.js";
import { UserAgent, type Pairing } from "./ua/useragent.js";
export interface ShimConfig {
    transport?: Transport;
    ui?: UserAgentUI;
    pairing?: Pairing;
    origin?: string;
    log?: (line: string) => void;
}
/** Merges c into the configuration. Objects started afterwards use it. */
export declare function configure(c: ShimConfig): void;
/** The user agent LP2P objects use when they start. */
export declare function getUserAgent(): UserAgent;
