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
const workbenchURL = new URL("../apps/user-uni/src/components/MiniProgramRoleWorkbench.vue", import.meta.url);

function sourceBetween(source, start, end) {
  const startIndex = source.indexOf(start);
  const endIndex = source.indexOf(end, startIndex + start.length);
  assert.notEqual(startIndex, -1, `missing start marker: ${start}`);
  assert.notEqual(endIndex, -1, `missing end marker: ${end}`);
  return source.slice(startIndex, endIndex);
}

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

test("AI image generator keeps brand selections and reference removal targets accessible", async () => {
  const source = await readFile(componentURL, "utf8");
  assert.match(source, /\.ai-image-generator__aspect\.is-selected,[\s\S]*?border-color:\s*var\(--image-brand\);/);
  assert.match(source, /\.ai-image-generator__aspect\.is-selected,[\s\S]*?color:\s*var\(--image-brand\);/);
  assert.match(source, /\.ai-image-generator__check\s*\{[\s\S]*?background:\s*var\(--image-brand\);/);
  assert.match(source, /\.ai-image-generator__reference-remove\s*\{[\s\S]*?width:\s*44px;[\s\S]*?height:\s*44px;/);
  assert.match(source, /ai-image-generator__reference-remove-glyph/);
  assert.match(source, /border-radius:\s*var\(--image-radius\);/);
});

test("AI image generator exposes fixed footer and visible interaction states", async () => {
  const source = await readFile(componentURL, "utf8");
  assert.match(source, /\.ai-image-generator__footer\s*\{[\s\S]*?position:\s*fixed;/);
  assert.match(source, /padding-bottom:\s*calc\(112px \+ env\(safe-area-inset-bottom\)\)/);
  assert.match(source, /@media \(hover: hover\)/);
  assert.match(source, /button:disabled/);
  assert.match(source, /v-if="selectingReference"/);
  assert.match(source, /ai-image-generator__reference-loading/);
  assert.match(source, /role="status"/);
  assert.match(source, /ai-image-generator__success/);
});

test("AI image generator preserves fixed-footer clearance at tablet widths", async () => {
  const source = await readFile(componentURL, "utf8");
  assert.match(
    source,
    /@media \(min-width: 768px\) \{[\s\S]*?\.ai-image-generator__content\s*\{[\s\S]*?padding-bottom:\s*calc\(112px \+ env\(safe-area-inset-bottom\)\);/,
  );
});

test("workbench renders AI image generation as a full-page controlled component", async () => {
  const source = await readFile(workbenchURL, "utf8");
  assert.match(source, /import AiImageGenerator/);
  assert.match(source, /isImageCreationPage/);
  assert.match(source, /<AiImageGenerator/);
  assert.match(source, /v-model:prompt="creationPrompt"/);
  assert.match(source, /v-model:aspect-ratio="imageAspectRatio"/);
  assert.match(source, /v-model:quality="imageQuality"/);
  assert.match(source, /v-model:model="selectedImageModelCode"/);
  assert.match(source, /v-model:count="imageCount"/);
  assert.match(source, /@generate="guestAwareGenerateTap"/);
});

test("image controls feed the existing generation request", async () => {
  const source = await readFile(workbenchURL, "utf8");
  assert.match(source, /businessSdk\.models\.list/);
  assert.match(source, /imageModelOptions/);
  assert.match(source, /model:\s*generationConfig\.model/);
  assert.match(source, /requestedQuality =[^;]*imageQuality\.value/s);
  assert.match(source, /requestedSize =[^;]*imageAspectRatio\.value/s);
  assert.match(source, /count:[^,]*imageCount\.value/s);
  assert.match(source, /businessSdk\.generation\.createTask/);
  assert.match(source, /uploadCreationReferenceImages/);
});

test("image integration preserves guest restore, inspiration drafts, and works routing", async () => {
  const source = await readFile(workbenchURL, "utf8");
  assert.match(source, /guestAwareGenerateTap/);
  assert.match(source, /activeInspirationDraft/);
  assert.match(source, /restoreCreationSource/);
  assert.match(source, /openLatestGenerationResult/);
  assert.match(source, /isGuest/);
});

test("image request controls stay isolated from free image edit requests", async () => {
  const source = await readFile(workbenchURL, "utf8");
  const submitSource = sourceBetween(source, "async function submitCreation", "async function uploadCreationReferenceImages");
  assert.match(submitSource, /const isImageGeneratorRequest = creationMode\.value === "image";/);
  assert.match(submitSource, /requestedQuality =[\s\S]*?isImageGeneratorRequest[\s\S]*?imageQuality\.value[\s\S]*?restoredCreationString\("quality", "imageQuality"\) \|\| "standard";/);
  assert.match(submitSource, /requestedSize =[\s\S]*?isImageGeneratorRequest[\s\S]*?imageAspectRatio\.value[\s\S]*?restoredCreationString\("size", "aspectRatio", "aspect_ratio"\) \|\| "1024x1024";/);
  assert.match(submitSource, /count: isImageGeneratorRequest[\s\S]*?imageCount\.value[\s\S]*?restoredCreationCount\(\)/);
  assert.match(submitSource, /parameters: mode === "video"[\s\S]*?isImageGeneratorRequest[\s\S]*?aspect_ratio: imageAspectRatio\.value[\s\S]*?: restoredCreationParams\.value/);
});

test("image success state opens the generated work from the image branch", async () => {
  const [componentSource, workbenchSource] = await Promise.all([
    readFile(componentURL, "utf8"),
    readFile(workbenchURL, "utf8"),
  ]);
  assert.match(componentSource, /"view-result": \[\]/);
  assert.match(componentSource, /ai-image-generator__view-result/);
  assert.match(componentSource, /@click='emit\("view-result"\)'/);
  const imageBranch = sourceBetween(
    workbenchSource,
    '<view v-if="isImageCreationPage" class="ai-image-generator-page">',
    '<template v-else-if="isFreeImageEditPage">',
  );
  assert.match(imageBranch, /@view-result="openLatestGenerationResult"/);
});

test("image header uses shared custom-navigation safe-area variables", async () => {
  const source = await readFile(componentURL, "utf8");
  const headerStyles = sourceBetween(source, ".ai-image-generator__header {", ".ai-image-generator__icon-button {");
  assert.match(headerStyles, /min-height:\s*var\(--header-height,\s*64px\);/);
  assert.match(headerStyles, /padding-top:\s*var\(--header-padding-top,\s*0px\);/);
  assert.match(headerStyles, /padding-right:\s*var\(--capsule-right-space,\s*0px\);/);
});

test("image controls persist for guest login and restore only approved values", async () => {
  const source = await readFile(workbenchURL, "utf8");
  const guestFlow = sourceBetween(source, "function guestAwareGenerateTap", "type NativeGenerateBridge");
  assert.match(guestFlow, /creationMode\.value === "image"[\s\S]*?aspectRatio: imageAspectRatio\.value[\s\S]*?quality: imageQuality\.value[\s\S]*?count: imageCount\.value/);

  const restoreControls = sourceBetween(source, "function restoreImageGeneratorControls", "const currentPageTitle");
  assert.match(restoreControls, /imageAspectOptions\.some/);
  assert.match(restoreControls, /imageQualityOptions\.includes/);
  assert.match(restoreControls, /imageCountOptions\.includes/);
  assert.match(restoreControls, /:\s*"auto"/);
  assert.match(restoreControls, /:\s*"1K"/);
  assert.match(restoreControls, /:\s*1;/);

  const sourceRestore = sourceBetween(source, "async function restoreCreationSource", "function applyVideoModelCapabilities");
  assert.match(sourceRestore, /restoreImageGeneratorControls\(restoredCreationParams\.value\)/);
  const mountedRestore = sourceBetween(source, "onMounted(() =>", "watch(() => authStore.token");
  assert.match(mountedRestore, /restoredCreationParams\.value =[\s\S]*?restoreImageGeneratorControls\(restoredCreationParams\.value\)/);
});
