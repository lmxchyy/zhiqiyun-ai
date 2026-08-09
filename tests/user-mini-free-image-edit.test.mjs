import assert from "node:assert/strict";
import test from "node:test";

import {
  freeImageEditPresets,
  freeImageEditValidationMessage,
  selectedFreeImageEditPresetId,
} from "../apps/user-uni/src/features/generation/freeImageEdit.ts";

const expectedPresets = [
  {
    id: "magazine-cover",
    title: "杂志封面",
    prompt: "将照片转换为时尚杂志封面，保留人物身份特征，使用高级摄影棚光线和简洁排版。",
  },
  {
    id: "remove-passersby",
    title: "去除路人",
    prompt: "去除主体人物之外的路人，保持主体人物、背景结构和画面光影自然。",
  },
  {
    id: "refine-makeup",
    title: "精致补妆",
    prompt: "为照片中的人物补充自然精致的妆容，保留原有五官与肤质，避免过度磨皮。",
  },
  {
    id: "restore-photo",
    title: "老照片修复",
    prompt: "修复照片的划痕、褪色和模糊区域，提升清晰度，同时保留原始人物特征和年代质感。",
  },
  {
    id: "product-retouch",
    title: "商品图精修",
    prompt: "清理商品周围杂物和瑕疵，优化光影与质感，保持商品结构、颜色和标识准确。",
  },
  {
    id: "change-hairstyle",
    title: "更换发型",
    prompt: "将人物发型改为自然的齐肩短发，保持脸型、五官、姿态和环境不变。",
  },
];

test("free image edit exposes the six approved presets with unique ids and nonempty prompts", () => {
  assert.deepEqual(freeImageEditPresets, expectedPresets);
  assert.equal(new Set(freeImageEditPresets.map((preset) => preset.id)).size, 6);
  assert.equal(freeImageEditPresets.every((preset) => preset.prompt.length > 0), true);
});

test("free image edit selects a preset only for an exact trimmed prompt", () => {
  assert.equal(
    selectedFreeImageEditPresetId(`  ${expectedPresets[0].prompt}  `),
    "magazine-cover",
  );
  assert.equal(selectedFreeImageEditPresetId(`${expectedPresets[0].prompt}！`), "");
  assert.equal(selectedFreeImageEditPresetId(""), "");
});

test("free image edit validates image before a missing prompt", () => {
  assert.equal(freeImageEditValidationMessage("", ""), "请先添加需要编辑的图片");
  assert.equal(freeImageEditValidationMessage("   ", "修改人物发型"), "请先添加需要编辑的图片");
});

test("free image edit requires a prompt after an image is present", () => {
  assert.equal(freeImageEditValidationMessage("/tmp/photo.png", ""), "请先选择图片效果或填写修改要求");
  assert.equal(freeImageEditValidationMessage("/tmp/photo.png", "   "), "请先选择图片效果或填写修改要求");
  assert.equal(freeImageEditValidationMessage("/tmp/photo.png", "修改人物发型"), "");
});

// === Task 2: FreeImageEditCreation.vue source-contract tests ===

import { readFile } from "node:fs/promises";

const componentURL = new URL("../apps/user-uni/src/components/creation/FreeImageEditCreation.vue", import.meta.url);

test("renders the free-image-edit Figma structure and interactions", async () => {
  const source = await readFile(componentURL, "utf8");
  assert.match(source, /自由P图/);
  assert.match(source, /请添加图片/);
  assert.match(source, /选择图片效果/);
  assert.match(source, /maxlength="3000"/);
  assert.match(source, /开始生成/);
  assert.match(source, /freeImageEditPresets/);
  assert.match(source, /emit\(["']choose-image["']\)/);
  assert.match(source, /emit\(["']generate["']\)/);
});

test("uses shared safe-area variables without inventing an App capsule", async () => {
  const source = await readFile(componentURL, "utf8");
  assert.match(source, /var\(--header-padding-top/);
  assert.match(source, /var\(--capsule-right-space/);
  assert.match(source, /env\(safe-area-inset-bottom/);
  assert.doesNotMatch(source, /getMenuButtonBoundingClientRect/);
});

// === Task 3: MiniProgramRoleWorkbench integration tests ===

const workbenchURL = new URL("../apps/user-uni/src/components/MiniProgramRoleWorkbench.vue", import.meta.url);

test("integrates free-image-edit without bypassing the existing generation chain", async () => {
  const source = await readFile(workbenchURL, "utf8");
  assert.match(source, /import FreeImageEditCreation/);
  assert.match(source, /creationMode === ['"]infographic['"]/);
  assert.match(source, /<FreeImageEditCreation/);
  assert.match(source, /@generate="guestAwareGenerateTap"/);
  assert.match(source, /freeImageEditValidationMessage/);
  assert.match(source, /businessSdk\.generation\.createTask/);
  assert.match(source, /style:.*infographic/);
});

test("replaces the single free-image-edit source without changing other reference modes", async () => {
  const source = await readFile(workbenchURL, "utf8");
  assert.match(source, /creationMode\.value === "infographic"/);
  assert.match(source, /creationReferencePaths\.value = paths\.filter\(Boolean\)\.slice\(0, 1\)/);
});
