import test from "node:test";
import assert from "node:assert/strict";
import {
  generationParametersFromDraft,
  taskRequestFromDraft,
} from "../packages/business-sdk/dist/mappers.js";

const textOnlyVideoCapabilities = {
  supportsTextToVideo: true,
  supportsImageToVideo: false,
  supportsFirstFrame: false,
  supportsLastFrame: false,
  maxReferenceImages: 0,
  supportedDurations: [5],
  supportedResolutions: ["720p"],
  supportedAspectRatios: ["16:9"],
  supportedParameters: [
    "duration",
    "resolution",
    "aspect_ratio",
    "fps",
    "generate_audio",
    "motion_strength",
    "camera_movement",
  ],
};

const dualVideoCapabilities = {
  ...textOnlyVideoCapabilities,
  supportsImageToVideo: true,
  supportsFirstFrame: true,
  maxReferenceImages: 1,
};

function videoDraft(overrides = {}) {
  return {
    mode: "video",
    videoMode: "TEXT_TO_VIDEO",
    prompt: "a cinematic city sunrise",
    model: "video-model",
    style: "cinematic",
    size: "16:9",
    quality: "720p",
    count: 1,
    referenceImages: [],
    duration: 5,
    videoCapabilities: dualVideoCapabilities,
    ...overrides,
  };
}

test("home creation draft metadata is not sent as model parameters", () => {
  const request = taskRequestFromDraft({
    mode: "image",
    prompt: "generate a product image",
    model: "gpt-image-2",
    style: "commercial",
    size: "1024x1024",
    quality: "standard",
    count: 1,
    referenceImages: [],
    parameters: {
      mode: "image",
      prompt: "generate a product image",
      referencePaths: [],
      files: [],
    },
  });

  assert.deepEqual(request.params, {
    size: "1024x1024",
    quality: "standard",
    n: 1,
  });
});

test("template parameters survive while navigation fields are removed", () => {
  const params = generationParametersFromDraft({
    mode: "image",
    prompt: "template prompt",
    model: "gpt-image-2",
    referenceImages: ["https://example.test/reference.png"],
    seed: 42,
    custom_schema_parameter: "preserved",
  });

  assert.deepEqual(params, {
    seed: 42,
    custom_schema_parameter: "preserved",
  });
});

test("asset recreation maps source ids to accepted provenance parameters", () => {
  const params = generationParametersFromDraft({
    intent: "regenerate",
    index: 0,
    sourceAssetId: "asset-1",
    sourceTaskId: "task-1",
    aspectRatio: "1:1",
    restoredParams: { seed: 7 },
  });

  assert.deepEqual(params, {
    seed: 7,
    sourceReferenceAssetId: "asset-1",
    sourceReferenceTaskId: "task-1",
  });
});

test("provider output metadata is not replayed as generation parameters", () => {
  const params = generationParametersFromDraft({
    restoredParams: {
      seed: 7,
      providerRevisedPrompt: "provider rewritten prompt",
      provider_revised_prompt: "provider rewritten prompt",
      referenceCount: 1,
      contentType: "image/png",
      providerTaskId: "provider-task",
      thumbnailUrl: "https://example.test/thumb.png",
      storageObjectKey: "tenant/asset.png",
      ai_generated: true,
      output_audit_status: "approved",
    },
  });

  assert.deepEqual(params, { seed: 7 });
});

test("explicit text-to-video request never carries image fields", () => {
  const request = taskRequestFromDraft(videoDraft({
    videoMode: "TEXT_TO_VIDEO",
    parameters: { motion_strength: "medium", generate_audio: true },
  }));

  assert.equal(request.type, "TEXT_TO_VIDEO");
  assert.deepEqual(request.params, {
    duration: 5,
    resolution: "720p",
    aspect_ratio: "16:9",
    generate_audio: true,
    motion_strength: "medium",
  });
});

test("text-to-video rejects a residual first frame instead of silently dropping it", () => {
  assert.throws(
    () => taskRequestFromDraft(videoDraft({ videoMode: "TEXT_TO_VIDEO", firstFrame: "https://example.test/first.png" })),
    error => error?.code === "VIDEO_TEXT_MODE_IMAGE_FORBIDDEN" && /文生视频/.test(error.message),
  );
});

test("image-to-video rejects a missing first frame", () => {
  assert.throws(
    () => taskRequestFromDraft(videoDraft({ videoMode: "IMAGE_TO_VIDEO" })),
    error => error?.code === "VIDEO_FIRST_FRAME_REQUIRED" && /首帧图/.test(error.message),
  );
});

test("image-to-video sends exactly one canonical first frame", () => {
  const request = taskRequestFromDraft(videoDraft({
    videoMode: "IMAGE_TO_VIDEO",
    firstFrame: "https://example.test/first.png",
  }));

  assert.equal(request.type, "IMAGE_TO_VIDEO");
  assert.equal(request.params.first_frame, "https://example.test/first.png");
  for (const key of ["reference_image", "referenceImages", "reference_images", "image_url", "image_urls"]) {
    assert.equal(Object.hasOwn(request.params, key), false, `${key} must not leak into the normalized request`);
  }
});

test("legacy video drafts with multiple reference images are rejected", () => {
  assert.throws(
    () => taskRequestFromDraft(videoDraft({
      videoMode: "IMAGE_TO_VIDEO",
      firstFrame: "https://example.test/first.png",
      referenceImages: ["https://example.test/first.png", "https://example.test/second.png"],
    })),
    error => error?.code === "VIDEO_IMAGE_LIMIT_EXCEEDED",
  );
});

test("text-only model rejects image-to-video mode", () => {
  assert.throws(
    () => taskRequestFromDraft(videoDraft({
      videoMode: "IMAGE_TO_VIDEO",
      firstFrame: "https://example.test/first.png",
      videoCapabilities: textOnlyVideoCapabilities,
    })),
    error => error?.code === "VIDEO_MODE_NOT_SUPPORTED",
  );
});

test("last frame is sent only when the model supports it", () => {
  const capabilities = {
    ...dualVideoCapabilities,
    supportsLastFrame: true,
    maxReferenceImages: 2,
  };
  const request = taskRequestFromDraft(videoDraft({
    videoMode: "IMAGE_TO_VIDEO",
    firstFrame: "https://example.test/first.png",
    lastFrame: "https://example.test/last.png",
    videoCapabilities: capabilities,
  }));

  assert.equal(request.params.first_frame, "https://example.test/first.png");
  assert.equal(request.params.last_frame, "https://example.test/last.png");
});

test("unsupported last frame returns a stable validation code", () => {
  assert.throws(
    () => taskRequestFromDraft(videoDraft({
      videoMode: "IMAGE_TO_VIDEO",
      firstFrame: "https://example.test/first.png",
      lastFrame: "https://example.test/last.png",
    })),
    error => error?.code === "VIDEO_LAST_FRAME_NOT_SUPPORTED",
  );
});

test("legacy capability defaults are safe and do not open image-to-video", () => {
  assert.deepEqual(normalizeVideoModelCapabilities(undefined), {
    supportsTextToVideo: true,
    supportsImageToVideo: false,
    supportsFirstFrame: false,
    supportsLastFrame: false,
    maxReferenceImages: 0,
    supportedDurations: [],
    supportedResolutions: [],
    supportedAspectRatios: [],
    supportedParameters: ["duration", "resolution", "aspect_ratio"],
  });
});

test("video request omits optional parameters that the Provider cannot transmit", () => {
  const request = taskRequestFromDraft(videoDraft({
    videoCapabilities: {
      ...dualVideoCapabilities,
      supportedParameters: ["duration", "resolution", "aspect_ratio"],
    },
    parameters: {
      fps: 30,
      generate_audio: true,
      motion_strength: "high",
      camera_movement: "push",
    },
  }));

  assert.deepEqual(request.params, {
    duration: 5,
    resolution: "720p",
    aspect_ratio: "16:9",
  });
});

test("video request preserves every supported canonical user selection", () => {
  const cases = [
    { aspectRatio: "16:9", resolution: "480p", duration: 4, fps: 24, audio: true, motion: "low", camera: "static" },
    { aspectRatio: "9:16", resolution: "720p", duration: 5, fps: 30, audio: false, motion: "medium", camera: "pan" },
    { aspectRatio: "1:1", resolution: "1080p", duration: 10, fps: 24, audio: true, motion: "high", camera: "push" },
    { aspectRatio: "16:9", resolution: "4k", duration: 15, fps: 30, audio: false, motion: "low", camera: "pull" },
  ];

  for (const item of cases) {
    const capabilities = {
      ...dualVideoCapabilities,
      supportedDurations: [4, 5, 10, 15],
      supportedResolutions: ["480p", "720p", "1080p", "4k"],
      supportedAspectRatios: ["16:9", "9:16", "1:1"],
    };
    const request = taskRequestFromDraft(videoDraft({
      size: item.aspectRatio,
      quality: item.resolution,
      duration: item.duration,
      videoCapabilities: capabilities,
      parameters: {
        duration: item.duration,
        resolution: item.resolution,
        aspect_ratio: item.aspectRatio,
        fps: item.fps,
        generate_audio: item.audio,
        motion_strength: item.motion,
        camera_movement: item.camera,
      },
    }));

    assert.deepEqual(request.params, {
      duration: item.duration,
      resolution: item.resolution,
      aspect_ratio: item.aspectRatio,
      fps: item.fps,
      generate_audio: item.audio,
      motion_strength: item.motion,
      camera_movement: item.camera,
    });
    assert.equal(Object.hasOwn(request.params, "ratio"), false);
  }
});

test("text-only model locks text mode and clears incompatible frames", () => {
  const result = reconcileVideoGenerationState({
    mode: "IMAGE_TO_VIDEO",
    firstFrame: "https://example.test/first.png",
    lastFrame: "https://example.test/last.png",
  }, textOnlyVideoCapabilities);

  assert.deepEqual(result, {
    mode: "TEXT_TO_VIDEO",
    firstFrame: "",
    lastFrame: "",
    modeChanged: true,
    clearedFirstFrame: true,
    clearedLastFrame: true,
  });
});

test("dual-mode model preserves an explicitly selected image mode", () => {
  const result = reconcileVideoGenerationState({
    mode: "IMAGE_TO_VIDEO",
    firstFrame: "https://example.test/first.png",
    lastFrame: "",
  }, dualVideoCapabilities);

  assert.equal(result.mode, "IMAGE_TO_VIDEO");
  assert.equal(result.firstFrame, "https://example.test/first.png");
  assert.equal(result.modeChanged, false);
});
