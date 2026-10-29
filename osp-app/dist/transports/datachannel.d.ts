import { type Frame } from "../messages.ts";
import { type CloseInfo, type OspTransport, type SendOptions, type TransportSink } from "../transport.ts";
/** The subset of RTCDataChannel / LP2PDataChannel this transport uses. */
export interface DataChannelLike {
    binaryType: string;
    readonly readyState: string;
    send(data: ArrayBuffer): void;
    close(): void;
    /** "open", "close" and "message" (a MessageEvent) are used. */
    addEventListener(type: string, listener: (ev: Event) => void): void;
    removeEventListener(type: string, listener: (ev: Event) => void): void;
}
export declare class DataChannelTransport implements OspTransport {
    #private;
    readonly ready: Promise<void>;
    readonly closed: Promise<CloseInfo>;
    constructor(channel: DataChannelLike);
    start(sink: TransportSink): void;
    send(frame: Frame, _options?: SendOptions): Promise<void>;
    endGroup(_group: string): Promise<void>;
    close(info?: Partial<CloseInfo>): void;
}
