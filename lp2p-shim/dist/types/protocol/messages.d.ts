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
    readonly DataTransportStartRequest: 1201;
    readonly DataTransportStartResponse: 1202;
    readonly DataTransportStreamRequest: 1203;
    readonly DataTransportStreamResponse: 1204;
    readonly DataTransportClose: 1205;
};
export type TypeKey = (typeof TypeKey)[keyof typeof TypeKey];
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
    type: typeof TypeKey.DataTransportStartRequest;
    requestId: number;
    exchangeId: number;
} | {
    type: typeof TypeKey.DataTransportStartResponse;
    requestId: number;
    result: number;
} | {
    type: typeof TypeKey.DataTransportStreamRequest;
    requestId: number;
    exchangeId: number;
    unidirectional: boolean;
} | {
    type: typeof TypeKey.DataTransportStreamResponse;
    requestId: number;
    result: number;
} | {
    type: typeof TypeKey.DataTransportClose;
    code: number;
    reason: string;
};
/** Encodes a message as varint type key + CBOR body. */
export declare function encodeMessage(m: Message): Uint8Array;
/** Decodes one message that fills buf exactly. */
export declare function decodeMessage(buf: Uint8Array): Message;
