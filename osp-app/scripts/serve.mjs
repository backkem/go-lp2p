// Serves the go-lp2p checkout on http://localhost:8080 (a secure context, which
// the LP2P API requires) so the browser example can load both osp-app/dist and
// lp2p-shim/dist: npm run serve, then open /osp-app/examples/browser/.
import { createServer } from "node:http";
import { readFile } from "node:fs/promises";
import { extname, join, normalize, dirname } from "node:path";
import { fileURLToPath } from "node:url";

const root = join(dirname(fileURLToPath(import.meta.url)), "..", "..");
const types = { ".html": "text/html", ".js": "text/javascript", ".mjs": "text/javascript", ".css": "text/css", ".json": "application/json" };
const port = Number(process.env.PORT ?? 8080);

createServer(async (req, res) => {
  let path = decodeURIComponent(new URL(req.url ?? "/", "http://x").pathname);
  if (path === "/") {
    res.writeHead(302, { location: "/osp-app/examples/browser/" }).end();
    return;
  }
  if (path.endsWith("/")) path += "index.html";
  const file = normalize(join(root, path));
  if (!file.startsWith(root) || file.includes("node_modules") || file.includes(`${root}/.git`)) {
    res.writeHead(403).end();
    return;
  }
  try {
    const body = await readFile(file);
    res.writeHead(200, { "content-type": types[extname(file)] ?? "application/octet-stream", "cache-control": "no-store" }).end(body);
  } catch {
    res.writeHead(404).end("not found");
  }
}).listen(port, "127.0.0.1", () => console.log(`http://localhost:${port}/osp-app/examples/browser/`));
