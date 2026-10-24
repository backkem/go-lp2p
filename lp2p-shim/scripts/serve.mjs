// Serves lp2p-shim/ on http://localhost:8080 (a secure context, which the
// LP2P API requires) for the demos: npm run serve, then open /demo/.
import { createServer } from "node:http";
import { readFile } from "node:fs/promises";
import { extname, join, normalize, dirname } from "node:path";
import { fileURLToPath } from "node:url";

const root = join(dirname(fileURLToPath(import.meta.url)), "..");
const types = { ".html": "text/html", ".js": "text/javascript", ".mjs": "text/javascript", ".css": "text/css", ".json": "application/json", ".svg": "image/svg+xml" };

export function serve(port = 0, host = "127.0.0.1") {
  const server = createServer(async (req, res) => {
    let path = decodeURIComponent(new URL(req.url ?? "/", "http://x").pathname);
    if (path === "/") {
      res.writeHead(302, { location: "/demo/" }).end();
      return;
    }
    if (path.endsWith("/")) path += "index.html";
    const file = normalize(join(root, path));
    if (!file.startsWith(root) || file.includes("node_modules")) {
      res.writeHead(403).end();
      return;
    }
    try {
      const body = await readFile(file);
      res.writeHead(200, { "content-type": types[extname(file)] ?? "application/octet-stream", "cache-control": "no-store" }).end(body);
    } catch {
      res.writeHead(404).end("not found");
    }
  });
  return new Promise((resolve) => server.listen(port, host, () => resolve(server)));
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  const port = Number(process.env.PORT ?? 8080);
  const server = await serve(port, process.env.HOST ?? "localhost");
  const { port: actual } = server.address();
  console.log(`lp2p-shim demos: http://localhost:${actual}/demo/`);
}
