// Builds the checked-in bundles in dist/:
//   lp2p-shim.js      standalone script (IIFE): installs the LP2P globals where missing, plus LP2PShim
//   lp2p-shim.min.js  the same, minified
//   lp2p-shim.esm.js  ES module: exports everything, installs nothing
//   types/            .d.ts files for the ES module
// The output must be deterministic: CI rebuilds and diffs it.
import { build } from "esbuild";
import { execFileSync } from "node:child_process";
import { readFileSync, readdirSync, rmSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const root = join(dirname(fileURLToPath(import.meta.url)), "..");
const { version } = JSON.parse(readFileSync(join(root, "package.json"), "utf8"));
const banner = `/*! lp2p-shim ${version} - experimental Local Peer-to-Peer API polyfill. NOT SECURE: no real authentication or encryption. https://github.com/backkem/go-lp2p/tree/main/lp2p-shim */`;

const common = {
  absWorkingDir: root,
  bundle: true,
  target: "es2022",
  platform: "browser",
  banner: { js: banner },
  legalComments: "none",
  logLevel: "warning",
  charset: "utf8",
};

rmSync(join(root, "dist"), { recursive: true, force: true });

await Promise.all([
  build({ ...common, entryPoints: ["src/polyfill.ts"], format: "iife", outfile: "dist/lp2p-shim.js" }),
  build({ ...common, entryPoints: ["src/polyfill.ts"], format: "iife", minify: true, outfile: "dist/lp2p-shim.min.js" }),
  build({ ...common, entryPoints: ["src/index.ts"], format: "esm", outfile: "dist/lp2p-shim.esm.js" }),
]);

execFileSync(process.execPath, [join(root, "node_modules/typescript/bin/tsc"), "-p", join(root, "tsconfig.json")], {
  stdio: "inherit",
});

// tsc keeps the ".ts" import specifiers of the sources in .d.ts files;
// point them at ".js" so any TypeScript configuration can consume them.
const types = join(root, "dist/types");
rmSync(join(types, "polyfill.d.ts"));
for (const entry of readdirSync(types, { recursive: true })) {
  const file = join(types, String(entry));
  if (!file.endsWith(".d.ts")) continue;
  const src = readFileSync(file, "utf8");
  const out = src.replace(/(from\s+["'](?:\.{1,2}\/)[^"']*?)\.ts(["'])/g, "$1.js$2").replace(/(import\(["'](?:\.{1,2}\/)[^"']*?)\.ts(["']\))/g, "$1.js$2");
  if (out !== src) writeFileSync(file, out);
}
