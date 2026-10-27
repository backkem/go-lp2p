/** OSP and LP2P type keys. */
export declare const TypeKey: {
    readonly DataFrame: 24;
    readonly AgentInfoRequest: 10;
    readonly AgentInfoResponse: 11;
    readonly AuthCapabilities: 1001;
    readonly AuthSpake2Confirmation: 1003;
    readonly AuthStatus: 1004;
    readonly AuthSpake2Handshake: 1005;
    readonly DataChannelOpenRequest: 1101;
    readonly DataChannelOpenResponse: 1102;
    readonly SessionRequest: 1110;
    readonly SessionResponse: 1111;
    readonly DataTransportStartRequest: 1201;
    readonly DataTransportStartResponse: 1202;
    readonly DataTransportStream: 1203;
    readonly DataTransportClose: 1205;
    readonly DataTransportDrain: 1206;
    readonly ShimHello: 32001;
    readonly ShimFinished: 32002;
};
export type TypeKey = (typeof TypeKey)[keyof typeof TypeKey];
/** exchange-id = uint .lt 2^62 */
export declare const MAX_EXCHANGE_ID: number;
/** data-transport-close reason: text .size (0..1024) */
export declare const MAX_CLOSE_REASON_BYTES = 1024;
/** data-transport-start-request session-binding: bytes .size 32 */
export declare const SESSION_BINDING_BYTES = 32;
/** agent-capability values. The LP2P spec writes "11OO" (letter O); 1100 is meant. */
export declare const AgentCapability: {
    readonly DataChannels: 1100;
    readonly QuicTransport: 1200;
};
/** data-channel-encoding-id */
export declare const DataEncoding: {
    readonly Blob: 0;
    readonly String: 1;
    readonly ArrayBuffer: 2;
};
export type DataEncoding = (typeof DataEncoding)[keyof typeof DataEncoding];
/** OSP application-protocol result codes, as used by data-channel-open-response. */
export declare const Result: {
    readonly Success: 1;
    readonly Timeout: 100;
    readonly TransientError: 101;
    readonly PermanentError: 102;
    readonly Terminating: 103;
    readonly UnknownError: 199;
};
/** auth-spake2-psk-status */
export declare const PskStatus: {
    readonly NeedsPresentation: 0;
    readonly Shown: 1;
    readonly Input: 2;
};
/** auth-status-result */
export declare const AuthResult: {
    readonly Authenticated: 0;
    readonly UnknownError: 1;
    readonly Timeout: 2;
    readonly SecretUnknown: 3;
    readonly ValidationTookTooLong: 4;
    readonly ProofInvalid: 5;
};
export interface AgentInfo {
    displayName: string;
    modelName: string;
    capabilities: number[];
    stateToken: string;
    locales: string[];
}
export type Message = {
    type: typeof TypeKey.DataFrame;
    encodingId: number;
    payload: Uint8Array;
} | {
    type: typeof TypeKey.AgentInfoRequest;
    requestId: number;
} | {
    type: typeof TypeKey.AgentInfoResponse;
    requestId: number;
    agentInfo: AgentInfo;
} | {
    type: typeof TypeKey.AuthCapabilities;
    pskEaseOfInput: number;
    pskInputMethods: number[];
    pskMinBitsOfEntropy: number;
} | {
    type: typeof TypeKey.AuthSpake2Handshake;
    initiationToken?: string;
    pskStatus: number;
    publicValue: Uint8Array;
} | {
    type: typeof TypeKey.AuthSpake2Confirmation;
    confirmationValue: Uint8Array;
} | {
    type: typeof TypeKey.AuthStatus;
    result: number;
} | {
    type: typeof TypeKey.DataChannelOpenRequest;
    requestId: number;
    channelId: number;
    label: string;
    protocol: string;
} | {
    type: typeof TypeKey.DataChannelOpenResponse;
    requestId: number;
    result: number;
} | {
    type: typeof TypeKey.SessionRequest;
    requestId: number;
    requestingOrigin: string;
} | {
    type: typeof TypeKey.SessionResponse;
    requestId: number;
    result: number;
} | {
    type: typeof TypeKey.DataTransportStartRequest;
    requestId: number;
    exchangeId: number;
    sessionBinding?: Uint8Array;
} | {
    type: typeof TypeKey.DataTransportStartResponse;
    requestId: number;
    result: number;
} | {
    type: typeof TypeKey.DataTransportStream;
    exchangeId: number;
} | {
    type: typeof TypeKey.DataTransportClose;
    code: number;
    reason: string;
} | {
    type: typeof TypeKey.DataTransportDrain;
} | {
    type: typeof TypeKey.ShimHello;
    signingKey: Uint8Array;
    exchangeKey: Uint8Array;
    nonce: Uint8Array;
} | {
    type: typeof TypeKey.ShimFinished;
    signature: Uint8Array;
};
export declare class DecodeError extends Error {
    name: string;
}
/** Encodes a message as varint type key + CBOR body. */
export declare function encodeMessage(m: Message): Uint8Array;
/** Decodes one message that fills buf exactly. */
export declare function decodeMessage(buf: Uint8Array): Message;
