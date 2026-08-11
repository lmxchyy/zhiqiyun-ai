# AI 生图 Vue/uni-app Component Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在现有用户端生图链路中交付与 React 视觉原型一致的 Vue 3 + TypeScript + uni-app 响应式 AI 生图组件。

**Architecture:** 新增一个受控展示组件 `AiImageGenerator.vue`，由 `MiniProgramRoleWorkbench.vue` 持有模型读取、登录恢复、上传、任务提交、轮询和作品跳转。新增纯函数模块保存生图选项与模型筛选/积分文案逻辑，使组件不直接访问 API，并以现有 `businessSdk.generation.createTask` 完成端到端接线。

**Tech Stack:** Vue 3.5、TypeScript 5.8、uni-app、现有 business SDK、scoped CSS、Node.js `node:test`

## Global Constraints

- 正式包不得引入 React、ReactDOM、Axios、iframe、微前端运行时或 Tailwind 运行时。
- 组件不得直接调用 `uni.request`；生成必须继续走 `businessSdk.generation.createTask`。
- 默认提示词必须为“例如：生成一张水果店开业促销海报，橙色系，高级感”，默认画幅必须为 `auto`。
- 品牌/选中色为 `#423499`，唯一主 CTA 为 `#FF771B` 且文字使用 `#231000`。
- 卡片圆角统一为 16px；触控目标不低于 44×44px；所有普通文字达到 WCAG AA。
- H5 允许 Plus Jakarta Sans / Be Vietnam Pro 字体栈；小程序/App 只使用系统字体回退，不下载远程字体。
- 完整覆盖 default、hover、pressed、focus-visible、selected、disabled、loading、success 与 error 状态。
- 保持 M1 游客登录恢复、M3 作品入口和 M4 灵感草稿带入；不修改 W1–W4、M2、M5、M6 与后端协议。
- 不重写整个 `MiniProgramRoleWorkbench.vue`；只增加独立分支、状态和提交参数接线。

---

## File Map

- Create `apps/user-uni/src/features/generation/imageCreation.ts`: 生图选项、模型筛选、模型选择和积分摘要纯函数。
- Create `apps/user-uni/src/components/creation/AiImageGenerator.vue`: 受控跨端 UI 与交互事件。
- Modify `apps/user-uni/src/components/MiniProgramRoleWorkbench.vue`: 完整页分支、业务状态、模型读取和现有生成链路接线。
- Create `tests/user-mini-image-generator.test.mjs`: 纯函数、组件源码契约和工作台集成回归。

---

### Task 1: 生图配置纯函数

**Files:**
- Create: `apps/user-uni/src/features/generation/imageCreation.ts`
- Create: `tests/user-mini-image-generator.test.mjs`

**Interfaces:**
- Consumes: `ModelInfo` from `@xianzhi/shared-types`.
- Produces: `ImageAspectRatio`, `ImageQuality`, `ImageGeneratorModelOption`, `imageAspectOptions`, `imageQualityOptions`, `imageCountOptions`, `imageModelOptions(models)`, `resolveImageModelCode(models, requested)`, `imagePointEstimateLabel(model, count)`.

- [ ] **Step 1: Write failing tests for defaults, capability filtering, selection, and estimates**

```js
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
```

- [ ] **Step 2: Run the tests and verify the missing-module failure**

Run: `node --test tests/user-mini-image-generator.test.mjs`

Expected: FAIL with `ERR_MODULE_NOT_FOUND` for `imageCreation.ts`.

- [ ] **Step 3: Implement the pure configuration module**

```ts
import type { ModelInfo } from "@xianzhi/shared-types";

export type ImageAspectRatio = "auto" | "1:1" | "16:9" | "9:16" | "4:3";
export type ImageQuality = "1K" | "2K";

export interface ImageGeneratorModelOption {
  code: string;
  name: string;
  pointCost?: number;
}

export const imageAspectOptions: Array<{ value: ImageAspectRatio; label: string }> = [
  { value: "auto", label: "auto" },
  { value: "1:1", label: "1:1" },
  { value: "16:9", label: "16:9" },
  { value: "9:16", label: "9:16" },
  { value: "4:3", label: "4:3" },
];

export const imageQualityOptions: ImageQuality[] = ["1K", "2K"];
export const imageCountOptions = [1, 2, 4] as const;

export function imageModelOptions(models: ModelInfo[]): ImageGeneratorModelOption[] {
  return models
    .filter(model => model.online !== false)
    .filter(model => (model.capabilities || []).some(capability => {
      const value = String(capability).toUpperCase();
      return value === "TEXT_TO_IMAGE" || value === "IMAGE_TO_IMAGE" || value === "IMAGE_GENERATION";
    }))
    .map(model => ({ code: model.code, name: model.name || model.code, pointCost: model.pointCost }));
}

export function resolveImageModelCode(models: ImageGeneratorModelOption[], requested: string): string {
  return models.some(model => model.code === requested) ? requested : models[0]?.code || "";
}

export function imagePointEstimateLabel(model: ImageGeneratorModelOption | undefined, count: number): string {
  return typeof model?.pointCost === "number" && model.pointCost >= 0
    ? `预计 ${Math.round(model.pointCost * Math.max(1, count))} 积分`
    : "以生成时结算为准";
}
```

- [ ] **Step 4: Run the focused tests**

Run: `node --test tests/user-mini-image-generator.test.mjs`

Expected: 4 tests PASS.

- [ ] **Step 5: Commit Task 1**

```bash
git add -- apps/user-uni/src/features/generation/imageCreation.ts tests/user-mini-image-generator.test.mjs
git commit -m "feat(miniprogram): add image generator configuration"
```

---

### Task 2: 受控 Vue/uni-app 生图组件

**Files:**
- Create: `apps/user-uni/src/components/creation/AiImageGenerator.vue`
- Modify: `tests/user-mini-image-generator.test.mjs`

**Interfaces:**
- Consumes: Task 1 types and option arrays.
- Produces: Vue component props `prompt`, `aspectRatio`, `quality`, `model`, `models`, `count`, `referenceImages`, `referenceLimit`, `busy`, `selectingReference`, `modelsLoading`, `disabledReason`, `error`, `statusMessage`, `estimateLabel`; emits `back`, `help`, `choose-reference`, `remove-reference`, `preview-reference`, `optimize`, `generate`, and controlled `update:*` events.

- [ ] **Step 1: Add failing source-contract tests**

Append to `tests/user-mini-image-generator.test.mjs`:

```js
import { readFile } from "node:fs/promises";

const componentURL = new URL("../apps/user-uni/src/components/creation/AiImageGenerator.vue", import.meta.url);

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
    assert.ok(source.includes(`\"${event}\"`), `missing emit: ${event}`);
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
```

- [ ] **Step 2: Run tests and verify the missing-component failure**

Run: `node --test tests/user-mini-image-generator.test.mjs`

Expected: Task 1 tests PASS; the three component tests FAIL with `ENOENT`.

- [ ] **Step 3: Implement the controlled component contract**

Create the component with this script contract:

```vue
<script setup lang="ts">
import { computed } from "vue";
import {
  imageAspectOptions,
  imageCountOptions,
  imageQualityOptions,
  type ImageAspectRatio,
  type ImageGeneratorModelOption,
  type ImageQuality,
} from "../../features/generation/imageCreation";

const props = withDefaults(defineProps<{
  prompt: string;
  aspectRatio: ImageAspectRatio;
  quality: ImageQuality;
  model: string;
  models: ImageGeneratorModelOption[];
  count: number;
  referenceImages: string[];
  referenceLimit?: number;
  busy?: boolean;
  selectingReference?: boolean;
  modelsLoading?: boolean;
  disabledReason?: string;
  error?: string;
  statusMessage?: string;
  estimateLabel: string;
}>(), {
  referenceLimit: 3,
  busy: false,
  selectingReference: false,
  modelsLoading: false,
  disabledReason: "",
  error: "",
  statusMessage: "",
});

const emit = defineEmits<{
  back: [];
  help: [];
  "choose-reference": [];
  "remove-reference": [index: number];
  "preview-reference": [index: number];
  optimize: [];
  generate: [];
  "update:prompt": [value: string];
  "update:aspectRatio": [value: ImageAspectRatio];
  "update:quality": [value: ImageQuality];
  "update:model": [value: string];
  "update:count": [value: number];
}>();

const canGenerate = computed(() => Boolean(props.prompt.trim()) && !props.busy && !props.disabledReason);
</script>
```

Implement the template using only uni-app primitives (`view`, `text`, `image`, `button`, `picker`, `textarea`). Keep the mobile structure in this exact order: header, helper copy, prompt/reference 16px card, aspect buttons, quality segmented control, model/count pickers, live error/status region, fixed action footer. Use `hover-class` for pressed feedback, `aria-pressed` plus a visible checkmark for selection, and `aria-live="polite"` for status changes.

The generate button must use:

```vue
<button
  class="ai-image-generator__generate"
  type="button"
  :disabled="!canGenerate"
  :aria-label="disabledReason || (busy ? '图片生成中' : '生成图片')"
  hover-class="ai-image-generator__generate--pressed"
  @click="emit('generate')"
>
  <text>{{ busy ? "生成中…" : "生成图片" }}</text>
</button>
```

Define scoped CSS variables and cross-platform fallbacks at the component root:

```css
.ai-image-generator {
  --image-brand: #423499;
  --image-brand-pressed: #30236f;
  --image-action: #ff771b;
  --image-action-pressed: #ed650a;
  --image-ink: #111827;
  --image-muted: #667085;
  --image-line: #e1e6f1;
  --image-page: #f7f8fc;
  --image-radius: 16px;
  min-height: 100vh;
  color: var(--image-ink);
  background: var(--image-page);
  font-family: "Be Vietnam Pro", system-ui, -apple-system, BlinkMacSystemFont, "PingFang SC", "Microsoft YaHei", sans-serif;
}

.ai-image-generator__title,
.ai-image-generator__heading,
.ai-image-generator__generate {
  font-family: "Plus Jakarta Sans", "PingFang SC", "Microsoft YaHei", sans-serif;
}

.ai-image-generator button,
.ai-image-generator__aspect,
.ai-image-generator__reference-add {
  min-height: 44px;
}

.ai-image-generator__generate {
  min-height: 52px;
  border-radius: var(--image-radius);
  color: #231000;
  background: var(--image-action);
}

.ai-image-generator button:focus-visible,
.ai-image-generator textarea:focus-visible {
  outline: 3px solid rgba(66, 52, 153, 0.32);
  outline-offset: 2px;
}

@media (prefers-reduced-motion: reduce) {
  .ai-image-generator * { transition-duration: 0.01ms !important; animation-duration: 0.01ms !important; }
}
```

Use `@media (min-width: 768px)` for a two-column model/count row and wider gutters, and `@media (min-width: 1200px)` for a maximum content width of 960px with the footer kept inside that content width.

- [ ] **Step 4: Run component tests and typecheck**

Run: `node --test tests/user-mini-image-generator.test.mjs`

Expected: 7 tests PASS.

Run: `npm.cmd --prefix apps/user-uni run typecheck`

Expected: exit code 0.

- [ ] **Step 5: Commit Task 2**

```bash
git add -- apps/user-uni/src/components/creation/AiImageGenerator.vue tests/user-mini-image-generator.test.mjs
git commit -m "feat(miniprogram): add responsive AI image generator"
```

---

### Task 3: 接入现有工作台和生成链路

**Files:**
- Modify: `apps/user-uni/src/components/MiniProgramRoleWorkbench.vue`
- Modify: `tests/user-mini-image-generator.test.mjs`

**Interfaces:**
- Consumes: `AiImageGenerator` props/events and Task 1 pure functions.
- Produces: `/pages/user/UserImageCreationPage` 的完整页生图体验，仍经现有登录、上传、`resolveBackendGenerationConfig`、`businessSdk.generation.createTask`、任务轮询和作品中心。

- [ ] **Step 1: Add failing integration-contract tests**

Append to `tests/user-mini-image-generator.test.mjs`:

```js
const workbenchURL = new URL("../apps/user-uni/src/components/MiniProgramRoleWorkbench.vue", import.meta.url);

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
```

- [ ] **Step 2: Run tests and verify integration failures**

Run: `node --test tests/user-mini-image-generator.test.mjs`

Expected: Task 1–2 tests PASS; the three workbench tests FAIL because the component is not imported or wired.

- [ ] **Step 3: Add the full-page branch without changing other creation modes**

At the top-level root class map, add `'ai-image-generator-shell': isImageCreationPage`; change the native safe spacer condition to `v-if="!isFreeImageEditPage && !isImageCreationPage"`.

Immediately before the existing free-image-edit branch, render:

```vue
<view v-if="isImageCreationPage" class="ai-image-generator-page">
  <AiImageGenerator
    v-model:prompt="creationPrompt"
    v-model:aspect-ratio="imageAspectRatio"
    v-model:quality="imageQuality"
    v-model:model="selectedImageModelCode"
    v-model:count="imageCount"
    :models="imageModels"
    :reference-images="creationReferencePaths"
    :reference-limit="creationReferenceLimit"
    :busy="generationBusy"
    :selecting-reference="creationReferenceSelecting"
    :models-loading="imageModelsLoading"
    :disabled-reason="imageGeneratorDisabledReason"
    :error="creationError"
    :status-message="imageGeneratorStatusMessage"
    :estimate-label="imageEstimateLabel"
    @back="returnToCreationHub"
    @help="showImageGeneratorHelp"
    @choose-reference="chooseCreationReferenceImages"
    @remove-reference="removeCreationReference"
    @preview-reference="previewCreationReference"
    @optimize="optimizeImagePrompt"
    @generate="guestAwareGenerateTap"
  />
</view>
```

Keep the existing free-image-edit branch as `v-else-if="isFreeImageEditPage"` and all remaining user views inside the existing `v-else` branch.

- [ ] **Step 4: Add image-only state and model initialization**

Import the component and helpers, then add:

```ts
const imageAspectRatio = ref<ImageAspectRatio>("auto");
const imageQuality = ref<ImageQuality>("1K");
const imageCount = ref(1);
const imageModels = ref<ImageGeneratorModelOption[]>([]);
const selectedImageModelCode = ref("");
const imageModelsLoading = ref(false);
const imageModelsError = ref("");

const isImageCreationPage = computed(
  () => isCreationDetail.value && creationMode.value === "image",
);
const selectedImageModel = computed(
  () => imageModels.value.find(model => model.code === selectedImageModelCode.value),
);
const imageEstimateLabel = computed(
  () => imagePointEstimateLabel(selectedImageModel.value, imageCount.value),
);
const imageGeneratorDisabledReason = computed(() => {
  if (imageModelsLoading.value) return "正在读取可用模型";
  if (imageModelsError.value) return imageModelsError.value;
  if (!selectedImageModelCode.value) return "暂无可用图片模型";
  return "";
});
const imageGeneratorStatusMessage = computed(() => generationBusy.value
  ? generationButtonLabel.value
  : latestGenerationTask.value?.tone === "success" ? "生成完成，可前往作品中心查看" : "");
```

Implement model loading without fallback model names:

```ts
async function initializeImageModels() {
  if (creationMode.value !== "image") return;
  imageModelsLoading.value = true;
  imageModelsError.value = "";
  try {
    imageModels.value = imageModelOptions(await businessSdk.models.list());
    const requested = rowString(restoredCreationParams.value, "model", "modelName") || activeCreation.value.model;
    selectedImageModelCode.value = resolveImageModelCode(imageModels.value, requested);
    if (!selectedImageModelCode.value) imageModelsError.value = "暂无可用图片模型";
  } catch {
    imageModels.value = [];
    selectedImageModelCode.value = "";
    imageModelsError.value = "图片模型读取失败，请稍后重试";
  } finally {
    imageModelsLoading.value = false;
  }
}
```

Call it on page load when `creationMode.value === "image"`, after login token restoration for the image page, and when selecting image creation mode. Implement `optimizeImagePrompt()` by appending `，突出商品主体，商业摄影质感，留出清晰文案空间` once; implement `showImageGeneratorHelp()` with a Chinese `uni.showModal` explaining prompt and reference images.

- [ ] **Step 5: Feed controlled values into the existing create-task call**

Update `activeCreationModel` so image mode uses `selectedImageModelCode.value` when available. In `submitCreation`, retain all existing capability resolution, upload, idempotency, legal acceptance, polling and error handling. Change only the image values:

```ts
const requestedQuality = mode === "video"
  ? String(finalVideoParameters.resolution || "")
  : imageQuality.value;
const requestedSize = mode === "video"
  ? String(finalVideoParameters.aspect_ratio || "")
  : imageAspectRatio.value;

// Inside createTask
count: mode === "image"
  ? constrainedSchemaNumber(generationConfig.schema, "n", imageCount.value, 1)
  : constrainedSchemaNumber(generationConfig.schema, "n", restoredCreationCount(), 1),
parameters: mode === "video"
  ? finalVideoParameters
  : { ...restoredCreationParams.value, aspect_ratio: imageAspectRatio.value },
```

Continue passing `model: generationConfig.model`, the uploaded `referenceImages`, existing source metadata, and the current `clientRequestId` behavior unchanged.

- [ ] **Step 6: Add full-page shell CSS only**

Mirror the proven free-image-edit shell behavior without touching unrelated selectors:

```css
.mini-workbench.ai-image-generator-shell {
  padding: 0;
  background: #f7f8fc;
}

.ai-image-generator-shell .role-content {
  margin-top: 0;
}

.ai-image-generator-page {
  min-height: 100vh;
}
```

- [ ] **Step 7: Run focused regressions and typecheck**

Run: `node --test tests/user-mini-image-generator.test.mjs tests/user-mini-guest-browse-login.test.mjs tests/user-mini-free-image-edit.test.mjs`

Expected: all tests PASS; M1 guest flow and M6 free-image-edit branch remain unchanged.

Run: `npm.cmd --prefix apps/user-uni run typecheck`

Expected: exit code 0.

- [ ] **Step 8: Commit Task 3**

```bash
git add -- apps/user-uni/src/components/MiniProgramRoleWorkbench.vue tests/user-mini-image-generator.test.mjs
git commit -m "feat(miniprogram): integrate AI image generator"
```

---

### Task 4: Cross-platform build and visual/interaction verification

**Files:**
- Modify only if verification exposes a scoped defect: `apps/user-uni/src/components/creation/AiImageGenerator.vue`, `apps/user-uni/src/components/MiniProgramRoleWorkbench.vue`, `tests/user-mini-image-generator.test.mjs`

**Interfaces:**
- Consumes: completed Tasks 1–3.
- Produces: build evidence, responsive screenshots, interaction evidence, and protected-surface checklist.

- [ ] **Step 1: Run H5 and WeChat builds**

Run: `npm.cmd --prefix apps/user-uni run build:h5`

Expected: exit code 0 and an H5 production bundle.

Run: `npm.cmd --prefix apps/user-uni run build:mp-weixin:local`

Expected: exit code 0; package-size check passes.

- [ ] **Step 2: Start the H5 preview and capture three widths**

Run: `npm.cmd --prefix apps/user-uni exec -- uni -p h5 --host 127.0.0.1 --port 4174`

Open `http://127.0.0.1:4174/#/pages/user/UserImageCreationPage` and capture:

- 390×844: no horizontal overflow, fixed CTA visible, content not hidden by safe area.
- 768×1024: model/count row uses two columns and prompt card remains readable.
- 1440×1000: content stays within 960px and the action bar aligns with it.

Compare each capture against the React source visual at `C:\Users\mosilyu\.codex\generated_images\019fd16d-92cf-7552-b1c2-cd466ae6f943\exec-41e9e001-1f62-4567-9946-76bd0064a3de.png` using `view_image`.

- [ ] **Step 3: Verify the interaction matrix**

In H5, exercise these exact paths:

1. Clear prompt → generate button disabled and Chinese reason exposed.
2. Restore default prompt → choose `1:1`, `2K`, another available model, and two images → each selection shows text plus visual selected state.
3. Add and remove a reference image → count and thumbnail update without losing prompt.
4. Trigger help and prompt optimization → modal opens; optimization appends once.
5. Submit while logged out → existing login recovery opens and preserves the draft.
6. Submit while logged in → button becomes `生成中…`, duplicate clicks are blocked, and success/error state appears in `aria-live` content.
7. Open the completed result → existing work-detail route opens; no second generation or charge occurs.
8. Keyboard Tab through H5 controls → every interactive control has a visible focus ring.
9. Enable reduced motion → functionality remains complete without long transitions.

- [ ] **Step 4: Run final regressions**

Run: `node --test tests/user-mini-image-generator.test.mjs tests/user-mini-guest-browse-login.test.mjs tests/user-mini-free-image-edit.test.mjs`

Run: `npm.cmd --prefix apps/user-uni run typecheck`

Run: `git diff --check -- apps/user-uni/src/components/creation/AiImageGenerator.vue apps/user-uni/src/features/generation/imageCreation.ts apps/user-uni/src/components/MiniProgramRoleWorkbench.vue tests/user-mini-image-generator.test.mjs`

Expected: all commands exit 0.

- [ ] **Step 5: Record the protected-surface result in the delivery**

Use this exact scope statement:

```text
本次实际改动范围 = 用户端 AI 生图独立全页组件、图片模型读取与现有生成链路参数接线。
- [x] M1 游客生成仍走登录恢复，登录默认落用户首页未改
- [x] M3 生成结果仍进入既有作品中心/详情链路
- [x] M4 灵感草稿与参考图仍可带入 AI 生图
- [x] M6 自由P图独立全页、默认预设和 #ff6b00 未改且专项测试通过
- [ ] W1–W4、M2、M5、P1、P2 未触及
```

- [ ] **Step 6: Commit scoped verification fixes, if any**

If Step 2 or Step 3 required a scoped code fix, stage only the four files listed in this task and commit:

```bash
git add -- apps/user-uni/src/components/creation/AiImageGenerator.vue apps/user-uni/src/features/generation/imageCreation.ts apps/user-uni/src/components/MiniProgramRoleWorkbench.vue tests/user-mini-image-generator.test.mjs
git commit -m "fix(miniprogram): polish AI image generator states"
```

If no code changed during verification, do not create an empty commit.
