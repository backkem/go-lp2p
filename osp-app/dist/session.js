// OspSession: the message layer of the application protocol over one
// OspTransport. It encodes and decodes messages, allocates request IDs and
// matches responses to requests, answers agent-info and agent-status
// requests, and applies the protocol's rules for unknown type keys.
// Presentation and remote playback build on it.
import { decodeBody, encodeBody, isSupported, messageName, typeKeyOf, SchemaError, } from "./messages.js";
import { Emitter } from "./emitter.js";
import { TransportClosedError } from "./transport.js";
/** application.bs: an unknown type key closes the connection with this code. */
export const UNKNOWN_TYPE_KEY_CODE = 404;
/** A response type for each request type. */
export const responseOf = {
    "agent-info-request": "agent-info-response",
    "agent-status-request": "agent-status-response",
    "presentation-url-availability-request": "presentation-url-availability-response",
    "presentation-start-request": "presentation-start-response",
    "presentation-termination-request": "presentation-termination-response",
    "presentation-connection-open-request": "presentation-connection-open-response",
    "remote-playback-availability-request": "remote-playback-availability-response",
    "remote-playback-start-request": "remote-playback-start-response",
    "remote-playback-termination-request": "remote-playback-termination-response",
    "remote-playback-modify-request": "remote-playback-modify-response",
};
export class OspSession extends Emitter {
    transport;
    /** The peer's agent-info, once requested with requestAgentInfo() or announced with agent-info-event. */
    remoteAgentInfo = null;
    closed;
    #options;
    #handlers = new Map();
    #pending = new Map();
    #nextId = 1;
    #isClosed = false;
    constructor(transport, options = {}) {
        super();
        this.transport = transport;
        this.#options = options;
        this.closed = transport.closed.then((info) => {
            this.#isClosed = true;
            const err = new TransportClosedError(`transport closed (${info.code}${info.reason ? `: ${info.reason}` : ""})`);
            for (const p of this.#pending.values())
                p.reject(err);
            this.#pending.clear();
            this.emit("close", info);
            return info;
        });
        this.handle("agent-info-event", (b) => {
            this.remoteAgentInfo = b.agentInfo;
        });
        if (options.agentInfo) {
            const info = options.agentInfo;
            this.handle("agent-info-request", (b) => void this.send("agent-info-response", { requestId: b.requestId, agentInfo: info }).catch(() => { }));
            this.handle("agent-status-request", (b) => void this.send("agent-status-response", { requestId: b.requestId }).catch(() => { }));
        }
        transport.start({ frame: (f) => this.#receive(f), malformed: (err) => this.emit("malformed", err) });
    }
    get isClosed() {
        return this.#isClosed;
    }
    /** Allocates a request ID. */
    nextRequestId() {
        return this.#options.nextRequestId ? this.#options.nextRequestId() : this.#nextId++;
    }
    /** Sends a message. */
    send(name, body, options) {
        if (!isSupported(name))
            return Promise.reject(new TypeError(`${name} is not supported`));
        let encoded;
        try {
            encoded = encodeBody(name, body);
        }
        catch (err) {
            return Promise.reject(err);
        }
        return this.transport.send({ typeKey: typeKeyOf(name), body: encoded }, options);
    }
    /** Sends a request with a fresh request ID and resolves with the matching response. */
    request(name, body, options = {}) {
        if (this.#isClosed)
            return Promise.reject(new TransportClosedError("session is closed"));
        const requestId = this.nextRequestId();
        const respName = responseOf[name];
        const key = `${typeKeyOf(respName)}:${requestId}`;
        return new Promise((resolve, reject) => {
            let timer;
            const done = () => {
                if (timer !== undefined)
                    clearTimeout(timer);
                this.#pending.delete(key);
            };
            this.#pending.set(key, {
                resolve: (v) => (done(), resolve(v)),
                reject: (e) => (done(), reject(e)),
                onResponse: options.onResponse,
            });
            if (options.timeout !== undefined) {
                timer = setTimeout(() => this.#pending.get(key)?.reject(new DOMException(`${name} timed out`, "TimeoutError")), options.timeout);
            }
            const full = { ...body, requestId };
            this.send(name, full, options).catch((err) => this.#pending.get(key)?.reject(err));
        });
    }
    /** Handles every received message of a type. Returns a function that removes the handler. */
    handle(name, fn) {
        let set = this.#handlers.get(name);
        if (!set)
            this.#handlers.set(name, (set = new Set()));
        const h = fn;
        set.add(h);
        return () => set.delete(h);
    }
    /** Asks the peer for its agent-info. */
    async requestAgentInfo(options) {
        const resp = await this.request("agent-info-request", {}, options);
        this.remoteAgentInfo = resp.agentInfo;
        return resp.agentInfo;
    }
    /** Closes the transport. */
    close(info) {
        this.transport.close(info);
    }
    #receive(frame) {
        if (this.#isClosed)
            return;
        const name = messageName(frame.typeKey);
        if (name === undefined) {
            this.emit("unknown", frame);
            if ((this.#options.onUnknownTypeKey ?? "close") === "close") {
                this.transport.close({ code: UNKNOWN_TYPE_KEY_CODE, reason: `unknown type key ${frame.typeKey}` });
            }
            return;
        }
        let msg;
        try {
            msg = decodeBody(frame.typeKey, frame.body);
        }
        catch (err) {
            this.emit("malformed", err instanceof SchemaError || err instanceof RangeError ? err : new Error(String(err)));
            return;
        }
        if (!isSupported(msg.name)) {
            this.emit("unsupported", msg);
            return;
        }
        const body = msg.body;
        if (msg.name.endsWith("-response") && typeof body.requestId === "number") {
            const p = this.#pending.get(`${frame.typeKey}:${body.requestId}`);
            if (p) {
                try {
                    p.onResponse?.(msg.body);
                }
                catch (err) {
                    p.reject(err);
                    return;
                }
                p.resolve(msg.body);
                return;
            }
            this.emit("unhandled", msg);
            return;
        }
        const set = this.#handlers.get(msg.name);
        if (!set || set.size === 0) {
            this.emit("unhandled", msg);
            return;
        }
        for (const fn of [...set]) {
            try {
                fn(msg.body, msg);
            }
            catch (err) {
                queueMicrotask(() => {
                    throw err;
                });
            }
        }
    }
}
