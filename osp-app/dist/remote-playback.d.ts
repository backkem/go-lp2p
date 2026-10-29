import { Emitter } from "./emitter.ts";
import { RemotePlaybackTerminationEventReason, RemotePlaybackTerminationRequestReason, Result, UrlAvailability, type RemotePlaybackControls, type RemotePlaybackSource, type RemotePlaybackState } from "./messages.ts";
import type { OspSession } from "./session.ts";
type TerminationRequestReason = (typeof RemotePlaybackTerminationRequestReason)[keyof typeof RemotePlaybackTerminationRequestReason];
type TerminationEventReason = (typeof RemotePlaybackTerminationEventReason)[keyof typeof RemotePlaybackTerminationEventReason];
/** A random 48-bit remote-playback-id. The prose asks for a UUID; the CDDL says uint. */
export declare function generateRemotePlaybackId(): number;
/** Merges a partial state (a state event or response) into a full one. */
export declare function mergeState(into: RemotePlaybackState, update: RemotePlaybackState): RemotePlaybackState;
export declare class RemotePlaybackError extends Error {
    name: string;
    readonly result: Result;
    constructor(message: string, result: Result);
}
export interface RemotePlaybackEvents {
    state: RemotePlaybackState;
    terminate: {
        reason: TerminationEventReason | TerminationRequestReason;
        local: boolean;
    };
}
/** A remote playback, as seen by the controller. */
export declare class RemotePlayback extends Emitter<RemotePlaybackEvents> {
    #private;
    readonly id: number;
    /** The playback state, merged from the start response, modify responses and state events. */
    readonly state: RemotePlaybackState;
    /** @internal */
    constructor(session: OspSession, id: number);
    get terminated(): boolean;
    /** Sends controls (pause, seek, volume...) and resolves with the updated state. */
    modify(controls: RemotePlaybackControls, options?: {
        timeout?: number;
    }): Promise<RemotePlaybackState>;
    terminate(reason?: TerminationRequestReason): Promise<void>;
    /** @internal */
    _state(update: RemotePlaybackState): void;
    /** @internal */
    _terminated(reason: TerminationEventReason | TerminationRequestReason, local: boolean): void;
}
export interface RemotePlaybackStartOptions {
    sources: RemotePlaybackSource[];
    textTrackUrls?: string[];
    headers?: [key: string, value: string][];
    controls?: RemotePlaybackControls;
    remotePlaybackId?: number;
    timeout?: number;
}
export interface RemotePlaybackControllerEvents {
    availability: {
        watchId: number;
        sources: RemotePlaybackSource[];
        availabilities: UrlAvailability[];
    };
}
/** The controlling side of the remote playback protocol, over one session. */
export declare class RemotePlaybackController extends Emitter<RemotePlaybackControllerEvents> {
    #private;
    readonly session: OspSession;
    constructor(session: OspSession);
    checkAvailability(sources: RemotePlaybackSource[], options?: {
        watchDuration?: number;
        timeout?: number;
    }): Promise<{
        watchId: number;
        availabilities: UrlAvailability[];
    }>;
    /**
     * Starts remote playback. The start response has no result field, so a
     * receiver that refuses answers and then sends a termination event; the
     * returned playback is then already terminated.
     */
    start(options: RemotePlaybackStartOptions): Promise<RemotePlayback>;
}
export interface PlaybackStartRequest {
    remotePlaybackId: number;
    sources: RemotePlaybackSource[];
    textTrackUrls: string[];
    headers: [string, string][];
    controls: RemotePlaybackControls;
    session: OspSession;
}
export interface RemotePlaybackReceiverOptions {
    /** Availability of a source. Default: "available" for http(s) URLs. */
    availabilityOf?: (source: RemotePlaybackSource) => UrlAvailability;
    /** Starts playback and returns its initial state, or null to refuse. Default: paused, from the controls. */
    start?: (req: PlaybackStartRequest) => RemotePlaybackState | null | Promise<RemotePlaybackState | null>;
    /** Applies controls. Default: mirrors paused, muted, volume, seek and playbackRate into the state. */
    modify?: (id: number, controls: RemotePlaybackControls, state: RemotePlaybackState) => RemotePlaybackState | Promise<RemotePlaybackState>;
}
export interface RemotePlaybackReceiverEvents {
    playback: {
        remotePlaybackId: number;
        sources: RemotePlaybackSource[];
        state: RemotePlaybackState;
    };
    modify: {
        remotePlaybackId: number;
        controls: RemotePlaybackControls;
        state: RemotePlaybackState;
    };
    terminated: {
        remotePlaybackId: number;
        reason: TerminationEventReason | TerminationRequestReason;
    };
}
/** Applies the controls that map directly onto state. */
export declare function applyControls(state: RemotePlaybackState, c: RemotePlaybackControls): RemotePlaybackState;
/** The receiving side of the remote playback protocol, serving any number of controller sessions. */
export declare class RemotePlaybackReceiver extends Emitter<RemotePlaybackReceiverEvents> {
    #private;
    constructor(options?: RemotePlaybackReceiverOptions);
    attach(session: OspSession): () => void;
    /** The state of a running playback. */
    stateOf(id: number): RemotePlaybackState | undefined;
    /** Updates a playback's state and sends a state event with the changed fields. */
    updateState(id: number, update: RemotePlaybackState): Promise<void>;
    /** Terminates a playback from the receiver side. */
    terminate(id: number, reason?: TerminationEventReason): Promise<void>;
}
export {};
