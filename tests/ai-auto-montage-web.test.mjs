import assert from "node:assert/strict";
import test from "node:test";
import { readFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");

async function read(rel) {
  return readFile(path.join(root, rel), "utf8");
}

test("web console registers AI 自动混剪 module and workbench", async () => {
  const app = await read("admin-vue/src/App.vue");
  assert.match(app, /userSmartVideo/);
  assert.match(app, /AI 自动混剪|AI自动混剪/);
  assert.match(app, /SmartVideoWorkbench/);

  const workbench = await read("admin-vue/src/components/SmartVideoWorkbench.vue");
  assert.match(workbench, /SmartVideoWorkbenchPage/);

  const page = await read("admin-vue/src/components/smart-video/SmartVideoWorkbenchPage.vue");
  assert.match(page, /SmartVideoUploadPanel|upload/i);
  assert.match(page, /SmartVideoStoryboard|storyboard|plan/i);
  assert.match(page, /SmartVideoRenderPanel|render|export/i);
});

test("web smart-video store and API wrap SDK paths without Axios in store", async () => {
  const api = await read("admin-vue/src/api/smartVideo.ts");
  assert.match(api, /video-projects/);

  const store = await read("admin-vue/src/stores/smartVideo.ts");
  assert.match(store, /useSmartVideoStore|defineStore/);
  assert.doesNotMatch(store, /\baxios\b/i);
});

test("web keeps userSmartVideo module registration", async () => {
  const app = await read("admin-vue/src/App.vue");
  assert.match(app, /userSmartVideo/);
  assert.match(app, /AI 自动混剪|AI自动混剪/);
});
