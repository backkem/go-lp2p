// Builds the checked-in dist/: one ES module per source file plus .d.ts
// files, emitted by tsc (rewriteRelativeImportExtensions turns the ".ts"
// specifiers of the sources into ".js"). No bundler: the output is plain ESM
// a browser can import directly. CI rebuilds and diffs it.
import { execFileSync } from "node:child_process";
import { rmSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const root = join(dirname(fileURLToPath(import.meta.url)), "..");
rmSync(join(root, "dist"), { recursive: true, force: true });
execFileSync(process.execPath, [join(root, "node_modules/typescript/bin/tsc"), "-p", join(root, "tsconfig.json")], {
  stdio: "inherit",
});
