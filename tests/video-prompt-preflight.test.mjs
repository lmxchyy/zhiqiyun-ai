import test from "node:test";
import assert from "node:assert/strict";
import {
  inspectVideoPromptPreflight,
  videoGenerationFailureMessage,
} from "../packages/shared-types/src/videoPromptPreflight.ts";

test("video prompt preflight warns when requested duration differs", () => {
  const mismatch = inspectVideoPromptPreflight({ prompt: "生成30秒宣传片", duration: 15, inputMode: "TEXT_TO_VIDEO" });
  assert.deepEqual(mismatch.warnings.map(item => item.code), ["VIDEO_PROMPT_DURATION_MISMATCH"]);

  const matching = inspectVideoPromptPreflight({ prompt: "生成15秒宣传片", duration: 15, inputMode: "TEXT_TO_VIDEO" });
  assert.equal(matching.warnings.some(item => item.code === "VIDEO_PROMPT_DURATION_MISMATCH"), false);
});

test("video prompt preflight checks reference images and text mode", () => {
  const missing = inspectVideoPromptPreflight({ prompt: "根据3张参考图片生成视频", duration: 15, inputMode: "TEXT_TO_VIDEO", referenceImageCount: 0 });
  assert.equal(missing.warnings.some(item => item.code === "VIDEO_PROMPT_REFERENCE_MISSING"), true);
  assert.equal(missing.warnings.some(item => item.code === "VIDEO_PROMPT_MODE_MISMATCH"), true);

  const present = inspectVideoPromptPreflight({ prompt: "根据3张参考图片生成视频", duration: 15, inputMode: "IMAGE_TO_VIDEO", referenceImageCount: 3 });
  assert.equal(present.warnings.some(item => item.code === "VIDEO_PROMPT_REFERENCE_MISSING"), false);
});

test("complexity is a soft warning and financial/compliance wording is not rejected", () => {
  const complex = inspectVideoPromptPreflight({
    prompt: "多场景、多镜头，添加字幕、配音并保持音画同步",
    duration: 15,
    inputMode: "TEXT_TO_VIDEO",
  });
  assert.equal(complex.warnings.some(item => item.code === "VIDEO_PROMPT_COMPLEXITY_WARNING"), true);
  assert.ok(complex.warnings.every(item => item.severity === "warning"));

  const compliance = inspectVideoPromptPreflight({
    prompt: "制作金融合规宣传片，强调风险提示与合规经营",
    duration: 15,
    inputMode: "TEXT_TO_VIDEO",
  });
  assert.deepEqual(compliance.warnings, []);
});

test("provider generation failure UX uses real billing state", () => {
  const released = videoGenerationFailureMessage({
    errorCode: "PROVIDER_ASYNC_GENERATION_FAILED",
    billingStatus: "RELEASED",
    releasedPoints: 600,
  });
  assert.match(released, /上游未能完成本次视频生成/);
  assert.match(released, /积分已退回\/释放/);

  const notReleased = videoGenerationFailureMessage({
    errorCode: "PROVIDER_ASYNC_GENERATION_FAILED",
    billingStatus: "RESERVED",
    releasedPoints: 0,
  });
  assert.match(notReleased, /积分状态将按现有规则处理/);
  assert.doesNotMatch(notReleased, /积分已退回\/释放/);
});
