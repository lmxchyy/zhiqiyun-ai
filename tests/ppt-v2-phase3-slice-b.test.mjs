import assert from "node:assert/strict";
import { execFileSync, spawnSync } from "node:child_process";
import { mkdtempSync, rmdirSync, unlinkSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";

import JSZip from "jszip";

import {
  assertValidDeckRevision,
  assertValidLayoutResult,
  assertValidRenderInput,
} from "../packages/ppt-v2/src/contract.mjs";
import { professionalLayoutDefinitions } from "../packages/ppt-v2/src/layout-compiler.mjs";
import { buildProfessionalDeck, runProfessionalQualityGate } from "../packages/ppt-v2/src/professional-deck.mjs";
import { PptxGenJSRenderer } from "../packages/ppt-v2/src/pptxgenjs-renderer.mjs";

const PNG_1X1 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=";
const PNG_SHA256 = "dfe7badda145ff43e5c49ea6e49e40559fe904a7876c78cbaf8a056b8084e5fe";

function sliceBInput(pageCount = 8, language = "en-US") {
  const chinese = language === "zh-CN";
  const slides = Array.from({ length: pageCount }, (_, index) => {
    const sequence = index + 1;
    const factual = sequence > 1 && sequence < pageCount;
    const withImage = sequence === 4 || (sequence === 6 && sequence < pageCount);
    return {
      slideId: `slide_slice_b_${String(sequence).padStart(2, "0")}`,
      title: chinese ? `第 ${sequence} 页` : `Slide ${sequence}`,
      purpose: chinese ? "支持管理层决策" : "Support a management decision",
      keyMessage: chinese ? `第 ${sequence} 页核心信息` : `Key message ${sequence}`,
      evidenceRequired: factual,
      evidenceRefs: factual ? ["claim_market"] : [],
      evidence: factual ? [{ claimId: "claim_market", rationale: "Verified market evidence directly supports this conclusion." }] : [],
      visualIntent: withImage ? "Professional evidence with a strong market image" : "Professional evidence summary",
      expectedElementTypes: withImage ? ["TEXT", "SHAPE", "IMAGE"] : ["TEXT", "SHAPE"],
    };
  });
  const contents = slides.map((objective, index) => ({
    slideId: objective.slideId,
    title: objective.title,
    subtitle: index === 0 ? (chinese ? "面向管理层" : "For management") : undefined,
    bodyBlocks: index > 0 && index < pageCount - 1
      ? [{ heading: chinese ? "判断" : "Finding", text: objective.keyMessage }]
      : [],
    bullets: index > 0 && index < pageCount - 1
      ? [chinese ? "基于已验证证据" : "Grounded in verified evidence", chinese ? "聚焦管理行动" : "Focused on management action"]
      : [],
    supportingText: objective.keyMessage,
    speakerNotes: `${objective.keyMessage}\nSource: https://example.test/market#claim`,
    assetIntents: objective.expectedElementTypes.includes("IMAGE")
		? [{ id: `provider_image_${index + 1}`, stableId: `asset_intent_${index + 1}`, prompt: "A professional electric vehicle market photograph", altText: "Electric vehicle market" }]
      : [],
    citationRefs: [...objective.evidenceRefs],
    layoutHint: index === 0 ? "cover" : index === pageCount - 1 ? "closing-action" : objective.expectedElementTypes.includes("IMAGE") ? (index % 2 === 0 ? "text-image" : "image-text") : "title-bullets",
  }));
  const assets = contents.flatMap((content) => content.assetIntents.map((intent) => ({
		id: `asset_${intent.stableId}`,
		intentId: intent.stableId,
    slideId: content.slideId,
    type: "image",
    mimeType: "image/png",
		uri: `asset://ppt-v2/${intent.stableId}.png`,
    sha256: PNG_SHA256,
    altText: intent.altText,
  })));
  return {
    generationJobId: "pptv2_job_slice_b",
    revision: 1,
    intent: {
      topic: chinese ? "新能源汽车行业" : "Electric vehicle market",
      goal: "industry-analysis",
      audience: chinese ? "公司管理层" : "company management",
      scenario: "management-report",
      language,
      professionalStyle: "professional-business",
    },
    research: {
      sources: [{ id: "source_market", title: "EV Market Report", type: "industry_report", locator: "https://example.test/market" }],
      citations: [{ id: "citation_market", sourceId: "source_market", locator: "https://example.test/market#claim" }],
      claims: [{ id: "claim_market", sourceId: "source_market", citationRefs: ["citation_market"], text: "EV demand is growing.", verificationStatus: "SOURCE_SUPPORTED" }],
    },
    storyline: {
      thesis: chinese ? "市场增长要求管理层现在做出选择。" : "Market growth requires a management decision now.",
      audienceTakeaway: chinese ? "聚焦高置信度机会。" : "Focus on high-confidence opportunities.",
      narrativeArc: ["context", "evidence", "action"],
      sections: [],
      closingAction: chinese ? "明确负责人和下一步。" : "Assign an owner and next step.",
    },
    approvedOutline: { id: "outline_slice_b", revision: 2, topic: "EV", language, pageCount, slides },
    slideContents: contents,
    assets,
  };
}

for (const pageCount of [6, 8, 10, 12]) {
  test(`approved ${pageCount}-page outline produces exactly ${pageCount} stable SlideIR pages`, () => {
    const first = buildProfessionalDeck(sliceBInput(pageCount));
    const second = buildProfessionalDeck(structuredClone(sliceBInput(pageCount)));

    assert.equal(first.deck.slides.length, pageCount);
    assert.deepEqual(first.deck.slides.map((slide) => slide.id), sliceBInput(pageCount).approvedOutline.slides.map((slide) => slide.slideId));
    assert.deepEqual(second, first);
    assert.equal(assertValidDeckRevision(first.deck), first.deck);
    assert.equal(assertValidLayoutResult(first.layoutResult), first.layoutResult);
    assert.equal(assertValidRenderInput(first.renderInput), first.renderInput);
    for (const slide of first.deck.slides) {
      assert.equal(Object.hasOwn(slide, "box"), false);
      assert.ok(slide.elements.every((element) => !Object.hasOwn(element, "x")));
    }
  });
}

test("factual slides preserve Claim to Citation to Source provenance and image elements use stable assets", () => {
  const result = buildProfessionalDeck(sliceBInput());
  const factual = result.deck.slides[1];
  const image = result.deck.slides[3].elements.find((element) => element.type === "image");

  assert.deepEqual(factual.citationRefs, ["claim_market"]);
  assert.deepEqual(result.deck.provenance.claims[0].citationRefs, ["citation_market"]);
  assert.equal(result.deck.provenance.citations[0].sourceId, "source_market");
  const crossSource = structuredClone(result.deck);
  crossSource.provenance.sources.push({ id: "source_other", title: "Other", type: "report", locator: "https://example.test/other" });
  crossSource.provenance.citations[0].sourceId = "source_other";
  assert.throws(() => assertValidDeckRevision(crossSource), /belongs to another source/);
  assert.equal(result.deck.provenance.sources[0].id, "source_market");
	assert.match(factual.speakerNotes, /EV Market Report — https:\/\/example\.test\/market#claim/);
  assert.equal(image.assetRef, "asset_asset_intent_4");
  assert.equal(result.deck.assetManifest.find((asset) => asset.id === image.assetRef).uri, "asset://ppt-v2/asset_intent_4.png");
});

test("all Professional layout definitions compile legal non-overlapping deterministic geometry", () => {
  const expected = ["cover", "section", "title-body", "title-bullets", "two-column", "text-image", "image-text", "key-metric", "closing-action"];
  assert.deepEqual(Object.keys(professionalLayoutDefinitions).sort(), expected.map((name) => `layout_professional_${name.replaceAll("-", "_")}_v1`).sort());
  for (const definition of Object.values(professionalLayoutDefinitions)) {
    const slots = Object.values(definition.slots);
    for (const slot of slots) {
      assert.ok(slot.x >= definition.safeArea.left && slot.y >= definition.safeArea.top);
      assert.ok(slot.x + slot.width <= 960 - definition.safeArea.right);
      assert.ok(slot.y + slot.height <= 540 - definition.safeArea.bottom);
    }
    for (let left = 0; left < slots.length; left += 1) {
      for (let right = left + 1; right < slots.length; right += 1) {
        const a = slots[left];
        const b = slots[right];
        const overlaps = a.x < b.x + b.width && a.x + a.width > b.x && a.y < b.y + b.height && a.y + a.height > b.y;
        assert.equal(overlaps, false, `${a.slot ?? left} overlaps ${b.slot ?? right}`);
      }
    }
  }
});

test("quality gate blocks missing citation, missing asset, invalid order, mismatch, overlap, bounds, and overflow", () => {
  const result = buildProfessionalDeck(sliceBInput());
  const mutations = [
    ["MISSING_CITATION", (copy) => { copy.deck.slides[1].citationRefs = []; }],
    ["BROKEN_ASSET_REFERENCE", (copy) => { copy.deck.slides[3].elements.find((item) => item.type === "image").assetRef = "asset_missing"; }],
    ["INVALID_PAGE_ORDER", (copy) => { copy.deck.slides[2].sequence = 8; }],
    ["PAGE_COUNT_MISMATCH", (copy) => { copy.deck.slides.pop(); copy.layoutResult.slides.pop(); }],
    ["ELEMENT_OVERLAP", (copy) => { copy.layoutResult.slides[0].elements[1].x = copy.layoutResult.slides[0].elements[0].x; copy.layoutResult.slides[0].elements[1].y = copy.layoutResult.slides[0].elements[0].y; }],
    ["ILLEGAL_BOUNDS", (copy) => { copy.layoutResult.slides[0].elements[0].x = -1; }],
    ["TEXT_OVERFLOW", (copy) => { copy.deck.slides[1].elements.find((item) => item.type === "text" && item.slot === "bullets").content.items = ["x".repeat(600)]; }],
  ];
  for (const [code, mutate] of mutations) {
    const copy = structuredClone(result);
    mutate(copy);
    const gate = runProfessionalQualityGate(copy.deck, copy.layoutResult);
    assert.equal(gate.valid, false, code);
    assert.ok(gate.diagnostics.some((item) => item.code === code), `${code}: ${JSON.stringify(gate.diagnostics)}`);
  }
});

test("approved IMAGE objectives cannot be silently rendered with a text-only layout", () => {
  const input = sliceBInput();
  const imageSlide = input.approvedOutline.slides.find((slide) => slide.expectedElementTypes.includes("IMAGE"));
  input.slideContents.find((content) => content.slideId === imageSlide.slideId).layoutHint = "title-body";
  assert.throws(() => buildProfessionalDeck(input), /image layout does not match approved objective/);
});

test("key metric content uses only the slots declared by its Professional layout", () => {
  const input = sliceBInput();
  input.slideContents[4].layoutHint = "key-metric";
  input.slideContents[4].bodyBlocks = [{ heading: "42%", text: "Verified market signal" }];
  const result = buildProfessionalDeck(input);
  const slide = result.deck.slides[4];
  assert.deepEqual(slide.elements.map((element) => element.slot), ["title", "metric", "metric-label", "body"]);
  assert.equal(result.layoutResult.slides[4].diagnostics.length, 0);
});

test("multi-page renderer emits native text and shapes plus resolved images", async () => {
  const result = buildProfessionalDeck(sliceBInput());
  const renderer = new PptxGenJSRenderer({
    resolveAsset(asset) {
      assert.match(asset.uri, /^asset:\/\//);
      return { data: `data:${asset.mimeType};base64,${PNG_1X1}` };
    },
  });
  const buffer = await renderer.render(result.renderInput);
  const zip = await JSZip.loadAsync(buffer);
  const paths = Object.keys(zip.files);

  assert.equal(paths.filter((path) => /^ppt\/slides\/slide\d+\.xml$/.test(path)).length, 8);
  assert.ok(paths.some((path) => /^ppt\/media\/image[^/]*\.png$/.test(path)), paths.filter((path) => path.startsWith("ppt/media/")).join(", "));
  const slide2 = await zip.file("ppt/slides/slide2.xml").async("string");
  const notes2 = await zip.file("ppt/notesSlides/notesSlide2.xml").async("string");
  assert.match(slide2, /Key message 2/);
  assert.match(notes2, /https:\/\/example\.test\/market#claim/);
  assert.deepEqual(await renderer.render(structuredClone(result.renderInput)), buffer);
});

test("renderer fails closed when private asset bytes are not resolved", async () => {
  const result = buildProfessionalDeck(sliceBInput());
  await assert.rejects(() => new PptxGenJSRenderer().render(result.renderInput), /asset.*not resolved/i);
});

test("OfficeCLI accepts Golden 2 multi-page research deck without repair", async () => {
  const result = buildProfessionalDeck(sliceBInput());
  const renderer = new PptxGenJSRenderer({ resolveAsset: () => ({ data: `data:image/png;base64,${PNG_1X1}` }) });
  const buffer = await renderer.render(result.renderInput);
  const directory = mkdtempSync(join(tmpdir(), "ppt-v2-golden-2-"));
  const output = join(directory, "golden-2-professional-research.pptx");
  writeFileSync(output, buffer);
  try {
    const validation = spawnSync("officecli", ["validate", output], { encoding: "utf8" });
    const validationOutput = `${validation.stdout ?? ""}\n${validation.stderr ?? ""}`;
    assert.equal(validation.status, 0, validationOutput);
    assert.match(validationOutput, /Validation passed: no errors found/i);
    assert.doesNotMatch(validationOutput, /repair warning|repair required|validation error/i);
  } finally {
    execFileSync("officecli", ["close", output], { encoding: "utf8" });
    unlinkSync(output);
    rmdirSync(directory);
  }
});

test("Chinese approved outline remains Chinese through SlideIR and PPTX metadata", () => {
  const result = buildProfessionalDeck(sliceBInput(10, "zh-CN"));
  assert.equal(result.renderInput.deckRevision.language, "zh-CN");
  assert.match(result.deck.slides[1].elements.find((item) => item.type === "text" && item.slot === "title").content.text, /第 2 页/);
});
