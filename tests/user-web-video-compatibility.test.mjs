import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";

const source = await readFile(
  new URL("../apps/user-uni/src/pages/AiCreationPage.vue", import.meta.url),
  "utf8",
);

test("existing Web video submission does not force audio for unsupported providers", () => {
  assert.doesNotMatch(source, /parameters:\s*\{\s*generate_audio:\s*true\s*,?\s*\}/s);
});

test("Web compatibility fix does not add dynamic parameter UI", () => {
  assert.doesNotMatch(source, /v-for="field in videoParameterFields"/);
  assert.doesNotMatch(source, /v31-video-parameter-panel/);
});
