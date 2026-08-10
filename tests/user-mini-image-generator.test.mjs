import assert from "node:assert/strict";
import test from "node:test";

import { taskRequestFromDraft } from "../packages/business-sdk/dist/mappers.js";
import * as imageCreation from "../apps/user-uni/src/features/generation/imageCreation.ts";

function exactImageSchema(modelName, fields) {
  return {
    module_code: "image_generation",
    model_name: modelName,
    schema: { fields },
    fields,
  };
}

function gptImageSchema(overrides = {}) {
  const fields = overrides.fields || [
    { key: "prompt", type: "textarea", required: true },
    {
      key: "size",
      type: "select",
      required: true,
      default: "1024x1024",
      options: ["1024x1024", "1536x1024", "1024x1536"],
    },
    {
      key: "quality",
      type: "select",
      required: true,
      default: "standard",
      options: ["standard", "high"],
    },
    { key: "n", type: "number", required: true, default: 1, min: 1, max: 8 },
  ];
  return exactImageSchema(overrides.modelName || "gpt-image-2", fields);
}

function enumerableCountSchema() {
  return gptImageSchema({
    fields: [
      {
        key: "size",
        type: "select",
        required: true,
        default: "1024x1024",
        options: ["1024x1024", "1536x1024", "1024x1536"],
      },
      {
        key: "quality",
        type: "select",
        required: true,
        default: "standard",
        options: ["standard", "high"],
      },
      {
        key: "n",
        type: "select",
        required: true,
        default: 1,
        options: [1, 2, 4],
        min: 1,
        max: 4,
      },
    ],
  });
}

function requiredFunction(name) {
  assert.equal(typeof imageCreation[name], "function", `${name} must be exported`);
  return imageCreation[name];
}

function availableContract(schema = gptImageSchema()) {
  const deriveImageCreationContract = requiredFunction("deriveImageCreationContract");
  const contract = deriveImageCreationContract("gpt-image-2", schema);
  assert.equal(contract.available, true, contract.reason);
  return contract;
}

test("image model options keep only explicitly online image-capable models", () => {
  const result = imageCreation.imageModelOptions([
    { code: "gpt-image-2", name: "GPT Image 2", capabilities: ["TEXT_TO_IMAGE"], online: true, pointCost: 10 },
    { code: "seedance", name: "Seedance", capabilities: ["TEXT_TO_VIDEO"], online: true },
    { code: "offline-image", name: "Offline", capabilities: ["IMAGE_TO_IMAGE"], online: false },
    { code: "unknown-online-image", name: "Unknown online state", capabilities: ["IMAGE_TO_IMAGE"] },
  ]);

  assert.deepEqual(result, [{ code: "gpt-image-2", name: "GPT Image 2", pointCost: 10 }]);
});

test("image model selection never switches an unavailable requested model", () => {
  const models = [
    { code: "gpt-image-2", name: "GPT Image 2", pointCost: 10 },
    { code: "seedream-4", name: "Seedream 4.0", pointCost: 12 },
  ];

  assert.equal(imageCreation.resolveImageModelCode(models, "seedream-4"), "seedream-4");
  assert.equal(imageCreation.resolveImageModelCode(models, "removed-model"), "");
  assert.equal(imageCreation.resolveImageModelCode([], "removed-model"), "");
});

test("exact image schema derives real size ratios, canonical qualities, and schema count default", () => {
  const deriveImageCreationContract = requiredFunction("deriveImageCreationContract");
  const contract = deriveImageCreationContract("gpt-image-2", gptImageSchema());

  assert.deepEqual(contract, {
    available: true,
    modelName: "gpt-image-2",
    sizeOptions: [
      { value: "1024x1024", label: "1:1" },
      { value: "1536x1024", label: "3:2" },
      { value: "1024x1536", label: "2:3" },
    ],
    qualityOptions: [
      { value: "standard", label: "standard" },
      { value: "high", label: "high" },
    ],
    countOptions: [{ value: 1, label: "1" }],
    defaultSelection: { size: "1024x1024", quality: "standard", count: 1 },
    declared: { size: true, quality: true, count: true },
    required: { size: true, quality: true, count: true },
  });
});

test("schema option derivation filters malformed dimensions, unsupported qualities, and non-positive counts", () => {
  const deriveImageCreationContract = requiredFunction("deriveImageCreationContract");
  const contract = deriveImageCreationContract("schema-model", exactImageSchema("schema-model", [
    {
      key: "size",
      required: true,
      default: "1280x720",
      options: ["1280x720", "720x1280", "0x720", "1280x0", "4:3", "1280.5x720"],
    },
    {
      key: "quality",
      required: true,
      default: "high",
      options: ["standard", "ultra", "high", "2K"],
    },
    {
      key: "n",
      required: true,
      default: 1,
      options: [1, 2, 4, 0, -1, 1.5, "2"],
      min: 1,
      max: 4,
    },
  ]));

  assert.equal(contract.available, true, contract.reason);
  assert.deepEqual(contract.sizeOptions, [
    { value: "1280x720", label: "16:9" },
    { value: "720x1280", label: "9:16" },
  ]);
  assert.deepEqual(contract.qualityOptions.map(option => option.value), ["standard", "high"]);
  assert.deepEqual(contract.countOptions.map(option => option.value), [1, 2, 4]);
});

test("missing exact schema returns a Chinese unavailable reason", () => {
  const deriveImageCreationContract = requiredFunction("deriveImageCreationContract");
  const contract = deriveImageCreationContract("gpt-image-2", undefined);

  assert.equal(contract.available, false);
  assert.match(contract.reason, /当前模型.*图片参数配置/);
});

test("mismatched schema model returns a Chinese unavailable reason instead of switching models", () => {
  const deriveImageCreationContract = requiredFunction("deriveImageCreationContract");
  const contract = deriveImageCreationContract("gpt-image-2", gptImageSchema({ modelName: "mock-standard" }));

  assert.equal(contract.available, false);
  assert.match(contract.reason, /与所选模型不一致/);
  assert.match(contract.reason, /gpt-image-2/);
  assert.match(contract.reason, /mock-standard/);
});

test("schema without a valid size enum is unavailable and never invents an option", () => {
  const deriveImageCreationContract = requiredFunction("deriveImageCreationContract");
  const contract = deriveImageCreationContract("broken-image", exactImageSchema("broken-image", [
    { key: "size", required: true, default: "auto", options: ["auto", "4:3", "0x1024"] },
  ]));

  assert.deepEqual(contract, {
    available: false,
    reason: "当前模型没有可用的图片尺寸选项",
  });
});

test("selection becomes canonical image fields only when each value is declared and supported", () => {
  const toCanonicalImageSelection = requiredFunction("toCanonicalImageSelection");
  const contract = availableContract(enumerableCountSchema());

  assert.deepEqual(
    toCanonicalImageSelection(contract, { size: "1536x1024", quality: "high", count: 2 }),
    { size: "1536x1024", quality: "high", count: 2 },
  );
});

test("selection omits schema-undeclared quality and count", () => {
  const deriveImageCreationContract = requiredFunction("deriveImageCreationContract");
  const toCanonicalImageSelection = requiredFunction("toCanonicalImageSelection");
  const contract = deriveImageCreationContract("cloudbase-image", exactImageSchema("cloudbase-image", [
    { key: "size", required: true, default: "1280x720", options: ["1024x1024", "1280x720"] },
  ]));
  assert.equal(contract.available, true, contract.reason);

  assert.deepEqual(toCanonicalImageSelection(contract, { size: "1280x720" }), { size: "1280x720" });
});

test("selection fails fast for unsupported, undeclared, missing, and alias values", () => {
  const deriveImageCreationContract = requiredFunction("deriveImageCreationContract");
  const toCanonicalImageSelection = requiredFunction("toCanonicalImageSelection");
  const fullContract = availableContract(enumerableCountSchema());
  const sizeOnlyContract = deriveImageCreationContract("cloudbase-image", exactImageSchema("cloudbase-image", [
    { key: "size", required: true, default: "1280x720", options: ["1280x720"] },
  ]));
  assert.equal(sizeOnlyContract.available, true, sizeOnlyContract.reason);

  const cases = [
    { contract: fullContract, selection: { size: "4:3", quality: "high", count: 2 }, message: /不支持图片尺寸/ },
    { contract: fullContract, selection: { size: "1536x1024", quality: "2K", count: 2 }, message: /不支持图片质量/ },
    { contract: fullContract, selection: { size: "1536x1024", quality: "high", count: 3 }, message: /不支持生成数量/ },
    { contract: fullContract, selection: { quality: "high", count: 2 }, message: /必须选择图片尺寸/ },
    { contract: sizeOnlyContract, selection: { size: "1280x720", quality: "high" }, message: /未声明图片质量/ },
    { contract: fullContract, selection: { size: "1536x1024", quality: "high", count: 2, aspect_ratio: "3:2" }, message: /不支持字段 aspect_ratio/ },
  ];

  for (const item of cases) {
    assert.throws(() => toCanonicalImageSelection(item.contract, item.selection), item.message);
  }
});

test("formal inspiration ratios restore only exact reduced schema ratios", () => {
  const restoreImageInspirationSelection = requiredFunction("restoreImageInspirationSelection");
  const contract = availableContract();
  const cases = [
    { ratio: "1:1", size: "1024x1024" },
    { ratio: "2:3", size: "1024x1536" },
    { ratio: "3:2", size: "1536x1024" },
  ];

  for (const item of cases) {
    assert.deepEqual(
      restoreImageInspirationSelection(contract, { ratio: item.ratio, quality: "high", count: 1 }),
      {
        compatible: true,
        selection: { size: item.size, quality: "high", count: 1 },
        canonical: { size: item.size, quality: "high", count: 1 },
      },
    );
  }
});

test("unsupported inspiration ratio returns a Chinese reason and no requestable canonical data", () => {
  const restoreImageInspirationSelection = requiredFunction("restoreImageInspirationSelection");
  const result = restoreImageInspirationSelection(availableContract(), {
    ratio: "4:3",
    quality: "high",
    count: 1,
  });

  assert.equal(result.compatible, false);
  assert.match(result.reason, /当前模型不支持灵感比例 4:3/);
  assert.equal("canonical" in result, false);
});

test("inspiration restore reads only ratio, quality, and count rather than deprecated aliases", () => {
  const restoreImageInspirationSelection = requiredFunction("restoreImageInspirationSelection");
  const result = restoreImageInspirationSelection(availableContract(), {
    aspectRatio: "1:1",
    aspect_ratio: "1:1",
    imageRatio: "1:1",
    imageQuality: "high",
    count: 1,
  });

  assert.equal(result.compatible, false);
  assert.match(result.reason, /灵感缺少有效的 ratio/);
  assert.equal("canonical" in result, false);
});

test("canonical helper output reaches the compiled business SDK body without aliases", () => {
  const toCanonicalImageSelection = requiredFunction("toCanonicalImageSelection");
  const canonical = toCanonicalImageSelection(availableContract(enumerableCountSchema()), {
    size: "1536x1024",
    quality: "high",
    count: 2,
  });
  const request = taskRequestFromDraft({
    mode: "image",
    prompt: "生成橙色系水果店开业海报",
    model: "gpt-image-2",
    style: "commercial",
    referenceImages: [],
    ...canonical,
    parameters: {
      ratio: "4:3",
      aspectRatio: "16:9",
      aspect_ratio: "16:9",
      imageRatio: "auto",
      imageQuality: "2K",
    },
  });

  assert.deepEqual(request, {
    type: "TEXT_TO_IMAGE",
    moduleCode: "image_generation",
    prompt: "生成橙色系水果店开业海报",
    model: "gpt-image-2",
    params: {
      size: "1536x1024",
      quality: "high",
      n: 2,
    },
  });
});

test("image estimate copy never multiplies client-side point cost", () => {
  assert.equal(
    imageCreation.imagePointEstimateLabel({ code: "gpt-image-2", name: "GPT Image 2", pointCost: 10 }, 4),
    "以生成时结算为准",
  );
  assert.equal(imageCreation.imagePointEstimateLabel(undefined, 1), "以生成时结算为准");
});

function canonicalFingerprintInput(count = 2) {
  return {
    model: "gpt-image-2",
    prompt: "生成橙色系水果店开业海报",
    referenceImages: ["https://example.test/reference.png"],
    selection: { size: "1536x1024", quality: "high", count },
  };
}

test("first image attempt creates an image-prefixed client request key", () => {
  const imageRequestFingerprint = requiredFunction("imageRequestFingerprint");
  const nextImageClientRequestKey = requiredFunction("nextImageClientRequestKey");
  const fingerprint = imageRequestFingerprint(canonicalFingerprintInput());

  assert.deepEqual(
    nextImageClientRequestKey({ fingerprint }, () => "uuid-first"),
    { fingerprint, clientRequestId: "image_uuid-first" },
  );
});

test("network-uncertain retry reuses the existing key for an identical canonical fingerprint", () => {
  const imageRequestFingerprint = requiredFunction("imageRequestFingerprint");
  const nextImageClientRequestKey = requiredFunction("nextImageClientRequestKey");
  const fingerprint = imageRequestFingerprint(canonicalFingerprintInput());
  const existing = { fingerprint, clientRequestId: "image_uuid-existing" };
  let factoryCalls = 0;

  const result = nextImageClientRequestKey(
    { fingerprint, existing, previousOutcome: "network-uncertain" },
    () => {
      factoryCalls += 1;
      return "must-not-be-used";
    },
  );

  assert.deepEqual(result, existing);
  assert.equal(factoryCalls, 0);
});

test("changed canonical input creates a new key even after a network-uncertain result", () => {
  const imageRequestFingerprint = requiredFunction("imageRequestFingerprint");
  const nextImageClientRequestKey = requiredFunction("nextImageClientRequestKey");
  const oldFingerprint = imageRequestFingerprint(canonicalFingerprintInput(1));
  const nextFingerprint = imageRequestFingerprint(canonicalFingerprintInput(2));

  assert.deepEqual(
    nextImageClientRequestKey({
      fingerprint: nextFingerprint,
      existing: { fingerprint: oldFingerprint, clientRequestId: "image_uuid-existing" },
      previousOutcome: "network-uncertain",
    }, () => "uuid-changed"),
    { fingerprint: nextFingerprint, clientRequestId: "image_uuid-changed" },
  );
});

test("retry after terminal failure creates a new key for the same canonical input", () => {
  const imageRequestFingerprint = requiredFunction("imageRequestFingerprint");
  const nextImageClientRequestKey = requiredFunction("nextImageClientRequestKey");
  const fingerprint = imageRequestFingerprint(canonicalFingerprintInput());

  assert.deepEqual(
    nextImageClientRequestKey({
      fingerprint,
      existing: { fingerprint, clientRequestId: "image_uuid-existing" },
      previousOutcome: "terminal-failure",
    }, () => "uuid-retry"),
    { fingerprint, clientRequestId: "image_uuid-retry" },
  );
});
