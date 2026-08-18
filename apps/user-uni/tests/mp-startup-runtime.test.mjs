import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

const appRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const vendorPath = path.resolve(appRoot, "dist", "build", "mp-weixin", "common", "vendor.js");
const imageGeneratorWxssPath = path.resolve(
  appRoot,
  "dist",
  "build",
  "mp-weixin",
  "components",
  "creation",
  "AiImageGenerator.wxss",
);
const imageGeneratorWxmlPath = path.resolve(
  appRoot,
  "dist",
  "build",
  "mp-weixin",
  "components",
  "creation",
  "AiImageGenerator.wxml",
);
const legacyUnsupportedDescendantUniversalSelector = /\s\*\.[a-zA-Z_-]/;

function selectorContainsUniversalToken(selector) {
  let quote = "";
  let attributeDepth = 0;
  for (let index = 0; index < selector.length; index += 1) {
    const character = selector[index];
    if (quote) {
      if (character === "\\") index += 1;
      else if (character === quote) quote = "";
      continue;
    }
    if (character === '"' || character === "'") {
      quote = character;
      continue;
    }
    if (character === "[") {
      attributeDepth += 1;
      continue;
    }
    if (character === "]" && attributeDepth > 0) {
      attributeDepth -= 1;
      continue;
    }
    if (character !== "*" || attributeDepth > 0) continue;

    const previous = selector[index - 1] || "";
    const next = selector[index + 1] || "";
    const validBefore = !previous || /[\s>+~,(|]/.test(previous);
    const validAfter = !next || /[\s.#:[>+~),|]/.test(next);
    if (validBefore && validAfter) return true;
  }
  return false;
}

function findUnsupportedUniversalSelectors(wxssSource) {
  const source = wxssSource.replace(/\/\*[\s\S]*?\*\//g, match => " ".repeat(match.length));
  const selectors = [];
  let segmentStart = 0;
  let braceDepth = 0;
  let quote = "";
  for (let index = 0; index < source.length; index += 1) {
    const character = source[index];
    if (quote) {
      if (character === "\\") index += 1;
      else if (character === quote) quote = "";
      continue;
    }
    if (character === '"' || character === "'") {
      quote = character;
      continue;
    }
    if (character === "}") {
      braceDepth = Math.max(0, braceDepth - 1);
      segmentStart = index + 1;
      continue;
    }
    if (character === ";" && braceDepth === 0) {
      segmentStart = index + 1;
      continue;
    }
    if (character !== "{") continue;

    const selector = source.slice(segmentStart, index).trim();
    if (selector && !selector.startsWith("@") && selectorContainsUniversalToken(selector)) {
      selectors.push(selector);
    }
    braceDepth += 1;
    segmentStart = index + 1;
  }
  return selectors;
}

test("release startup excludes unconfigured uni statistics reporting", () => {
  const vendorSource = fs.readFileSync(vendorPath, "utf8");

  assert.doesNotMatch(vendorSource, /\[uni统计 2\.0\]/);
  assert.doesNotMatch(vendorSource, /统计上报超时\(preloadAssets\)/);
});

test("release image generator excludes WXSS-invalid universal selectors", () => {
  const wxssSource = fs.readFileSync(imageGeneratorWxssPath, "utf8");

  assert.deepEqual(findUnsupportedUniversalSelectors(wxssSource), []);
});

test("WXSS selector scan catches every descendant universal selector form", () => {
  const legacyMiss = ".foo * {}";
  assert.doesNotMatch(legacyMiss, legacyUnsupportedDescendantUniversalSelector);
  assert.deepEqual(findUnsupportedUniversalSelectors(legacyMiss), [".foo *"]);

  for (const [invalidSource, expectedSelector] of [
    [".foo * {}", ".foo *"],
    [".foo *.bar {}", ".foo *.bar"],
    [".foo *:first-child {}", ".foo *:first-child"],
    [".foo > * {}", ".foo > *"],
    [".foo>* + .bar {}", ".foo>* + .bar"],
    ['@import "./base.wxss"; .foo * {}', ".foo *"],
  ]) {
    assert.deepEqual(findUnsupportedUniversalSelectors(invalidSource), [expectedSelector]);
  }

  const legalSource = `
    /* .ignored * {} */
    @media (min-width: 320px) { .foo > .bar { color: red; } }
    .foo::before { content: "*"; }
    .foo[data-token="*"] { color: blue; }
  `;
  assert.deepEqual(findUnsupportedUniversalSelectors(legalSource), []);
});

test("release image generator owns safe-area variables and keeps actions on one line", () => {
  const wxmlSource = fs.readFileSync(imageGeneratorWxmlPath, "utf8");
  const wxssSource = fs.readFileSync(imageGeneratorWxssPath, "utf8");
  const rootTag = wxmlSource.match(/^<view\b[^>]*class="ai-image-generator\b[^>]*>/)?.[0] || "";

  assert.match(rootTag, /\bstyle="{{[^\"]+}}"/);
  assert.match(wxssSource, /\.ai-image-generator__header[^{}]*\{[^}]*box-sizing:\s*border-box/);
  assert.match(wxssSource, /padding-right:\s*max\(16px,\s*var\(--capsule-right-space,\s*0px\)\)/);
  assert.match(wxssSource, /\.ai-image-generator__view-result[^{}]*\{[^}]*flex:\s*0 0 auto[^}]*white-space:\s*nowrap/);
  assert.match(wxssSource, /\.ai-image-generator__generate[^{}]*\{[^}]*flex:\s*0 0 auto[^}]*white-space:\s*nowrap/);
});
