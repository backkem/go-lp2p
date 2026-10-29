// OspTransport over anything shaped like RTCDataChannel: RTCDataChannel
// itself, LP2PDataChannel, or a test fake. Nothing is imported; the shape
// below is all that is used.
//
// Mapping: one OSP message per channel message, framed as type-key varint +
// CBOR, sent as an ArrayBuffer. The channel must be reliable and ordered (the
// default), so every ordering group is trivially kept and groups are ignored.
// A channel has no close code: close() just closes the channel, and the peer
// sees code 0.
import { decodeFrame, encodeFrame } from "../messages.js";
import { TransportClosedError } from "../transport.js";
export class DataChannelTransport {
    ready;
    closed;
    #channel;
    #sink = null;
    #queue = [];
    #isClosed = false;
    #closeInfo = { code: 0, reason: "" };
    constructor(channel) {
        this.#channel = channel;
        channel.binaryType = "arraybuffer";
        this.ready = new Promise((resolve, reject) => {
            if (channel.readyState === "open")
                return resolve();
            if (channel.readyState === "closing" || channel.readyState === "closed")
                return reject(new TransportClosedError("data channel is closed"));
            const onOpen = () => {
                channel.removeEventListener("open", onOpen);
                resolve();
            };
            channel.addEventListener("open", onOpen);
            channel.addEventListener("close", () => reject(new TransportClosedError("data channel closed before opening")));
        });
        this.ready.catch(() => { });
        this.closed = new Promise((resolve) => {
            const done = () => {
                this.#isClosed = true;
                resolve(this.#closeInfo);
            };
            if (channel.readyState === "closed")
                done();
            else
                channel.addEventListener("close", done);
        });
        channel.addEventListener("message", this.#onMessage);
    }
    #onMessage = (ev) => {
        const data = ev.data;
        let buf;
        if (data instanceof ArrayBuffer)
            buf = new Uint8Array(data);
        else if (ArrayBuffer.isView(data))
            buf = new Uint8Array(data.buffer, data.byteOffset, data.byteLength);
        else {
            this.#sink?.malformed(new TypeError(`data channel: expected a binary message, got ${typeof data}`));
            return;
        }
        let frame;
        try {
            frame = decodeFrame(buf);
        }
        catch (err) {
            this.#sink?.malformed(err);
            return;
        }
        if (this.#sink)
            this.#sink.frame(frame);
        else
            this.#queue.push(frame);
    };
    start(sink) {
        // Messages that arrived earlier are delivered after the current task, so
        // the caller can register handlers right after creating the session.
        queueMicrotask(() => {
            const queued = this.#queue;
            this.#queue = [];
            this.#sink = sink;
            for (const f of queued)
                sink.frame(f);
        });
    }
    async send(frame, _options) {
        await this.ready;
        if (this.#isClosed || this.#channel.readyState !== "open")
            throw new TransportClosedError("data channel is closed");
        const bytes = encodeFrame(frame);
        this.#channel.send(bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.byteLength));
    }
    async endGroup(_group) { }
    close(info = {}) {
        if (this.#isClosed)
            return;
        this.#closeInfo = { code: info.code ?? 0, reason: info.reason ?? "" };
        this.#channel.removeEventListener("message", this.#onMessage);
        this.#channel.close();
    }
}
