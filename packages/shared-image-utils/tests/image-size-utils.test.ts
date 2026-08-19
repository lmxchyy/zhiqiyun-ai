import assert from "node:assert/strict";
import test from "node:test";

import {
  canonicalSizeParts,
  classifyCommonAspectRatio,
  deriveRatioFromSize,
  deriveTierFromSize,
  deriveRatioFromSizeValue,
  deriveTierFromSizeValue,
  displayImageSizeLabel,
  findSizeByRatioAndTier,
  getAvailableRatios,
  getAvailableTiersForRatio,
  getVisibleTiersForRatio,
  hasAutoOption,
  isCanonicalImageSize,
} from "../src/index.ts";

// ─── A. WxH → ratio / tier ───────────────────────────────────────────

test("A.1 1024x1024 → 1K + 1:1", () => {
  assert.equal(deriveRatioFromSize(1024, 1024), "1:1");
  assert.equal(deriveTierFromSize(1024, 1024), "1K");
});

test("A.2 2048x2048 → 2K + 1:1", () => {
  assert.equal(deriveRatioFromSize(2048, 2048), "1:1");
  assert.equal(deriveTierFromSize(2048, 2048), "2K");
});

test("A.3 2048x1152 → 2K + 16:9", () => {
  assert.equal(deriveRatioFromSize(2048, 1152), "16:9");
  assert.equal(deriveTierFromSize(2048, 1152), "2K");
});

test("A.4 3840x2160 → 4K + 16:9", () => {
  assert.equal(deriveRatioFromSize(3840, 2160), "16:9");
  assert.equal(deriveTierFromSize(3840, 2160), "4K");
});

test("A.5 1344x1008 → 1K + 4:3", () => {
  assert.equal(deriveRatioFromSize(1344, 1008), "4:3");
  assert.equal(deriveTierFromSize(1344, 1008), "1K");
});

test("A.6 1536x2048 → 2K + 3:4", () => {
  assert.equal(deriveRatioFromSize(1536, 2048), "3:4");
  assert.equal(deriveTierFromSize(1536, 2048), "2K");
});

test("A.7 1280x720 → 720p + 16:9", () => {
  assert.equal(deriveRatioFromSize(1280, 720), "16:9");
  assert.equal(deriveTierFromSize(1280, 720), "720p");
});

// ─── B. ratio + tier → size ──────────────────────────────────────────

const FULL_SCHEMA_SIZES = [
  "auto",
  "1024x1024",
  "1536x1024",
  "1024x1536",
  "1280x720",
  "720x1280",
  "2048x1152",
  "2048x2048",
  "3840x2160",
  "2160x3840",
  "1344x1008",
  "2048x1536",
  "3264x2448",
  "1008x1344",
  "1536x2048",
  "2448x3264",
  "2048x1360",
  "3520x2352",
  "1360x2048",
  "2352x3520",
  "1152x2048",
];

test("B.1 16:9 + 2K → 2048x1152", () => {
  assert.equal(findSizeByRatioAndTier(FULL_SCHEMA_SIZES, "16:9", "2K"), "2048x1152");
});

test("B.2 1:1 + 1K → 1024x1024", () => {
  assert.equal(findSizeByRatioAndTier(FULL_SCHEMA_SIZES, "1:1", "1K"), "1024x1024");
});

test("B.3 1:1 + 2K → 2048x2048", () => {
  assert.equal(findSizeByRatioAndTier(FULL_SCHEMA_SIZES, "1:1", "2K"), "2048x2048");
});

test("B.4 1:1 + 4K → 3840x2160 is NOT 1:1, should be undefined", () => {
  assert.equal(findSizeByRatioAndTier(FULL_SCHEMA_SIZES, "1:1", "4K"), undefined);
});

test("B.5 16:9 + 4K → 3840x2160", () => {
  assert.equal(findSizeByRatioAndTier(FULL_SCHEMA_SIZES, "16:9", "4K"), "3840x2160");
});

test("B.6 4:3 + 1K → 1344x1008", () => {
  assert.equal(findSizeByRatioAndTier(FULL_SCHEMA_SIZES, "4:3", "1K"), "1344x1008");
});

test("B.7 4:3 + 2K → 2048x1536", () => {
  assert.equal(findSizeByRatioAndTier(FULL_SCHEMA_SIZES, "4:3", "2K"), "2048x1536");
});

test("B.8 3:4 + 1K → 1008x1344", () => {
  assert.equal(findSizeByRatioAndTier(FULL_SCHEMA_SIZES, "3:4", "1K"), "1008x1344");
});

test("B.9 9:16 + 2K → 1152x2048", () => {
  assert.equal(findSizeByRatioAndTier(FULL_SCHEMA_SIZES, "9:16", "2K"), "1152x2048");
});

// ─── C. 非法组合 ──────────────────────────────────────────────────────

const LIMITED_SIZES = ["auto", "1024x1024", "1536x1024", "1024x1536", "2048x2048"];

test("C.1 3:2 + 2K not in limited schema → undefined", () => {
  assert.equal(findSizeByRatioAndTier(LIMITED_SIZES, "3:2", "2K"), undefined);
});

test("C.2 16:9 + 4K not in limited schema → undefined", () => {
  assert.equal(findSizeByRatioAndTier(LIMITED_SIZES, "16:9", "4K"), undefined);
});

// ─── D. auto ──────────────────────────────────────────────────────────

test("D.1 hasAutoOption detects auto in schema", () => {
  assert.equal(hasAutoOption(FULL_SCHEMA_SIZES), true);
  assert.equal(hasAutoOption(["1024x1024"]), false);
});

test("D.2 deriveTierFromSizeValue('auto') returns 'auto'", () => {
  assert.equal(deriveTierFromSizeValue("auto"), "auto");
});

test("D.3 deriveRatioFromSizeValue('auto') returns undefined", () => {
  assert.equal(deriveRatioFromSizeValue("auto"), undefined);
});

// ─── E. quality (separate dimension, not tested here but verified by separation) ──

test("E.1 displayImageSizeLabel does not mention quality", () => {
  const label = displayImageSizeLabel("2048x1152");
  assert.ok(!label.includes("low") && !label.includes("medium") && !label.includes("high"));
  assert.ok(label.includes("2K"));
  assert.ok(label.includes("16:9"));
});

test("E.2 displayImageSizeLabel('auto') returns 'auto'", () => {
  assert.equal(displayImageSizeLabel("auto"), "auto");
});

// ─── F. 模型切换 (simulated via different schema sets) ───────────────

test("F.1 model with 2K + 16:9, switch to model without that combo → undefined", () => {
  const modelASizes = ["auto", "1024x1024", "2048x1152", "2048x2048"];
  const modelBSizes = ["auto", "1024x1024", "1536x1024", "1024x1536"];
  assert.equal(findSizeByRatioAndTier(modelASizes, "16:9", "2K"), "2048x1152");
  assert.equal(findSizeByRatioAndTier(modelBSizes, "16:9", "2K"), undefined);
});

// ─── G. 三端一致 (same schema → same ratio/tier sets) ────────────────

test("G.1 groupSizesByRatio produces consistent ratio set", () => {
  const ratios = getAvailableRatios(FULL_SCHEMA_SIZES);
  assert.ok(ratios.includes("1:1"));
  assert.ok(ratios.includes("16:9"));
  assert.ok(ratios.includes("9:16"));
  assert.ok(ratios.includes("4:3"));
  assert.ok(ratios.includes("3:4"));
  assert.ok(ratios.includes("3:2"));
});

test("G.2 getAvailableTiersForRatio produces consistent tier set", () => {
  const tiers16x9 = getAvailableTiersForRatio(FULL_SCHEMA_SIZES, "16:9");
  assert.ok(tiers16x9.includes("720p"));
  assert.ok(tiers16x9.includes("2K"));
  assert.ok(tiers16x9.includes("4K"));
});

test("G.3 canonicalSizeParts validates correctly", () => {
  assert.deepEqual(canonicalSizeParts("1024x1024"), [1024, 1024]);
  assert.equal(canonicalSizeParts("auto"), undefined);
  assert.equal(canonicalSizeParts("invalid"), undefined);
  assert.equal(canonicalSizeParts(123 as unknown as string), undefined);
});

test("G.4 isCanonicalImageSize validates correctly", () => {
  assert.equal(isCanonicalImageSize("auto"), true);
  assert.equal(isCanonicalImageSize("1024x1024"), true);
  assert.equal(isCanonicalImageSize("invalid"), false);
  assert.equal(isCanonicalImageSize(""), false);
});

test("H.1 near-common sizes classify to 3:2 / 2:3 instead of reduced odd ratios", () => {
  assert.equal(deriveRatioFromSize(2048, 1360), "128:85");
  assert.equal(deriveRatioFromSize(1360, 2048), "85:128");
  assert.equal(deriveRatioFromSize(3520, 2352), "220:147");
  assert.equal(deriveRatioFromSize(2352, 3520), "147:220");
  assert.equal(classifyCommonAspectRatio(2048, 1360), "3:2");
  assert.equal(classifyCommonAspectRatio(1360, 2048), "2:3");
  assert.equal(classifyCommonAspectRatio(3520, 2352), "3:2");
  assert.equal(classifyCommonAspectRatio(2352, 3520), "2:3");
  assert.equal(deriveRatioFromSizeValue("2048x1360"), "3:2");
  assert.equal(deriveRatioFromSizeValue("3520x2352"), "3:2");
});

test("H.2 common ratio list hides reduced odd ratios", () => {
  const ratios = getAvailableRatios(FULL_SCHEMA_SIZES);
  assert.deepEqual(ratios, ["1:1", "16:9", "9:16", "4:3", "3:4", "3:2", "2:3"]);
  assert.ok(!ratios.includes("128:85"));
  assert.ok(!ratios.includes("85:128"));
  assert.ok(!ratios.includes("220:147"));
  assert.ok(!ratios.includes("147:220"));
});

test("H.3 3:2 / 2:3 near-common sizes resolve to schema WxH, never invented 3840x3840", () => {
  assert.equal(findSizeByRatioAndTier(FULL_SCHEMA_SIZES, "3:2", "1K"), "1536x1024");
  assert.equal(findSizeByRatioAndTier(FULL_SCHEMA_SIZES, "3:2", "2K"), "2048x1360");
  assert.equal(findSizeByRatioAndTier(FULL_SCHEMA_SIZES, "3:2", "4K"), "3520x2352");
  assert.equal(findSizeByRatioAndTier(FULL_SCHEMA_SIZES, "2:3", "1K"), "1024x1536");
  assert.equal(findSizeByRatioAndTier(FULL_SCHEMA_SIZES, "2:3", "2K"), "1360x2048");
  assert.equal(findSizeByRatioAndTier(FULL_SCHEMA_SIZES, "2:3", "4K"), "2352x3520");
  assert.equal(findSizeByRatioAndTier(FULL_SCHEMA_SIZES, "1:1", "4K"), undefined);
  assert.ok(!FULL_SCHEMA_SIZES.includes("3840x3840"));
  assert.notEqual(findSizeByRatioAndTier(FULL_SCHEMA_SIZES, "1:1", "4K"), "3840x3840");
  assert.deepEqual(getAvailableTiersForRatio(FULL_SCHEMA_SIZES, "3:2"), ["1K", "2K", "4K"]);
  assert.deepEqual(getAvailableTiersForRatio(FULL_SCHEMA_SIZES, "1:1"), ["1K", "2K"]);
  assert.deepEqual(getVisibleTiersForRatio(FULL_SCHEMA_SIZES, "16:9"), ["2K", "4K"]);
  assert.deepEqual(getVisibleTiersForRatio(FULL_SCHEMA_SIZES, "1:1"), ["1K", "2K"]);
});
