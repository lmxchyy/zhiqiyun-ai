import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";

const optionsSource = await readFile(new URL("../admin-vue/src/utils/videoGeneration.ts", import.meta.url), "utf8");
const appSource = await readFile(new URL("../admin-vue/src/App.vue", import.meta.url), "utf8");

test("admin exposes preview and per-second Grok models as distinct contracts", () => {
  assert.ok(optionsSource.includes('"Grok Imagine Video 1.5 Preview": "grok-imagine-video-1.5-preview"'));
  assert.ok(optionsSource.includes('"Grok Imagine Video 1.5": "grok-imagine-1.5-video"'));
  assert.match(optionsSource, /length:\s*25[\s\S]*index \+ 6/);
  assert.match(optionsSource, /maxReferenceImages:\s*7/);
  assert.match(optionsSource, /requiresReferenceImage:\s*true/);
  assert.match(optionsSource, /"Grok Imagine Video 1\.5"[\s\S]*supportsAudio:\s*false/);
});

test("admin uploads all references allowed by the selected model", () => {
  assert.match(appSource, /:multiple="videoReferenceLimit > 1"/);
  assert.ok(appSource.includes("videoImageFiles"));
  assert.match(appSource, /Promise\.all\(videoImageFiles\.map/);
  assert.match(appSource, /image_urls:\s*videoReferenceImageUrls/);
  assert.match(appSource, /videoModelRequiresReferenceImage\(selectedVideoModel\.value\)/);
  assert.match(appSource, /videoModelSupportsAudio\.value \? \{/);
});
