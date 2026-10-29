import { type CBORValue } from "./codec/cbor.ts";
import { SchemaError, type Codec, type Infer } from "./schema.ts";
export declare const AgentCapability: {
    readonly receiveAudio: 1;
    readonly receiveVideo: 2;
    readonly receivePresentation: 3;
    readonly controlPresentation: 4;
    readonly receiveRemotePlayback: 5;
    readonly controlRemotePlayback: 6;
    readonly receiveStreaming: 7;
    readonly sendStreaming: 8;
};
export declare const UrlAvailability: {
    readonly available: 0;
    readonly unavailable: 1;
    readonly invalid: 10;
};
export type UrlAvailability = (typeof UrlAvailability)[keyof typeof UrlAvailability];
export declare const Result: {
    readonly success: 1;
    readonly invalidUrl: 10;
    readonly invalidPresentationId: 11;
    readonly timeout: 100;
    readonly transientError: 101;
    readonly permanentError: 102;
    readonly terminating: 103;
    readonly unknownError: 199;
};
export type Result = (typeof Result)[keyof typeof Result];
export declare const TerminationSource: {
    readonly controller: 1;
    readonly receiver: 2;
    readonly unknown: 255;
};
export type TerminationSource = (typeof TerminationSource)[keyof typeof TerminationSource];
export declare const TerminationReason: {
    readonly applicationRequest: 1;
    readonly userRequest: 2;
    readonly receiverReplacedPresentation: 20;
    readonly receiverIdleTooLong: 30;
    readonly receiverAttemptedToNavigate: 31;
    readonly receiverPoweringDown: 100;
    readonly receiverError: 101;
    readonly unknown: 255;
};
export type TerminationReason = (typeof TerminationReason)[keyof typeof TerminationReason];
export declare const ConnectionCloseReason: {
    readonly closeMethodCalled: 1;
    readonly connectionObjectDiscarded: 10;
    readonly unrecoverableErrorWhileSendingOrReceivingMessage: 100;
};
export type ConnectionCloseReason = (typeof ConnectionCloseReason)[keyof typeof ConnectionCloseReason];
export declare const RemotePlaybackTerminationRequestReason: {
    readonly userTerminatedViaController: 11;
    readonly unknown: 255;
};
export declare const RemotePlaybackTerminationEventReason: {
    readonly receiverCalledTerminate: 1;
    readonly userTerminatedViaReceiver: 2;
    readonly receiverIdleTooLong: 30;
    readonly receiverPoweringDown: 100;
    readonly receiverCrashed: 101;
    readonly unknown: 255;
};
export declare const TextTrackMode: {
    readonly disabled: 1;
    readonly showing: 2;
    readonly hidden: 3;
};
export declare const agentInfo: Codec<{
    displayName: string;
    capabilities: number[];
    stateToken: string;
    locales: string[];
    modelName?: string | undefined;
}> & {
    readonly fields: {
        readonly displayName: import("./schema.ts").Field<string, false>;
        readonly modelName: import("./schema.ts").Field<string, true>;
        readonly capabilities: import("./schema.ts").Field<number[], false>;
        readonly stateToken: import("./schema.ts").Field<string, false>;
        readonly locales: import("./schema.ts").Field<string[], false>;
    };
};
export type AgentInfo = Infer<typeof agentInfo>;
export declare const remotePlaybackSource: Codec<{
    url: string;
    extendedMimeType: string;
}> & {
    readonly fields: {
        readonly url: import("./schema.ts").Field<string, false>;
        readonly extendedMimeType: import("./schema.ts").Field<string, false>;
    };
};
export type RemotePlaybackSource = Infer<typeof remotePlaybackSource>;
export declare const remotePlaybackControls: Codec<{
    source?: {
        url: string;
        extendedMimeType: string;
    } | undefined;
    preload?: 0 | 2 | 1 | undefined;
    loop?: boolean | undefined;
    paused?: boolean | undefined;
    muted?: boolean | undefined;
    volume?: number | undefined;
    seek?: number | undefined;
    fastSeek?: number | undefined;
    playbackRate?: number | undefined;
    poster?: string | undefined;
    enabledAudioTrackIds?: string[] | undefined;
    selectedVideoTrackId?: string | undefined;
    addedTextTracks?: {
        kind: 2 | 1 | 5 | 3 | 4;
        label?: string | undefined;
        language?: string | undefined;
    }[] | undefined;
    changedTextTracks?: {
        id: string;
        mode: 2 | 1 | 3;
        addedCues?: {
            id: string;
            range: [start: number, end: number];
            text: string;
        }[] | undefined;
        removedCueIds?: string[] | undefined;
    }[] | undefined;
}> & {
    readonly fields: {
        readonly source: import("./schema.ts").Field<{
            url: string;
            extendedMimeType: string;
        }, true>;
        readonly preload: import("./schema.ts").Field<0 | 2 | 1, true>;
        readonly loop: import("./schema.ts").Field<boolean, true>;
        readonly paused: import("./schema.ts").Field<boolean, true>;
        readonly muted: import("./schema.ts").Field<boolean, true>;
        readonly volume: import("./schema.ts").Field<number, true>;
        readonly seek: import("./schema.ts").Field<number, true>;
        readonly fastSeek: import("./schema.ts").Field<number, true>;
        readonly playbackRate: import("./schema.ts").Field<number, true>;
        readonly poster: import("./schema.ts").Field<string, true>;
        readonly enabledAudioTrackIds: import("./schema.ts").Field<string[], true>;
        readonly selectedVideoTrackId: import("./schema.ts").Field<string, true>;
        readonly addedTextTracks: import("./schema.ts").Field<{
            kind: 2 | 1 | 5 | 3 | 4;
            label?: string | undefined;
            language?: string | undefined;
        }[], true>;
        readonly changedTextTracks: import("./schema.ts").Field<{
            id: string;
            mode: 2 | 1 | 3;
            addedCues?: {
                id: string;
                range: [start: number, end: number];
                text: string;
            }[] | undefined;
            removedCueIds?: string[] | undefined;
        }[], true>;
    };
};
export type RemotePlaybackControls = Infer<typeof remotePlaybackControls>;
export declare const remotePlaybackState: Codec<{
    supports?: {
        rate: boolean;
        preload: boolean;
        poster: boolean;
        addedTextTrack: boolean;
        addedCues: boolean;
    } | undefined;
    source?: {
        url: string;
        extendedMimeType: string;
    } | undefined;
    loading?: 0 | 2 | 1 | 3 | undefined;
    loaded?: 0 | 2 | 1 | 3 | 4 | undefined;
    error?: [code: 2 | 1 | 5 | 3 | 4, message: string] | undefined;
    epoch?: number | null | undefined;
    duration?: number | null | undefined;
    bufferedTimeRanges?: [start: number, end: number][] | undefined;
    seekableTimeRanges?: [start: number, end: number][] | undefined;
    playedTimeRanges?: [start: number, end: number][] | undefined;
    position?: number | undefined;
    playbackRate?: number | undefined;
    paused?: boolean | undefined;
    seeking?: boolean | undefined;
    stalled?: boolean | undefined;
    ended?: boolean | undefined;
    volume?: number | undefined;
    muted?: boolean | undefined;
    resolution?: {
        height: number;
        width: number;
    } | null | undefined;
    audioTracks?: {
        enabled: boolean;
        id: string;
        label: string;
        language: string;
    }[] | undefined;
    videoTracks?: {
        selected: boolean;
        id: string;
        label: string;
        language: string;
    }[] | undefined;
    textTracks?: {
        mode: 2 | 1 | 3;
        id: string;
        label: string;
        language: string;
    }[] | undefined;
}> & {
    readonly fields: {
        readonly supports: import("./schema.ts").Field<{
            rate: boolean;
            preload: boolean;
            poster: boolean;
            addedTextTrack: boolean;
            addedCues: boolean;
        }, true>;
        readonly source: import("./schema.ts").Field<{
            url: string;
            extendedMimeType: string;
        }, true>;
        readonly loading: import("./schema.ts").Field<0 | 2 | 1 | 3, true>;
        readonly loaded: import("./schema.ts").Field<0 | 2 | 1 | 3 | 4, true>;
        readonly error: import("./schema.ts").Field<[code: 2 | 1 | 5 | 3 | 4, message: string], true>;
        readonly epoch: import("./schema.ts").Field<number | null, true>;
        readonly duration: import("./schema.ts").Field<number | null, true>;
        readonly bufferedTimeRanges: import("./schema.ts").Field<[start: number, end: number][], true>;
        readonly seekableTimeRanges: import("./schema.ts").Field<[start: number, end: number][], true>;
        readonly playedTimeRanges: import("./schema.ts").Field<[start: number, end: number][], true>;
        readonly position: import("./schema.ts").Field<number, true>;
        readonly playbackRate: import("./schema.ts").Field<number, true>;
        readonly paused: import("./schema.ts").Field<boolean, true>;
        readonly seeking: import("./schema.ts").Field<boolean, true>;
        readonly stalled: import("./schema.ts").Field<boolean, true>;
        readonly ended: import("./schema.ts").Field<boolean, true>;
        readonly volume: import("./schema.ts").Field<number, true>;
        readonly muted: import("./schema.ts").Field<boolean, true>;
        readonly resolution: import("./schema.ts").Field<{
            height: number;
            width: number;
        } | null, true>;
        readonly audioTracks: import("./schema.ts").Field<{
            enabled: boolean;
            id: string;
            label: string;
            language: string;
        }[], true>;
        readonly videoTracks: import("./schema.ts").Field<{
            selected: boolean;
            id: string;
            label: string;
            language: string;
        }[], true>;
        readonly textTracks: import("./schema.ts").Field<{
            mode: 2 | 1 | 3;
            id: string;
            label: string;
            language: string;
        }[], true>;
    };
};
export type RemotePlaybackState = Infer<typeof remotePlaybackState>;
interface MessageDef<C extends Codec<unknown>> {
    readonly typeKey: number;
    readonly codec: C;
    /** False for messages that are recognized but whose protocol is out of scope. */
    readonly supported: boolean;
}
export declare const messages: {
    readonly "agent-info-request": MessageDef<Codec<{
        requestId: number;
    }> & {
        readonly fields: {
            readonly requestId: import("./schema.ts").Field<number, false>;
        };
    }>;
    readonly "agent-info-response": MessageDef<Codec<{
        requestId: number;
        agentInfo: {
            displayName: string;
            capabilities: number[];
            stateToken: string;
            locales: string[];
            modelName?: string | undefined;
        };
    }> & {
        readonly fields: {
            readonly requestId: import("./schema.ts").Field<number, false>;
            readonly agentInfo: import("./schema.ts").Field<{
                displayName: string;
                capabilities: number[];
                stateToken: string;
                locales: string[];
                modelName?: string | undefined;
            }, false>;
        };
    }>;
    readonly "agent-info-event": MessageDef<Codec<{
        agentInfo: {
            displayName: string;
            capabilities: number[];
            stateToken: string;
            locales: string[];
            modelName?: string | undefined;
        };
    }> & {
        readonly fields: {
            readonly agentInfo: import("./schema.ts").Field<{
                displayName: string;
                capabilities: number[];
                stateToken: string;
                locales: string[];
                modelName?: string | undefined;
            }, false>;
        };
    }>;
    readonly "agent-status-request": MessageDef<Codec<{
        requestId: number;
        status?: {
            status: string;
        } | undefined;
    }> & {
        readonly fields: {
            readonly requestId: import("./schema.ts").Field<number, false>;
            readonly status: import("./schema.ts").Field<{
                status: string;
            }, true>;
        };
    }>;
    readonly "agent-status-response": MessageDef<Codec<{
        requestId: number;
        status?: {
            status: string;
        } | undefined;
    }> & {
        readonly fields: {
            readonly requestId: import("./schema.ts").Field<number, false>;
            readonly status: import("./schema.ts").Field<{
                status: string;
            }, true>;
        };
    }>;
    readonly "presentation-url-availability-request": MessageDef<Codec<{
        requestId: number;
        urls: string[];
        watchDuration: number;
        watchId: number;
    }> & {
        readonly fields: {
            readonly requestId: import("./schema.ts").Field<number, false>;
            readonly urls: import("./schema.ts").Field<string[], false>;
            readonly watchDuration: import("./schema.ts").Field<number, false>;
            readonly watchId: import("./schema.ts").Field<number, false>;
        };
    }>;
    readonly "presentation-url-availability-response": MessageDef<Codec<{
        requestId: number;
        urlAvailabilities: (0 | 1 | 10)[];
    }> & {
        readonly fields: {
            readonly requestId: import("./schema.ts").Field<number, false>;
            readonly urlAvailabilities: import("./schema.ts").Field<(0 | 1 | 10)[], false>;
        };
    }>;
    readonly "presentation-url-availability-event": MessageDef<Codec<{
        watchId: number;
        urlAvailabilities: (0 | 1 | 10)[];
    }> & {
        readonly fields: {
            readonly watchId: import("./schema.ts").Field<number, false>;
            readonly urlAvailabilities: import("./schema.ts").Field<(0 | 1 | 10)[], false>;
        };
    }>;
    readonly "presentation-start-request": MessageDef<Codec<{
        requestId: number;
        presentationId: string;
        url: string;
        headers: [key: string, value: string][];
    }> & {
        readonly fields: {
            readonly requestId: import("./schema.ts").Field<number, false>;
            readonly presentationId: import("./schema.ts").Field<string, false>;
            readonly url: import("./schema.ts").Field<string, false>;
            readonly headers: import("./schema.ts").Field<[key: string, value: string][], false>;
        };
    }>;
    readonly "presentation-start-response": MessageDef<Codec<{
        requestId: number;
        result: 1 | 10 | 11 | 100 | 101 | 102 | 103 | 199;
        connectionId: number;
        httpResponseCode?: number | undefined;
    }> & {
        readonly fields: {
            readonly requestId: import("./schema.ts").Field<number, false>;
            readonly result: import("./schema.ts").Field<1 | 10 | 11 | 100 | 101 | 102 | 103 | 199, false>;
            readonly connectionId: import("./schema.ts").Field<number, false>;
            readonly httpResponseCode: import("./schema.ts").Field<number, true>;
        };
    }>;
    readonly "presentation-termination-request": MessageDef<Codec<{
        requestId: number;
        presentationId: string;
        reason: 2 | 1 | 255 | 31 | 20 | 100 | 101 | 30;
    }> & {
        readonly fields: {
            readonly requestId: import("./schema.ts").Field<number, false>;
            readonly presentationId: import("./schema.ts").Field<string, false>;
            readonly reason: import("./schema.ts").Field<2 | 1 | 255 | 31 | 20 | 100 | 101 | 30, false>;
        };
    }>;
    readonly "presentation-termination-response": MessageDef<Codec<{
        requestId: number;
        result: 1 | 10 | 11 | 100 | 101 | 102 | 103 | 199;
    }> & {
        readonly fields: {
            readonly requestId: import("./schema.ts").Field<number, false>;
            readonly result: import("./schema.ts").Field<1 | 10 | 11 | 100 | 101 | 102 | 103 | 199, false>;
        };
    }>;
    readonly "presentation-termination-event": MessageDef<Codec<{
        presentationId: string;
        source: 2 | 1 | 255;
        reason: 2 | 1 | 255 | 31 | 20 | 100 | 101 | 30;
    }> & {
        readonly fields: {
            readonly presentationId: import("./schema.ts").Field<string, false>;
            readonly source: import("./schema.ts").Field<2 | 1 | 255, false>;
            readonly reason: import("./schema.ts").Field<2 | 1 | 255 | 31 | 20 | 100 | 101 | 30, false>;
        };
    }>;
    readonly "presentation-connection-open-request": MessageDef<Codec<{
        requestId: number;
        presentationId: string;
        url: string;
    }> & {
        readonly fields: {
            readonly requestId: import("./schema.ts").Field<number, false>;
            readonly presentationId: import("./schema.ts").Field<string, false>;
            readonly url: import("./schema.ts").Field<string, false>;
        };
    }>;
    readonly "presentation-connection-open-response": MessageDef<Codec<{
        requestId: number;
        result: 1 | 10 | 11 | 100 | 101 | 102 | 103 | 199;
        connectionId: number;
        connectionCount: number;
    }> & {
        readonly fields: {
            readonly requestId: import("./schema.ts").Field<number, false>;
            readonly result: import("./schema.ts").Field<1 | 10 | 11 | 100 | 101 | 102 | 103 | 199, false>;
            readonly connectionId: import("./schema.ts").Field<number, false>;
            readonly connectionCount: import("./schema.ts").Field<number, false>;
        };
    }>;
    readonly "presentation-connection-close-event": MessageDef<Codec<{
        connectionId: number;
        reason: 1 | 10 | 100;
        connectionCount: number;
        errorMessage?: string | undefined;
    }> & {
        readonly fields: {
            readonly connectionId: import("./schema.ts").Field<number, false>;
            readonly reason: import("./schema.ts").Field<1 | 10 | 100, false>;
            readonly errorMessage: import("./schema.ts").Field<string, true>;
            readonly connectionCount: import("./schema.ts").Field<number, false>;
        };
    }>;
    readonly "presentation-change-event": MessageDef<Codec<{
        presentationId: string;
        connectionCount: number;
    }> & {
        readonly fields: {
            readonly presentationId: import("./schema.ts").Field<string, false>;
            readonly connectionCount: import("./schema.ts").Field<number, false>;
        };
    }>;
    readonly "presentation-connection-message": MessageDef<Codec<{
        connectionId: number;
        message: string | Uint8Array<ArrayBufferLike>;
    }> & {
        readonly fields: {
            readonly connectionId: import("./schema.ts").Field<number, false>;
            readonly message: import("./schema.ts").Field<string | Uint8Array<ArrayBufferLike>, false>;
        };
    }>;
    readonly "remote-playback-availability-request": MessageDef<Codec<{
        requestId: number;
        sources: {
            url: string;
            extendedMimeType: string;
        }[];
        watchDuration: number;
        watchId: number;
    }> & {
        readonly fields: {
            readonly requestId: import("./schema.ts").Field<number, false>;
            readonly sources: import("./schema.ts").Field<{
                url: string;
                extendedMimeType: string;
            }[], false>;
            readonly watchDuration: import("./schema.ts").Field<number, false>;
            readonly watchId: import("./schema.ts").Field<number, false>;
        };
    }>;
    readonly "remote-playback-availability-response": MessageDef<Codec<{
        requestId: number;
        urlAvailabilities: (0 | 1 | 10)[];
    }> & {
        readonly fields: {
            readonly requestId: import("./schema.ts").Field<number, false>;
            readonly urlAvailabilities: import("./schema.ts").Field<(0 | 1 | 10)[], false>;
        };
    }>;
    readonly "remote-playback-availability-event": MessageDef<Codec<{
        watchId: number;
        urlAvailabilities: (0 | 1 | 10)[];
    }> & {
        readonly fields: {
            readonly watchId: import("./schema.ts").Field<number, false>;
            readonly urlAvailabilities: import("./schema.ts").Field<(0 | 1 | 10)[], false>;
        };
    }>;
    readonly "remote-playback-start-request": MessageDef<Codec<{
        requestId: number;
        remotePlaybackId: number;
        sources?: {
            url: string;
            extendedMimeType: string;
        }[] | undefined;
        textTrackUrls?: string[] | undefined;
        headers?: [key: string, value: string][] | undefined;
        controls?: {
            source?: {
                url: string;
                extendedMimeType: string;
            } | undefined;
            preload?: 0 | 2 | 1 | undefined;
            loop?: boolean | undefined;
            paused?: boolean | undefined;
            muted?: boolean | undefined;
            volume?: number | undefined;
            seek?: number | undefined;
            fastSeek?: number | undefined;
            playbackRate?: number | undefined;
            poster?: string | undefined;
            enabledAudioTrackIds?: string[] | undefined;
            selectedVideoTrackId?: string | undefined;
            addedTextTracks?: {
                kind: 2 | 1 | 5 | 3 | 4;
                label?: string | undefined;
                language?: string | undefined;
            }[] | undefined;
            changedTextTracks?: {
                id: string;
                mode: 2 | 1 | 3;
                addedCues?: {
                    id: string;
                    range: [start: number, end: number];
                    text: string;
                }[] | undefined;
                removedCueIds?: string[] | undefined;
            }[] | undefined;
        } | undefined;
        remoting?: CBORValue;
    }> & {
        readonly fields: {
            readonly requestId: import("./schema.ts").Field<number, false>;
            readonly remotePlaybackId: import("./schema.ts").Field<number, false>;
            readonly sources: import("./schema.ts").Field<{
                url: string;
                extendedMimeType: string;
            }[], true>;
            readonly textTrackUrls: import("./schema.ts").Field<string[], true>;
            readonly headers: import("./schema.ts").Field<[key: string, value: string][], true>;
            readonly controls: import("./schema.ts").Field<{
                source?: {
                    url: string;
                    extendedMimeType: string;
                } | undefined;
                preload?: 0 | 2 | 1 | undefined;
                loop?: boolean | undefined;
                paused?: boolean | undefined;
                muted?: boolean | undefined;
                volume?: number | undefined;
                seek?: number | undefined;
                fastSeek?: number | undefined;
                playbackRate?: number | undefined;
                poster?: string | undefined;
                enabledAudioTrackIds?: string[] | undefined;
                selectedVideoTrackId?: string | undefined;
                addedTextTracks?: {
                    kind: 2 | 1 | 5 | 3 | 4;
                    label?: string | undefined;
                    language?: string | undefined;
                }[] | undefined;
                changedTextTracks?: {
                    id: string;
                    mode: 2 | 1 | 3;
                    addedCues?: {
                        id: string;
                        range: [start: number, end: number];
                        text: string;
                    }[] | undefined;
                    removedCueIds?: string[] | undefined;
                }[] | undefined;
            }, true>;
            readonly remoting: import("./schema.ts").Field<CBORValue, true>;
        };
    }>;
    readonly "remote-playback-start-response": MessageDef<Codec<{
        requestId: number;
        state?: {
            supports?: {
                rate: boolean;
                preload: boolean;
                poster: boolean;
                addedTextTrack: boolean;
                addedCues: boolean;
            } | undefined;
            source?: {
                url: string;
                extendedMimeType: string;
            } | undefined;
            loading?: 0 | 2 | 1 | 3 | undefined;
            loaded?: 0 | 2 | 1 | 3 | 4 | undefined;
            error?: [code: 2 | 1 | 5 | 3 | 4, message: string] | undefined;
            epoch?: number | null | undefined;
            duration?: number | null | undefined;
            bufferedTimeRanges?: [start: number, end: number][] | undefined;
            seekableTimeRanges?: [start: number, end: number][] | undefined;
            playedTimeRanges?: [start: number, end: number][] | undefined;
            position?: number | undefined;
            playbackRate?: number | undefined;
            paused?: boolean | undefined;
            seeking?: boolean | undefined;
            stalled?: boolean | undefined;
            ended?: boolean | undefined;
            volume?: number | undefined;
            muted?: boolean | undefined;
            resolution?: {
                height: number;
                width: number;
            } | null | undefined;
            audioTracks?: {
                enabled: boolean;
                id: string;
                label: string;
                language: string;
            }[] | undefined;
            videoTracks?: {
                selected: boolean;
                id: string;
                label: string;
                language: string;
            }[] | undefined;
            textTracks?: {
                mode: 2 | 1 | 3;
                id: string;
                label: string;
                language: string;
            }[] | undefined;
        } | undefined;
        remoting?: CBORValue;
    }> & {
        readonly fields: {
            readonly requestId: import("./schema.ts").Field<number, false>;
            readonly state: import("./schema.ts").Field<{
                supports?: {
                    rate: boolean;
                    preload: boolean;
                    poster: boolean;
                    addedTextTrack: boolean;
                    addedCues: boolean;
                } | undefined;
                source?: {
                    url: string;
                    extendedMimeType: string;
                } | undefined;
                loading?: 0 | 2 | 1 | 3 | undefined;
                loaded?: 0 | 2 | 1 | 3 | 4 | undefined;
                error?: [code: 2 | 1 | 5 | 3 | 4, message: string] | undefined;
                epoch?: number | null | undefined;
                duration?: number | null | undefined;
                bufferedTimeRanges?: [start: number, end: number][] | undefined;
                seekableTimeRanges?: [start: number, end: number][] | undefined;
                playedTimeRanges?: [start: number, end: number][] | undefined;
                position?: number | undefined;
                playbackRate?: number | undefined;
                paused?: boolean | undefined;
                seeking?: boolean | undefined;
                stalled?: boolean | undefined;
                ended?: boolean | undefined;
                volume?: number | undefined;
                muted?: boolean | undefined;
                resolution?: {
                    height: number;
                    width: number;
                } | null | undefined;
                audioTracks?: {
                    enabled: boolean;
                    id: string;
                    label: string;
                    language: string;
                }[] | undefined;
                videoTracks?: {
                    selected: boolean;
                    id: string;
                    label: string;
                    language: string;
                }[] | undefined;
                textTracks?: {
                    mode: 2 | 1 | 3;
                    id: string;
                    label: string;
                    language: string;
                }[] | undefined;
            }, true>;
            readonly remoting: import("./schema.ts").Field<CBORValue, true>;
        };
    }>;
    readonly "remote-playback-termination-request": MessageDef<Codec<{
        requestId: number;
        remotePlaybackId: number;
        reason: 255 | 11;
    }> & {
        readonly fields: {
            readonly requestId: import("./schema.ts").Field<number, false>;
            readonly remotePlaybackId: import("./schema.ts").Field<number, false>;
            readonly reason: import("./schema.ts").Field<255 | 11, false>;
        };
    }>;
    readonly "remote-playback-termination-response": MessageDef<Codec<{
        requestId: number;
        result: 1 | 10 | 11 | 100 | 101 | 102 | 103 | 199;
    }> & {
        readonly fields: {
            readonly requestId: import("./schema.ts").Field<number, false>;
            readonly result: import("./schema.ts").Field<1 | 10 | 11 | 100 | 101 | 102 | 103 | 199, false>;
        };
    }>;
    readonly "remote-playback-termination-event": MessageDef<Codec<{
        remotePlaybackId: number;
        reason: 2 | 1 | 255 | 100 | 101 | 30;
    }> & {
        readonly fields: {
            readonly remotePlaybackId: import("./schema.ts").Field<number, false>;
            readonly reason: import("./schema.ts").Field<2 | 1 | 255 | 100 | 101 | 30, false>;
        };
    }>;
    readonly "remote-playback-modify-request": MessageDef<Codec<{
        requestId: number;
        remotePlaybackId: number;
        controls: {
            source?: {
                url: string;
                extendedMimeType: string;
            } | undefined;
            preload?: 0 | 2 | 1 | undefined;
            loop?: boolean | undefined;
            paused?: boolean | undefined;
            muted?: boolean | undefined;
            volume?: number | undefined;
            seek?: number | undefined;
            fastSeek?: number | undefined;
            playbackRate?: number | undefined;
            poster?: string | undefined;
            enabledAudioTrackIds?: string[] | undefined;
            selectedVideoTrackId?: string | undefined;
            addedTextTracks?: {
                kind: 2 | 1 | 5 | 3 | 4;
                label?: string | undefined;
                language?: string | undefined;
            }[] | undefined;
            changedTextTracks?: {
                id: string;
                mode: 2 | 1 | 3;
                addedCues?: {
                    id: string;
                    range: [start: number, end: number];
                    text: string;
                }[] | undefined;
                removedCueIds?: string[] | undefined;
            }[] | undefined;
        };
    }> & {
        readonly fields: {
            readonly requestId: import("./schema.ts").Field<number, false>;
            readonly remotePlaybackId: import("./schema.ts").Field<number, false>;
            readonly controls: import("./schema.ts").Field<{
                source?: {
                    url: string;
                    extendedMimeType: string;
                } | undefined;
                preload?: 0 | 2 | 1 | undefined;
                loop?: boolean | undefined;
                paused?: boolean | undefined;
                muted?: boolean | undefined;
                volume?: number | undefined;
                seek?: number | undefined;
                fastSeek?: number | undefined;
                playbackRate?: number | undefined;
                poster?: string | undefined;
                enabledAudioTrackIds?: string[] | undefined;
                selectedVideoTrackId?: string | undefined;
                addedTextTracks?: {
                    kind: 2 | 1 | 5 | 3 | 4;
                    label?: string | undefined;
                    language?: string | undefined;
                }[] | undefined;
                changedTextTracks?: {
                    id: string;
                    mode: 2 | 1 | 3;
                    addedCues?: {
                        id: string;
                        range: [start: number, end: number];
                        text: string;
                    }[] | undefined;
                    removedCueIds?: string[] | undefined;
                }[] | undefined;
            }, false>;
        };
    }>;
    readonly "remote-playback-modify-response": MessageDef<Codec<{
        requestId: number;
        result: 1 | 10 | 11 | 100 | 101 | 102 | 103 | 199;
        state?: {
            supports?: {
                rate: boolean;
                preload: boolean;
                poster: boolean;
                addedTextTrack: boolean;
                addedCues: boolean;
            } | undefined;
            source?: {
                url: string;
                extendedMimeType: string;
            } | undefined;
            loading?: 0 | 2 | 1 | 3 | undefined;
            loaded?: 0 | 2 | 1 | 3 | 4 | undefined;
            error?: [code: 2 | 1 | 5 | 3 | 4, message: string] | undefined;
            epoch?: number | null | undefined;
            duration?: number | null | undefined;
            bufferedTimeRanges?: [start: number, end: number][] | undefined;
            seekableTimeRanges?: [start: number, end: number][] | undefined;
            playedTimeRanges?: [start: number, end: number][] | undefined;
            position?: number | undefined;
            playbackRate?: number | undefined;
            paused?: boolean | undefined;
            seeking?: boolean | undefined;
            stalled?: boolean | undefined;
            ended?: boolean | undefined;
            volume?: number | undefined;
            muted?: boolean | undefined;
            resolution?: {
                height: number;
                width: number;
            } | null | undefined;
            audioTracks?: {
                enabled: boolean;
                id: string;
                label: string;
                language: string;
            }[] | undefined;
            videoTracks?: {
                selected: boolean;
                id: string;
                label: string;
                language: string;
            }[] | undefined;
            textTracks?: {
                mode: 2 | 1 | 3;
                id: string;
                label: string;
                language: string;
            }[] | undefined;
        } | undefined;
    }> & {
        readonly fields: {
            readonly requestId: import("./schema.ts").Field<number, false>;
            readonly result: import("./schema.ts").Field<1 | 10 | 11 | 100 | 101 | 102 | 103 | 199, false>;
            readonly state: import("./schema.ts").Field<{
                supports?: {
                    rate: boolean;
                    preload: boolean;
                    poster: boolean;
                    addedTextTrack: boolean;
                    addedCues: boolean;
                } | undefined;
                source?: {
                    url: string;
                    extendedMimeType: string;
                } | undefined;
                loading?: 0 | 2 | 1 | 3 | undefined;
                loaded?: 0 | 2 | 1 | 3 | 4 | undefined;
                error?: [code: 2 | 1 | 5 | 3 | 4, message: string] | undefined;
                epoch?: number | null | undefined;
                duration?: number | null | undefined;
                bufferedTimeRanges?: [start: number, end: number][] | undefined;
                seekableTimeRanges?: [start: number, end: number][] | undefined;
                playedTimeRanges?: [start: number, end: number][] | undefined;
                position?: number | undefined;
                playbackRate?: number | undefined;
                paused?: boolean | undefined;
                seeking?: boolean | undefined;
                stalled?: boolean | undefined;
                ended?: boolean | undefined;
                volume?: number | undefined;
                muted?: boolean | undefined;
                resolution?: {
                    height: number;
                    width: number;
                } | null | undefined;
                audioTracks?: {
                    enabled: boolean;
                    id: string;
                    label: string;
                    language: string;
                }[] | undefined;
                videoTracks?: {
                    selected: boolean;
                    id: string;
                    label: string;
                    language: string;
                }[] | undefined;
                textTracks?: {
                    mode: 2 | 1 | 3;
                    id: string;
                    label: string;
                    language: string;
                }[] | undefined;
            }, true>;
        };
    }>;
    readonly "remote-playback-state-event": MessageDef<Codec<{
        remotePlaybackId: number;
        state: {
            supports?: {
                rate: boolean;
                preload: boolean;
                poster: boolean;
                addedTextTrack: boolean;
                addedCues: boolean;
            } | undefined;
            source?: {
                url: string;
                extendedMimeType: string;
            } | undefined;
            loading?: 0 | 2 | 1 | 3 | undefined;
            loaded?: 0 | 2 | 1 | 3 | 4 | undefined;
            error?: [code: 2 | 1 | 5 | 3 | 4, message: string] | undefined;
            epoch?: number | null | undefined;
            duration?: number | null | undefined;
            bufferedTimeRanges?: [start: number, end: number][] | undefined;
            seekableTimeRanges?: [start: number, end: number][] | undefined;
            playedTimeRanges?: [start: number, end: number][] | undefined;
            position?: number | undefined;
            playbackRate?: number | undefined;
            paused?: boolean | undefined;
            seeking?: boolean | undefined;
            stalled?: boolean | undefined;
            ended?: boolean | undefined;
            volume?: number | undefined;
            muted?: boolean | undefined;
            resolution?: {
                height: number;
                width: number;
            } | null | undefined;
            audioTracks?: {
                enabled: boolean;
                id: string;
                label: string;
                language: string;
            }[] | undefined;
            videoTracks?: {
                selected: boolean;
                id: string;
                label: string;
                language: string;
            }[] | undefined;
            textTracks?: {
                mode: 2 | 1 | 3;
                id: string;
                label: string;
                language: string;
            }[] | undefined;
        };
    }> & {
        readonly fields: {
            readonly remotePlaybackId: import("./schema.ts").Field<number, false>;
            readonly state: import("./schema.ts").Field<{
                supports?: {
                    rate: boolean;
                    preload: boolean;
                    poster: boolean;
                    addedTextTrack: boolean;
                    addedCues: boolean;
                } | undefined;
                source?: {
                    url: string;
                    extendedMimeType: string;
                } | undefined;
                loading?: 0 | 2 | 1 | 3 | undefined;
                loaded?: 0 | 2 | 1 | 3 | 4 | undefined;
                error?: [code: 2 | 1 | 5 | 3 | 4, message: string] | undefined;
                epoch?: number | null | undefined;
                duration?: number | null | undefined;
                bufferedTimeRanges?: [start: number, end: number][] | undefined;
                seekableTimeRanges?: [start: number, end: number][] | undefined;
                playedTimeRanges?: [start: number, end: number][] | undefined;
                position?: number | undefined;
                playbackRate?: number | undefined;
                paused?: boolean | undefined;
                seeking?: boolean | undefined;
                stalled?: boolean | undefined;
                ended?: boolean | undefined;
                volume?: number | undefined;
                muted?: boolean | undefined;
                resolution?: {
                    height: number;
                    width: number;
                } | null | undefined;
                audioTracks?: {
                    enabled: boolean;
                    id: string;
                    label: string;
                    language: string;
                }[] | undefined;
                videoTracks?: {
                    selected: boolean;
                    id: string;
                    label: string;
                    language: string;
                }[] | undefined;
                textTracks?: {
                    mode: 2 | 1 | 3;
                    id: string;
                    label: string;
                    language: string;
                }[] | undefined;
            }, false>;
        };
    }>;
    readonly "audio-frame": MessageDef<Codec<CBORValue>>;
    readonly "video-frame": MessageDef<Codec<CBORValue>>;
    readonly "data-frame": MessageDef<Codec<CBORValue>>;
    readonly "streaming-capabilities-request": MessageDef<Codec<CBORValue>>;
    readonly "streaming-capabilities-response": MessageDef<Codec<CBORValue>>;
    readonly "streaming-session-start-request": MessageDef<Codec<CBORValue>>;
    readonly "streaming-session-start-response": MessageDef<Codec<CBORValue>>;
    readonly "streaming-session-modify-request": MessageDef<Codec<CBORValue>>;
    readonly "streaming-session-modify-response": MessageDef<Codec<CBORValue>>;
    readonly "streaming-session-terminate-request": MessageDef<Codec<CBORValue>>;
    readonly "streaming-session-terminate-response": MessageDef<Codec<CBORValue>>;
    readonly "streaming-session-terminate-event": MessageDef<Codec<CBORValue>>;
    readonly "streaming-session-sender-stats-event": MessageDef<Codec<CBORValue>>;
    readonly "streaming-session-receiver-stats-event": MessageDef<Codec<CBORValue>>;
};
export type MessageName = keyof typeof messages;
export type MessageBody<N extends MessageName> = Infer<(typeof messages)[N]["codec"]>;
/** Names of messages that carry a request-id at key 0. */
export type RequestName = {
    [N in MessageName]: MessageBody<N> extends {
        requestId: number;
    } ? N : never;
}[MessageName];
export declare function messageName(typeKey: number): MessageName | undefined;
export declare function typeKeyOf(name: MessageName): number;
export declare function isSupported(name: MessageName): boolean;
/** A decoded message. Body is typed by name. */
export type Message = {
    [N in MessageName]: {
        name: N;
        typeKey: number;
        body: MessageBody<N>;
    };
}[MessageName];
/** Encodes a message body to CBOR (without the type key), validating it. */
export declare function encodeBody<N extends MessageName>(name: N, body: MessageBody<N>): Uint8Array;
/** Decodes a CBOR body for a known type key. Throws SchemaError or RangeError. */
export declare function decodeBody(typeKey: number, body: Uint8Array): Message;
/** One OSP message on the wire: its type key and its CBOR body. */
export interface Frame {
    typeKey: number;
    /** Exactly one CBOR data item. */
    body: Uint8Array;
}
/** type key varint followed by the CBOR body (application.bs "Message delivery using CBOR"). */
export declare function encodeFrame(frame: Frame): Uint8Array;
/** Decodes exactly one framed message (a data channel message). */
export declare function decodeFrame(buf: Uint8Array): Frame;
/**
 * Splits a byte stream (a QUIC/WebTransport stream) into framed messages.
 * Push chunks as they arrive; complete frames come out. A CBOR item's length
 * is only known by parsing it, so the item is parsed here and again by
 * decodeBody; messages are small.
 */
export declare class FrameReader {
    #private;
    push(chunk: Uint8Array): Frame[];
    /** True if bytes of an incomplete message are buffered. */
    get pending(): boolean;
}
export { SchemaError };
