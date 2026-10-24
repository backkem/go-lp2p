import type { DataChannelEndpoint } from "../protocol/session.js";
export interface LP2PDataChannelInit {
    protocol?: string;
    id?: number;
}
/** Not in the LP2P IDL (yet); same values as RTCDataChannelState. */
export type LP2PDataChannelState = "connecting" | "open" | "closing" | "closed";
/** Creates the API object for a session-layer channel. Internal. */
export declare function adoptChannel(endpoint: DataChannelEndpoint, incoming: boolean): LP2PDataChannel;
/** WebIDL [EnforceRange] unsigned short. */
export declare function enforceUnsignedShort(v: unknown, what: string): number;
export declare class LP2PDataChannel extends EventTarget {
    #private;
    /**
     * The IDL gives LP2PDataChannel a public constructor, but says nothing
     * about what a channel made that way is attached to. The shim lets you
     * construct one, and it is permanently "closed": channels that carry data
     * come from LP2PConnection.createDataChannel() or the datachannel event.
     */
    constructor(label: string, dataChannelDict?: LP2PDataChannelInit);
    get label(): string;
    get protocol(): string;
    get id(): number | null;
    /** Extension: not in the LP2P IDL, but needed to use the channel safely. */
    get readyState(): LP2PDataChannelState;
    get binaryType(): BinaryType;
    set binaryType(v: BinaryType);
    get onopen(): ((this: EventTarget, ev: Event) => unknown) | null;
    set onopen(fn: ((this: EventTarget, ev: Event) => unknown) | null);
    get onerror(): ((this: EventTarget, ev: Event) => unknown) | null;
    set onerror(fn: ((this: EventTarget, ev: Event) => unknown) | null);
    get onclosing(): ((this: EventTarget, ev: Event) => unknown) | null;
    set onclosing(fn: ((this: EventTarget, ev: Event) => unknown) | null);
    get onclose(): ((this: EventTarget, ev: Event) => unknown) | null;
    set onclose(fn: ((this: EventTarget, ev: Event) => unknown) | null);
    get onmessage(): ((this: EventTarget, ev: MessageEvent) => unknown) | null;
    set onmessage(fn: ((this: EventTarget, ev: MessageEvent) => unknown) | null);
    /**
     * Sends data. Strings go as encoding-id string, ArrayBuffer and views as
     * array-buffer, Blobs as blob. Blobs are read asynchronously; messages
     * sent after a Blob wait for it, so order is kept.
     */
    send(data: string | Blob | ArrayBuffer | ArrayBufferView): undefined;
    /**
     * Closes the channel. readyState becomes "closing" and closing fires (as
     * in go-lp2p); close fires once the channel is closed. Messages queued
     * behind a Blob are sent first.
     */
    close(): undefined;
}
