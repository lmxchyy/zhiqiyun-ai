import test from "node:test";
import assert from "node:assert/strict";
import {
  generationParametersFromDraft,
  taskRequestFromDraft,
} from "../packages/business-sdk/dist/mappers.js";

test("home creation draft metadata is not sent as model parameters", () => {
  const request = taskRequestFromDraft({
    mode: "image",
    prompt: "generate a product image",
    model: "gpt-image-2",
    style: "commercial",
    size: "1024x1024",
    quality: "standard",
    count: 1,
    referenceImages: [],
    parameters: {
      mode: "image",
      prompt: "generate a product image",
      referencePaths: [],
      files: [],
    },
  });

  assert.deepEqual(request.params, {
    size: "1024x1024",
    quality: "standard",
    n: 1,
  });
});

test("template parameters survive while navigation fields are removed", () => {
  const params = generationParametersFromDraft({
    mode: "image",
    prompt: "template prompt",
    model: "gpt-image-2",
    referenceImages: ["https://example.test/reference.png"],
    seed: 42,
    custom_schema_parameter: "preserved",
  });

  assert.deepEqual(params, {
    seed: 42,
    custom_schema_parameter: "preserved",
  });
});

test("asset recreation maps source ids to accepted provenance parameters", () => {
  const params = generationParametersFromDraft({
    intent: "regenerate",
    index: 0,
    sourceAssetId: "asset-1",
    sourceTaskId: "task-1",
    aspectRatio: "1:1",
    restoredParams: { seed: 7 },
  });

  assert.deepEqual(params, {
    seed: 7,
    sourceReferenceAssetId: "asset-1",
    sourceReferenceTaskId: "task-1",
  });
});

test("provider output metadata is not replayed as generation parameters", () => {
  const params = generationParametersFromDraft({
    restoredParams: {
      seed: 7,
      providerRevisedPrompt: "provider rewritten prompt",
      provider_revised_prompt: "provider rewritten prompt",
    },
  });

  assert.deepEqual(params, { seed: 7 });
});
