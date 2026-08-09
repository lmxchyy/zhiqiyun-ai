import assert from "node:assert/strict";
import test from "node:test";

import {
  imageAspectOptions,
  imageCountOptions,
  imageModelOptions,
  imagePointEstimateLabel,
  imageQualityOptions,
  resolveImageModelCode,
} from "../apps/user-uni/src/features/generation/imageCreation.ts";

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
