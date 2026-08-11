import assert from "node:assert/strict";
import test from "node:test";
import { readFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";

import { v531Capabilities } from "../apps/user-uni/src/config/v531.ts";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");

async function read(rel) {
  return readFile(path.join(root, rel), "utf8");
}

test("M6 featured pair remains AI设计 then 自由P图 before AI混剪", () => {
  assert.equal(v531Capabilities[0].title, "AI设计");
  assert.equal(v531Capabilities[1].title, "自由P图");
  const featured = v531Capabilities.slice(0, 2).map((item) => item.title);
  assert.deepEqual(featured, ["AI设计", "自由P图"]);

  const montageIndex = v531Capabilities.findIndex(
    (item) => item.id === "montage" || item.title === "AI混剪",
  );
  assert.ok(montageIndex > 1);
  assert.equal(v531Capabilities[montageIndex].title, "AI混剪");
  assert.equal(v531Capabilities[montageIndex].routeMode, "montage");
});

test("mini packageSmartVideo pages and SDK wiring are present", async () => {
  const pagesJson = await read("apps/user-uni/src/pages.json");
  assert.match(pagesJson, /"root":\s*"packageSmartVideo"/);
  assert.match(pagesJson, /pages\/create/);
  assert.match(pagesJson, /pages\/plan/);
  assert.match(pagesJson, /pages\/render/);

  const createPage = await read("apps/user-uni/src/packageSmartVideo/pages/create.vue");
  const planPage = await read("apps/user-uni/src/packageSmartVideo/pages/plan.vue");
  const renderPage = await read("apps/user-uni/src/packageSmartVideo/pages/render.vue");
  for (const source of [createPage, planPage, renderPage]) {
    assert.doesNotMatch(source, /\buni\.request\b/);
    assert.doesNotMatch(source, /\baxios\b/i);
  }

  const api = await read("apps/user-uni/src/api/smart-video.ts");
  assert.match(api, /createSmartVideoSdk/);
  assert.doesNotMatch(api, /\buni\.request\b/);
});
