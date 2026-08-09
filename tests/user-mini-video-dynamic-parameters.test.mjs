import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";

const source = await readFile(
  new URL("../apps/user-uni/src/components/MiniProgramRoleWorkbench.vue", import.meta.url),
  "utf8",
);

test("mini-program video page derives controls from model schema contract", () => {
  for (const token of [
    "deriveEditableVideoFields",
    "transitionVideoParameterValues",
    "buildVideoSubmissionParameters",
    "videoParameterFields",
    "videoParameterValues",
    "requestVideoModelSwitch",
    "basicVideoSelectFields",
    "videoAudioField",
    "advancedVideoParameterFields",
    "video-basic-card",
  ]) {
    assert.ok(source.includes(token), `missing dynamic video form token: ${token}`);
  }
  assert.match(source, /v-for="field in basicVideoSelectFields"/);
  assert.match(source, /v-for="field in advancedVideoParameterFields"/);
  assert.match(source, /videoBooleanParameterValue\('generate_audio'\)/);
});

test("model switch confirms before clearing an existing reference image", () => {
  assert.ok(source.includes("当前模型不支持参考图，切换后将移除已上传图片，是否继续？"));
  const switchStart = source.indexOf("async function requestVideoModelSwitch");
  const switchEnd = source.indexOf("\n}", switchStart);
  const switchBody = source.slice(switchStart, switchEnd);
  assert.ok(switchBody.includes("confirmVideoReferenceRemoval"));
  assert.ok(switchBody.indexOf("confirmVideoReferenceRemoval") < switchBody.indexOf("commitVideoModelConfig"));
});

test("model and parameter changes refresh a guarded point estimate", () => {
  assert.ok(source.includes("videoEstimateSequence"));
  assert.ok(source.includes("scheduleVideoEstimate"));
  assert.ok(source.includes("businessSdk.generation.estimateVideo"));
  assert.ok(source.includes("预计消耗"));
  assert.ok(source.includes("正式提交时以后端为准"));
});

test("video submission uses only the final editable parameter contract", () => {
  assert.match(source, /buildVideoSubmissionParameters\(\s*videoParameterValues\.value,\s*videoParameterFields\.value/);
  assert.ok(!source.includes("parameters: restoredCreationParams.value"));
});

test("reference upload visibility remains tied to real image-to-video support", () => {
  assert.match(source, /videoGenerationMode\.value === "IMAGE_TO_VIDEO"/);
  assert.match(source, /videoModelCapabilities\.value\.supportsImageToVideo/);
});

test("reference upload switches a supported text-to-video model after image selection", () => {
  const chooseStart = source.indexOf("function chooseCreationReferenceImages");
  const chooseEnd = source.indexOf("function appendCreationReferencePaths", chooseStart);
  const chooseBody = source.slice(chooseStart, chooseEnd);
  assert.doesNotMatch(chooseBody, /videoGenerationMode\.value !== "IMAGE_TO_VIDEO"/);

  const appendStart = chooseEnd;
  const appendEnd = source.indexOf("function setCreationReferenceSelecting", appendStart);
  const appendBody = source.slice(appendStart, appendEnd);
  assert.match(appendBody, /videoModelCapabilities\.value\.supportsImageToVideo/);
  assert.match(appendBody, /videoGenerationMode\.value = "IMAGE_TO_VIDEO"/);
});
