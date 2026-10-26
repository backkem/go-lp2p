// Compile-time check that the shim's classes match the API surface typed
// from the LP2P WebIDL (../../src/lp2p.d.ts at the repo root). Checked by
// `npm run typecheck`; never run.

import * as shim from "../src/index.ts";

type Assert<T extends true> = T;
/** Every IDL member exists on the implementation. */
type Covers<Impl, Spec> = [Exclude<keyof Spec, keyof Impl>] extends [never] ? true : false;
type Assignable<Impl, Spec> = Impl extends Spec ? true : false;

export type Checks = [
  Assert<Covers<shim.LP2PReceiver, LP2PReceiver>>,
  Assert<Covers<shim.LP2PRequest, LP2PRequest>>,
  Assert<Covers<shim.LP2PConnection, LP2PConnection>>,
  Assert<Covers<shim.LP2PDataChannel, LP2PDataChannel>>,
  Assert<Covers<shim.LP2PConnectionEvent, LP2PConnectionEvent>>,
  Assert<Covers<shim.LP2PDataChannelEvent, LP2PDataChannelEvent>>,
  Assert<Covers<shim.LP2PQuicTransport, LP2PQuicTransport>>,
  Assert<Covers<shim.LP2PQuicTransportListener, LP2PQuicTransportListener>>,
  Assert<Covers<shim.LP2PDatagramDuplexStream, WebTransportDatagramDuplexStream>>,
  Assert<Covers<shim.LP2PBidirectionalStream, WebTransportBidirectionalStream>>,
  Assert<Assignable<shim.LP2PQuicTransportListener, LP2PQuicTransportListener>>,
  // Member types too, where event handler parameter variance doesn't get in the way.
  Assert<Assignable<shim.LP2PDataChannel, LP2PDataChannel>>,
  Assert<Assignable<ReturnType<shim.LP2PRequest["start"]>, Promise<unknown>>>,
  Assert<Assignable<ReturnType<shim.LP2PReceiver["start"]>, Promise<undefined>>>,
  // Dictionaries.
  Assert<Assignable<LP2PReceiverOptions, shim.LP2PReceiverOptions>>,
  Assert<Assignable<LP2PRequestOptions, shim.LP2PRequestOptions>>,
  Assert<Assignable<LP2PDataChannelInit, shim.LP2PDataChannelInit>>,
  Assert<Assignable<LP2PQuicTransportInit, shim.LP2PQuicTransportInit>>,
  Assert<Assignable<LP2PQuicTransportListenerInit, shim.LP2PQuicTransportListenerInit>>,
];

// Constructors take the IDL's arguments.
export function constructors(c: shim.LP2PConnection, ch: shim.LP2PDataChannel): void {
  new shim.LP2PReceiver({ nickname: "n" });
  new shim.LP2PRequest({ nickname: "n" });
  new shim.LP2PDataChannel("label", { protocol: "p", id: 1 });
  new shim.LP2PConnectionEvent("connection", { connection: c });
  new shim.LP2PDataChannelEvent("datachannel", { channel: ch });
  const dc: LP2PDataChannel = c.createDataChannel("x", { protocol: "", id: 2 });
  void dc;
  new shim.LP2PQuicTransport(c, {});
  new shim.LP2PQuicTransport(new shim.LP2PRequest(), {});
  new shim.LP2PQuicTransport(new shim.LP2PReceiver());
  new shim.LP2PQuicTransportListener(new shim.LP2PReceiver(), {});
}
