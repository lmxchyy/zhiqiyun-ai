import assert from "node:assert/strict";
import test from "node:test";
import { readFile } from "node:fs/promises";

import { v531Capabilities } from "../apps/user-uni/src/config/v531.ts";

test("M6 featured pair stays AI设计 then 自由P图, with AI混剪 after", () => {
  assert.equal(v531Capabilities[0].id, "image");
  assert.equal(v531Capabilities[0].title, "AI设计");
  assert.equal(v531Capabilities[0].routeMode, "image");

  assert.equal(v531Capabilities[1].id, "office");
  assert.equal(v531Capabilities[1].title, "自由P图");
  assert.equal(v531Capabilities[1].routeMode, "infographic");

  const montageIndex = v531Capabilities.findIndex(
    (item) => item.id === "montage" || item.title === "AI混剪",
  );
  assert.ok(montageIndex > 1, "AI混剪 must sit after the featured pair");
  assert.equal(v531Capabilities[montageIndex].routeMode, "montage");
  assert.equal(v531Capabilities[montageIndex].title, "AI混剪");
});

test("smart video mini-program package and navigation are registered", async () => {
  const pagesJson = await readFile(new URL("../apps/user-uni/src/pages.json", import.meta.url), "utf8");
  assert.match(pagesJson, /"root":\s*"packageSmartVideo"/);
  assert.match(pagesJson, /pages\/create/);
  assert.match(pagesJson, /pages\/plan/);
  assert.match(pagesJson, /pages\/render/);

  const routes = await readFile(
    new URL("../apps/user-uni/src/config/miniProgramPages.ts", import.meta.url),
    "utf8",
  );
  assert.match(routes, /montage:\s*"\/packageSmartVideo\/pages\/create"/);

  const apiSource = await readFile(
    new URL("../apps/user-uni/src/api/smart-video.ts", import.meta.url),
    "utf8",
  );
  assert.match(apiSource, /createSmartVideoSdk/);
  assert.match(apiSource, /createMultipartUploadController/);
  assert.doesNotMatch(apiSource, /\buni\.request\b/);
  assert.doesNotMatch(apiSource, /\baxios\b/i);
});
