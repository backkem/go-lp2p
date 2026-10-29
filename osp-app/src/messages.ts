// The OSP application protocol messages (application.bs, CDDL in
// application_messages.cddl), with their type keys. Each message is declared
// once as a schema; its TypeScript type is derived from it.
//
// Streaming messages are registered by type key, so they are recognized (an
// unknown type key closes the connection), but their bodies are opaque: the
// streaming protocol is out of scope.

import { decodeCBORPrefix, encodeCBOR, IncompleteError, type CBORValue } from "./codec/cbor.ts";
import { decodeVarint, encodeVarint } from "./codec/varint.ts";
import {
  any,
  array,
  bool,
  bytesOrText,
  choice,
  float64,
  int,
  nullable,
  opt,
  req,
  struct,
  text,
  tuple,
  uint,
  SchemaError,
  type Codec,
  type Infer,
} from "./schema.ts";

// ---- Shared types -------------------------------------------------------

export const AgentCapability = {
  receiveAudio: 1,
  receiveVideo: 2,
  receivePresentation: 3,
  controlPresentation: 4,
  receiveRemotePlayback: 5,
  controlRemotePlayback: 6,
  receiveStreaming: 7,
  sendStreaming: 8,
} as const;

export const UrlAvailability = { available: 0, unavailable: 1, invalid: 10 } as const;
export type UrlAvailability = (typeof UrlAvailability)[keyof typeof UrlAvailability];

export const Result = {
  success: 1,
  invalidUrl: 10,
  invalidPresentationId: 11,
  timeout: 100,
  transientError: 101,
  permanentError: 102,
  terminating: 103,
  unknownError: 199,
} as const;
export type Result = (typeof Result)[keyof typeof Result];

export const TerminationSource = { controller: 1, receiver: 2, unknown: 255 } as const;
export type TerminationSource = (typeof TerminationSource)[keyof typeof TerminationSource];

export const TerminationReason = {
  applicationRequest: 1,
  userRequest: 2,
  receiverReplacedPresentation: 20,
  receiverIdleTooLong: 30,
  receiverAttemptedToNavigate: 31,
  receiverPoweringDown: 100,
  receiverError: 101,
  unknown: 255,
} as const;
export type TerminationReason = (typeof TerminationReason)[keyof typeof TerminationReason];

export const ConnectionCloseReason = {
  closeMethodCalled: 1,
  connectionObjectDiscarded: 10,
  unrecoverableErrorWhileSendingOrReceivingMessage: 100,
} as const;
export type ConnectionCloseReason = (typeof ConnectionCloseReason)[keyof typeof ConnectionCloseReason];

export const RemotePlaybackTerminationRequestReason = { userTerminatedViaController: 11, unknown: 255 } as const;
export const RemotePlaybackTerminationEventReason = {
  receiverCalledTerminate: 1,
  userTerminatedViaReceiver: 2,
  receiverIdleTooLong: 30,
  receiverPoweringDown: 100,
  receiverCrashed: 101,
  unknown: 255,
} as const;

export const TextTrackMode = { disabled: 1, showing: 2, hidden: 3 } as const;

const requestId = req(0, uint);
const urlAvailability = choice(UrlAvailability);
const result = choice(Result);
const httpHeader = tuple<[key: string, value: string]>(text, text);
const mediaTimeline = float64;
const mediaTimelineRange = tuple<[start: number, end: number]>(mediaTimeline, mediaTimeline);
const textTrackMode = choice(TextTrackMode);

export const agentInfo = struct({
  displayName: req(0, text),
  // Required in the CDDL, optional in the prose; openscreen-go omits it when
  // empty. Accepted when absent; set it for CDDL-strict peers.
  modelName: opt(1, text),
  // Open: the capabilities registry (capabilities.md) and extensions such as
  // LP2P's data-channels (1100) and quic-transport (1200) add values.
  capabilities: req(2, array(uint)),
  stateToken: req(3, text),
  locales: req(4, array(text)),
});
export type AgentInfo = Infer<typeof agentInfo>;

const status = struct({ status: req(0, text) });

export const remotePlaybackSource = struct({ url: req(0, text), extendedMimeType: req(1, text) });
export type RemotePlaybackSource = Infer<typeof remotePlaybackSource>;

const addedTextTrack = struct({
  kind: req(0, choice({ subtitles: 1, captions: 2, descriptions: 3, chapters: 4, metadata: 5 })),
  label: opt(1, text),
  language: opt(2, text),
});

const textTrackCue = struct({ id: req(0, text), range: req(1, mediaTimelineRange), text: req(2, text) });

const changedTextTrack = struct({
  id: req(0, text),
  mode: req(1, textTrackMode),
  addedCues: opt(2, array(textTrackCue)),
  removedCueIds: opt(3, array(text)),
});

export const remotePlaybackControls = struct({
  source: opt(0, remotePlaybackSource),
  preload: opt(1, choice({ none: 0, metadata: 1, auto: 2 })),
  loop: opt(2, bool),
  paused: opt(3, bool),
  muted: opt(4, bool),
  volume: opt(5, float64),
  seek: opt(6, mediaTimeline),
  fastSeek: opt(7, mediaTimeline),
  playbackRate: opt(8, float64),
  poster: opt(9, text),
  enabledAudioTrackIds: opt(10, array(text)),
  selectedVideoTrackId: opt(11, text),
  addedTextTracks: opt(12, array(addedTextTrack)),
  changedTextTracks: opt(13, array(changedTextTrack)),
});
export type RemotePlaybackControls = Infer<typeof remotePlaybackControls>;

const videoResolution = struct({ height: req(0, uint), width: req(1, uint) });
const mediaError = tuple<[code: 1 | 2 | 3 | 4 | 5, message: string]>(
  choice({ userAborted: 1, networkError: 2, decodeError: 3, sourceNotSupported: 4, unknownError: 5 }),
  text,
);
const trackState = { id: req(0, text), label: req(1, text), language: req(2, text) };

export const remotePlaybackState = struct({
  supports: opt(
    0,
    struct({ rate: req(0, bool), preload: req(1, bool), poster: req(2, bool), addedTextTrack: req(3, bool), addedCues: req(4, bool) }),
  ),
  source: opt(1, remotePlaybackSource),
  loading: opt(2, choice({ empty: 0, idle: 1, loading: 2, noSource: 3 })),
  loaded: opt(3, choice({ nothing: 0, metadata: 1, current: 2, future: 3, enough: 4 })),
  error: opt(4, mediaError),
  epoch: opt(5, nullable(int)),
  duration: opt(6, nullable(mediaTimeline)),
  bufferedTimeRanges: opt(7, array(mediaTimelineRange)),
  seekableTimeRanges: opt(8, array(mediaTimelineRange)),
  playedTimeRanges: opt(9, array(mediaTimelineRange)),
  position: opt(10, mediaTimeline),
  playbackRate: opt(11, float64),
  paused: opt(12, bool),
  seeking: opt(13, bool),
  stalled: opt(14, bool),
  ended: opt(15, bool),
  volume: opt(16, float64),
  muted: opt(17, bool),
  resolution: opt(18, nullable(videoResolution)),
  audioTracks: opt(19, array(struct({ ...trackState, enabled: req(3, bool) }))),
  videoTracks: opt(20, array(struct({ ...trackState, selected: req(3, bool) }))),
  textTracks: opt(21, array(struct({ ...trackState, mode: req(3, textTrackMode) }))),
});
export type RemotePlaybackState = Infer<typeof remotePlaybackState>;

// ---- Messages -----------------------------------------------------------

interface MessageDef<C extends Codec<unknown>> {
  readonly typeKey: number;
  readonly codec: C;
  /** False for messages that are recognized but whose protocol is out of scope. */
  readonly supported: boolean;
}

function m<C extends Codec<unknown>>(typeKey: number, codec: C): MessageDef<C> {
  return { typeKey, codec, supported: true };
}

/** Recognized, body kept opaque (streaming). */
function opaque(typeKey: number): MessageDef<Codec<CBORValue>> {
  return { typeKey, codec: any, supported: false };
}

export const messages = {
  // Metadata discovery and agent status.
  "agent-info-request": m(10, struct({ requestId })),
  "agent-info-response": m(11, struct({ requestId, agentInfo: req(1, agentInfo) })),
  "agent-info-event": m(120, struct({ agentInfo: req(0, agentInfo) })),
  "agent-status-request": m(12, struct({ requestId, status: opt(1, status) })),
  "agent-status-response": m(13, struct({ requestId, status: opt(1, status) })),

  // Presentation protocol.
  "presentation-url-availability-request": m(
    14,
    struct({ requestId, urls: req(1, array(text, 1)), watchDuration: req(2, uint), watchId: req(3, uint) }),
  ),
  "presentation-url-availability-response": m(15, struct({ requestId, urlAvailabilities: req(1, array(urlAvailability, 1)) })),
  "presentation-url-availability-event": m(103, struct({ watchId: req(0, uint), urlAvailabilities: req(1, array(urlAvailability, 1)) })),
  "presentation-start-request": m(
    104,
    struct({ requestId, presentationId: req(1, text), url: req(2, text), headers: req(3, array(httpHeader)) }),
  ),
  "presentation-start-response": m(
    105,
    struct({ requestId, result: req(1, result), connectionId: req(2, uint), httpResponseCode: opt(3, uint) }),
  ),
  "presentation-termination-request": m(
    106,
    struct({ requestId, presentationId: req(1, text), reason: req(2, choice(TerminationReason)) }),
  ),
  "presentation-termination-response": m(107, struct({ requestId, result: req(1, result) })),
  "presentation-termination-event": m(
    108,
    struct({ presentationId: req(0, text), source: req(1, choice(TerminationSource)), reason: req(2, choice(TerminationReason)) }),
  ),
  "presentation-connection-open-request": m(109, struct({ requestId, presentationId: req(1, text), url: req(2, text) })),
  "presentation-connection-open-response": m(
    110,
    struct({ requestId, result: req(1, result), connectionId: req(2, uint), connectionCount: req(3, uint) }),
  ),
  "presentation-connection-close-event": m(
    113,
    struct({
      connectionId: req(0, uint),
      reason: req(1, choice(ConnectionCloseReason)),
      errorMessage: opt(2, text),
      connectionCount: req(3, uint),
    }),
  ),
  "presentation-change-event": m(121, struct({ presentationId: req(0, text), connectionCount: req(1, uint) })),
  "presentation-connection-message": m(16, struct({ connectionId: req(0, uint), message: req(1, bytesOrText) })),

  // Remote playback protocol. remoting (streaming) is passed through opaque.
  "remote-playback-availability-request": m(
    17,
    struct({ requestId, sources: req(1, array(remotePlaybackSource)), watchDuration: req(2, uint), watchId: req(3, uint) }),
  ),
  "remote-playback-availability-response": m(18, struct({ requestId, urlAvailabilities: req(1, array(urlAvailability)) })),
  "remote-playback-availability-event": m(114, struct({ watchId: req(0, uint), urlAvailabilities: req(1, array(urlAvailability)) })),
  "remote-playback-start-request": m(
    115,
    struct({
      requestId,
      remotePlaybackId: req(1, uint),
      sources: opt(2, array(remotePlaybackSource)),
      textTrackUrls: opt(3, array(text)),
      headers: opt(4, array(httpHeader)),
      controls: opt(5, remotePlaybackControls),
      remoting: opt(6, any),
    }),
  ),
  "remote-playback-start-response": m(116, struct({ requestId, state: opt(1, remotePlaybackState), remoting: opt(2, any) })),
  "remote-playback-termination-request": m(
    117,
    struct({ requestId, remotePlaybackId: req(1, uint), reason: req(2, choice(RemotePlaybackTerminationRequestReason)) }),
  ),
  "remote-playback-termination-response": m(118, struct({ requestId, result: req(1, result) })),
  "remote-playback-termination-event": m(
    119,
    struct({ remotePlaybackId: req(0, uint), reason: req(1, choice(RemotePlaybackTerminationEventReason)) }),
  ),
  "remote-playback-modify-request": m(19, struct({ requestId, remotePlaybackId: req(1, uint), controls: req(2, remotePlaybackControls) })),
  "remote-playback-modify-response": m(20, struct({ requestId, result: req(1, result), state: opt(2, remotePlaybackState) })),
  "remote-playback-state-event": m(21, struct({ remotePlaybackId: req(0, uint), state: req(1, remotePlaybackState) })),

  // Streaming protocol: out of scope, recognized only.
  "audio-frame": opaque(22),
  "video-frame": opaque(23),
  "data-frame": opaque(24),
  "streaming-capabilities-request": opaque(122),
  "streaming-capabilities-response": opaque(123),
  "streaming-session-start-request": opaque(124),
  "streaming-session-start-response": opaque(125),
  "streaming-session-modify-request": opaque(126),
  "streaming-session-modify-response": opaque(127),
  "streaming-session-terminate-request": opaque(128),
  "streaming-session-terminate-response": opaque(129),
  "streaming-session-terminate-event": opaque(130),
  "streaming-session-sender-stats-event": opaque(131),
  "streaming-session-receiver-stats-event": opaque(132),
} as const;

export type MessageName = keyof typeof messages;
export type MessageBody<N extends MessageName> = Infer<(typeof messages)[N]["codec"]>;

/** Names of messages that carry a request-id at key 0. */
export type RequestName = { [N in MessageName]: MessageBody<N> extends { requestId: number } ? N : never }[MessageName];

const byTypeKey = new Map<number, MessageName>();
for (const [name, def] of Object.entries(messages) as [MessageName, MessageDef<Codec<unknown>>][]) {
  byTypeKey.set(def.typeKey, name);
}

export function messageName(typeKey: number): MessageName | undefined {
  return byTypeKey.get(typeKey);
}

export function typeKeyOf(name: MessageName): number {
  return messages[name].typeKey;
}

export function isSupported(name: MessageName): boolean {
  return messages[name].supported;
}

/** A decoded message. Body is typed by name. */
export type Message = { [N in MessageName]: { name: N; typeKey: number; body: MessageBody<N> } }[MessageName];

/** Encodes a message body to CBOR (without the type key), validating it. */
export function encodeBody<N extends MessageName>(name: N, body: MessageBody<N>): Uint8Array {
  const codec = messages[name].codec as Codec<MessageBody<N>>;
  return encodeCBOR(codec.encode(body, name));
}

/** Decodes a CBOR body for a known type key. Throws SchemaError or RangeError. */
export function decodeBody(typeKey: number, body: Uint8Array): Message {
  const name = messageName(typeKey);
  if (name === undefined) throw new RangeError(`unknown type key ${typeKey}`);
  const [value, end] = decodeCBORPrefix(body);
  if (end !== body.length) throw new RangeError("cbor: trailing bytes");
  const codec = messages[name].codec as Codec<unknown>;
  return { name, typeKey, body: codec.decode(value, name) } as Message;
}

// ---- Framing ------------------------------------------------------------

/** One OSP message on the wire: its type key and its CBOR body. */
export interface Frame {
  typeKey: number;
  /** Exactly one CBOR data item. */
  body: Uint8Array;
}

/** type key varint followed by the CBOR body (application.bs "Message delivery using CBOR"). */
export function encodeFrame(frame: Frame): Uint8Array {
  const key = encodeVarint(frame.typeKey);
  const out = new Uint8Array(key.length + frame.body.length);
  out.set(key);
  out.set(frame.body, key.length);
  return out;
}

/** Decodes exactly one framed message (a data channel message). */
export function decodeFrame(buf: Uint8Array): Frame {
  const [typeKey, n] = decodeVarint(buf);
  const [, end] = decodeCBORPrefix(buf, n);
  if (end !== buf.length) throw new RangeError("frame: trailing bytes after the CBOR item");
  return { typeKey, body: buf.subarray(n) };
}

/**
 * Splits a byte stream (a QUIC/WebTransport stream) into framed messages.
 * Push chunks as they arrive; complete frames come out. A CBOR item's length
 * is only known by parsing it, so the item is parsed here and again by
 * decodeBody; messages are small.
 */
export class FrameReader {
  #buf = new Uint8Array(0);

  push(chunk: Uint8Array): Frame[] {
    if (this.#buf.length === 0) this.#buf = chunk.slice();
    else {
      const next = new Uint8Array(this.#buf.length + chunk.length);
      next.set(this.#buf);
      next.set(chunk, this.#buf.length);
      this.#buf = next;
    }
    const out: Frame[] = [];
    let pos = 0;
    for (;;) {
      if (pos === this.#buf.length) break;
      try {
        const [typeKey, n] = decodeVarint(this.#buf, pos);
        const [, end] = decodeCBORPrefix(this.#buf, pos + n);
        out.push({ typeKey, body: this.#buf.slice(pos + n, end) });
        pos = end;
      } catch (err) {
        if (err instanceof IncompleteError) break;
        throw err;
      }
    }
    this.#buf = this.#buf.slice(pos);
    return out;
  }

  /** True if bytes of an incomplete message are buffered. */
  get pending(): boolean {
    return this.#buf.length > 0;
  }
}

export { SchemaError };
