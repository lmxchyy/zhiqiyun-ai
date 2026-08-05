import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import test from "node:test";

import { neutralizeIgnoredAssets } from "../apps/user-uni/scripts/mp-package-assets.cjs";

test("neutralizeIgnoredAssets empties only generated assets excluded from upload", () => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "xianzhi-mp-assets-"));
  const ignoredFolder = path.join(root, "static", "app-icons");
  const ignoredSuffix = path.join(root, "static", "fallbacks", "unused.webp");
  const retainedFile = path.join(root, "static", "fallbacks", "default-cover.jpg");

  fs.mkdirSync(ignoredFolder, { recursive: true });
  fs.mkdirSync(path.dirname(ignoredSuffix), { recursive: true });
  fs.writeFileSync(path.join(ignoredFolder, "icon.png"), Buffer.alloc(300, 1));
  fs.writeFileSync(path.join(ignoredFolder, "runtime.js"), Buffer.alloc(150, 4));
  fs.writeFileSync(ignoredSuffix, Buffer.alloc(200, 2));
  fs.writeFileSync(retainedFile, Buffer.alloc(100, 3));

  const neutralized = neutralizeIgnoredAssets(root, [
    { type: "folder", value: "static/app-icons" },
    { type: "suffix", value: ".webp" }
  ]);

  assert.deepEqual(neutralized.sort(), [
    "static/app-icons/icon.png",
    "static/fallbacks/unused.webp"
  ]);
  assert.equal(fs.statSync(path.join(ignoredFolder, "icon.png")).size, 0);
  assert.equal(fs.statSync(path.join(ignoredFolder, "runtime.js")).size, 150);
  assert.equal(fs.statSync(ignoredSuffix).size, 0);
  assert.equal(fs.statSync(retainedFile).size, 100);
});
