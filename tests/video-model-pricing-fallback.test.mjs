import test from "node:test";
import assert from "node:assert/strict";
import {
  estimateFormalVideoPoints,
  sortVideoModelsByListPrice,
  videoModelSubtitle,
} from "../apps/user-uni/src/features/generation/videoModelPricing.ts";

test("formal video models sort cheap to expensive", () => {
  const sorted = sortVideoModelsByListPrice([
    { code: "seedance-fast-2.0" },
    { code: "doubao-seedance-2.0" },
    { code: "grok-imagine-1.5-video" },
    { code: "grok-imagine-video-1.5-preview" },
  ]);
  assert.deepEqual(sorted.map(item => item.code), [
    "grok-imagine-1.5-video",
    "grok-imagine-video-1.5-preview",
    "doubao-seedance-2.0",
    "seedance-fast-2.0",
  ]);
});

test("subtitle falls back to formal price and capability hints", () => {
  assert.equal(
    videoModelSubtitle({
      code: "grok-imagine-1.5-video",
      videoCapabilities: {
        supportsTextToVideo: true,
        supportsImageToVideo: true,
        maxReferenceImages: 7,
        supportedDurations: [6, 7, 8, 9, 10, 15, 20, 30],
      },
    }),
    "15 积分/秒 · 文生/图生 · 6–30s · 最多7图",
  );
  assert.equal(
    videoModelSubtitle({
      code: "grok-imagine-video-1.5-preview",
      videoCapabilities: {
        supportsTextToVideo: false,
        supportsImageToVideo: true,
        maxReferenceImages: 1,
        supportedDurations: [10, 15],
      },
    }),
    "100 积分/次 · 仅图生 · 10/15s · 需1张参考图",
  );
});

test("local formal estimate matches published rules", () => {
  assert.equal(estimateFormalVideoPoints("grok-imagine-video-1.5-preview", 10, "720p"), 100);
  assert.equal(estimateFormalVideoPoints("grok-imagine-1.5-video", 6, "720p"), 90);
  assert.equal(estimateFormalVideoPoints("seedance-fast-2.0", 5, "720p"), 600);
  assert.equal(estimateFormalVideoPoints("doubao-seedance-2.0", 5, "720p"), 600);
});
