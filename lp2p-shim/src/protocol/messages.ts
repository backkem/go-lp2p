// Open Screen Protocol message shapes used by the shim, with the LP2P
// extension messages from the LP2P spec's Appendix A.
//
// On the wire every message is a QUIC varint type key followed by one CBOR
// map with integer keys, exactly as OSP frames messages on QUIC streams. The
// shim's transports carry messages rather than byte streams, so each
// transport message holds one OSP message; a byte-stream transport (QUIC,
// WebSocket to a native helper) can concatenate them unchanged, since both
// parts are self-delimiting.

import { decodeCBORPrefix, encodeCBOR, type CBORValue } from "../codec/cbor.ts";
import { decodeVarint, encodeVarint } from "../codec/varint.ts";

/** OSP and LP2P type keys. */
export const TypeKey = {
  DataFrame: 24,
  AgentInfoRequest: 10,
  AgentInfoResponse: 11,
  AuthCapabilities: 1001,
  AuthSpake2Confirmation: 1003,
  AuthStatus: 1004,
  AuthSpake2Handshake: 1005,
  DataChannelOpenRequest: 1101,
  DataChannelOpenResponse: 1102,
  // Pooled WebTransport. Not in the LP2P spec, whose binding dials a
  // separate QUIC connection instead (see webtransport.ts). 1201-1204 are
  // go-lp2p's work-in-progress messages (openscreen-go/network/messages_wip.go);
  // 1203 gains key 2 (unidirectional). 1205 is the shim's own.
  DataTransportStartRequest: 1201,
  DataTransportStartResponse: 1202,
  DataTransportStreamRequest: 1203,
  DataTransportStreamResponse: 1204,
  DataTransportClose: 1205,
} as const;
export type TypeKey = (typeof TypeKey)[keyof typeof TypeKey];

/** agent-capability values. The LP2P spec writes "11OO" (letter O); 1100 is meant. */
export const AgentCapability = {
  DataChannels: 1100,
  QuicTransport: 1200,
} as const;

/** data-channel-encoding-id */
export const DataEncoding = {
  Blob: 0,
  String: 1,
  ArrayBuffer: 2,
} as const;
export type DataEncoding = (typeof DataEncoding)[keyof typeof DataEncoding];

/** OSP application-protocol result codes, as used by data-channel-open-response. */
export const Result = {
  Success: 1,
  Timeout: 100,
  TransientError: 101,
  PermanentError: 102,
  Terminating: 103,
  UnknownError: 199,
} as const;

/** auth-spake2-psk-status */
export const PskStatus = {
  NeedsPresentation: 0,
  Shown: 1,
  Input: 2,
} as const;

/** auth-status-result */
export const AuthResult = {
  Authenticated: 0,
  UnknownError: 1,
  Timeout: 2,
  SecretUnknown: 3,
  ValidationTookTooLong: 4,
  ProofInvalid: 5,
} as const;

export interface AgentInfo {
  displayName: string;
  modelName: string;
  capabilities: number[];
  stateToken: string;
  locales: string[];
}

export type Message =
  | { type: typeof TypeKey.DataFrame; encodingId: number; payload: Uint8Array }
  | { type: typeof TypeKey.AgentInfoRequest; requestId: number }
  | { type: typeof TypeKey.AgentInfoResponse; requestId: number; agentInfo: AgentInfo }
  | {
      type: typeof TypeKey.AuthCapabilities;
      pskEaseOfInput: number;
      pskInputMethods: number[];
      pskMinBitsOfEntropy: number;
    }
  | { type: typeof TypeKey.AuthSpake2Handshake; initiationToken?: string; pskStatus: number; publicValue: Uint8Array }
  | { type: typeof TypeKey.AuthSpake2Confirmation; confirmationValue: Uint8Array }
  | { type: typeof TypeKey.AuthStatus; result: number }
  | { type: typeof TypeKey.DataChannelOpenRequest; requestId: number; channelId: number; label: string; protocol: string }
  | { type: typeof TypeKey.DataChannelOpenResponse; requestId: number; result: number }
  | { type: typeof TypeKey.DataTransportStartRequest; requestId: number; exchangeId: number }
  | { type: typeof TypeKey.DataTransportStartResponse; requestId: number; result: number }
  | { type: typeof TypeKey.DataTransportStreamRequest; requestId: number; exchangeId: number; unidirectional: boolean }
  | { type: typeof TypeKey.DataTransportStreamResponse; requestId: number; result: number }
  | { type: typeof TypeKey.DataTransportClose; code: number; reason: string };

type CBORMap = { [key: string]: CBORValue };

function toCBOR(m: Message): CBORMap {
  switch (m.type) {
    case TypeKey.DataFrame:
      return { 0: m.encodingId, 4: m.payload };
    case TypeKey.AgentInfoRequest:
      return { 0: m.requestId };
    case TypeKey.AgentInfoResponse: {
      const a = m.agentInfo;
      return {
        0: m.requestId,
        1: { 0: a.displayName, 1: a.modelName, 2: a.capabilities, 3: a.stateToken, 4: a.locales },
      };
    }
    case TypeKey.AuthCapabilities:
      return { 0: m.pskEaseOfInput, 1: m.pskInputMethods, 2: m.pskMinBitsOfEntropy };
    case TypeKey.AuthSpake2Handshake:
      return {
        0: m.initiationToken === undefined ? {} : { 0: m.initiationToken },
        1: m.pskStatus,
        2: m.publicValue,
      };
    case TypeKey.AuthSpake2Confirmation:
      return { 0: m.confirmationValue };
    case TypeKey.AuthStatus:
      return { 0: m.result };
    case TypeKey.DataChannelOpenRequest:
      return { 0: m.requestId, 1: m.channelId, 2: m.label, 3: m.protocol };
    case TypeKey.DataChannelOpenResponse:
    case TypeKey.DataTransportStartResponse:
    case TypeKey.DataTransportStreamResponse:
      return { 0: m.requestId, 1: m.result };
    case TypeKey.DataTransportStartRequest:
      return { 0: m.requestId, 1: m.exchangeId };
    case TypeKey.DataTransportStreamRequest:
      return m.unidirectional ? { 0: m.requestId, 1: m.exchangeId, 2: true } : { 0: m.requestId, 1: m.exchangeId };
    case TypeKey.DataTransportClose:
      return { 0: m.code, 1: m.reason };
  }
}

class DecodeError extends Error {
  override name = "DecodeError";
}

function uint(map: CBORMap, key: number, what: string): number {
  const v = map[key];
  if (typeof v !== "number" || !Number.isSafeInteger(v) || v < 0) throw new DecodeError(`${what}: field ${key} must be uint`);
  return v;
}
function text(map: CBORMap, key: number, what: string): string {
  const v = map[key];
  if (typeof v !== "string") throw new DecodeError(`${what}: field ${key} must be text`);
  return v;
}
function bytes(map: CBORMap, key: number, what: string): Uint8Array {
  const v = map[key];
  if (!(v instanceof Uint8Array)) throw new DecodeError(`${what}: field ${key} must be bytes`);
  return v;
}
function asMap(v: CBORValue | undefined, what: string): CBORMap {
  if (typeof v !== "object" || v === null || v instanceof Uint8Array || Array.isArray(v)) {
    throw new DecodeError(`${what}: expected a map`);
  }
  return v;
}
function list<T>(map: CBORMap, key: number, what: string, check: (v: CBORValue) => v is T & CBORValue): T[] {
  const v = map[key];
  if (!Array.isArray(v) || !v.every(check)) throw new DecodeError(`${what}: field ${key} has the wrong type`);
  return v as T[];
}
const isUint = (v: CBORValue): v is number => typeof v === "number" && Number.isSafeInteger(v) && v >= 0;
const isText = (v: CBORValue): v is string => typeof v === "string";

function fromCBOR(type: number, value: CBORValue): Message {
  const m = asMap(value, `type ${type}`);
  switch (type) {
    case TypeKey.DataFrame:
      return { type, encodingId: uint(m, 0, "data-frame"), payload: bytes(m, 4, "data-frame") };
    case TypeKey.AgentInfoRequest:
      return { type, requestId: uint(m, 0, "agent-info-request") };
    case TypeKey.AgentInfoResponse: {
      const what = "agent-info";
      const a = asMap(m[1], what);
      return {
        type,
        requestId: uint(m, 0, "agent-info-response"),
        agentInfo: {
          displayName: text(a, 0, what),
          modelName: text(a, 1, what),
          capabilities: list(a, 2, what, isUint),
          stateToken: text(a, 3, what),
          locales: list(a, 4, what, isText),
        },
      };
    }
    case TypeKey.AuthCapabilities:
      return {
        type,
        pskEaseOfInput: uint(m, 0, "auth-capabilities"),
        pskInputMethods: list(m, 1, "auth-capabilities", isUint),
        pskMinBitsOfEntropy: uint(m, 2, "auth-capabilities"),
      };
    case TypeKey.AuthSpake2Handshake: {
      const token = asMap(m[0], "auth-initiation-token")[0];
      return {
        type,
        ...(typeof token === "string" ? { initiationToken: token } : {}),
        pskStatus: uint(m, 1, "auth-spake2-handshake"),
        publicValue: bytes(m, 2, "auth-spake2-handshake"),
      };
    }
    case TypeKey.AuthSpake2Confirmation:
      return { type, confirmationValue: bytes(m, 0, "auth-spake2-confirmation") };
    case TypeKey.AuthStatus:
      return { type, result: uint(m, 0, "auth-status") };
    case TypeKey.DataChannelOpenRequest:
      return {
        type,
        requestId: uint(m, 0, "data-channel-open-request"),
        channelId: uint(m, 1, "data-channel-open-request"),
        label: text(m, 2, "data-channel-open-request"),
        protocol: text(m, 3, "data-channel-open-request"),
      };
    case TypeKey.DataChannelOpenResponse:
      return {
        type,
        requestId: uint(m, 0, "data-channel-open-response"),
        result: uint(m, 1, "data-channel-open-response"),
      };
    case TypeKey.DataTransportStartRequest:
      return { type, requestId: uint(m, 0, "data-transport-start-request"), exchangeId: uint(m, 1, "data-transport-start-request") };
    case TypeKey.DataTransportStartResponse:
    case TypeKey.DataTransportStreamResponse:
      return { type, requestId: uint(m, 0, "data-transport-response"), result: uint(m, 1, "data-transport-response") };
    case TypeKey.DataTransportStreamRequest:
      return {
        type,
        requestId: uint(m, 0, "data-transport-stream-request"),
        exchangeId: uint(m, 1, "data-transport-stream-request"),
        unidirectional: m[2] === true,
      };
    case TypeKey.DataTransportClose:
      return { type, code: uint(m, 0, "data-transport-close"), reason: text(m, 1, "data-transport-close") };
    default:
      throw new DecodeError(`unknown type key ${type}`);
  }
}

/** Encodes a message as varint type key + CBOR body. */
export function encodeMessage(m: Message): Uint8Array {
  const key = encodeVarint(m.type);
  const body = encodeCBOR(toCBOR(m));
  const out = new Uint8Array(key.length + body.length);
  out.set(key);
  out.set(body, key.length);
  return out;
}

/** Decodes one message that fills buf exactly. */
export function decodeMessage(buf: Uint8Array): Message {
  const [type, n] = decodeVarint(buf);
  const [value, end] = decodeCBORPrefix(buf, n);
  if (end !== buf.length) throw new DecodeError("trailing bytes after message");
  return fromCBOR(type, value);
}
