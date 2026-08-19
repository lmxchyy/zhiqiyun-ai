import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { createRequire } from "node:module";
import test from "node:test";

function loadBusinessSdkModule() {
  const moduleURL = new URL("../packages/business-sdk/src/mappers.ts", import.meta.url);
  const compiled = typescript.transpileModule(readFileSync(moduleURL, "utf8"), {
    compilerOptions: {
      module: typescript.ModuleKind.CommonJS,
      target: typescript.ScriptTarget.ES2022,
      esModuleInterop: true,
    },
  }).outputText;
  const module = { exports: {} };
  // eslint-disable-next-line no-new-func
  new Function("module", "exports", compiled)(module, module.exports);
  return module.exports;
}

const requireFromUserUni = createRequire(new URL("../apps/user-uni/package.json", import.meta.url));
const vueRuntime = requireFromUserUni("vue");
const vueCompiler = requireFromUserUni("@vue/compiler-sfc");
const typescript = requireFromUserUni("typescript");
const businessSdk = loadBusinessSdkModule();
const { taskRequestFromDraft } = businessSdk;

function loadSharedImageUtilsModule() {
  const moduleURL = new URL("../packages/shared-image-utils/src/index.ts", import.meta.url);
  const compiled = typescript.transpileModule(readFileSync(moduleURL, "utf8"), {
    compilerOptions: {
      module: typescript.ModuleKind.CommonJS,
      target: typescript.ScriptTarget.ES2022,
      esModuleInterop: true,
    },
  }).outputText;
  const module = { exports: {} };
  // eslint-disable-next-line no-new-func
  new Function("module", "exports", compiled)(module, module.exports);
  return module.exports;
}

function loadImageCreationModule() {
  const moduleURL = new URL("../apps/user-uni/src/features/generation/imageCreation.ts", import.meta.url);
  const compiled = typescript.transpileModule(readFileSync(moduleURL, "utf8"), {
    compilerOptions: {
      module: typescript.ModuleKind.CommonJS,
      target: typescript.ScriptTarget.ES2022,
      esModuleInterop: true,
    },
  }).outputText;
  const sharedImageUtils = loadSharedImageUtilsModule();
  const businessSdk = loadBusinessSdkModule();
  const module = { exports: {} };
  const localRequire = specifier => {
    if (specifier === "@xianzhi/business-sdk") return businessSdk;
    if (specifier === "@xianzhi/shared-image-utils") return sharedImageUtils;
    throw new Error(`unexpected image creation dependency ${specifier}`);
  };
  // eslint-disable-next-line no-new-func
  new Function("require", "module", "exports", compiled)(localRequire, module, module.exports);
  return module.exports;
}

const imageCreation = loadImageCreationModule();

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
    if (specifier === "../../composables/useMiniProgramNavigation") {
      return {
        useMiniProgramNavigation: () => ({
          navigationStyle: vueRuntime.computed(() => ({})),
        }),
      };
    }
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
  const { onUpdateSize, ...rest } = props || {};
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
        ...rest,
        "onUpdate:size": (value) => {
          if (typeof onUpdateSize === "function") onUpdateSize(value);
        },
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
    sizeOptions: [{ value: "1024x1024", label: "1K · 1:1" }],
    selectedRatio: "1:1",
    selectedTier: "1K",
    availableRatios: ["auto", "1:1"],
    availableTiers: ["1K"],
    quality: "auto",
    qualityOptions: [{ value: "auto", label: "自动" }],
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
      default: "auto",
      options: ["auto", "low", "medium", "high"],
    },
    {
      key: "n",
      type: "number",
      required: true,
      default: 1,
      options: [1, 2, 3, 4],
      min: 1,
      max: 4,
    },
  ];
  return exactImageSchema(overrides.modelName || "gpt-image-2", fields);
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

test("exact image schema derives real size ratios, canonical qualities, and count options", () => {
  const deriveImageCreationContract = requiredFunction("deriveImageCreationContract");
  const contract = deriveImageCreationContract("gpt-image-2", gptImageSchema());

  assert.deepEqual(contract, {
    available: true,
    modelName: "gpt-image-2",
    sizeOptions: [
      { value: "1024x1024", label: "1K · 1:1" },
      { value: "1536x1024", label: "1K · 3:2" },
      { value: "1024x1536", label: "1K · 2:3" },
    ],
    qualityOptions: [
      { value: "auto", label: "auto" },
      { value: "low", label: "low" },
      { value: "medium", label: "medium" },
      { value: "high", label: "high" },
    ],
    countOptions: [
      { value: 1, label: "1" },
      { value: 2, label: "2" },
      { value: 3, label: "3" },
      { value: 4, label: "4" },
    ],
    defaultSelection: { size: "1024x1024", quality: "auto", count: 1 },
    declared: { size: true, quality: true, count: true },
    required: { size: true, quality: true, count: true },
  });
});

test("official gpt image schema default quality low and n 1", () => {
  const deriveImageCreationContract = requiredFunction("deriveImageCreationContract");
  const initialImageSelection = requiredFunction("initialImageSelection");
  const contract = deriveImageCreationContract("gpt-image-2", gptImageSchema({
    fields: [
      { key: "prompt", type: "textarea", required: true },
      { key: "size", type: "select", required: false, default: "auto", options: ["auto", "1024x1024", "1536x1024", "1024x1536", "1280x720", "2048x2048", "3840x2160"] },
      { key: "quality", type: "select", required: false, default: "low", options: ["auto", "low", "medium", "high"] },
      { key: "n", type: "number", required: false, default: 1, options: [1, 2, 3, 4], min: 1, max: 4 },
    ],
  }));
  assert.equal(contract.available, true);
  if (!contract.available) return;
  assert.equal(contract.defaultSelection.quality, "low");
  assert.equal(contract.defaultSelection.count, 1);
  const selection = initialImageSelection(contract);
  assert.equal(selection.quality, "low");
  assert.equal(selection.count, 1);
});

test("initial image selection prefers quality low even when schema default is auto", () => {
  const deriveImageCreationContract = requiredFunction("deriveImageCreationContract");
  const initialImageSelection = requiredFunction("initialImageSelection");
  const contract = deriveImageCreationContract("gpt-image-2", gptImageSchema());
  assert.equal(contract.available, true);
  if (!contract.available) return;
  assert.equal(contract.defaultSelection.quality, "auto");
  const selection = initialImageSelection(contract);
  assert.equal(selection.quality, "low");
  assert.equal(selection.count, 1);
});

const GPT_IMAGE_NEAR_COMMON_SIZES = [
  "auto",
  "1024x1024",
  "1536x1024",
  "1024x1536",
  "1280x720",
  "720x1280",
  "2048x1152",
  "2048x2048",
  "3840x2160",
  "2160x3840",
  "1344x1008",
  "2048x1536",
  "3264x2448",
  "1008x1344",
  "1536x2048",
  "2448x3264",
  "2048x1360",
  "3520x2352",
  "1360x2048",
  "2352x3520",
  "1152x2048",
];

test("gpt image ratio grouping hides odd reduced ratios and maps near-common WxH", () => {
  const getAvailableRatios = requiredFunction("getAvailableRatiosExport");
  const findSizeByRatioAndTier = requiredFunction("findSizeByRatioAndTierExport");
  const getAvailableTiersForRatio = requiredFunction("getAvailableTiersForRatioExport");
  const resolveSizeFromRatioTier = requiredFunction("resolveSizeFromRatioTier");
  const classifyCommonAspectRatio = requiredFunction("classifyCommonAspectRatioExport");

  const ratios = getAvailableRatios(GPT_IMAGE_NEAR_COMMON_SIZES);
  assert.deepEqual(ratios, ["1:1", "16:9", "9:16", "4:3", "3:4", "3:2", "2:3"]);
  assert.ok(!ratios.includes("128:85"));
  assert.ok(!ratios.includes("85:128"));
  assert.ok(!ratios.includes("220:147"));
  assert.ok(!ratios.includes("147:220"));

  assert.equal(classifyCommonAspectRatio(2048, 1360), "3:2");
  assert.equal(findSizeByRatioAndTier(GPT_IMAGE_NEAR_COMMON_SIZES, "3:2", "2K"), "2048x1360");
  assert.equal(findSizeByRatioAndTier(GPT_IMAGE_NEAR_COMMON_SIZES, "3:2", "4K"), "3520x2352");
  assert.equal(findSizeByRatioAndTier(GPT_IMAGE_NEAR_COMMON_SIZES, "2:3", "2K"), "1360x2048");
  assert.equal(findSizeByRatioAndTier(GPT_IMAGE_NEAR_COMMON_SIZES, "2:3", "4K"), "2352x3520");
  assert.equal(findSizeByRatioAndTier(GPT_IMAGE_NEAR_COMMON_SIZES, "1:1", "4K"), undefined);
  assert.equal(resolveSizeFromRatioTier(GPT_IMAGE_NEAR_COMMON_SIZES, "auto", "auto"), "auto");
  assert.equal(resolveSizeFromRatioTier(GPT_IMAGE_NEAR_COMMON_SIZES, "auto", "1K"), "auto");
  assert.equal(resolveSizeFromRatioTier(GPT_IMAGE_NEAR_COMMON_SIZES, "auto", "2K"), "auto");
  assert.equal(resolveSizeFromRatioTier(GPT_IMAGE_NEAR_COMMON_SIZES, "auto", "4K"), "auto");
  assert.equal(resolveSizeFromRatioTier(GPT_IMAGE_NEAR_COMMON_SIZES, "3:2", "2K"), "2048x1360");
  assert.ok(!GPT_IMAGE_NEAR_COMMON_SIZES.includes("3840x3840"));
  assert.notEqual(findSizeByRatioAndTier(GPT_IMAGE_NEAR_COMMON_SIZES, "1:1", "4K"), "3840x3840");

  assert.deepEqual(getAvailableTiersForRatio(GPT_IMAGE_NEAR_COMMON_SIZES, "1:1"), ["1K", "2K"]);
  assert.deepEqual(getAvailableTiersForRatio(GPT_IMAGE_NEAR_COMMON_SIZES, "3:2"), ["1K", "2K", "4K"]);
  assert.ok(!getAvailableTiersForRatio(GPT_IMAGE_NEAR_COMMON_SIZES, "16:9").includes("720p"));
});

test("size labels distinguish 1K and 2K squares and keep submitting WxH", () => {
  const displayImageSizeLabel = requiredFunction("displayImageSizeLabel");
  const deriveImageCreationContract = requiredFunction("deriveImageCreationContract");
  const toCanonicalImageSelection = requiredFunction("toCanonicalImageSelection");
  assert.equal(displayImageSizeLabel("1024x1024"), "1K · 1:1");
  assert.equal(displayImageSizeLabel("2048x2048"), "2K · 1:1");
  assert.equal(displayImageSizeLabel("1280x720"), "720p · 16:9");
  assert.equal(displayImageSizeLabel("3840x2160"), "4K · 16:9");
  assert.equal(displayImageSizeLabel("1792x1024"), "2K · 1792x1024");

  const contract = deriveImageCreationContract("gpt-image-2", gptImageSchema({
    fields: [
      { key: "prompt", type: "textarea", required: true },
      { key: "size", type: "select", required: false, default: "auto", options: ["auto", "1024x1024", "2048x2048"] },
      { key: "quality", type: "select", required: false, default: "low", options: ["auto", "low", "medium", "high"] },
      { key: "n", type: "number", required: false, default: 1, options: [1], min: 1, max: 4 },
    ],
  }));
  assert.equal(contract.available, true);
  if (!contract.available) return;
  assert.deepEqual(contract.sizeOptions.map(option => option.label), ["auto", "1K · 1:1", "2K · 1:1"]);
  assert.deepEqual(toCanonicalImageSelection(contract, { size: "2048x2048", quality: "auto", count: 1 }), {
    size: "2048x2048",
    quality: "auto",
    count: 1,
  });
  assert.deepEqual(toCanonicalImageSelection(contract, { size: "1024x1024", quality: "low", count: 1 }), {
    size: "1024x1024",
    quality: "low",
    count: 1,
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
    { value: "1280x720", label: "720p · 16:9" },
    { value: "720x1280", label: "720p · 9:16" },
  ]);
  assert.deepEqual(contract.qualityOptions.map(option => option.value), ["high"]);
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
    { key: "size", required: true, default: "4:3", options: ["4:3", "0x1024"] },
  ]));

  assert.deepEqual(contract, {
    available: false,
    reason: "当前模型没有可用的图片尺寸选项",
  });
});

test("selection becomes canonical image fields only when each value is declared and supported", () => {
  const toCanonicalImageSelection = requiredFunction("toCanonicalImageSelection");
  const contract = availableContract(gptImageSchema());

  assert.deepEqual(
    toCanonicalImageSelection(contract, { size: "1536x1024", quality: "high", count: 2 }),
    { size: "1536x1024", quality: "high", count: 2 },
  );
  assert.deepEqual(
    toCanonicalImageSelection(contract, { size: "1536x1024", quality: "low", count: 3 }),
    { size: "1536x1024", quality: "low", count: 3 },
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
  const fullContract = availableContract(gptImageSchema());
  const sizeOnlyContract = deriveImageCreationContract("cloudbase-image", exactImageSchema("cloudbase-image", [
    { key: "size", required: true, default: "1280x720", options: ["1280x720"] },
  ]));
  assert.equal(sizeOnlyContract.available, true, sizeOnlyContract.reason);

  const cases = [
    { contract: fullContract, selection: { size: "4:3", quality: "high", count: 2 }, message: /不支持图片尺寸/ },
    { contract: fullContract, selection: { size: "1536x1024", quality: "2K", count: 2 }, message: /不支持图片质量/ },
    { contract: fullContract, selection: { size: "1536x1024", quality: "high", count: 5 }, message: /不支持生成数量/ },
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

test("inspiration draft restore maps schema-controlled size then builds existing generation request", () => {
  const restoreImageInspirationSelection = requiredFunction("restoreImageInspirationSelection");
  const canonicalImageParameters = requiredFunction("canonicalImageParameters");
  const contract = availableContract();
  const restored = restoreImageInspirationSelection(contract, { ratio: "1:1", quality: "high", count: 1 });
  assert.equal(restored.compatible, true);
  const draft = {
    contractVersion: 1,
    templateRef: { id: "template-image-1", slug: "product-hero", version: 3 },
    contentType: "image",
    handoff: { targetType: "IMAGE_CREATION" },
    values: { subject: "白底" },
    materials: [{ inputKey: "references", assetId: "asset-1" }],
    basePrompt: "SERVER COMPOSED PROMPT",
    parameters: { ratio: "1:1", quality: "high", count: 1 },
    capabilityKey: "image_generation",
    modelHint: "gpt-image-2",
    integrityToken: "signed-token",
    createdAt: "2026-08-12T00:00:00Z",
    expiresAt: "2026-08-12T00:30:00Z",
  };
  const request = taskRequestFromDraft({
    mode: "image",
    prompt: draft.basePrompt,
    model: "gpt-image-2",
    style: "commercial",
    size: restored.selection.size,
    quality: restored.selection.quality,
    count: restored.selection.count,
    referenceImages: ["/owned.png"],
    parameters: canonicalImageParameters({ ...draft.parameters, inspirationDraft: draft }),
  });
  assert.equal(request.prompt, "SERVER COMPOSED PROMPT");
  assert.equal(request.model, "gpt-image-2");
  assert.equal(request.moduleCode, "image_generation");
  assert.equal(request.params.size, "1024x1024");
  assert.equal(request.params.quality, "high");
  assert.equal(request.params.n, 1);
  assert.deepEqual(request.params.inspirationDraft.templateRef, draft.templateRef);
  assert.equal("pointCost" in request, false);
  assert.equal("billingType" in request, false);
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
  const canonical = toCanonicalImageSelection(availableContract(gptImageSchema()), {
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

function imageTaskSubmissionInput(overrides = {}) {
  return {
    contract: availableContract(gptImageSchema()),
    selection: { size: "1536x1024", quality: "high", count: 2 },
    prompt: "fruit poster with references",
    model: "gpt-image-2",
    style: "commercial",
    sourceReferences: ["wxfile://reference-a.png", "wxfile://reference-b.png"],
    negativePrompt: "watermark",
    parameters: { seed: 42 },
    ...overrides,
  };
}

function imageTask(id, status = "PENDING") {
  return {
    id,
    type: "IMAGE_TO_IMAGE",
    status,
    progress: 0,
    prompt: "fruit poster with references",
    model: "gpt-image-2",
    pointCost: 12,
    resultIds: [],
    params: {},
  };
}

function expectedImageRequest(clientRequestId, referenceImages) {
  return {
    type: "IMAGE_TO_IMAGE",
    clientRequestId,
    moduleCode: "image_generation",
    prompt: "fruit poster with references",
    model: "gpt-image-2",
    params: {
      seed: 42,
      size: "1536x1024",
      quality: "high",
      n: 2,
      negative_prompt: "watermark",
      reference_image: referenceImages[0],
      referenceImages: referenceImages.map((url, index) => ({ url, name: `reference-${index + 1}` })),
    },
  };
}

test("production image task orchestration reuses uploaded URLs, fingerprint, request, and key after an uncertain create", async () => {
  const submitCanonicalImageTask = requiredFunction("submitCanonicalImageTask");
  const uploadCalls = [];
  const createdDrafts = [];
  let createCalls = 0;
  const clientIds = ["first", "must-not-rotate"];
  const dependencies = {
    uploadReferences: async references => {
      uploadCalls.push([...references]);
      return references.map((_, index) => `https://uploads.test/batch-1-${index + 1}.png`);
    },
    createTask: async draft => {
      createdDrafts.push(draft);
      createCalls += 1;
      if (createCalls === 1) throw Object.assign(new Error("network request failed"), { statusCode: 0 });
      return imageTask("task-success");
    },
    clientIdFactory: () => clientIds.shift(),
  };
  const input = imageTaskSubmissionInput();

  const first = await submitCanonicalImageTask(input, dependencies);
  assert.equal(first.ok, false);
  assert.equal(first.state.previousOutcome, "network-uncertain");
  const retry = await submitCanonicalImageTask({ ...input, ...first.state }, dependencies);

  const uploadedURLs = [
    "https://uploads.test/batch-1-1.png",
    "https://uploads.test/batch-1-2.png",
  ];
  assert.equal(retry.ok, true);
  assert.deepEqual(uploadCalls, [input.sourceReferences]);
  assert.equal(createCalls, 2);
  assert.deepEqual(createdDrafts.map(draft => draft.clientRequestId), ["image_first", "image_first"]);
  assert.deepEqual(createdDrafts.map(draft => draft.referenceImages), [uploadedURLs, uploadedURLs]);
  assert.equal(first.fingerprint, retry.fingerprint);
  assert.deepEqual(first.requestSnapshot, retry.requestSnapshot);
  assert.deepEqual(first.finalRequest, expectedImageRequest("image_first", uploadedURLs));
  assert.deepEqual(retry.finalRequest, first.finalRequest);
  assert.deepEqual(retry.state.requestKey, first.state.requestKey);
});

test("production image task orchestration treats changed reference values and order as new requests", async () => {
  const submitCanonicalImageTask = requiredFunction("submitCanonicalImageTask");
  const cases = [
    {
      name: "value",
      initial: ["wxfile://reference-a.png", "wxfile://reference-b.png"],
      changed: ["wxfile://reference-a.png", "wxfile://reference-c.png"],
    },
    {
      name: "order",
      initial: ["wxfile://reference-a.png", "wxfile://reference-b.png"],
      changed: ["wxfile://reference-b.png", "wxfile://reference-a.png"],
    },
  ];

  for (const item of cases) {
    const uploadCalls = [];
    let createCalls = 0;
    const clientIds = [`${item.name}-first`, `${item.name}-changed`];
    const dependencies = {
      uploadReferences: async references => {
        uploadCalls.push([...references]);
        return references.map(reference => `https://uploads.test/${reference.slice("wxfile://".length)}`);
      },
      createTask: async () => {
        createCalls += 1;
        if (createCalls === 1) throw Object.assign(new Error("network request failed"), { statusCode: 0 });
        return imageTask(`task-${item.name}`);
      },
      clientIdFactory: () => clientIds.shift(),
    };
    const initialInput = imageTaskSubmissionInput({ sourceReferences: item.initial });
    const first = await submitCanonicalImageTask(initialInput, dependencies);
    const changed = await submitCanonicalImageTask({
      ...imageTaskSubmissionInput({ sourceReferences: item.changed }),
      ...first.state,
    }, dependencies);

    assert.equal(first.ok, false, item.name);
    assert.equal(changed.ok, true, item.name);
    assert.deepEqual(uploadCalls, [item.initial, item.changed], item.name);
    assert.equal(createCalls, 2, item.name);
    assert.notEqual(changed.fingerprint, first.fingerprint, item.name);
    assert.equal(changed.state.requestKey.clientRequestId, `image_${item.name}-changed`, item.name);
    assert.notDeepEqual(changed.finalRequest, first.finalRequest, item.name);
  }
});

test("production image task orchestration retransmits references and rotates the key after a terminal task", async () => {
  const submitCanonicalImageTask = requiredFunction("submitCanonicalImageTask");
  const uploadCalls = [];
  const createdDrafts = [];
  const clientIds = ["first", "terminal-retry"];
  const dependencies = {
    uploadReferences: async references => {
      uploadCalls.push([...references]);
      return references.map(reference => `https://uploads.test/${reference.slice("wxfile://".length)}`);
    },
    createTask: async draft => {
      createdDrafts.push(draft);
      return imageTask(`task-${createdDrafts.length}`, createdDrafts.length === 1 ? "FAILED" : "PENDING");
    },
    clientIdFactory: () => clientIds.shift(),
  };
  const input = imageTaskSubmissionInput({ sourceReferences: ["wxfile://reference-a.png"] });

  const terminal = await submitCanonicalImageTask(input, dependencies);
  const retry = await submitCanonicalImageTask({ ...input, ...terminal.state }, dependencies);

  assert.equal(terminal.ok, true);
  assert.equal(terminal.state.previousOutcome, "terminal-failure");
  assert.equal(retry.ok, true);
  assert.deepEqual(uploadCalls, [input.sourceReferences, input.sourceReferences]);
  assert.deepEqual(createdDrafts.map(draft => draft.clientRequestId), ["image_first", "image_terminal-retry"]);
  assert.equal(retry.fingerprint, terminal.fingerprint);
  assert.notEqual(retry.state.requestKey.clientRequestId, terminal.state.requestKey.clientRequestId);
});

test("production image task orchestration does not create a task or update cache after upload failure", async () => {
  const submitCanonicalImageTask = requiredFunction("submitCanonicalImageTask");
  const uploadError = new Error("upload failed");
  let createCalls = 0;
  const input = imageTaskSubmissionInput({ sourceReferences: ["wxfile://reference-a.png"] });

  const result = await submitCanonicalImageTask(input, {
    uploadReferences: async () => { throw uploadError; },
    createTask: async () => {
      createCalls += 1;
      return imageTask("must-not-create");
    },
    clientIdFactory: () => "must-not-create",
  });

  assert.equal(result.ok, false);
  assert.equal(result.error, uploadError);
  assert.equal(createCalls, 0);
  assert.equal(result.state.uploadCache, undefined);
  assert.equal(result.state.requestKey, undefined);
  assert.equal(result.state.previousOutcome, undefined);
  assert.equal(result.draft, undefined);
});

test("production image task orchestration retransmits and rotates the key after an explicit 4xx", async () => {
  const submitCanonicalImageTask = requiredFunction("submitCanonicalImageTask");
  const uploadCalls = [];
  const createdDrafts = [];
  let createCalls = 0;
  const clientIds = ["first", "after-4xx"];
  const dependencies = {
    uploadReferences: async references => {
      uploadCalls.push([...references]);
      return references.map(reference => `https://uploads.test/${reference.slice("wxfile://".length)}`);
    },
    createTask: async draft => {
      createdDrafts.push(draft);
      createCalls += 1;
      if (createCalls === 1) throw Object.assign(new Error("bad request"), { statusCode: 400 });
      return imageTask("task-after-4xx");
    },
    clientIdFactory: () => clientIds.shift(),
  };
  const input = imageTaskSubmissionInput({ sourceReferences: ["wxfile://reference-a.png"] });

  const failed = await submitCanonicalImageTask(input, dependencies);
  const retry = await submitCanonicalImageTask({ ...input, ...failed.state }, dependencies);

  assert.equal(failed.ok, false);
  assert.equal(failed.state.previousOutcome, "terminal-failure");
  assert.equal(retry.ok, true);
  assert.deepEqual(uploadCalls, [input.sourceReferences, input.sourceReferences]);
  assert.deepEqual(createdDrafts.map(draft => draft.clientRequestId), ["image_first", "image_after-4xx"]);
  assert.equal(retry.fingerprint, failed.fingerprint);
  assert.notEqual(retry.state.requestKey.clientRequestId, failed.state.requestKey.clientRequestId);
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
      options: ["auto", "high"],
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

test("model switch rejects pending size missing from the new schema and keeps canonical WxH or auto", () => {
  const deriveImageCreationContract = requiredFunction("deriveImageCreationContract");
  const toCanonicalImageSelection = requiredFunction("toCanonicalImageSelection");
  const initialImageSelection = requiredFunction("initialImageSelection");
  const findSizeByRatioAndTier = requiredFunction("findSizeByRatioAndTierExport");
  const previousContract = deriveImageCreationContract("gpt-image-2", gptImageSchema({
    fields: [
      { key: "prompt", type: "textarea", required: true },
      {
        key: "size",
        type: "select",
        required: false,
        default: "auto",
        options: GPT_IMAGE_NEAR_COMMON_SIZES,
      },
      { key: "quality", type: "select", required: false, default: "low", options: ["auto", "low", "medium", "high"] },
      { key: "n", type: "number", required: false, default: 1, options: [1], min: 1, max: 4 },
    ],
  }));
  const nextContract = deriveImageCreationContract("gpt-image-2", gptImageSchema({
    fields: [
      { key: "prompt", type: "textarea", required: true },
      {
        key: "size",
        type: "select",
        required: false,
        default: "auto",
        options: ["auto", "1024x1024", "2048x2048"],
      },
      { key: "quality", type: "select", required: false, default: "low", options: ["auto", "low", "medium", "high"] },
      { key: "n", type: "number", required: false, default: 1, options: [1], min: 1, max: 4 },
    ],
  }));
  assert.equal(previousContract.available, true);
  assert.equal(nextContract.available, true);
  if (!previousContract.available || !nextContract.available) return;

  assert.deepEqual(
    toCanonicalImageSelection(previousContract, { size: "2048x1360", quality: "low", count: 1 }),
    { size: "2048x1360", quality: "low", count: 1 },
  );
  assert.throws(
    () => toCanonicalImageSelection(nextContract, { size: "2048x1360", quality: "low", count: 1 }),
    /不支持图片尺寸 2048x1360/,
  );
  assert.throws(
    () => toCanonicalImageSelection(nextContract, { size: "2K", quality: "low", count: 1 }),
    /不支持图片尺寸 2K/,
  );
  assert.throws(
    () => toCanonicalImageSelection(nextContract, { size: "3840x3840", quality: "low", count: 1 }),
    /不支持图片尺寸 3840x3840/,
  );
  assert.equal(findSizeByRatioAndTier(nextContract.sizeOptions.map(option => option.value), "3:2", "2K"), undefined);
  assert.equal(findSizeByRatioAndTier(nextContract.sizeOptions.map(option => option.value), "1:1", "4K"), undefined);
  assert.deepEqual(initialImageSelection(nextContract), { size: "auto", quality: "low", count: 1 });
});

test("production image draft reaches the compiled SDK with canonical top-level fields only", () => {
  const buildCanonicalImageDraft = requiredFunction("buildCanonicalImageDraft");
  const contract = availableContract(gptImageSchema());
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
      quality: "auto",
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
    selectedRatio: "1:1",
    selectedTier: "1K",
    availableRatios: ["auto", "1:1"],
    availableTiers: ["1K", "2K"],
    size: "1536x1024",
    sizeOptions: [
      { value: "1024x1024", label: "1K · 1:1" },
      { value: "1536x1024", label: "1K · 3:2" },
    ],
    quality: undefined,
    qualityOptions: [],
    count: undefined,
    countOptions: [],
  }));
  try {
    const ratioButtons = hostNodes(mounted.root)
      .filter(node => hostClass(node).split(/\s+/).some(token => token.includes("ai-image-generator__ratio-chip")));
    assert.deepEqual(ratioButtons.map(hostText), ["自动", "1:1✓"]);
    const tierButtons = hostNodes(mounted.root)
      .filter(node => hostClass(node).split(/\s+/).some(token => token.includes("ai-image-generator__tier-chip")));
    assert.deepEqual(tierButtons.map(hostText), ["1K✓", "2K"]);
    assert.doesNotMatch(hostText(mounted.root), /standard|hd/);
    assert.match(hostText(mounted.root), /1:1/);
  } finally {
    mounted.unmount();
  }
});

test("mounted 2K square chip still submits WxH 2048x2048", () => {
  const emittedSize = [];
  const emittedRatio = [];
  const emittedTier = [];
  const mounted = mountAiImageGenerator(imageComponentProps({
    selectedRatio: "1:1",
    selectedTier: "1K",
    availableRatios: ["auto", "1:1"],
    availableTiers: ["1K", "2K"],
    size: "1024x1024",
    sizeOptions: [
      { value: "1024x1024", label: "1K · 1:1" },
      { value: "2048x2048", label: "2K · 1:1" },
    ],
    quality: "auto",
    qualityOptions: [
      { value: "auto", label: "auto" },
      { value: "low", label: "low" },
    ],
    count: 1,
    countOptions: [{ value: 1, label: "1" }],
    onUpdateSize: (value) => emittedSize.push(value),
    "onUpdate:selectedRatio": (value) => emittedRatio.push(value),
    "onUpdate:selectedTier": (value) => emittedTier.push(value),
  }));
  try {
    const ratioButtons = hostNodes(mounted.root)
      .filter(node => hostClass(node).split(/\s+/).some(token => token.includes("ai-image-generator__ratio-chip")));
    const tierButtons = hostNodes(mounted.root)
      .filter(node => hostClass(node).split(/\s+/).some(token => token.includes("ai-image-generator__tier-chip")));
    assert.deepEqual(ratioButtons.map(hostText), ["自动", "1:1✓"]);
    assert.deepEqual(tierButtons.map(hostText), ["1K✓", "2K"]);
    tierButtons[1].props.onClick();
    assert.deepEqual(emittedRatio, []);
    assert.deepEqual(emittedTier, ["2K"]);
    assert.deepEqual(emittedSize, []);
  } finally {
    mounted.unmount();
  }
});

test("auto ratio hides clarity row; concrete ratio shows 1K/2K/4K; quality stays internal", () => {
  const autoMounted = mountAiImageGenerator(imageComponentProps({
    selectedRatio: "auto",
    selectedTier: "auto",
    availableRatios: ["auto", "1:1", "16:9", "9:16", "4:3", "3:4", "3:2", "2:3", "128:85", "85:128", "220:147", "147:220"],
    availableTiers: ["1K", "2K", "4K"],
    size: "auto",
    quality: "low",
    qualityOptions: [
      { value: "auto", label: "auto" },
      { value: "low", label: "low" },
      { value: "medium", label: "medium" },
      { value: "high", label: "high" },
    ],
  }));
  try {
    const ratioButtons = hostNodes(autoMounted.root)
      .filter(node => hostClass(node).split(/\s+/).some(token => token.includes("ai-image-generator__ratio-chip")));
    assert.deepEqual(ratioButtons.map(hostText), ["自动✓", "1:1", "16:9", "9:16", "4:3", "3:4", "3:2", "2:3"]);
    assert.doesNotMatch(hostText(autoMounted.root), /128:85|85:128|220:147|147:220/);
    assert.doesNotMatch(hostText(autoMounted.root), /图片清晰度/);
    assert.equal(
      hostNodes(autoMounted.root).filter(node => hostClass(node).split(/\s+/).some(token => token.includes("ai-image-generator__tier-chip"))).length,
      0,
    );
    assert.doesNotMatch(hostText(autoMounted.root), /生成质量/);
  } finally {
    autoMounted.unmount();
  }

  const ratioMounted = mountAiImageGenerator(imageComponentProps({
    selectedRatio: "3:2",
    selectedTier: "2K",
    availableRatios: ["auto", "1:1", "3:2", "2:3"],
    availableTiers: ["720p", "1K", "2K", "4K"],
    size: "2048x1360",
    quality: "low",
    qualityOptions: [
      { value: "auto", label: "auto" },
      { value: "low", label: "low" },
    ],
  }));
  try {
    assert.match(hostText(ratioMounted.root), /图片清晰度/);
    const tierButtons = hostNodes(ratioMounted.root)
      .filter(node => hostClass(node).split(/\s+/).some(token => token.includes("ai-image-generator__tier-chip")));
    assert.deepEqual(tierButtons.map(hostText), ["1K", "2K✓", "4K"]);
    assert.doesNotMatch(hostText(ratioMounted.root), /720P|720p/);
    assert.match(hostText(ratioMounted.root), /当前输出 2048x1360/);
    assert.doesNotMatch(hostText(ratioMounted.root), /生成质量/);
    assert.doesNotMatch(hostText(ratioMounted.root), /3840x3840/);
  } finally {
    ratioMounted.unmount();
  }
});

test("web image workspace reloads schema on model switch and labels 1K vs 2K", () => {
  const source = readFileSync(new URL("../admin-vue/src/App.vue", import.meta.url), "utf8");
  assert.match(source, /loadAiImageModuleSchema\(true\)/);
  assert.match(source, /applyOnlineImageFormToLoadedSchema\(\)/);
  assert.match(source, /displayGptImageSizeLabel/);
  const labelSource = readFileSync(new URL("../admin-vue/src/utils/gptImageSizeLabel.ts", import.meta.url), "utf8");
  assert.match(labelSource, /shared-image-utils/);
  assert.match(labelSource, /displayGptImageSizeLabel/);
  const editor = readFileSync(new URL("../admin-vue/src/components/billing/PlanEditorDialog.vue", import.meta.url), "utf8");
  assert.match(editor, /value="auto"/);
  assert.match(editor, /value="low"/);
  assert.match(editor, /value="medium"/);
  assert.doesNotMatch(editor, /value="standard"/);
  assert.doesNotMatch(editor, /value="hd"/);
  assert.match(editor, /canonicalizeImageQualityLimits/);
});

test("workbench wires exact image schema and delegates image submission to the production orchestrator", () => {
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
  assert.match(source, /exactModuleSchemaPath\("image_generation", requestedModel, isGuest\.value\)/);
  assert.match(source, /resolveImageSchemaFetchResult/);
  assert.match(source, /restoreImageInspirationSelection/);
  assert.match(source, /inspirationDraft: activeInspirationDraft.value/);
  assert.match(source, /await submitCanonicalImageTask\(/);
  assert.match(source, /watch\(selectedImageModelCode/);
  assert.match(source, /void loadImageSchemaForModel\(modelCode\)/);
  assert.match(source, /toCanonicalImageSelection\(contract, pending\)/);
  assert.match(source, /imageReferenceUploadCache/);
  assert.match(source, /if \(ratio === "auto"\)/);
  assert.match(source, /imageSize\.value = "auto"/);
  assert.match(source, /selectedTier\.value = "auto"/);
  assert.match(source, /imageSize\.value = resolved \|\| ""/);
  assert.doesNotMatch(source, /imageAspectOptions|imageAspectRatio|type ImageAspectRatio|type ImageQuality/);
  assert.doesNotMatch(source, /parameters:\s*\{[^}]*aspect_ratio/s);
  assert.doesNotMatch(source, /3840x3840/);
  assert.doesNotMatch(source, /resolutionTier|params\.tier|"tier":\s*"2K"/);
});

function loadVideoModelSwitchHarness(rejection) {
  const componentURL = new URL("../apps/user-uni/src/components/MiniProgramRoleWorkbench.vue", import.meta.url);
  const source = readFileSync(componentURL, "utf8");
  const start = source.indexOf("async function requestVideoModelSwitch");
  const end = source.indexOf("\nasync function initializeVideoModelForm", start);
  assert.notEqual(start, -1, "requestVideoModelSwitch must exist");
  assert.notEqual(end, -1, "initializeVideoModelForm must follow requestVideoModelSwitch");

  const compiled = typescript.transpileModule(
    `${source.slice(start, end)}\nmodule.exports = requestVideoModelSwitch;`,
    {
      compilerOptions: {
        module: typescript.ModuleKind.CommonJS,
        target: typescript.ScriptTarget.ES2022,
      },
    },
  ).outputText;
  const selectedVideoModelCode = { value: "current-video-model" };
  const videoParameterFields = { value: [{ key: "duration" }] };
  const videoModelSwitching = { value: false };
  const videoModelError = { value: "" };
  const creationMode = { value: "video" };
  const isGuest = { value: true };
  let commitCount = 0;
  const dependencies = {
    selectedVideoModelCode,
    videoParameterFields,
    videoModelSwitchSequence: 0,
    videoModelSwitching,
    videoModelError,
    resolveBackendGenerationConfig: async () => { throw rejection; },
    creationMode,
    isGuest,
    videoConfigRemovesReferences: () => false,
    confirmVideoReferenceRemoval: async () => true,
    commitVideoModelConfig: config => {
      commitCount += 1;
      selectedVideoModelCode.value = config.model;
    },
  };
  const module = { exports: {} };
  // eslint-disable-next-line no-new-func
  new Function(...Object.keys(dependencies), "module", "exports", compiled)(
    ...Object.values(dependencies),
    module,
    module.exports,
  );
  return {
    requestVideoModelSwitch: module.exports,
    selectedVideoModelCode,
    videoModelSwitching,
    videoModelError,
    commitCount: () => commitCount,
  };
}

test("video model switch reports schema request failures without rejecting or switching models", async t => {
  const failures = [
    Object.assign(new Error("该模型尚未完成小程序上线合规审核"), { statusCode: 403 }),
    Object.assign(new Error("网络连接失败，请检查网络后重试"), { statusCode: 0 }),
  ];

  for (const failure of failures) {
    await t.test(failure.statusCode === 403 ? "403 compliance rejection" : "network failure", async () => {
      const harness = loadVideoModelSwitchHarness(failure);

      await assert.doesNotReject(() => harness.requestVideoModelSwitch("requested-video-model"));
      assert.equal(harness.videoModelError.value, failure.message);
      assert.equal(harness.videoModelSwitching.value, false);
      assert.equal(harness.selectedVideoModelCode.value, "current-video-model");
      assert.equal(harness.commitCount(), 0);
    });
  }
});
