// CBOR test vectors. The "go" set is produced by openscreen-go's encoder
// (test/govectors); the "cddl" set is hand-encoded from the CDDL.

export const PID = "SD3I7BVM34KZ4M2CT4YG7KENWJ";
export const URL1 = "https://example.com/";

export interface Vector {
  name: string;
  body: Record<string, unknown>;
  hex: string;
}

export const goVectors: Vector[] = [
  {
    name: "agent-info-response",
    body: { requestId: 1, agentInfo: { displayName: "TV", modelName: "M1", capabilities: [3, 5], stateToken: "abcd1234", locales: ["en-US"] } },
    hex: "a2000101a50062545601624d310282030503686162636431323334048165656e2d5553",
  },
  {
    name: "agent-info-response",
    body: { requestId: 1, agentInfo: { displayName: "TV", capabilities: [3], stateToken: "abcd1234", locales: [] } },
    hex: "a2000101a400625456028103036861626364313233340480",
  },
  {
    name: "presentation-url-availability-request",
    body: { requestId: 2, urls: [URL1], watchDuration: 60000000, watchId: 1 },
    hex: "a4000201817468747470733a2f2f6578616d706c652e636f6d2f021a039387000301",
  },
  { name: "presentation-url-availability-response", body: { requestId: 2, urlAvailabilities: [0, 1, 10] }, hex: "a20002018300010a" },
  { name: "presentation-url-availability-event", body: { watchId: 1, urlAvailabilities: [1] }, hex: "a20001018101" },
  {
    name: "presentation-start-request",
    body: { requestId: 3, presentationId: PID, url: URL1, headers: [["Accept-Language", "en"]] },
    hex: "a4000301781a534433493742564d33344b5a344d324354345947374b454e574a027468747470733a2f2f6578616d706c652e636f6d2f0381826f4163636570742d4c616e677561676562656e",
  },
  {
    name: "presentation-start-request",
    body: { requestId: 3, presentationId: PID, url: URL1, headers: [] },
    hex: "a4000301781a534433493742564d33344b5a344d324354345947374b454e574a027468747470733a2f2f6578616d706c652e636f6d2f0380",
  },
  { name: "presentation-start-response", body: { requestId: 3, result: 1, connectionId: 7, httpResponseCode: 200 }, hex: "a40003010102070318c8" },
  { name: "presentation-start-response", body: { requestId: 3, result: 10, connectionId: 0 }, hex: "a30003010a0200" },
  {
    name: "presentation-termination-request",
    body: { requestId: 4, presentationId: PID, reason: 1 },
    hex: "a3000401781a534433493742564d33344b5a344d324354345947374b454e574a0201",
  },
  { name: "presentation-termination-response", body: { requestId: 4, result: 1 }, hex: "a200040101" },
  {
    name: "presentation-termination-event",
    body: { presentationId: PID, source: 2, reason: 100 },
    hex: "a300781a534433493742564d33344b5a344d324354345947374b454e574a0102021864",
  },
  {
    name: "presentation-connection-open-request",
    body: { requestId: 5, presentationId: PID, url: URL1 },
    hex: "a3000501781a534433493742564d33344b5a344d324354345947374b454e574a027468747470733a2f2f6578616d706c652e636f6d2f",
  },
  { name: "presentation-connection-open-response", body: { requestId: 5, result: 1, connectionId: 8, connectionCount: 2 }, hex: "a40005010102080302" },
  {
    name: "presentation-connection-close-event",
    body: { connectionId: 8, reason: 1, errorMessage: "bye", connectionCount: 1 },
    hex: "a40008010102636279650301",
  },
  {
    name: "presentation-change-event",
    body: { presentationId: PID, connectionCount: 1 },
    hex: "a200781a534433493742564d33344b5a344d324354345947374b454e574a0101",
  },
  { name: "presentation-connection-message", body: { connectionId: 7, message: "hello" }, hex: "a20007016568656c6c6f" },
  { name: "presentation-connection-message", body: { connectionId: 7, message: Uint8Array.of(1, 2, 3) }, hex: "a200070143010203" },
];

export const cddlVectors: Vector[] = [
  { name: "agent-info-request", body: { requestId: 1 }, hex: "a10001" },
  { name: "agent-status-request", body: { requestId: 1, status: { status: "ok" } }, hex: "a2000101a100626f6b" },
  { name: "agent-status-response", body: { requestId: 1 }, hex: "a10001" },
  {
    name: "remote-playback-modify-request",
    body: { requestId: 9, remotePlaybackId: 42, controls: { paused: true, volume: 0.5, seek: 10 } },
    hex: "a30009" + "01182a" + "02a3" + "03f5" + "05fb3fe0000000000000" + "06fb4024000000000000",
  },
  {
    name: "remote-playback-state-event",
    body: { remotePlaybackId: 42, state: { paused: false, position: 1.5, duration: null, bufferedTimeRanges: [[0, 2]] } },
    hex: "a200182a01a4" + "06f6" + "078182fb0000000000000000fb4000000000000000" + "0afb3ff8000000000000" + "0cf4",
  },
  { name: "remote-playback-termination-event", body: { remotePlaybackId: 42, reason: 2 }, hex: "a200182a0102" },
  { name: "remote-playback-start-response", body: { requestId: 6, state: { error: [4, "nope"] } }, hex: "a2000601a104820464" + "6e6f7065" },
];
