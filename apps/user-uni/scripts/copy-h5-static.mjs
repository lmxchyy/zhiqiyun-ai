#!/usr/bin/env node
/**
 * uni H5 build only copies a subset of /static (icons/brand/fallbacks).
 * Wireless canvas iframe needs smart-canvas.html + its js/css/vendor assets.
 */
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const sourceRoot = path.join(root, "static");
const destRoot = path.join(root, "dist", "build", "h5", "static");

const entries = [
  "smart-canvas.html",
  "js",
  "css",
  "vendor",
  "images",
];

function ensureDir(dir) {
  fs.mkdirSync(dir, { recursive: true });
}

function copyFileRobust(source, dest) {
  ensureDir(path.dirname(dest));
  const data = fs.readFileSync(source);
  const tmp = `${dest}.${process.pid}.tmp`;
  fs.writeFileSync(tmp, data);
  try {
    fs.renameSync(tmp, dest);
  } catch {
    fs.writeFileSync(dest, data);
    try { fs.unlinkSync(tmp); } catch {}
  }
}

function copyDirRobust(source, dest) {
  ensureDir(dest);
  for (const entry of fs.readdirSync(source, { withFileTypes: true })) {
    const from = path.join(source, entry.name);
    const to = path.join(dest, entry.name);
    if (entry.isDirectory()) copyDirRobust(from, to);
    else if (entry.isFile()) copyFileRobust(from, to);
  }
}

function copyEntry(name) {
  const source = path.join(sourceRoot, name);
  const dest = path.join(destRoot, name);
  if (!fs.existsSync(source)) {
    console.warn(`[copy-h5-static] skip missing: ${name}`);
    return;
  }
  const stat = fs.statSync(source);
  if (stat.isDirectory()) copyDirRobust(source, dest);
  else copyFileRobust(source, dest);
  console.log(`[copy-h5-static] synced ${name}`);
}

if (!fs.existsSync(path.join(root, "dist", "build", "h5"))) {
  console.warn("[copy-h5-static] skip: dist/build/h5 missing (run build:h5 first)");
  process.exit(0);
}
if (!fs.existsSync(sourceRoot)) {
  console.warn(`[copy-h5-static] skip: missing ${sourceRoot}`);
  process.exit(0);
}

ensureDir(destRoot);
for (const entry of entries) copyEntry(entry);
console.log(`[copy-h5-static] done -> ${destRoot}`);
