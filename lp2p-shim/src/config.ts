// Shim-wide configuration: which transport and UI the LP2P objects use.
// Pages call LP2PShim.configure({...}) before start(); otherwise the
// defaults are the postMessage transport and the DOM UI (or, without a
// DOM, the AutoUI with pairing "none").

import { PostMessageTransport } from "./transport/postmessage.ts";
import type { Transport } from "./transport/types.ts";
import { AutoUI, type UserAgentUI } from "./ua/ui.ts";
import { DomUI } from "./ua/dom-ui.ts";
import { UserAgent, type Pairing } from "./ua/useragent.ts";

export interface ShimConfig {
  transport?: Transport;
  ui?: UserAgentUI;
  pairing?: Pairing;
  origin?: string;
  log?: (line: string) => void;
}

let config: ShimConfig = {};
let current: UserAgent | null = null;

/** Merges c into the configuration. Objects started afterwards use it. */
export function configure(c: ShimConfig): void {
  config = { ...config, ...c };
  current = null;
}

/** The user agent LP2P objects use when they start. */
export function getUserAgent(): UserAgent {
  if (current) return current;
  const hasDOM = typeof document !== "undefined";
  current = new UserAgent({
    transport: config.transport ?? (config.transport = new PostMessageTransport()),
    ui: config.ui ?? (hasDOM ? new DomUI() : new AutoUI()),
    pairing: config.pairing ?? (config.ui || hasDOM ? "pin" : "none"),
    ...(config.origin !== undefined ? { origin: config.origin } : {}),
    ...(config.log !== undefined ? { log: config.log } : {}),
  });
  return current;
}
