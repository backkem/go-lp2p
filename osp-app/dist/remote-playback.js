// The remote playback protocol (application.bs "Remote Playback Protocol"),
// without media remoting: the streaming groups of the start messages are
// passed through opaque. Messages about one remote playback are sent in the
// ordering group "remote-playback:<id>".
import { Emitter } from "./emitter.js";
import { RemotePlaybackTerminationEventReason, RemotePlaybackTerminationRequestReason, Result, UrlAvailability, } from "./messages.js";
const groupOf = (id) => `remote-playback:${id}`;
/** A random 48-bit remote-playback-id. The prose asks for a UUID; the CDDL says uint. */
export function generateRemotePlaybackId() {
    const b = crypto.getRandomValues(new Uint16Array(3));
    return (b[0] * 2 ** 32) + (b[1] * 2 ** 16) + b[2];
}
/** Merges a partial state (a state event or response) into a full one. */
export function mergeState(into, update) {
    return Object.assign(into, update);
}
export class RemotePlaybackError extends Error {
    name = "RemotePlaybackError";
    result;
    constructor(message, result) {
        super(message);
        this.result = result;
    }
}
/** A remote playback, as seen by the controller. */
export class RemotePlayback extends Emitter {
    id;
    /** The playback state, merged from the start response, modify responses and state events. */
    state = {};
    #terminated = false;
    #session;
    /** @internal */
    constructor(session, id) {
        super();
        this.#session = session;
        this.id = id;
    }
    get terminated() {
        return this.#terminated;
    }
    /** Sends controls (pause, seek, volume...) and resolves with the updated state. */
    async modify(controls, options = {}) {
        const resp = await this.#session.request("remote-playback-modify-request", { remotePlaybackId: this.id, controls }, { group: groupOf(this.id), ...(options.timeout === undefined ? {} : { timeout: options.timeout }) });
        if (resp.result !== Result.success)
            throw new RemotePlaybackError(`modify failed with result ${resp.result}`, resp.result);
        if (resp.state)
            this._state(resp.state);
        return this.state;
    }
    async terminate(reason = RemotePlaybackTerminationRequestReason.userTerminatedViaController) {
        const resp = await this.#session.request("remote-playback-termination-request", { remotePlaybackId: this.id, reason }, { group: groupOf(this.id) });
        if (resp.result !== Result.success)
            throw new RemotePlaybackError(`termination failed with result ${resp.result}`, resp.result);
        this._terminated(reason, true);
    }
    /** @internal */
    _state(update) {
        mergeState(this.state, update);
        this.emit("state", this.state);
    }
    /** @internal */
    _terminated(reason, local) {
        if (this.#terminated)
            return;
        this.#terminated = true;
        void this.#session.transport.endGroup(groupOf(this.id)).catch(() => { });
        this.emit("terminate", { reason, local });
    }
}
/** The controlling side of the remote playback protocol, over one session. */
export class RemotePlaybackController extends Emitter {
    session;
    #playbacks = new Map();
    #watches = new Map();
    #nextWatchId = 1;
    constructor(session) {
        super();
        this.session = session;
        session.handle("remote-playback-availability-event", (b) => {
            this.emit("availability", { watchId: b.watchId, sources: this.#watches.get(b.watchId) ?? [], availabilities: b.urlAvailabilities });
        });
        session.handle("remote-playback-state-event", (b) => this.#playbacks.get(b.remotePlaybackId)?._state(b.state));
        session.handle("remote-playback-termination-event", (b) => {
            const pb = this.#playbacks.get(b.remotePlaybackId);
            this.#playbacks.delete(b.remotePlaybackId);
            pb?._terminated(b.reason, false);
        });
    }
    async checkAvailability(sources, options = {}) {
        const watchId = this.#nextWatchId++;
        this.#watches.set(watchId, sources);
        const resp = await this.session.request("remote-playback-availability-request", { sources, watchDuration: options.watchDuration ?? 0, watchId }, options.timeout === undefined ? {} : { timeout: options.timeout });
        return { watchId, availabilities: resp.urlAvailabilities };
    }
    /**
     * Starts remote playback. The start response has no result field, so a
     * receiver that refuses answers and then sends a termination event; the
     * returned playback is then already terminated.
     */
    async start(options) {
        const id = options.remotePlaybackId ?? generateRemotePlaybackId();
        const pb = new RemotePlayback(this.session, id);
        this.#playbacks.set(id, pb);
        pb.on("terminate", () => this.#playbacks.delete(id));
        try {
            await this.session.request("remote-playback-start-request", {
                remotePlaybackId: id,
                sources: options.sources,
                ...(options.textTrackUrls ? { textTrackUrls: options.textTrackUrls } : {}),
                ...(options.headers ? { headers: options.headers } : {}),
                ...(options.controls ? { controls: options.controls } : {}),
            }, {
                group: groupOf(id),
                onResponse: (b) => {
                    if (b.state)
                        pb._state(b.state);
                },
                ...(options.timeout === undefined ? {} : { timeout: options.timeout }),
            });
        }
        catch (err) {
            this.#playbacks.delete(id);
            throw err;
        }
        return pb;
    }
}
/** Applies the controls that map directly onto state. */
export function applyControls(state, c) {
    if (c.source !== undefined)
        state.source = c.source;
    if (c.paused !== undefined)
        state.paused = c.paused;
    if (c.muted !== undefined)
        state.muted = c.muted;
    if (c.volume !== undefined)
        state.volume = c.volume;
    if (c.playbackRate !== undefined)
        state.playbackRate = c.playbackRate;
    if (c.seek !== undefined)
        state.position = c.seek;
    if (c.fastSeek !== undefined)
        state.position = c.fastSeek;
    return state;
}
/** The receiving side of the remote playback protocol, serving any number of controller sessions. */
export class RemotePlaybackReceiver extends Emitter {
    #options;
    #playbacks = new Map();
    constructor(options = {}) {
        super();
        this.#options = options;
    }
    attach(session) {
        const availabilityOf = this.#options.availabilityOf ?? ((s) => (/^https?:\/\//.test(s.url) ? UrlAvailability.available : UrlAvailability.invalid));
        const offs = [
            session.handle("remote-playback-availability-request", (b) => {
                void session.send("remote-playback-availability-response", { requestId: b.requestId, urlAvailabilities: b.sources.map(availabilityOf) }).catch(() => { });
            }),
            session.handle("remote-playback-start-request", (b) => void this.#onStart(session, b)),
            session.handle("remote-playback-modify-request", (b) => void this.#onModify(session, b)),
            session.handle("remote-playback-termination-request", (b) => {
                const pb = this.#playbacks.get(b.remotePlaybackId);
                const result = pb && pb.session === session ? Result.success : Result.invalidPresentationId;
                void session.send("remote-playback-termination-response", { requestId: b.requestId, result }, { group: groupOf(b.remotePlaybackId) }).catch(() => { });
                if (result === Result.success)
                    this.#end(b.remotePlaybackId, b.reason);
            }),
        ];
        const detach = () => {
            for (const off of offs)
                off();
            for (const [id, pb] of this.#playbacks)
                if (pb.session === session)
                    this.#playbacks.delete(id);
        };
        void session.closed.then(detach);
        return detach;
    }
    /** The state of a running playback. */
    stateOf(id) {
        return this.#playbacks.get(id)?.state;
    }
    /** Updates a playback's state and sends a state event with the changed fields. */
    async updateState(id, update) {
        const pb = this.#playbacks.get(id);
        if (!pb)
            throw new RangeError(`no remote playback ${id}`);
        mergeState(pb.state, update);
        await pb.session.send("remote-playback-state-event", { remotePlaybackId: id, state: update }, { group: groupOf(id) });
    }
    /** Terminates a playback from the receiver side. */
    async terminate(id, reason = RemotePlaybackTerminationEventReason.receiverCalledTerminate) {
        const pb = this.#playbacks.get(id);
        if (!pb)
            return;
        await pb.session.send("remote-playback-termination-event", { remotePlaybackId: id, reason }, { group: groupOf(id) }).catch(() => { });
        this.#end(id, reason);
    }
    #end(id, reason) {
        const pb = this.#playbacks.get(id);
        if (!pb)
            return;
        this.#playbacks.delete(id);
        void pb.session.transport.endGroup(groupOf(id)).catch(() => { });
        this.emit("terminated", { remotePlaybackId: id, reason });
    }
    async #onStart(session, b) {
        const id = b.remotePlaybackId;
        const group = groupOf(id);
        const req = {
            remotePlaybackId: id,
            sources: b.sources ?? [],
            textTrackUrls: b.textTrackUrls ?? [],
            headers: b.headers ?? [],
            controls: b.controls ?? {},
            session,
        };
        let state;
        if (this.#playbacks.has(id) || req.sources.length === 0)
            state = null; // Duplicate id, or remoting only (out of scope).
        else if (this.#options.start)
            state = await Promise.resolve(this.#options.start(req)).catch(() => null);
        else
            state = applyControls({ paused: true, source: req.sources[0] }, req.controls);
        if (state === null) {
            // No result field to refuse with: answer, then terminate.
            await session.send("remote-playback-start-response", { requestId: b.requestId }, { group }).catch(() => { });
            await session.send("remote-playback-termination-event", { remotePlaybackId: id, reason: RemotePlaybackTerminationEventReason.unknown }, { group }).catch(() => { });
            void session.transport.endGroup(group).catch(() => { });
            return;
        }
        this.#playbacks.set(id, { session, state });
        await session.send("remote-playback-start-response", { requestId: b.requestId, state }, { group }).catch(() => { });
        this.emit("playback", { remotePlaybackId: id, sources: req.sources, state });
    }
    async #onModify(session, b) {
        const id = b.remotePlaybackId;
        const group = groupOf(id);
        const pb = this.#playbacks.get(id);
        if (!pb || pb.session !== session) {
            await session.send("remote-playback-modify-response", { requestId: b.requestId, result: Result.invalidPresentationId }, { group }).catch(() => { });
            return;
        }
        let result = Result.success;
        try {
            const next = this.#options.modify ? await this.#options.modify(id, b.controls, pb.state) : applyControls(pb.state, b.controls);
            mergeState(pb.state, next);
        }
        catch {
            result = Result.unknownError;
        }
        await session.send("remote-playback-modify-response", { requestId: b.requestId, result, state: pb.state }, { group }).catch(() => { });
        if (result === Result.success)
            this.emit("modify", { remotePlaybackId: id, controls: b.controls, state: pb.state });
    }
}
