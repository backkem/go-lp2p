import { test } from "node:test";
import assert from "node:assert/strict";
import {
  RemotePlaybackController,
  RemotePlaybackError,
  RemotePlaybackReceiver,
  RemotePlaybackTerminationEventReason,
  Result,
  UrlAvailability,
  type RemotePlaybackState,
} from "../src/index.ts";
import { kinds, next, sessionPair, type Kind } from "./helpers.ts";

const src = { url: "https://example.com/v.mp4", extendedMimeType: 'video/mp4; codecs="avc1.42E01E"' };

function setup(kind: Kind, receiver = new RemotePlaybackReceiver()) {
  const { a, b } = sessionPair(kind);
  receiver.attach(b);
  return { controller: new RemotePlaybackController(a), receiver, a };
}

for (const kind of kinds) {
  test(`${kind}: remote playback availability`, async () => {
    const { controller, a } = setup(kind);
    const r = await controller.checkAvailability([src, { url: "rtsp://x", extendedMimeType: "" }]);
    assert.deepEqual(r.availabilities, [UrlAvailability.available, UrlAvailability.invalid]);
    a.close();
  });

  test(`${kind}: start, modify, state events, terminate`, async () => {
    const { controller, receiver, a } = setup(kind);
    const pb = await controller.start({ sources: [src], controls: { muted: true } });
    assert.deepEqual(pb.state, { paused: true, source: src, muted: true });

    const state = await pb.modify({ paused: false, volume: 0.5, seek: 12.25 });
    assert.equal(state.paused, false);
    assert.equal(state.volume, 0.5);
    assert.equal(state.position, 12.25);

    const ev = next<RemotePlaybackState>((fn) => pb.on("state", fn));
    await receiver.updateState(pb.id, { position: 13, duration: 60, bufferedTimeRanges: [[0, 30]] });
    const s = await ev;
    assert.equal(s.position, 13);
    assert.equal(s.duration, 60);
    assert.deepEqual(s.bufferedTimeRanges, [[0, 30]]);
    assert.equal(s.paused, false, "partial updates merge");

    const ended = next<{ remotePlaybackId: number }>((fn) => receiver.on("terminated", fn));
    await pb.terminate();
    assert.equal((await ended).remotePlaybackId, pb.id);
    assert.equal(pb.terminated, true);
    await assert.rejects(pb.modify({ paused: true }), (e: unknown) => e instanceof RemotePlaybackError && e.result !== Result.success);
    a.close();
  });

  test(`${kind}: the receiver terminates`, async () => {
    const { controller, receiver, a } = setup(kind);
    const pb = await controller.start({ sources: [src] });
    const t = next<{ reason: number; local: boolean }>((fn) => pb.on("terminate", fn));
    await receiver.terminate(pb.id, RemotePlaybackTerminationEventReason.receiverIdleTooLong);
    assert.deepEqual(await t, { reason: RemotePlaybackTerminationEventReason.receiverIdleTooLong, local: false });
    a.close();
  });

  test(`${kind}: a refused start answers, then terminates`, async () => {
    const { controller, a } = setup(kind, new RemotePlaybackReceiver({ start: () => null }));
    const pb = await controller.start({ sources: [src] });
    if (!pb.terminated) await next((fn) => pb.on("terminate", fn));
    assert.equal(pb.terminated, true);
    a.close();
  });
}
