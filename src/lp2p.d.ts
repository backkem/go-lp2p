class LP2PReceiver extends EventTarget {
    constructor(options?: LP2PReceiverOptions): LP2PReceiver;
    onconnection: EventHandler<LP2PConnectionEvent>;
    start(): Promise<undefined>;
}
interface LP2PReceiverOptions {
    nickname?: string;
}
class LP2PConnectionEvent extends Event {
    constructor(type: string, connectionEventInitDict: LP2PConnectionEventInit): LP2PConnectionEvent;
    readonly connection: LP2PConnection;
}
interface LP2PConnectionEventInit extends EventInit {
    connection: LP2PConnection;
}
class LP2PRequest {
    constructor(options?: LP2PRequestOptions): LP2PRequest;
    start(): Promise<LP2PConnection>;
}
interface LP2PRequestOptions {
    nickname?: string;
}
interface LP2PConnection extends EventTarget {
}
class LP2PDataChannel extends EventTarget {
    constructor(label: string, dataChannelDict?: LP2PDataChannelInit): LP2PDataChannel;
    readonly label: string;
    readonly protocol: string;
    readonly id: number | null;
    onopen: EventHandler<Event>;
    onerror: EventHandler<Event>;
    onclosing: EventHandler<Event>;
    onclose: EventHandler<Event>;
    close(): undefined;
    onmessage: EventHandler<MessageEvent>;
    binaryType: BinaryType;
    send(data: string | Blob | ArrayBuffer | ArrayBufferView): undefined;
}
interface LP2PDataChannelInit {
    protocol?: string;
    id?: number;
}
interface LP2PConnection {
    createDataChannel(label: string, dataChannelDict?: LP2PDataChannelInit): LP2PDataChannel;
    ondatachannel: EventHandler<LP2PDataChannelEvent>;
}
class LP2PDataChannelEvent extends Event {
    constructor(type: string, DataChannelEventInitDict: LP2PDataChannelEventInit): LP2PDataChannelEvent;
    readonly channel: LP2PDataChannel;
}
interface LP2PDataChannelEventInit extends EventInit {
    channel: LP2PDataChannel;
}
class LP2PQuicTransport extends WebTransport {
    constructor(source: LP2PRequest | LP2PReceiver, quicTransportDict?: LP2PQuicTransportInit): LP2PQuicTransport;
    constructor(connection: LP2PConnection, quicTransportDict?: LP2PQuicTransportInit): LP2PQuicTransport;
}

interface LP2PQuicTransportInit {
}
class LP2PQuicTransportListener {
    constructor(source: LP2PRequest | LP2PReceiver, quicTransportListenerDict?: LP2PQuicTransportListenerInit): LP2PQuicTransportListener;
    readonly ready: Promise<undefined>;
    readonly incomingTransports: ReadableStream<LP2PQuicTransport>;
}
interface LP2PQuicTransportListenerInit {
}