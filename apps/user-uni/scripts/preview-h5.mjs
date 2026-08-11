#!/usr/bin/env node
/**
 * Serve production H5 build for Playwright smoke.
 * Assets are emitted under /h5/* while smoke navigates from origin root (/app/...).
 * This server maps both /h5/* and /* onto dist/build/h5, and proxies /api to the local API.
 */
import http from "node:http";
import fs from "node:fs";
import path from "node:path";
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../dist/build/h5");
const apiTarget = new URL(process.env.XIANZHI_API_BASE_URL || "http://127.0.0.1:3100");

function readArg(name, fallback = "") {
  const prefix = `--${name}`;
  const argv = process.argv.slice(2);
  for (let i = 0; i < argv.length; i += 1) {
    const token = argv[i];
    if (token === prefix && argv[i + 1]) return argv[i + 1];
    if (token.startsWith(`${prefix}=`)) return token.slice(prefix.length + 1);
  }
  return fallback;
}

const host = process.env.HOST || readArg("host", "127.0.0.1");
// CLI --port must win over leftover PORT=5173 from uni e2e shells.
const port = Number(readArg("port", "") || process.env.USER_H5_PREVIEW_PORT || process.env.PORT || "4173");

const mime = {
  ".html": "text/html; charset=utf-8",
  ".js": "text/javascript; charset=utf-8",
  ".css": "text/css; charset=utf-8",
  ".json": "application/json",
  ".png": "image/png",
  ".jpg": "image/jpeg",
  ".jpeg": "image/jpeg",
  ".gif": "image/gif",
  ".svg": "image/svg+xml",
  ".webp": "image/webp",
  ".ico": "image/x-icon",
  ".woff": "font/woff",
  ".woff2": "font/woff2",
  ".map": "application/json",
};

function resolveUrlPath(rawUrl) {
  const pathname = decodeURIComponent(String(rawUrl || "/").split("?")[0] || "/");
  if (pathname === "/h5" || pathname === "/h5/") return "/index.html";
  if (pathname.startsWith("/h5/")) return pathname.slice(3);
  return pathname;
}

function safeJoin(base, requestPath) {
  const normalized = path.normalize(requestPath).replace(/^(\.\.[/\\])+/, "");
  const absolute = path.join(base, normalized);
  if (!absolute.startsWith(base)) return null;
  return absolute;
}

function sendFile(res, filePath) {
  const ext = path.extname(filePath).toLowerCase();
  res.writeHead(200, { "Content-Type": mime[ext] || "application/octet-stream", "Cache-Control": "no-cache" });
  fs.createReadStream(filePath).pipe(res);
}

function proxyApi(req, res) {
  const targetPath = req.url || "/";
  const upstream = http.request(
    {
      protocol: apiTarget.protocol,
      hostname: apiTarget.hostname,
      port: apiTarget.port || (apiTarget.protocol === "https:" ? 443 : 80),
      method: req.method,
      path: targetPath,
      headers: {
        ...req.headers,
        host: apiTarget.host,
      },
    },
    (upstreamRes) => {
      res.writeHead(upstreamRes.statusCode || 502, upstreamRes.headers);
      upstreamRes.pipe(res);
    },
  );
  upstream.on("error", (error) => {
    res.writeHead(502, { "Content-Type": "application/json" });
    res.end(JSON.stringify({ error: `API proxy failed: ${error.message}` }));
  });
  req.pipe(upstream);
}

function ensureCanvasStaticSynced() {
  const script = path.resolve(root, "../../scripts", "copy-h5-static.mjs");
  if (!fs.existsSync(script)) return;
  const result = spawnSync(process.execPath, [script], { stdio: "inherit" });
  if (result.status !== 0) {
    console.warn(`[preview-h5] canvas static sync exited with ${result.status}`);
  }
}

if (!fs.existsSync(path.join(root, "index.html"))) {
  console.error(`H5 build missing at ${root}. Run: npm run build:h5`);
  process.exit(1);
}
ensureCanvasStaticSynced();

const server = http.createServer((req, res) => {
  const url = String(req.url || "/");
  if (url === "/api" || url.startsWith("/api/") || url.startsWith("/h5/api/")) {
    if (url.startsWith("/h5/api/")) req.url = url.slice(3);
    proxyApi(req, res);
    return;
  }

  const mapped = resolveUrlPath(url);
  let filePath = safeJoin(root, mapped);
  if (!filePath) {
    res.writeHead(403).end("Forbidden");
    return;
  }
  if (fs.existsSync(filePath) && fs.statSync(filePath).isDirectory()) {
    filePath = path.join(filePath, "index.html");
  }
  if (fs.existsSync(filePath) && fs.statSync(filePath).isFile()) {
    sendFile(res, filePath);
    return;
  }
  // SPA fallback for history routes like /app/video-generation
  sendFile(res, path.join(root, "index.html"));
});

server.listen(port, host, () => {
  console.log(`user-h5 preview ready at http://${host}:${port}/ (api -> ${apiTarget.origin})`);
});
