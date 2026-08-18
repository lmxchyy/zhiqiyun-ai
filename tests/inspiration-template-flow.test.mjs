import assert from "node:assert/strict";
import test from "node:test";

import * as inspirationDraft from "../apps/user-uni/src/features/inspiration/draft.ts";
import * as inspirationHandoff from "../apps/user-uni/src/features/inspiration/handoff.ts";
const inspirationContracts = await import("../apps/user-uni/src/features/inspiration/contracts.ts").catch(() => ({}));
const inspirationReferenceAsset = await import("../apps/user-uni/src/features/inspiration/referenceAsset.ts").catch(() => ({}));

function exported(module, name) {
  assert.equal(typeof module[name], "function", `${name} must be exported`);
  return module[name];
}

const publicDetailFixture = {
  item: {
    id: "template-image-1",
    slug: "product-hero",
    title: "商品主图",
    description: "上传商品图，生成简洁主图",
    contentType: "image",
    categoryId: "product",
    coverUrl: "/cover.png",
    resultUrl: "/result.png",
    platforms: ["miniprogram"],
    tags: ["电商"],
    templateVersion: 3,
    favorite: false,
    viewCount: 9,
    favoriteCount: 2,
    useCount: 4,
    generateCount: 3,
    schema: {
      inputs: [
        {
          key: "references",
          type: "IMAGE",
          label: "商品图片",
          required: true,
          section: "materials",
          order: 10,
          validation: { minItems: 1, maxItems: 2, accept: ["image/png", "image/jpeg"] },
        },
        { key: "subject", type: "TEXT", label: "画面要求", required: true, section: "requirements", order: 20 },
        {
          key: "style",
          type: "SELECT",
          control: "SEGMENTED",
          label: "风格",
          default: "clean",
          section: "preferences",
          order: 30,
          options: [{ label: "简洁", value: "clean" }, { label: "自然", value: "natural" }],
        },
        {
          key: "strength",
          type: "NUMBER",
          control: "SLIDER",
          label: "效果强度",
          default: 60,
          section: "advanced",
          order: 40,
          advanced: true,
          validation: { min: 0, max: 100 },
          visibleWhen: { inputKey: "style", operator: "eq", value: "clean" },
        },
        { key: "protectLogo", type: "BOOLEAN", label: "保留标识", default: true, section: "preferences", order: 35 },
      ],
      presentation: { heroLabel: "示例结果" },
      presets: { inputDefaults: { subject: "白色背景" } },
      handoff: { targetType: "IMAGE_CREATION" },
    },
    prompt: "must not be consumed",
    composer: { key: "must-not-leak" },
    bindings: [{ source: "subject", target: "prompt" }],
    modelHint: "must-not-leak",
    executorKey: "must-not-leak",
    workflow: { nodes: [] },
    failurePolicy: { strategy: "fail" },
    definition_json: { prompt: "must-not-leak" },
  },
  aiGenerated: true,
};

test("public detail parser keeps only the public projection", () => {
  const normalize = exported(inspirationContracts, "normalizePublicTemplateDetailResponse");
  if (!normalize) return;
  const detail = normalize(publicDetailFixture);
  assert.equal(detail.item.slug, "product-hero");
  assert.equal(detail.item.schema.inputs.length, 5);
  for (const forbidden of ["prompt", "composer", "bindings", "modelHint", "executorKey", "workflow", "failurePolicy", "definition_json"]) {
    assert.equal(forbidden in detail.item, false, `frontend DTO retained ${forbidden}`);
  }
});

test("schema renderer resolves text, select, boolean, slider and asset controls", () => {
  const resolve = exported(inspirationContracts, "templateInputControl");
  if (!resolve) return;
  assert.equal(resolve({ type: "TEXT" }), "TEXT");
  assert.equal(resolve({ type: "TEXTAREA" }), "TEXTAREA");
  assert.equal(resolve({ type: "SELECT" }), "SELECT");
  assert.equal(resolve({ type: "SELECT", control: "SEGMENTED" }), "SEGMENTED");
  assert.equal(resolve({ type: "BOOLEAN" }), "BOOLEAN");
  assert.equal(resolve({ type: "NUMBER", control: "SLIDER" }), "SLIDER");
  assert.equal(resolve({ type: "IMAGE" }), "ASSET_UPLOAD");
});

test("schema inputs initialize, sort, group and react to visibility without scenario branches", () => {
  const initialValues = exported(inspirationContracts, "templateInitialValues");
  const groups = exported(inspirationContracts, "groupTemplateInputs");
  if (!initialValues || !groups) return;
  const inputs = publicDetailFixture.item.schema.inputs;
  const values = initialValues(inputs, publicDetailFixture.item.schema.presets.inputDefaults);
  assert.deepEqual(values, { subject: "白色背景", style: "clean", strength: 60, protectLogo: true });
  const visibleGroups = groups(inputs, values);
  assert.deepEqual(visibleGroups.map(group => group.key), ["materials", "requirements", "preferences", "advanced"]);
  assert.deepEqual(visibleGroups.flatMap(group => group.inputs.map(input => input.key)), ["references", "subject", "style", "protectLogo", "strength"]);
  assert.equal(groups(inputs, { ...values, style: "natural" }).flatMap(group => group.inputs).some(input => input.key === "strength"), false);
});

test("schema validation enforces required values plus asset count and MIME", () => {
  const validate = exported(inspirationContracts, "validateTemplateInputValues");
  if (!validate) return;
  const inputs = publicDetailFixture.item.schema.inputs;
  const missing = validate(inputs, { style: "clean" }, {});
  assert.equal(missing.subject, "请填写画面要求");
  assert.equal(missing.references, "请上传商品图片");
  const wrongMime = validate(inputs, { subject: "白底", style: "clean" }, {
    references: [{ assetId: "asset-1", mimeType: "image/gif", status: "uploaded" }],
  });
  assert.equal(wrongMime.references, "商品图片格式不受支持");
  assert.deepEqual(validate(inputs, { subject: "白底", style: "clean", strength: 60 }, {
    references: [{ assetId: "asset-1", mimeType: "image/png", status: "uploaded" }],
  }), {});
});

test("asset upload capacity can satisfy minItems when maxItems is omitted", () => {
  const maximum = exported(inspirationContracts, "templateAssetMaxItems");
  if (!maximum) return;
  assert.equal(maximum({ type: "IMAGE", validation: { minItems: 3 } }), 3);
  assert.equal(maximum({ type: "IMAGE", validation: { minItems: 2, maxItems: 5 } }), 5);
  assert.equal(maximum({ type: "IMAGE" }), 1);
});

test("compose body contains only the template version, values and asset references", () => {
  const build = exported(inspirationContracts, "buildInspirationComposeRequest");
  if (!build) return;
  const body = build(3, { subject: "白底", style: "clean" }, {
    references: [{ assetId: "asset-1", mimeType: "image/png", status: "uploaded", previewUrl: "https://private/url.png" }],
  });
  assert.deepEqual(body, {
    templateVersion: 3,
    values: { subject: "白底", style: "clean" },
    materials: [{ inputKey: "references", assetId: "asset-1" }],
  });
  assert.equal("prompt" in body, false);
  assert.equal(JSON.stringify(body).includes("private/url.png"), false);
});

test("creation draft is saved verbatim and expires without client-side prompt composition", () => {
  const save = exported(inspirationDraft, "saveInspirationDraft");
  const read = exported(inspirationDraft, "readInspirationDraft");
  if (!save || !read) return;
  let stored;
  globalThis.uni = {
    setStorageSync(_key, value) { stored = value; },
    getStorageSync() { return stored; },
    removeStorageSync() { stored = undefined; },
  };
  const draft = {
    contractVersion: 1,
    templateRef: { id: "template-image-1", slug: "product-hero", version: 3 },
    contentType: "image",
    handoff: { targetType: "IMAGE_CREATION", targetKey: "image_creation", intentKey: "product_image" },
    values: { subject: "白底" },
    materials: [{ inputKey: "references", assetId: "asset-1" }],
    basePrompt: "SERVER COMPOSED PROMPT",
    negativePrompt: "SERVER NEGATIVE PROMPT",
    parameters: { size: "1024x1024" },
    capabilityKey: "image_generation",
    modelHint: "gpt-image-2",
    integrityToken: "signed-token",
    createdAt: "2026-08-12T00:00:00Z",
    expiresAt: "2026-08-12T00:30:00Z",
  };
  let saved;
  assert.doesNotThrow(() => { saved = save(draft, Date.parse("2026-08-12T00:10:00Z")); });
  assert.deepEqual(saved, draft);
  assert.deepEqual(stored, draft);
  assert.equal(read("template-image-1", Date.parse("2026-08-12T00:29:59Z")).basePrompt, "SERVER COMPOSED PROMPT");
  assert.equal(read("template-image-1", Date.parse("2026-08-12T00:30:00Z")), null);
  assert.throws(() => save(draft, Date.parse("2026-08-12T00:30:00Z")), /已过期/);
  stored = { ...draft, contractVersion: 2, basePrompt: "CLIENT FORGED" };
  assert.equal(read("template-image-1", Date.parse("2026-08-12T00:10:00Z")), null);
});

test("image handoff restores server prompt, parameters and owned asset references", async () => {
  const resolveMaterials = exported(inspirationDraft, "resolveInspirationDraftMaterialURLs");
  const route = exported(inspirationHandoff, "inspirationDraftRoute");
  if (!resolveMaterials || !route) return;
  const requested = [];
  const draft = {
    contractVersion: 1,
    templateRef: { id: "template-image-1", slug: "product-hero", version: 3 },
    contentType: "image",
    handoff: { targetType: "IMAGE_CREATION", intentKey: "optional-hint-only" },
    values: { subject: "白底" },
    materials: [{ inputKey: "references", assetId: "asset-1" }],
    basePrompt: "SERVER COMPOSED PROMPT",
    parameters: { size: "1024x1024" },
    capabilityKey: "image_generation",
    modelHint: "gpt-image-2",
    integrityToken: "signed-token",
    createdAt: "2026-08-12T00:00:00Z",
    expiresAt: "2026-08-12T00:30:00Z",
  };
  const urls = await resolveMaterials(draft, async assetId => {
    requested.push(assetId);
    return { remoteUrl: "/api/v1/reference-images/owned.png" };
  }, value => `https://assets.example${value}`);
  assert.deepEqual(requested, ["asset-1"]);
  assert.deepEqual(urls, ["https://assets.example/api/v1/reference-images/owned.png"]);
  assert.equal(route(draft), "/pages/user/UserImageCreationPage?templateId=template-image-1");
  assert.equal(draft.basePrompt, "SERVER COMPOSED PROMPT");
  assert.deepEqual(draft.parameters, { size: "1024x1024" });
  assert.equal(draft.capabilityKey, "image_generation");
  assert.equal(draft.modelHint, "gpt-image-2");
});

test("compose errors distinguish authentication, version conflict, input, material and schema failures", () => {
  const action = exported(inspirationContracts, "inspirationComposeErrorAction");
  if (!action) return;
  assert.equal(action({ statusCode: 401, payload: { code: "AUTH_REQUIRED" } }), "auth");
  assert.equal(action({ statusCode: 409, payload: { code: "INSPIRATION_TEMPLATE_VERSION_CONFLICT" } }), "reload");
  assert.equal(action({ statusCode: 422, payload: { code: "INPUT_REQUIRED" } }), "input");
  assert.equal(action({ statusCode: 422, payload: { code: "INSPIRATION_MATERIAL_INVALID" } }), "material");
  assert.equal(action({ statusCode: 500, payload: { code: "INSPIRATION_TEMPLATE_DEFINITION_INVALID" } }), "schema");
  assert.equal(action({ statusCode: 0 }), "network");
});

test("use_template event failure does not block IMAGE_CREATION navigation", async () => {
  const handoff = exported(inspirationHandoff, "completeInspirationHandoff");
  if (!handoff) return;
  const calls = [];
  const draft = {
    templateRef: { id: "template-image-1", slug: "product-hero", version: 3 },
    contentType: "image",
    handoff: { targetType: "IMAGE_CREATION" },
  };
  const url = await handoff(draft, {
    save(value) { calls.push(["save", value.templateRef.id]); },
    recordUse() { calls.push(["event"]); return Promise.reject(new Error("analytics unavailable")); },
    navigate(target) { calls.push(["navigate", target]); },
  });
  assert.equal(url, "/pages/user/UserImageCreationPage?templateId=template-image-1");
  assert.deepEqual(calls, [
    ["save", "template-image-1"],
    ["event"],
    ["navigate", "/pages/user/UserImageCreationPage?templateId=template-image-1"],
  ]);
});

test("reference upload requires a server-owned asset id and never treats URL as authority", () => {
  const normalize = exported(inspirationReferenceAsset, "referenceAssetFromPayload");
  if (!normalize) return;
  assert.deepEqual(normalize({ item: { assetId: "asset-1", name: "photo.png", url: "/reference/photo.png", contentType: "image/png" } }), {
    assetId: "asset-1",
    name: "photo.png",
    previewUrl: "/reference/photo.png",
    mimeType: "image/png",
  });
  assert.throws(() => normalize({ item: { url: "/reference/no-id.png" } }), /assetId/);
});
