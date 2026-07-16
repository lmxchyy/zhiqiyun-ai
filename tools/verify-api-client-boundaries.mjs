import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const repoRoot = path.resolve(fileURLToPath(new URL("..", import.meta.url)));
const sourceRoots = ["admin-vue/src", "apps/user-uni/src"];
const allowedTransportFiles = new Set([
  "admin-vue/src/api/client.ts",
  "apps/user-uni/src/api/client.ts",
]);
const sourceExtensions = new Set([".js", ".jsx", ".ts", ".tsx", ".vue"]);
const transportPatterns = [
  { label: "fetch", expression: /\bfetch\s*\(/g },
  { label: "XMLHttpRequest", expression: /\bXMLHttpRequest\b/g },
  { label: "uni.request/uploadFile/downloadFile", expression: /\buni\s*\.\s*(?:request|uploadFile|downloadFile)\s*\(/g },
  { label: "direct axios method", expression: /\baxios\s*\.\s*(?:get|post|put|patch|delete|request)\s*\(/g },
];

function sourceFiles(relativeRoot) {
  const pending = [path.join(repoRoot, relativeRoot)];
  const files = [];
  while (pending.length) {
    const current = pending.pop();
    for (const entry of fs.readdirSync(current, { withFileTypes: true })) {
      const absolutePath = path.join(current, entry.name);
      if (entry.isDirectory()) pending.push(absolutePath);
      else if (sourceExtensions.has(path.extname(entry.name))) files.push(absolutePath);
    }
  }
  return files;
}

const violations = [];
for (const absolutePath of sourceRoots.flatMap(sourceFiles)) {
  const relativePath = path.relative(repoRoot, absolutePath).replaceAll("\\", "/");
  if (allowedTransportFiles.has(relativePath)) continue;
  const source = fs.readFileSync(absolutePath, "utf8");
  for (const { label, expression } of transportPatterns) {
    expression.lastIndex = 0;
    for (let match = expression.exec(source); match; match = expression.exec(source)) {
      const line = source.slice(0, match.index).split(/\r?\n/).length;
      violations.push(`${relativePath}:${line} uses ${label}`);
    }
  }
}

if (violations.length) {
  console.error("API client boundary violations found:\n" + violations.map(item => `- ${item}`).join("\n"));
  process.exitCode = 1;
} else {
  console.log("API client boundary check passed: raw transports are confined to the approved client modules.");
}
