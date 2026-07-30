import assert from "node:assert/strict";
import test from "node:test";

import * as draftModule from "../apps/user-uni/src/features/inspiration/draft.ts";
import * as inspirationTypes from "../apps/user-uni/src/features/inspiration/types.ts";

const { saveInspirationDraft } = draftModule;

test("photo restoration draft keeps UI requirements separate and does not preload the example image", () => {
  let stored;
  globalThis.uni = {
    setStorageSync(_key, value) {
      stored = value;
    },
  };

  const draft = saveInspirationDraft({
    modelAvailable: true,
    compatibleModelId: "",
    aiGenerated: true,
    item: {
      id: "template-photo-restoration",
      title: "老照片修复",
      description: "让旧照片恢复清晰色彩",
      contentType: "image",
      categoryId: "inspiration-category-image-enhancement",
      coverUrl: "https://example.com/cover.webp",
      prompt: "restore the uploaded old photo",
      negativePrompt: "do not change identity",
      modelId: "image-edit-model",
      scenarioCode: "photo_restoration",
      displayConfig: {
        comparisonMode: "side_by_side",
        beforeUrl: "https://example.com/before.jpg",
        afterUrl: "https://example.com/after.jpg",
      },
      inputRequirements: {
        referenceImageRequired: true,
        referenceImageMin: 1,
        referenceImageMax: 1,
      },
      presetConfig: {
        colorMode: "natural",
        identityProtection: true,
      },
      parameters: { size: "1024x1024", quality: "high" },
      referenceAssets: ["https://example.com/example-only.jpg"],
      favorite: false,
      viewCount: 0,
      favoriteCount: 0,
      useCount: 0,
      generateCount: 0,
    },
  });

  assert.equal(draft.scenarioCode, "photo_restoration");
  assert.deepEqual(draft.inputRequirements, {
    referenceImageRequired: true,
    referenceImageMin: 1,
    referenceImageMax: 1,
  });
  assert.equal(draft.displayConfig?.comparisonMode, "side_by_side");
  assert.equal(draft.presetConfig?.colorMode, "natural");
  assert.deepEqual(draft.referenceAssets, []);
  assert.deepEqual(stored, draft);
});

test("photo restoration requires one user photo before generation", () => {
  const validate = draftModule.inspirationReferenceValidationMessage;
  assert.equal(typeof validate, "function");
  if (typeof validate !== "function") return;

  const draft = {
    scenarioCode: "photo_restoration",
    inputRequirements: {
      referenceImageRequired: true,
      referenceImageMin: 1,
      referenceImageMax: 1,
    },
  };

  assert.equal(validate(draft, 0), "请先上传需要修复的照片");
  assert.equal(validate(draft, 1), "");
  assert.equal(validate(draft, 2), "最多上传 1 张参考图片");
});

test("photo restoration exposes the configured reference image limit to the creation page", () => {
  const resolveLimit = draftModule.inspirationReferenceLimit;
  assert.equal(typeof resolveLimit, "function");
  if (typeof resolveLimit !== "function") return;

  assert.equal(resolveLimit({
    scenarioCode: "photo_restoration",
    inputRequirements: {
      referenceImageRequired: true,
      referenceImageMin: 1,
      referenceImageMax: 1,
    },
  }), 1);
  assert.equal(resolveLimit(null), 3);
});

test("before and after comparison is enabled only when both images are configured", () => {
  const resolve = inspirationTypes.inspirationComparisonSources;
  assert.equal(typeof resolve, "function");
  if (typeof resolve !== "function") return;

  assert.deepEqual(resolve({
    comparisonMode: "side_by_side",
    beforeUrl: " https://example.com/before.jpg ",
    afterUrl: "https://example.com/after.jpg",
  }), {
    mode: "side_by_side",
    beforeUrl: "https://example.com/before.jpg",
    afterUrl: "https://example.com/after.jpg",
  });
  assert.equal(resolve({ comparisonMode: "side_by_side", beforeUrl: "https://example.com/before.jpg" }), null);
});
