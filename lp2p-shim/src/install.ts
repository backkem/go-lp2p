// Installs the LP2P interfaces as globals, polyfill style.

import { LP2PConnection } from "./api/connection.ts";
import { LP2PDataChannel } from "./api/datachannel.ts";
import { LP2PConnectionEvent, LP2PDataChannelEvent } from "./api/events.ts";
import { LP2PReceiver } from "./api/receiver.ts";
import { LP2PRequest } from "./api/request.ts";

export const interfaces = {
  LP2PReceiver,
  LP2PRequest,
  LP2PConnection,
  LP2PDataChannel,
  LP2PConnectionEvent,
  LP2PDataChannelEvent,
} as const;

export interface InstallOptions {
  /** Replace interfaces the browser already has. Default false. */
  force?: boolean;
}

/**
 * Defines the LP2P interfaces on globalThis where missing (or everywhere
 * with force). Like the real API ([SecureContext]), nothing is installed in
 * an insecure context. Returns the names that were installed.
 */
export function install(options: InstallOptions = {}): string[] {
  if ((globalThis as { isSecureContext?: boolean }).isSecureContext === false) return [];
  const installed: string[] = [];
  for (const [name, value] of Object.entries(interfaces)) {
    if (!options.force && name in globalThis) continue;
    Object.defineProperty(globalThis, name, { value, writable: true, configurable: true, enumerable: false });
    installed.push(name);
  }
  return installed;
}
