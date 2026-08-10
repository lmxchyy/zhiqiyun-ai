import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { createRequire } from "node:module";
import test from "node:test";

import { taskRequestFromDraft } from "../packages/business-sdk/dist/mappers.js";
import * as imageCreation from "../apps/user-uni/src/features/generation/imageCreation.ts";

const requireFromUserUni = createRequire(new URL("../apps/user-uni/package.json", import.meta.url));
const vueRuntime = requireFromUserUni("vue");
const vueCompiler = requireFromUserUni("@vue/compiler-sfc");
const typescript = requireFromUserUni("typescript");

function loadAiImageGeneratorComponent() {
  const componentURL = new URL("../apps/user-uni/src/components/creation/AiImageGenerator.vue", import.meta.url);
  const source = readFileSync(componentURL, "utf8");
  const { descriptor, errors } = vueCompiler.parse(source, { filename: componentURL.pathname });
  assert.deepEqual(errors, []);
  const script = vueCompiler.compileScript(descriptor, {
    id: "user-mini-ai-image-generator-test",
    inlineTemplate: true,
    templateOptions: {
      compilerOptions: {
        isCustomElement: tag => ["view", "text", "image", "button", "picker", "textarea", "scroll-view"].includes(tag),
      },
    },
  });
  const compiled = typescript.transpileModule(script.content, {
    compilerOptions: {
      module: typescript.ModuleKind.CommonJS,
      target: typescript.ScriptTarget.ES2022,
      esModuleInterop: true,
    },
  }).outputText;
  const module = { exports: {} };
  const localRequire = specifier => {
    if (specifier === "vue") return vueRuntime;
    if (specifier.includes("features/generation/imageCreation")) return imageCreation;
    throw new Error(`unexpected component dependency ${specifier}`);
  };
  // eslint-disable-next-line no-new-func
  new Function("require", "module", "exports", compiled)(localRequire, module, module.exports);
  return module.exports.default;
}

function createHostNode(type, text = "") {
  return { type, text, props: {}, children: [], parent: null };
}

function insertHostNode(child, parent, anchor = null) {
  child.parent = parent;
  const index = anchor ? parent.children.indexOf(anchor) : -1;
  if (index >= 0) parent.children.splice(index, 0, child);
  else parent.children.push(child);
}

function mountAiImageGenerator(props) {
  const emitted = [];
  const renderer = vueRuntime.createRenderer({
    patchProp(node, key, _previous, value) { node.props[key] = value; },
    insert: insertHostNode,
    remove(node) {
      if (!node.parent) return;
      const index = node.parent.children.indexOf(node);
      if (index >= 0) node.parent.children.splice(index, 1);
      node.parent = null;
    },
    createElement: type => createHostNode(type),
    createText: text => createHostNode("#text", text),
    createComment: text => createHostNode("#comment", text),
    setText(node, text) { node.text = text; },
    setElementText(node, text) {
      const child = createHostNode("#text", text);
      child.parent = node;
      node.children = [child];
    },
    parentNode: node => node.parent,
    nextSibling(node) {
      if (!node.parent) return null;
      const index = node.parent.children.indexOf(node);
      return node.parent.children[index + 1] || null;
    },
    querySelector: () => null,
    setScopeId(node, id) { node.props[id] = ""; },
    cloneNode: node => ({ ...node, props: { ...node.props }, children: [...node.children], parent: null }),
    insertStaticContent(content, parent, anchor) {
      const node = createHostNode("#static", content);
      insertHostNode(node, parent, anchor);
      return [node, node];
    },
  });
  const component = loadAiImageGeneratorComponent();
  const root = createHostNode("#root");
  const app = renderer.createApp({
    render() {
      return vueRuntime.h(component, {
        ...props,
        onGenerate: () => emitted.push("generate"),
        onRetry: () => emitted.push("retry"),
      });
    },
  });
  for (const name of ["picker", "scroll-view"]) {
    app.component(name, {
      inheritAttrs: false,
      setup(_componentProps, context) {
        return () => vueRuntime.h(`${name}-host`, context.attrs, context.slots.default?.());
      },
    });
  }
  app.mount(root);
  return { root, emitted, unmount: () => app.unmount() };
}

function hostNodes(node) {
  return [node, ...node.children.flatMap(hostNodes)];
}

function hostClass(node) {
  const value = node.props.class;
  return Array.isArray(value) ? value.flat(Infinity).filter(Boolean).join(" ") : String(value || "");
}

function findHostByClass(root, className) {
  return hostNodes(root).find(node => hostClass(node).split(/\s+/).includes(className));
}

function hostText(node) {
  if (node.type === "#comment") return "";
  return `${node.text || ""}${node.children.map(hostText).join("")}`;
}

function imageComponentProps(overrides = {}) {
  return {
    prompt: "生成橙色系水果店海报",
    size: "1024x1024",
    sizeOptions: [{ value: "1024x1024", label: "1:1" }],
    quality: "standard",
    qualityOptions: [{ value: "standard", label: "标准" }],
    model: "gpt-image-2",
    models: [{ code: "gpt-image-2", name: "GPT Image 2" }],
    count: 1,
    countOptions: [{ value: 1, label: "1" }],
    referenceImages: [],
    referenceLimit: 3,
    busy: false,
    selectingReference: false,
    modelsLoading: false,
    schemaStatus: "ready",
    schemaMessage: "图片参数已就绪",
    disabledReason: "",
    error: "",
    statusMessage: "",
    statusTone: "idle",
    retryAvailable: false,
    estimateLabel: "以生成时结算为准",
    ...overrides,
  };
}

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

function canonicalFingerprintRequest(overrides = {}) {
  const parameters = {
    seed: 42,
    custom_schema_parameter: "warm-light",
    sourceAssetId: "asset-source-1",
    sourceTaskId: "task-source-1",
    ...(overrides.parameters || {}),
  };
  return taskRequestFromDraft({
    mode: "image",
    model: "gpt-image-2",
    prompt: "生成橙色系水果店开业海报",
    style: "commercial",
    size: "1536x1024",
    quality: "high",
    count: 2,
    negativePrompt: "watermark",
    referenceImages: [
      "https://example.test/reference-a.png",
      "https://example.test/reference-b.png",
    ],
    clientRequestId: "image_request_a",
    ...overrides,
    parameters,
  });
}

function reverseObjectKeyInsertion(value) {
  if (Array.isArray(value)) return value.map(reverseObjectKeyInsertion);
  if (!value || typeof value !== "object") return value;
  return Object.fromEntries(
    Object.entries(value)
      .reverse()
      .map(([key, nested]) => [key, reverseObjectKeyInsertion(nested)]),
  );
}

function fingerprintOf(request) {
  const imageRequestFingerprint = requiredFunction("imageRequestFingerprint");
  try {
    return imageRequestFingerprint(request);
  } catch (error) {
    assert.fail(`imageRequestFingerprint must accept a complete canonical request: ${error}`);
  }
}

test("first image attempt creates an image-prefixed client request key", () => {
  const nextImageClientRequestKey = requiredFunction("nextImageClientRequestKey");
  const fingerprint = fingerprintOf(canonicalFingerprintRequest());

  assert.deepEqual(
    nextImageClientRequestKey({ fingerprint }, () => "uuid-first"),
    { fingerprint, clientRequestId: "image_uuid-first" },
  );
});

test("network-uncertain retry reuses the existing key for an identical canonical fingerprint", () => {
  const nextImageClientRequestKey = requiredFunction("nextImageClientRequestKey");
  const fingerprint = fingerprintOf(canonicalFingerprintRequest());
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

test("every complete canonical request semantic changes the fingerprint", () => {
  const baseFingerprint = fingerprintOf(canonicalFingerprintRequest());
  const mutations = [
    {
      name: "negative_prompt",
      request: canonicalFingerprintRequest({ negativePrompt: "text" }),
    },
    {
      name: "arbitrary custom parameter",
      request: canonicalFingerprintRequest({ parameters: { custom_schema_parameter: "cool-light" } }),
    },
    {
      name: "seed custom parameter",
      request: canonicalFingerprintRequest({ parameters: { seed: 99 } }),
    },
    {
      name: "reference image order",
      request: canonicalFingerprintRequest({
        referenceImages: [
          "https://example.test/reference-b.png",
          "https://example.test/reference-a.png",
        ],
      }),
    },
    {
      name: "reference image value",
      request: canonicalFingerprintRequest({
        referenceImages: [
          "https://example.test/reference-a.png",
          "https://example.test/reference-c.png",
        ],
      }),
    },
    {
      name: "source asset provenance",
      request: canonicalFingerprintRequest({ parameters: { sourceAssetId: "asset-source-2" } }),
    },
    {
      name: "source task provenance",
      request: canonicalFingerprintRequest({ parameters: { sourceTaskId: "task-source-2" } }),
    },
  ];

  for (const mutation of mutations) {
    assert.notEqual(
      fingerprintOf(mutation.request),
      baseFingerprint,
      `${mutation.name} must produce a new fingerprint`,
    );
  }
});

test("recursive object key insertion order does not change the fingerprint", () => {
  const request = canonicalFingerprintRequest();

  assert.equal(
    fingerprintOf(reverseObjectKeyInsertion(request)),
    fingerprintOf(request),
  );
});

test("client request id is pure idempotency metadata and does not change the fingerprint", () => {
  assert.equal(
    fingerprintOf(canonicalFingerprintRequest({ clientRequestId: "image_request_a" })),
    fingerprintOf(canonicalFingerprintRequest({ clientRequestId: "image_request_b" })),
  );
});

test("changed negative prompt creates a new key after a network-uncertain result", () => {
  const nextImageClientRequestKey = requiredFunction("nextImageClientRequestKey");
  const oldFingerprint = fingerprintOf(canonicalFingerprintRequest({ negativePrompt: "watermark" }));
  const nextFingerprint = fingerprintOf(canonicalFingerprintRequest({ negativePrompt: "text" }));

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
  const nextImageClientRequestKey = requiredFunction("nextImageClientRequestKey");
  const fingerprint = fingerprintOf(canonicalFingerprintRequest());

  assert.deepEqual(
    nextImageClientRequestKey({
      fingerprint,
      existing: { fingerprint, clientRequestId: "image_uuid-existing" },
      previousOutcome: "terminal-failure",
    }, () => "uuid-retry"),
    { fingerprint, clientRequestId: "image_uuid-retry" },
  );
});

test("schema fetch applies only the latest response for the still-selected exact model", () => {
  const resolveImageSchemaFetchResult = requiredFunction("resolveImageSchemaFetchResult");

  assert.deepEqual(resolveImageSchemaFetchResult({
    requestedModel: "gpt-image-2",
    currentModel: "cloudbase-image",
    requestSequence: 1,
    latestSequence: 2,
    response: gptImageSchema(),
  }), { applied: false });

  const mismatch = resolveImageSchemaFetchResult({
    requestedModel: "gpt-image-2",
    currentModel: "gpt-image-2",
    requestSequence: 3,
    latestSequence: 3,
    response: gptImageSchema({ modelName: "mock-standard" }),
  });
  assert.equal(mismatch.applied, true);
  assert.equal(mismatch.status, "error");
  assert.match(mismatch.message, /与所选模型不一致/);

  const ready = resolveImageSchemaFetchResult({
    requestedModel: "gpt-image-2",
    currentModel: "gpt-image-2",
    requestSequence: 4,
    latestSequence: 4,
    response: gptImageSchema(),
  });
  assert.equal(ready.applied, true);
  assert.equal(ready.status, "ready");
  assert.equal(ready.contract.modelName, "gpt-image-2");
});

test("model schema reset uses declared defaults and otherwise the first canonical option", () => {
  const initialImageSelection = requiredFunction("initialImageSelection");
  const contract = availableContract(exactImageSchema("gpt-image-2", [
    {
      key: "size",
      type: "select",
      required: true,
      options: ["1536x1024", "1024x1024"],
    },
    {
      key: "quality",
      type: "select",
      required: false,
      default: "high",
      options: ["standard", "high"],
    },
    {
      key: "n",
      type: "select",
      required: false,
      options: [2, 4],
      min: 1,
      max: 4,
    },
  ]));

  assert.deepEqual(initialImageSelection(contract), {
    size: "1536x1024",
    quality: "high",
    count: 2,
  });
});

test("production image draft reaches the compiled SDK with canonical top-level fields only", () => {
  const buildCanonicalImageDraft = requiredFunction("buildCanonicalImageDraft");
  const contract = availableContract(enumerableCountSchema());
  const draft = buildCanonicalImageDraft({
    contract,
    selection: { size: "1024x1536", quality: "high", count: 2 },
    prompt: "生成纵向水果促销海报",
    model: "gpt-image-2",
    style: "commercial",
    referenceImages: ["https://example.test/reference.png"],
    negativePrompt: "watermark",
    parameters: {
      ratio: "4:3",
      aspectRatio: "16:9",
      aspect_ratio: "16:9",
      imageRatio: "auto",
      imageQuality: "2K",
      size: "1024x1024",
      quality: "standard",
      count: 4,
      n: 4,
      prompt: "旧草稿提示词",
      model: "old-model",
      modelName: "old-model-name",
      mode: "image",
      contentType: "image",
      referenceImages: ["https://example.test/old-reference.png"],
      referencePaths: ["/tmp/old-reference.png"],
      negativePrompt: "old-negative",
      negative_prompt: "old-negative-alias",
      style: "old-style",
      stylePreset: "old-style-preset",
      seed: 42,
      sourceAssetId: "asset-source-1",
    },
  });

  assert.deepEqual(draft.parameters, {
    seed: 42,
    sourceAssetId: "asset-source-1",
  });

  assert.deepEqual(taskRequestFromDraft(draft), {
    type: "IMAGE_TO_IMAGE",
    moduleCode: "image_generation",
    prompt: "生成纵向水果促销海报",
    model: "gpt-image-2",
    params: {
      size: "1024x1536",
      quality: "high",
      n: 2,
      negative_prompt: "watermark",
      reference_image: "https://example.test/reference.png",
      referenceImages: [
        { url: "https://example.test/reference.png", name: "reference-1" },
      ],
      seed: 42,
      sourceReferenceAssetId: "asset-source-1",
    },
  });
});

test("image generator view state exposes empty, loading, and terminal retry behavior", () => {
  const imageGeneratorViewState = requiredFunction("imageGeneratorViewState");

  assert.deepEqual(imageGeneratorViewState({
    prompt: "   ",
    busy: false,
    disabledReason: "",
    statusTone: "idle",
    statusMessage: "",
    error: "",
    retryAvailable: false,
  }), {
    canSubmit: false,
    disabledReason: "请先描述想生成的图片",
    primaryAction: "generate",
    primaryLabel: "生成图片",
    showSpinner: false,
    showRetry: false,
    tone: "idle",
    liveMessage: "请先描述想生成的图片",
  });

  const loading = imageGeneratorViewState({
    prompt: "生成海报",
    busy: true,
    disabledReason: "",
    statusTone: "loading",
    statusMessage: "生成中…",
    error: "",
    retryAvailable: false,
  });
  assert.equal(loading.canSubmit, false);
  assert.equal(loading.primaryLabel, "图片生成中…");
  assert.equal(loading.showSpinner, true);
  assert.equal(loading.tone, "loading");

  const failed = imageGeneratorViewState({
    prompt: "生成海报",
    busy: false,
    disabledReason: "",
    statusTone: "error",
    statusMessage: "",
    error: "模型生成失败，请调整描述后重试",
    retryAvailable: true,
  });
  assert.equal(failed.canSubmit, true);
  assert.equal(failed.primaryAction, "retry");
  assert.equal(failed.primaryLabel, "重新生成");
  assert.equal(failed.showRetry, true);
  assert.equal(failed.tone, "error");
  assert.equal(failed.liveMessage, "模型生成失败，请调整描述后重试");
});

test("network-uncertain errors reuse a key while explicit HTTP failures rotate it", () => {
  const imageRequestOutcomeForError = requiredFunction("imageRequestOutcomeForError");
  assert.equal(imageRequestOutcomeForError({ statusCode: 0 }), "network-uncertain");
  assert.equal(imageRequestOutcomeForError(new TypeError("Network request failed")), "network-uncertain");
  assert.equal(imageRequestOutcomeForError({ statusCode: 400 }), "terminal-failure");
  assert.equal(imageRequestOutcomeForError(new Error("validation failed")), "terminal-failure");
});

test("mounted image component exposes the empty prompt reason and suppresses generate", () => {
  const mounted = mountAiImageGenerator(imageComponentProps({ prompt: "   " }));
  try {
    const generate = findHostByClass(mounted.root, "ai-image-generator__generate");
    assert.ok(generate);
    assert.equal(generate.props.disabled, true);
    assert.equal(generate.props["aria-describedby"], "image-generator-disabled-reason");
    const reason = hostNodes(mounted.root).find(node => node.props.id === "image-generator-disabled-reason");
    assert.ok(reason);
    assert.equal(hostText(reason), "请先描述想生成的图片");
    generate.props.onClick();
    assert.deepEqual(mounted.emitted, []);
  } finally {
    mounted.unmount();
  }
});

test("mounted image component renders loading tone and a reduced-motion-safe spinner", () => {
  const mounted = mountAiImageGenerator(imageComponentProps({
    busy: true,
    statusTone: "loading",
    statusMessage: "生成中…",
  }));
  try {
    const live = findHostByClass(mounted.root, "ai-image-generator__live-region");
    const generate = findHostByClass(mounted.root, "ai-image-generator__generate");
    assert.ok(live);
    assert.ok(hostClass(live).includes("is-loading"));
    assert.equal(hostText(generate), "图片生成中…");
    assert.ok(findHostByClass(generate, "ai-image-generator__generate-spinner"));
    assert.equal(generate.props.disabled, true);
  } finally {
    mounted.unmount();
  }
});

test("mounted terminal failure uses error tone and emits retry instead of generate", () => {
  const mounted = mountAiImageGenerator(imageComponentProps({
    statusTone: "error",
    error: "模型生成失败，请调整描述后重试",
    retryAvailable: true,
  }));
  try {
    const live = findHostByClass(mounted.root, "ai-image-generator__live-region");
    const generate = findHostByClass(mounted.root, "ai-image-generator__generate");
    assert.ok(hostClass(live).includes("is-error"));
    assert.match(hostText(live), /模型生成失败，请调整描述后重试/);
    assert.equal(hostText(generate), "重新生成");
    generate.props.onClick();
    assert.deepEqual(mounted.emitted, ["retry"]);
  } finally {
    mounted.unmount();
  }
});

test("mounted image component renders only exact dynamic schema controls", () => {
  const mounted = mountAiImageGenerator(imageComponentProps({
    size: "1536x1024",
    sizeOptions: [
      { value: "1024x1024", label: "1:1" },
      { value: "1536x1024", label: "3:2" },
    ],
    quality: undefined,
    qualityOptions: [],
    count: undefined,
    countOptions: [],
  }));
  try {
    const sizeButtons = hostNodes(mounted.root)
      .filter(node => hostClass(node).split(/\s+/).includes("ai-image-generator__aspect"));
    assert.deepEqual(sizeButtons.map(hostText), ["1:1", "3:2✓"]);
    assert.doesNotMatch(hostText(mounted.root), /图片清晰度|张数|auto|1K|2K|4:3/);
  } finally {
    mounted.unmount();
  }
});

test("workbench wires exact image schema, canonical draft, and retry helpers without legacy controls", () => {
  const source = readFileSync(
    new URL("../apps/user-uni/src/components/MiniProgramRoleWorkbench.vue", import.meta.url),
    "utf8",
  );

  assert.match(source, /v-model:size="imageSize"/);
  assert.match(source, /:size-options="imageSizeOptions"/);
  assert.match(source, /:quality-options="imageQualityOptions"/);
  assert.match(source, /:count-options="imageCountOptions"/);
  assert.match(source, /:status-tone="imageGeneratorStatusTone"/);
  assert.match(source, /@retry="retryImageGeneration"/);
  assert.match(source, /module-schema\?module_code=.*model_name=/);
  assert.match(source, /resolveImageSchemaFetchResult/);
  assert.match(source, /restoreImageInspirationSelection/);
  assert.match(source, /buildCanonicalImageDraft/);
  assert.match(source, /taskRequestFromDraft/);
  assert.match(source, /imageRequestFingerprint/);
  assert.match(source, /nextImageClientRequestKey/);
  assert.match(source, /clientRequestId/);
  assert.doesNotMatch(source, /imageAspectOptions|imageAspectRatio|type ImageAspectRatio|type ImageQuality/);
  assert.doesNotMatch(source, /parameters:\s*\{[^}]*aspect_ratio/s);
});
