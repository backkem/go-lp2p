// Entry point of the standalone script (dist/lp2p-shim.js): installs the
// LP2P globals where missing and exposes everything else as LP2PShim.

import * as LP2PShim from "./index.ts";

const installed = LP2PShim.install();
Object.defineProperty(globalThis, "LP2PShim", {
  value: Object.freeze({ ...LP2PShim, installed }),
  writable: true,
  configurable: true,
  enumerable: false,
});
