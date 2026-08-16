import assert from "node:assert/strict";
import test from "node:test";

import { golden2PreviewFixture } from "./ppt-preview/golden-2-preview-fixture.mjs";

function revisionFromGolden2() {
  const base = golden2PreviewFixture().projection;
  return structuredClone(base);
}

function applyGolden3Edits(base) {
  const revision = structuredClone(base);
  const slides = revision.deck.slides;
  const unchanged = slides.filter((slide) => ![slides[1].id, slides[3].id, slides[5].id].includes(slide.id));
  slides[1].elements[0].text = "Verified demand signals are reshaping priorities.";
  revision.layoutResult.slides.find((slide) => slide.slideId === slides[3].id).layoutId = "two-column";
  const image = slides[5].elements.find((element) => element.type === "image");
  if (image) image.assetRef = "asset_intent_4";
  revision.revision += 1;
  return { revision, unchanged };
}

test("Golden 3 preserves unaffected semantic and geometry parity", () => {
  const base = revisionFromGolden2();
  const { revision, unchanged } = applyGolden3Edits(base);
  assert.equal(revision.revision, base.revision + 1);
  for (const slide of unchanged) {
    const next = revision.deck.slides.find((candidate) => candidate.id === slide.id);
    assert.deepEqual(next, slide, `unaffected slide ${slide.id} changed`);
    const beforeLayout = base.layoutResult.slides.find((candidate) => candidate.slideId === slide.id);
    const afterLayout = revision.layoutResult.slides.find((candidate) => candidate.slideId === slide.id);
    assert.deepEqual(afterLayout, beforeLayout, `unaffected geometry ${slide.id} changed`);
  }
  assert.notDeepEqual(revision.deck.slides[1], base.deck.slides[1]);
  assert.notDeepEqual(revision.layoutResult.slides.find((slide) => slide.slideId === base.deck.slides[3].id), base.layoutResult.slides.find((slide) => slide.slideId === base.deck.slides[3].id));
  assert.equal(revision.deck.slides[5].id, base.deck.slides[5].id);
});

test("Golden 3 preview projection retains stable element identities and private assets", () => {
  const base = revisionFromGolden2();
  const { revision } = applyGolden3Edits(base);
  const baseIDs = new Set(base.deck.slides.flatMap((slide) => slide.elements.map((element) => element.id)));
  const nextIDs = new Set(revision.deck.slides.flatMap((slide) => slide.elements.map((element) => element.id)));
  assert.deepEqual(nextIDs, baseIDs);
  for (const asset of revision.assets) assert.match(asset.url, /^data:image\//);
});
