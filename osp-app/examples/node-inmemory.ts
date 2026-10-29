// A controller and a receiver in one Node process, over an in-memory
// RTCDataChannel stand-in built from a MessageChannel. Run: npm run example
//
// Swap the transport for WebTransportTransport(new WebTransport(url)) or
// DataChannelTransport(rtcDataChannel) and nothing else changes.

import {
  DataChannelTransport,
  OspSession,
  PresentationController,
  PresentationReceiver,
  RemotePlaybackController,
  RemotePlaybackReceiver,
  AgentCapability,
  type DataChannelLike,
} from "../src/index.ts";

/** Two connected objects shaped like RTCDataChannel. */
function channelPair(): [DataChannelLike, DataChannelLike] {
  const { port1, port2 } = new MessageChannel();
  const wrap = (port: MessagePort): DataChannelLike => {
    port.start();
    return {
      binaryType: "arraybuffer",
      readyState: "open",
      send: (data) => port.postMessage(data, [data]),
      close: () => port.close(),
      addEventListener: (type, fn) => port.addEventListener(type, fn),
      removeEventListener: (type, fn) => port.removeEventListener(type, fn),
    };
  };
  return [wrap(port1), wrap(port2)];
}

const [tvEnd, phoneEnd] = channelPair();

// The receiver ("TV").
const tv = new OspSession(new DataChannelTransport(tvEnd), {
  agentInfo: {
    displayName: "Living room TV",
    modelName: "osp-app example",
    capabilities: [AgentCapability.receivePresentation, AgentCapability.receiveRemotePlayback],
    stateToken: "a1b2c3d4",
    locales: ["en-US"],
  },
});
const presentations = new PresentationReceiver();
presentations.attach(tv);
presentations.on("connection", (conn) => {
  console.log(`TV     | connection ${conn.connectionId} to ${conn.url}`);
  conn.on("message", (m) => void conn.send(`echo: ${String(m)}`));
  conn.on("terminate", ({ reason }) => console.log(`TV     | presentation terminated, reason ${reason}`));
});
new RemotePlaybackReceiver().attach(tv);

// The controller ("phone").
const phone = new OspSession(new DataChannelTransport(phoneEnd));
const info = await phone.requestAgentInfo();
console.log(`phone  | connected to "${info.displayName}", capabilities ${info.capabilities.join(", ")}`);

const controller = new PresentationController(phone);
const url = "https://example.com/slides";
const { availabilities } = await controller.checkAvailability([url]);
console.log(`phone  | ${url} availability: ${availabilities[0] === 0 ? "available" : availabilities[0]}`);

const conn = await controller.start(url);
console.log(`phone  | started presentation ${conn.presentationId}`);
const reply = new Promise((r) => conn.on("message", r));
await conn.send("next slide");
console.log(`phone  | got "${String(await reply)}"`);
await controller.terminate(conn.presentationId);

const playback = await new RemotePlaybackController(phone).start({
  sources: [{ url: "https://example.com/video.mp4", extendedMimeType: "video/mp4" }],
});
const state = await playback.modify({ paused: false, seek: 42 });
console.log(`phone  | remote playback ${playback.id}: paused=${state.paused} position=${state.position}`);
await playback.terminate();

phone.close();
console.log("phone  | done");
