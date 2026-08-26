import test from "node:test";
import assert from "node:assert/strict";
import {
  DEFAULT_VIDEO_MODEL_CODE,
  estimateFormalVideoPoints,
  pickDefaultVideoModelCode,
  sortVideoModelsByListPrice,
  videoModelSubtitle,
} from "../apps/user-uni/src/features/generation/videoModelPricing.ts";

test("formal video models sort cheap to expensive", () => {
  const sorted = sortVideoModelsByListPrice([
    { code: "seedance-fast-2.0", listPricePoints: 600 },
    { code: "doubao-seedance-2.0", listPricePoints: 480 },
    { code: "grok-imagine-1.5-video", listPricePoints: 90 },
    { code: "grok-imagine-video-1.5-preview", listPricePoints: 100 },
  ]);
  assert.deepEqual(sorted.map(item => item.code), [
    "grok-imagine-1.5-video",
    "grok-imagine-video-1.5-preview",
    "doubao-seedance-2.0",
    "seedance-fast-2.0",
  ]);
});

test("subtitle keeps capability hints without inventing a price", () => {
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
    "文生/图生 · 6–30s · 最多7图",
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
    "仅图生 · 10/15s · 需1张参考图",
  );
});

test("local estimate never invents a price when quote API is unavailable", () => {
  assert.equal(estimateFormalVideoPoints("grok-imagine-video-1.5-preview", 10, "720p"), 0);
  assert.equal(estimateFormalVideoPoints("grok-imagine-1.5-video", 6, "720p"), 0);
  assert.equal(estimateFormalVideoPoints("seedance-fast-2.0", 5, "720p"), 0);
  assert.equal(estimateFormalVideoPoints("doubao-seedance-2.0", 5, "720p"), 0);
});

test("default video model prefers grok-imagine-1.5-video over cheapest list item", () => {
  assert.equal(DEFAULT_VIDEO_MODEL_CODE, "grok-imagine-1.5-video");
  assert.equal(
    pickDefaultVideoModelCode([
      "grok-imagine-video-1.5-preview",
      "grok-imagine-1.5-video",
      "seedance-fast-2.0",
    ]),
    "grok-imagine-1.5-video",
  );
  assert.equal(
    pickDefaultVideoModelCode(["seedance-fast-2.0", "doubao-seedance-2.0"]),
    "seedance-fast-2.0",
  );
});
