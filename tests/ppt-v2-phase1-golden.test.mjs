import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import { fileURLToPath } from "node:url";

import {
  assertValidDeckRevision,
  assertValidLayoutResult,
  assertValidRenderInput,
  assertValidSlideIR,
} from "../packages/ppt-v2/src/contract.mjs";
import { PptxGenJSRenderer } from "../packages/ppt-v2/src/pptxgenjs-renderer.mjs";
import { buildPhase1VerticalSlice } from "../packages/ppt-v2/src/vertical-slice.mjs";

function fixture(name) {
  const path = fileURLToPath(new URL(`../contracts/ppt-v2/fixtures/${name}`, import.meta.url));
  return JSON.parse(readFileSync(path, "utf8"));
}

test("Golden 1 Professional Business Deck has 100% semantic and geometry parity", () => {
  const input = fixture("golden-1-professional-business.legacy.json");
  const expectedSlides = fixture("golden-1-professional-business.slide-ir.json");
  const expectedLayout = fixture("golden-1-professional-business.layout-result.json");
  const result = buildPhase1VerticalSlice(input);

  assert.deepEqual(result.deck.slides, expectedSlides);
  assert.deepEqual(result.layoutResult, expectedLayout);
  assert.equal(result.contentPlan.kind, "fixed-two-slide");
  assert.deepEqual(result.contentPlan.pages, [
    { sequence: 1, role: "cover" },
    { sequence: 2, role: "content" },
  ]);
  assert.equal(result.deck.slides.flatMap((slide) => slide.elements).length, 7);
  assert.equal(result.layoutResult.slides.flatMap((slide) => slide.elements).length, 7);
  assert.equal(result.deck.migrationTrace.unmappedFields.length, 0);

  for (const slide of expectedSlides) {
    assert.equal(assertValidSlideIR(slide), slide);
  }
  assert.equal(assertValidDeckRevision(result.deck), result.deck);
  assert.equal(assertValidLayoutResult(expectedLayout), expectedLayout);
  assert.equal(assertValidRenderInput(result.renderInput), result.renderInput);
});

test("Golden 1 renderer is repeatable for the frozen fixtures", async () => {
  const result = buildPhase1VerticalSlice(fixture("golden-1-professional-business.legacy.json"));
  const renderer = new PptxGenJSRenderer();

  const first = await renderer.render(result.renderInput);
  const second = await renderer.render(structuredClone(result.renderInput));

  assert.deepEqual(second, first);
});

