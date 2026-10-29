// osp-app: the Open Screen Protocol application protocol over pluggable transports.
export { encodeCBOR, decodeCBOR, decodeCBORPrefix, Float64, IncompleteError } from "./codec/cbor.js";
export { encodeVarint, decodeVarint } from "./codec/varint.js";
export { SchemaError } from "./schema.js";
export * from "./messages.js";
export { TransportClosedError } from "./transport.js";
export { DataChannelTransport } from "./transports/datachannel.js";
export { WebTransportTransport, } from "./transports/webtransport.js";
export { OspSession, UNKNOWN_TYPE_KEY_CODE, responseOf } from "./session.js";
export { Emitter } from "./emitter.js";
export * from "./presentation.js";
export * from "./remote-playback.js";
export const version = "0.1.0";
