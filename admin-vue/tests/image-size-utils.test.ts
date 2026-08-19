import assert from "node:assert/strict";
import test from "node:test";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

// Load the browser global module by evaluating it in a mock window context.
const mockWindow = {};
const __dirname = path.dirname(fileURLToPath(import.meta.url));
const code = fs.readFileSync(
  path.resolve(__dirname, "../public/static/js/image-size-utils.js"),
  "utf-8",
);
const fn = new Function("window", code);
fn(mockWindow);
const U = mockWindow.ImageSizeUtils;

// ─── A. WxH → ratio / tier ───────────────────────────────────────────

test("Web A.1 1024x1024 → 1K + 1:1", () => {
  assert.equal(U.deriveRatio(1024, 1024), "1:1");
  assert.equal(U.deriveTier(1024, 1024), "1K");
});

test("Web A.2 2048x2048 → 2K + 1:1", () => {
  assert.equal(U.deriveRatio(2048, 2048), "1:1");
  assert.equal(U.deriveTier(2048, 2048), "2K");
});

test("Web A.3 2048x1152 → 2K + 16:9", () => {
  assert.equal(U.deriveRatio(2048, 1152), "16:9");
  assert.equal(U.deriveTier(2048, 1152), "2K");
});

test("Web A.4 3840x2160 → 4K + 16:9", () => {
  assert.equal(U.deriveRatio(3840, 2160), "16:9");
  assert.equal(U.deriveTier(3840, 2160), "4K");
});

// ─── B. ratio + tier → size ──────────────────────────────────────────

const FULL_SCHEMA_SIZES = [
  "auto", "1024x1024", "1536x1024", "1024x1536", "1280x720", "720x1280",
  "2048x1152", "2048x2048", "3840x2160", "2160x3840",
  "1344x1008", "2048x1536", "3264x2448",
  "1008x1344", "1536x2048", "2448x3264",
  "2048x1360", "3520x2352",
  "1360x2048", "2352x3520",
  "1152x2048",
];

test("Web B.1 16:9 + 2K → 2048x1152", () => {
  assert.equal(U.findSizeByRatioAndTier(FULL_SCHEMA_SIZES, "16:9", "2K"), "2048x1152");
});

test("Web B.2 1:1 + 1K → 1024x1024", () => {
  assert.equal(U.findSizeByRatioAndTier(FULL_SCHEMA_SIZES, "1:1", "1K"), "1024x1024");
});

test("Web B.3 16:9 + 4K → 3840x2160", () => {
  assert.equal(U.findSizeByRatioAndTier(FULL_SCHEMA_SIZES, "16:9", "4K"), "3840x2160");
});

test("Web B.4 4:3 + 1K → 1344x1008", () => {
  assert.equal(U.findSizeByRatioAndTier(FULL_SCHEMA_SIZES, "4:3", "1K"), "1344x1008");
});

// ─── C. 非法组合 ──────────────────────────────────────────────────────

test("Web C.1 3:2 + 2K not in limited schema → undefined", () => {
  const limited = ["auto", "1024x1024", "1536x1024", "1024x1536", "2048x2048"];
  assert.equal(U.findSizeByRatioAndTier(limited, "3:2", "2K"), undefined);
});

// ─── D. auto ──────────────────────────────────────────────────────────

test("Web D.1 hasAutoOption detects auto", () => {
  assert.equal(U.hasAutoOption(FULL_SCHEMA_SIZES), true);
  assert.equal(U.hasAutoOption(["1024x1024"]), false);
});

// ─── G. 三端一致 ─────────────────────────────────────────────────────

test("Web G.1 same ratio set as shared package", () => {
  const ratios = U.getAvailableRatios(FULL_SCHEMA_SIZES);
  assert.ok(ratios.includes("1:1"));
  assert.ok(ratios.includes("16:9"));
  assert.ok(ratios.includes("9:16"));
  assert.ok(ratios.includes("4:3"));
  assert.ok(ratios.includes("3:4"));
  assert.ok(ratios.includes("3:2"));
});

test("Web G.2 same tier set for 16:9", () => {
  const tiers = U.getAvailableTiersForRatio(FULL_SCHEMA_SIZES, "16:9");
  assert.ok(tiers.includes("720p"));
  assert.ok(tiers.includes("2K"));
  assert.ok(tiers.includes("4K"));
});

test("Web G.3 displayImageSizeLabel matches shared package format", () => {
  assert.equal(U.displayImageSizeLabel("auto"), "auto");
  const label = U.displayImageSizeLabel("2048x1152");
  assert.ok(label.includes("2K"));
  assert.ok(label.includes("16:9"));
});

test("Web H.1 3:2 / 2:3 near-common sizes match schema WxH", () => {
  assert.equal(U.classifyCommonAspectRatio(2048, 1360), "3:2");
  assert.equal(U.classifyCommonAspectRatio(3520, 2352), "3:2");
  assert.equal(U.findSizeByRatioAndTier(FULL_SCHEMA_SIZES, "3:2", "2K"), "2048x1360");
  assert.equal(U.findSizeByRatioAndTier(FULL_SCHEMA_SIZES, "3:2", "4K"), "3520x2352");
  assert.equal(U.findSizeByRatioAndTier(FULL_SCHEMA_SIZES, "2:3", "2K"), "1360x2048");
  assert.equal(U.findSizeByRatioAndTier(FULL_SCHEMA_SIZES, "2:3", "4K"), "2352x3520");
  assert.equal(U.findSizeByRatioAndTier(FULL_SCHEMA_SIZES, "1:1", "4K"), undefined);
  const ratios = U.getAvailableRatios(FULL_SCHEMA_SIZES);
  assert.ok(!ratios.includes("128:85"));
  assert.ok(!ratios.includes("85:128"));
  assert.ok(!ratios.includes("220:147"));
  assert.ok(!ratios.includes("147:220"));
});
