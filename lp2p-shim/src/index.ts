// lp2p-shim: an experimental polyfill for the WICG Local Peer-to-Peer API.
// Not secure; see README.md.

export { LP2PReceiver, type LP2PReceiverOptions } from "./api/receiver.ts";
export { LP2PRequest, type LP2PRequestOptions } from "./api/request.ts";
export { LP2PConnection } from "./api/connection.ts";
export { LP2PDataChannel, type LP2PDataChannelInit, type LP2PDataChannelState } from "./api/datachannel.ts";
export {
  LP2PConnectionEvent,
  LP2PDataChannelEvent,
  type LP2PConnectionEventInit,
  type LP2PDataChannelEventInit,
} from "./api/events.ts";

export { configure, getUserAgent, type ShimConfig } from "./config.ts";
export { install, interfaces, type InstallOptions } from "./install.ts";

export { UserAgent, type UserAgentConfig, type Pairing, type GrantedSession } from "./ua/useragent.ts";
export {
  AutoUI,
  peerLabel,
  type AutoUIOptions,
  type PeerInfo,
  type PeerPicker,
  type Permission,
  type PermissionRequest,
  type PINPresentation,
  type PINPrompt,
  type UserAgentUI,
} from "./ua/ui.ts";
export { DomUI } from "./ua/dom-ui.ts";

export type {
  Advertisement,
  Closer,
  DiscoveredPeer,
  DiscoveryEvent,
  Transport,
  TransportConnection,
  TransportStream,
} from "./transport/types.ts";
export { BaseConnection, BaseStream } from "./transport/base.ts";
export { PostMessageTransport, type PostMessageTransportOptions } from "./transport/postmessage.ts";
export { WebRTCTransport, type WebRTCTransportOptions } from "./transport/webrtc.ts";
export {
  BroadcastChannelSignaler,
  ManualSignaler,
  PortSignaler,
  type PortLike,
  type SignalMessage,
  type Signaler,
} from "./transport/signaling.ts";
export { Rendezvous, type RendezvousOptions } from "./transport/rendezvous.ts";

export { Session, DataChannelEndpoint, ControlChannel, ProtocolError, type Role } from "./protocol/session.ts";
export * as messages from "./protocol/messages.ts";
export { encodeCBOR, decodeCBOR, type CBORValue } from "./codec/cbor.ts";
export { encodeVarint, decodeVarint } from "./codec/varint.ts";

export const version = "0.1.0";
