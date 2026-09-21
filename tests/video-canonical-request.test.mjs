import test from "node:test";
import assert from "node:assert/strict";
import {
  CanonicalVideoRequestError,
  buildCanonicalVideoRequest,
  canonicalVideoRequestRepresentation,
} from "../packages/shared-types/src/videoCanonicalRequest.ts";

const capabilities = {
  supported_durations: [5, 10, 15, 30],
  supported_resolutions: ["480p", "720p"],
  supported_aspect_ratios: ["16:9", "9:16", "1:1"],
};

function build(overrides = {}) {
  return buildCanonicalVideoRequest({
    prompt: "a cinematic product reveal",
    structured: {
      model: "grok-imagine-1.5-video",
      input_mode: "TEXT_TO_VIDEO",
      duration: 15,
      aspect_ratio: "16:9",
      resolution: "720p",
      ...overrides,
    },
    capabilities,
  });
}

test("structured video parameters build the canonical execution", () => {
  const request = build();
  assert.equal(request.schema_version, 1);
  assert.deepEqual(request.execution, {
    model: "grok-imagine-1.5-video",
    input_mode: "TEXT_TO_VIDEO",
    duration_seconds: 15,
    aspect_ratio: "16:9",
    resolution: "720p",
    reference_images: [],
    optional_parameters: {},
  });
  assert.equal(request.consistency_result.status, "ok");
});

test("quality is an input alias and is normalized to resolution", () => {
  const request = build({ resolution: undefined, quality: "720P" });
  assert.equal(request.execution.resolution, "720p");
  assert.equal("quality" in request.execution, false);
});

test("prompt duration conflict never overwrites structured duration", () => {
  const rebuilt = buildCanonicalVideoRequest({
    prompt: "生成30秒视频",
    structured: {
      model: "grok-imagine-1.5-video",
      input_mode: "TEXT_TO_VIDEO",
      duration: 15,
      aspect_ratio: "16:9",
      resolution: "720p",
    },
    capabilities,
  });
  assert.equal(rebuilt.execution.duration_seconds, 15);
  assert.equal(rebuilt.consistency_result.warnings[0].code, "VIDEO_PROMPT_DURATION_MISMATCH");
});

test("prompt aspect ratio conflict never overwrites structured aspect ratio", () => {
  const request = buildCanonicalVideoRequest({
    prompt: "生成 9:16 竖屏视频",
    structured: {
      model: "grok-imagine-1.5-video",
      input_mode: "TEXT_TO_VIDEO",
      duration: 15,
      aspect_ratio: "16:9",
      resolution: "720p",
    },
    capabilities,
  });
  assert.equal(request.execution.aspect_ratio, "16:9");
  assert.equal(request.consistency_result.warnings[0].code, "VIDEO_PROMPT_ASPECT_RATIO_MISMATCH");
});

test("timeline ranges do not create a duration hint", () => {
  const request = buildCanonicalVideoRequest({
    prompt: "0-8s 开场，8-18s 展示，18-30s 结尾",
    structured: {
      model: "grok-imagine-1.5-video",
      input_mode: "TEXT_TO_VIDEO",
      duration: 30,
      aspect_ratio: "16:9",
      resolution: "720p",
    },
    capabilities,
  });
  assert.equal(request.prompt_intent_hints.requested_duration_seconds, undefined);
  assert.equal(request.consistency_result.status, "ok");
});

test("reference image intent does not inject missing structured references", () => {
  const request = buildCanonicalVideoRequest({
    prompt: "根据3张参考图片生成视频",
    structured: {
      model: "grok-imagine-1.5-video",
      input_mode: "TEXT_TO_VIDEO",
      duration: 15,
      aspect_ratio: "16:9",
      resolution: "720p",
    },
    capabilities,
  });
  assert.deepEqual(request.execution.reference_images, []);
  assert.equal(request.prompt_intent_hints.reference_image_requested, true);
  assert.equal(request.consistency_result.warnings.some(item => item.code === "VIDEO_PROMPT_REFERENCE_MISSING"), true);
});

test("canonical representation is deterministic", () => {
  const first = canonicalVideoRequestRepresentation(build({ parameters: { generate_audio: true } }));
  const second = canonicalVideoRequestRepresentation(build({ parameters: { generate_audio: true } }));
  assert.equal(first, second);
});

test("same normalized input produces the same canonical representation", () => {
  const first = canonicalVideoRequestRepresentation(build({ aspect_ratio: "9 : 16", resolution: "720P" }));
  const second = canonicalVideoRequestRepresentation(build({ aspect_ratio: "9:16", resolution: "720p" }));
  assert.equal(first, second);
});

test("missing required fields fail with a canonical error", () => {
  assert.throws(
    () => build({ model: undefined }),
    error => error instanceof CanonicalVideoRequestError && error.code === "VIDEO_CANONICAL_REQUIRED",
  );
});

test("invalid mode, duration, and capability values are rejected", () => {
  assert.throws(() => build({ input_mode: "VIDEO_TO_VIDEO" }), /input_mode is invalid/);
  assert.throws(() => build({ duration: 0 }), /duration is invalid/);
  assert.throws(() => build({ resolution: "4k" }), /resolution is not supported/);
  assert.throws(() => build({ aspect_ratio: "4:3" }), /aspect_ratio is not supported/);
});

test("intent hints and consistency result cannot change execution fields", () => {
  const request = buildCanonicalVideoRequest({
    prompt: "30秒 9:16 1080p，根据参考图片",
    structured: {
      model: "grok-imagine-1.5-video",
      input_mode: "TEXT_TO_VIDEO",
      duration: 15,
      aspect_ratio: "16:9",
      resolution: "720p",
    },
    capabilities,
  });
  assert.deepEqual(
    [request.execution.duration_seconds, request.execution.aspect_ratio, request.execution.resolution, request.execution.reference_images],
    [15, "16:9", "720p", []],
  );
  assert.equal(request.prompt_intent_hints.requested_duration_seconds, 30);
  assert.equal(request.consistency_result.status, "warning");
});
