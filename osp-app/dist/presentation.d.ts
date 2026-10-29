import { Emitter } from "./emitter.ts";
import { ConnectionCloseReason, Result, TerminationReason, TerminationSource, UrlAvailability } from "./messages.ts";
import type { OspSession } from "./session.ts";
export type PresentationMessage = string | Uint8Array;
/** A start, reconnect or terminate request failed with this result. */
export declare class PresentationError extends Error {
    name: string;
    readonly result: Result;
    constructor(message: string, result: Result);
}
/** A random presentation identifier: 24 ASCII alphanumerics (the Presentation API asks for at least 16). */
export declare function generatePresentationId(): string;
/** Presentation API 6.1: at least 16 ASCII characters. */
export declare function isValidPresentationId(id: string): boolean;
export type ConnectionState = "connected" | "closed" | "terminated";
export interface ConnectionCloseInfo {
    reason: ConnectionCloseReason;
    errorMessage?: string;
    /** Whether this side closed the connection. */
    local: boolean;
}
export interface ConnectionEvents {
    message: PresentationMessage;
    close: ConnectionCloseInfo;
    terminate: {
        source: TerminationSource;
        reason: TerminationReason;
    };
    /** presentation-change-event: another connection to the presentation opened or closed. */
    change: {
        connectionCount: number;
    };
}
interface ConnectionSide {
    send(m: PresentationMessage): Promise<void>;
    close(reason: ConnectionCloseReason, errorMessage?: string): Promise<void>;
    terminate(reason: TerminationReason): Promise<void>;
}
/** One presentation connection, on either side. */
export declare class PresentationConnection extends Emitter<ConnectionEvents> {
    #private;
    readonly presentationId: string;
    readonly url: string;
    readonly connectionId: number;
    /** The number of open connections to the presentation, as last reported. */
    connectionCount: number;
    /** @internal */
    constructor(presentationId: string, url: string, connectionId: number, connectionCount: number, side: ConnectionSide);
    get state(): ConnectionState;
    /** Sends a text (string) or binary (Uint8Array) presentation message. */
    send(message: PresentationMessage): Promise<void>;
    /** Closes this connection without terminating the presentation. */
    close(reason?: ConnectionCloseReason, errorMessage?: string): Promise<void>;
    /** Terminates the presentation (controller: termination request; receiver: termination event). */
    terminate(reason?: TerminationReason): Promise<void>;
    /** @internal */
    _message(m: PresentationMessage): void;
    /** @internal */
    _closed(info: ConnectionCloseInfo): void;
    /** @internal */
    _terminated(source: TerminationSource, reason: TerminationReason): void;
    /** @internal */
    _change(connectionCount: number): void;
}
export interface AvailabilityResult {
    watchId: number;
    availabilities: UrlAvailability[];
}
export interface ControllerEvents {
    /** presentation-url-availability-event for a watch of checkAvailability(). */
    availability: {
        watchId: number;
        urls: string[];
        availabilities: UrlAvailability[];
    };
    /** presentation-termination-event. */
    terminated: {
        presentationId: string;
        source: TerminationSource;
        reason: TerminationReason;
    };
    /** A connection message or close event for a connection-id this controller doesn't know. */
    unknownConnection: number;
}
export interface StartOptions {
    /** Defaults to generatePresentationId(). */
    presentationId?: string;
    headers?: [key: string, value: string][];
    /** Milliseconds to wait for the response. */
    timeout?: number;
}
/** The controlling side of the presentation protocol, over one session. */
export declare class PresentationController extends Emitter<ControllerEvents> {
    #private;
    readonly session: OspSession;
    constructor(session: OspSession);
    /** Open connections, by connection-id. */
    get connections(): ReadonlyMap<number, PresentationConnection>;
    /**
     * Asks which of urls the receiver can present. With watchDuration (in
     * microseconds, as on the wire) the receiver sends "availability" events
     * for changes until it expires.
     */
    checkAvailability(urls: string[], options?: {
        watchDuration?: number;
        timeout?: number;
    }): Promise<AvailabilityResult>;
    /** Starts a presentation of url and returns its first connection. */
    start(url: string, options?: StartOptions): Promise<PresentationConnection>;
    /** Opens another connection to a running presentation (Presentation API reconnect). */
    reconnect(presentationId: string, url: string, options?: {
        timeout?: number;
    }): Promise<PresentationConnection>;
    /** Terminates a presentation. Resolves once the receiver confirmed it. */
    terminate(presentationId: string, reason?: TerminationReason, options?: {
        timeout?: number;
    }): Promise<void>;
}
export interface StartRequest {
    presentationId: string;
    url: string;
    headers: [key: string, value: string][];
    session: OspSession;
}
export interface ReceiverOptions {
    /** Availability of a URL. Default: setAvailability() values, else "available" for http(s) URLs, "invalid" otherwise. */
    availabilityOf?: (url: string) => UrlAvailability;
    /**
     * Loads a presentation (application.bs: respond "after either the
     * presentation URL has been fetched and loaded, or the receiver has failed
     * to do so"). Default: success.
     */
    start?: (req: StartRequest) => {
        result: Result;
        httpResponseCode?: number;
    } | Promise<{
        result: Result;
        httpResponseCode?: number;
    }>;
    /** Decides a termination request. Default: success. */
    terminate?: (presentationId: string, reason: TerminationReason, session: OspSession) => Result | Promise<Result>;
}
export interface ReceiverEvents {
    /** A presentation started. */
    presentation: {
        presentationId: string;
        url: string;
    };
    /** A connection opened (by start or reconnect). */
    connection: PresentationConnection;
    /** A presentation was terminated, by a controller or by terminate(). */
    terminated: {
        presentationId: string;
        source: TerminationSource;
        reason: TerminationReason;
    };
}
/** The receiving side of the presentation protocol, serving any number of controller sessions. */
export declare class PresentationReceiver extends Emitter<ReceiverEvents> {
    #private;
    constructor(options?: ReceiverOptions);
    /** Running presentations: id to URL. */
    get presentations(): ReadonlyMap<string, string>;
    /** Serves a controller on session. Returns a function that detaches it. */
    attach(session: OspSession): () => void;
    /** Sets a URL's availability and notifies the controllers watching it. */
    setAvailability(url: string, availability: UrlAvailability): void;
    /** Terminates a presentation from the receiver side (termination event to every connected controller). */
    terminate(presentationId: string, reason?: TerminationReason): Promise<void>;
}
export {};
