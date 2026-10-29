// Type-level check, run by `npm run typecheck`: the platform classes and the
// lp2p-shim classes fit the transports' structural shapes. Type-only: nothing
// from lp2p-shim is bundled or imported at run time.

import type { LP2PDataChannel, LP2PQuicTransport } from "../../lp2p-shim/dist/types/index.d.ts";
import type { DataChannelLike, WebTransportLike } from "../src/index.ts";

declare const rtc: RTCDataChannel;
declare const lp2pChannel: LP2PDataChannel;
declare const wt: WebTransport;
declare const lp2pQuic: LP2PQuicTransport;

export const shapes: [DataChannelLike, DataChannelLike, WebTransportLike, WebTransportLike] = [rtc, lp2pChannel, wt, lp2pQuic];
