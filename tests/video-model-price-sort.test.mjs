import test from "node:test";
import assert from "node:assert/strict";
import { videoModelOptionsFromPublicModels } from "../admin-vue/src/utils/videoGeneration.ts";

test("web video model options map API title and price/capability subtitle", () => {
  const options = videoModelOptionsFromPublicModels([
    { code: "gpt-image-2", capabilities: ["TEXT_TO_IMAGE"], listPricePoints: 10 },
    {
      code: "grok-imagine-1.5-video",
      name: "Grok Imagine Video 1.5",
      displayName: "Grok Imagine Video 1.5",
      capabilities: ["TEXT_TO_VIDEO", "IMAGE_TO_VIDEO"],
      listPricePoints: 90,
      priceHint: "15 积分/秒",
      capabilityHint: "文生/图生 · 6–30s · 最多7图",
      priceLabel: "15 积分/秒 · 文生/图生 · 6–30s · 最多7图",
    },
    {
      code: "grok-imagine-video-1.5-preview",
      name: "Grok Imagine Video 1.5 Preview",
      displayName: "Grok Imagine Video 1.5 Preview",
      capabilities: ["IMAGE_TO_VIDEO"],
      listPricePoints: 100,
      priceHint: "100 积分/次",
      capabilityHint: "仅图生 · 10/15s · 需1张参考图",
    },
    {
      code: "seedance-fast-2.0",
      name: "Seedance 2.0",
      displayName: "Seedance 2.0",
      capabilities: ["TEXT_TO_VIDEO"],
      listPricePoints: 480,
      priceHint: "80 积分/秒",
      capabilityHint: "文生/图生",
    },
  ]);

  assert.deepEqual(options.map((item) => item.code), [
    "grok-imagine-1.5-video",
    "grok-imagine-video-1.5-preview",
    "seedance-fast-2.0",
  ]);
  assert.equal(options[0].name, "Grok Imagine Video 1.5");
  assert.equal(options[0].desc, "15 积分/秒 · 文生/图生 · 6–30s · 最多7图");
  assert.equal(options[1].desc, "100 积分/次 · 仅图生 · 10/15s · 需1张参考图");
  assert.equal(options[2].listPricePoints, 480);
});
