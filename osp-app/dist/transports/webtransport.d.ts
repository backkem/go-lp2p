import { type Frame } from "../messages.ts";
import { type CloseInfo, type OspTransport, type SendOptions, type TransportSink } from "../transport.ts";
export interface StreamWriterLike {
    write(chunk: Uint8Array): Promise<void>;
    close(): Promise<void>;
    abort(reason?: unknown): Promise<void>;
    releaseLock(): void;
}
export interface WritableLike {
    getWriter(): StreamWriterLike;
}
export interface StreamReaderLike<T> {
    read(): Promise<{
        done: true;
        value?: T | undefined;
    } | {
        done: false;
        value: T;
    }>;
    cancel(reason?: unknown): Promise<void>;
    releaseLock(): void;
}
export interface ReadableLike<T> {
    getReader(): StreamReaderLike<T>;
}
/** The subset of WebTransport / LP2PQuicTransport this transport uses. */
export interface WebTransportLike {
    readonly ready: Promise<unknown>;
    readonly closed: Promise<{
        closeCode?: number;
        reason?: string;
    } | undefined>;
    close(closeInfo?: {
        closeCode?: number;
        reason?: string;
    }): void;
    createUnidirectionalStream(): Promise<WritableLike>;
    readonly incomingUnidirectionalStreams: ReadableLike<ReadableLike<Uint8Array>>;
}
export declare class WebTransportTransport implements OspTransport {
    #private;
    readonly ready: Promise<void>;
    readonly closed: Promise<CloseInfo>;
    constructor(wt: WebTransportLike);
    start(sink: TransportSink): void;
    send(frame: Frame, options?: SendOptions): Promise<void>;
    endGroup(group: string): Promise<void>;
    close(info?: Partial<CloseInfo>): void;
}
