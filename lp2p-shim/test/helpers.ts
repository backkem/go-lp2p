import { PostMessageTransport, type PostMessageTransportOptions } from "../src/transport/postmessage.ts";
import type { Transport } from "../src/transport/types.ts";
import { AutoUI, type AutoUIOptions } from "../src/ua/ui.ts";

let n = 0;
/** A fresh BroadcastChannel name, so tests don't see each other. */
export function channelName(): string {
  return `lp2p-shim-test-${process.pid}-${n++}`;
}

/** Two postMessage transports on the same fresh channel, as two "pages". */
export function transportPair(options: PostMessageTransportOptions = {}): [PostMessageTransport, PostMessageTransport] {
  const channel = channelName();
  const opts = { channelName: channel, queryInterval: 30, ttl: 200, ...options };
  return [new PostMessageTransport(opts), new PostMessageTransport(opts)];
}

/** AutoUIs for a receiver and a requester whose PIN entry reads the receiver's screen. */
export function pairedUIs(
  receiver: AutoUIOptions = {},
  requester: AutoUIOptions = {},
): [AutoUI, AutoUI] {
  let shown: ((pin: string) => void) | null = null;
  let pin: Promise<string> = new Promise((r) => (shown = r));
  return [
    new AutoUI({
      onPIN: (p) => {
        shown?.(p.pin);
      },
      ...receiver,
    }),
    new AutoUI({
      enterPIN: async () => {
        const p = await pin;
        pin = new Promise((r) => (shown = r));
        return p;
      },
      ...requester,
    }),
  ];
}

export function closeAll(...ts: Transport[]): void {
  for (const t of ts) (t as { close?: () => void }).close?.();
}

export function once<T extends Event>(target: EventTarget, type: string): Promise<T> {
  return new Promise((resolve) => target.addEventListener(type, (e) => resolve(e as T), { once: true }));
}

export function delay(ms: number): Promise<void> {
  return new Promise((r) => setTimeout(r, ms));
}
