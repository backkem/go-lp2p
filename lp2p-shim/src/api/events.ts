// Event classes from the LP2P WebIDL, and event handler IDL attributes.

import type { LP2PConnection } from "./connection.ts";
import type { LP2PDataChannel } from "./datachannel.ts";

export interface LP2PConnectionEventInit extends EventInit {
  connection: LP2PConnection;
}

// Internal state lives in WeakMaps rather than #private fields, which would
// make the classes nominal and break structural checks against the IDL types.
const connections = new WeakMap<Event, LP2PConnection>();
const channels = new WeakMap<Event, LP2PDataChannel>();

export class LP2PConnectionEvent extends Event {

  constructor(type: string, connectionEventInitDict: LP2PConnectionEventInit) {
    super(type, connectionEventInitDict);
    if (!connectionEventInitDict?.connection) throw new TypeError("LP2PConnectionEventInit.connection is required");
    connections.set(this, connectionEventInitDict.connection);
  }

  get connection(): LP2PConnection {
    return connections.get(this) as LP2PConnection;
  }
}

export interface LP2PDataChannelEventInit extends EventInit {
  channel: LP2PDataChannel;
}

export class LP2PDataChannelEvent extends Event {

  constructor(type: string, DataChannelEventInitDict: LP2PDataChannelEventInit) {
    super(type, DataChannelEventInitDict);
    if (!DataChannelEventInitDict?.channel) throw new TypeError("LP2PDataChannelEventInit.channel is required");
    channels.set(this, DataChannelEventInitDict.channel);
  }

  get channel(): LP2PDataChannel {
    return channels.get(this) as LP2PDataChannel;
  }
}

type Handler = ((this: EventTarget, ev: Event) => unknown) | null;

const handlers = new WeakMap<EventTarget, Map<string, { fn: Handler; listener: EventListener }>>();

/** Getter half of an event handler IDL attribute. */
export function getHandler(target: EventTarget, type: string): Handler {
  return handlers.get(target)?.get(type)?.fn ?? null;
}

/**
 * Setter half of an event handler IDL attribute: like the platform, the
 * handler keeps its place among the listeners when replaced, and anything
 * that is not a function clears it.
 */
export function setHandler(target: EventTarget, type: string, fn: unknown): void {
  let map = handlers.get(target);
  if (!map) handlers.set(target, (map = new Map()));
  const entry = map.get(type);
  const value = typeof fn === "function" ? (fn as Handler) : null;
  if (entry) {
    entry.fn = value;
    return;
  }
  if (!value) return;
  const slot = {
    fn: value,
    listener: (ev: Event) => {
      slot.fn?.call(target, ev);
    },
  };
  map.set(type, slot);
  target.addEventListener(type, slot.listener);
}
