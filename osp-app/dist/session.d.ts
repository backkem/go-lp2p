import { type AgentInfo, type Frame, type Message, type MessageBody, type MessageName } from "./messages.ts";
import { Emitter } from "./emitter.ts";
import { type CloseInfo, type OspTransport, type SendOptions } from "./transport.ts";
/** application.bs: an unknown type key closes the connection with this code. */
export declare const UNKNOWN_TYPE_KEY_CODE = 404;
export interface SessionOptions {
    /**
     * The local agent-info. When set, agent-info-request and
     * agent-status-request are answered automatically.
     */
    agentInfo?: AgentInfo;
    /**
     * Allocates request IDs. The default counts from 1 per session. OSP
     * implementations (openscreen-go) keep one counter per remote agent across
     * connections; pass a shared allocator to do the same.
     */
    nextRequestId?: () => number;
    /**
     * What to do with an unknown type key: "close" (application.bs: close with
     * error code 404 and the type key in the reason) or "ignore". Default "close".
     */
    onUnknownTypeKey?: "close" | "ignore";
}
export interface SessionEvents {
    /** A message with a supported type that no handler took, or a response to no pending request. */
    unhandled: Message;
    /** A recognized message of an out-of-scope protocol (streaming). Dropped. */
    unsupported: Message;
    /** A message with an unknown type key. */
    unknown: Frame;
    /** A message that could not be decoded. Dropped. */
    malformed: Error;
    /** The transport closed. */
    close: CloseInfo;
}
type Body<N extends MessageName> = MessageBody<N>;
type WithoutId<T> = T extends {
    requestId: number;
} ? Omit<T, "requestId"> : T;
/** A response type for each request type. */
export declare const responseOf: {
    readonly "agent-info-request": "agent-info-response";
    readonly "agent-status-request": "agent-status-response";
    readonly "presentation-url-availability-request": "presentation-url-availability-response";
    readonly "presentation-start-request": "presentation-start-response";
    readonly "presentation-termination-request": "presentation-termination-response";
    readonly "presentation-connection-open-request": "presentation-connection-open-response";
    readonly "remote-playback-availability-request": "remote-playback-availability-response";
    readonly "remote-playback-start-request": "remote-playback-start-response";
    readonly "remote-playback-termination-request": "remote-playback-termination-response";
    readonly "remote-playback-modify-request": "remote-playback-modify-response";
};
export type OutgoingRequest = keyof typeof responseOf;
export type ResponseTo<N extends OutgoingRequest> = (typeof responseOf)[N];
export interface RequestOptions<R extends MessageName> extends SendOptions {
    /** Runs synchronously when the response arrives, before later messages are dispatched. */
    onResponse?: (body: Body<R>) => void;
    /** Rejects with a TimeoutError after this many milliseconds. */
    timeout?: number;
}
export declare class OspSession extends Emitter<SessionEvents> {
    #private;
    readonly transport: OspTransport;
    /** The peer's agent-info, once requested with requestAgentInfo() or announced with agent-info-event. */
    remoteAgentInfo: AgentInfo | null;
    readonly closed: Promise<CloseInfo>;
    constructor(transport: OspTransport, options?: SessionOptions);
    get isClosed(): boolean;
    /** Allocates a request ID. */
    nextRequestId(): number;
    /** Sends a message. */
    send<N extends MessageName>(name: N, body: Body<N>, options?: SendOptions): Promise<void>;
    /** Sends a request with a fresh request ID and resolves with the matching response. */
    request<N extends OutgoingRequest>(name: N, body: WithoutId<Body<N>>, options?: RequestOptions<ResponseTo<N>>): Promise<Body<ResponseTo<N>>>;
    /** Handles every received message of a type. Returns a function that removes the handler. */
    handle<N extends MessageName>(name: N, fn: (body: Body<N>, msg: Message) => void): () => void;
    /** Asks the peer for its agent-info. */
    requestAgentInfo(options?: SendOptions): Promise<AgentInfo>;
    /** Closes the transport. */
    close(info?: Partial<CloseInfo>): void;
}
export {};
