import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

import {
  imageAspectOptions,
  imageCountOptions,
  imageModelOptions,
  imagePointEstimateLabel,
  imageQualityOptions,
  resolveImageModelCode,
} from "../apps/user-uni/src/features/generation/imageCreation.ts";

const componentURL = new URL("../apps/user-uni/src/components/creation/AiImageGenerator.vue", import.meta.url);

test("image creation exposes approved defaults and options", () => {
  assert.equal(imageAspectOptions[0].value, "auto");
  assert.deepEqual(imageAspectOptions.map(item => item.value), ["auto", "1:1", "16:9", "9:16", "4:3"]);
  assert.deepEqual(imageQualityOptions, ["1K", "2K"]);
  assert.deepEqual(imageCountOptions, [1, 2, 4]);
});

test("image model options keep only online image-capable models", () => {
  const result = imageModelOptions([
    { code: "gpt-image-2", name: "GPT Image 2", capabilities: ["TEXT_TO_IMAGE"], online: true, pointCost: 10 },
    { code: "seedance", name: "Seedance", capabilities: ["TEXT_TO_VIDEO"], online: true },
    { code: "offline-image", name: "Offline", capabilities: ["IMAGE_TO_IMAGE"], online: false },
    { code: "unknown-online-image", name: "Unknown online state", capabilities: ["IMAGE_TO_IMAGE"] },
  ]);
  assert.deepEqual(result, [{ code: "gpt-image-2", name: "GPT Image 2", pointCost: 10 }]);
});

test("image model selection preserves an available request and otherwise uses the first model", () => {
  const models = [
    { code: "gpt-image-2", name: "GPT Image 2", pointCost: 10 },
    { code: "seedream-4", name: "Seedream 4.0", pointCost: 12 },
  ];
  assert.equal(resolveImageModelCode(models, "seedream-4"), "seedream-4");
  assert.equal(resolveImageModelCode(models, "removed-model"), "gpt-image-2");
  assert.equal(resolveImageModelCode([], "removed-model"), "");
});

test("image estimate never invents a missing price", () => {
  assert.equal(imagePointEstimateLabel({ code: "gpt-image-2", name: "GPT Image 2", pointCost: 10 }, 2), "预计 20 积分");
  assert.equal(imagePointEstimateLabel(undefined, 1), "以生成时结算为准");
});

test("AI image generator renders the approved structure and defaults", async () => {
  const source = await readFile(componentURL, "utf8");
  for (const text of ["AI生图", "今天想生成什么？", "添加参考", "画幅比例", "图片清晰度", "模型", "张数", "生成图片"]) {
    assert.ok(source.includes(text), `missing copy: ${text}`);
  }
  assert.match(source, /例如：生成一张水果店开业促销海报，橙色系，高级感/);
  assert.match(source, /imageAspectOptions/);
  assert.match(source, /imageQualityOptions/);
  assert.match(source, /imageCountOptions/);
});

test("AI image generator exposes controlled interactions and accessibility states", async () => {
  const source = await readFile(componentURL, "utf8");
  for (const event of ["back", "choose-reference", "remove-reference", "preview-reference", "optimize", "generate", "update:prompt", "update:aspectRatio", "update:quality", "update:model", "update:count"]) {
    assert.ok(source.includes(`"${event}"`), `missing emit: ${event}`);
  }
  assert.match(source, /aria-pressed/);
  assert.match(source, /aria-live="polite"/);
  assert.match(source, /disabledReason/);
  assert.match(source, /env\(safe-area-inset-bottom\)/);
});

test("AI image generator locks the approved visual tokens", async () => {
  const source = await readFile(componentURL, "utf8");
  assert.match(source, /--image-brand:\s*#423499/);
  assert.match(source, /--image-action:\s*#ff771b/i);
  assert.match(source, /--image-radius:\s*16px/);
  assert.match(source, /color:\s*#231000/);
  assert.match(source, /min-height:\s*44px/);
  assert.match(source, /:focus-visible/);
  assert.match(source, /prefers-reduced-motion/);
});
