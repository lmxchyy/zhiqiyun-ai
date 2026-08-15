import assert from "node:assert/strict";
import test from "node:test";

import {
  assertValidDeckRevision,
  assertValidLayoutResult,
  assertValidRenderInput,
} from "../packages/ppt-v2/src/contract.mjs";
import {
  COVER_LAYOUT_ID,
  STANDARD_CONTENT_LAYOUT_ID,
  compileDeckLayout,
  phase1LayoutDefinitions,
} from "../packages/ppt-v2/src/layout-compiler.mjs";
import {
  adaptLegacyGenerateRequestToDeckSpec,
  adaptLegacyOutlineToStoryline,
  adaptLegacyTaskContextToGenerationContext,
  adaptOutlineSlideToObjective,
  adaptStorylineToOutlinePlan,
} from "../packages/ppt-v2/src/migration-adapter.mjs";
import { buildPhase1VerticalSlice } from "../packages/ppt-v2/src/vertical-slice.mjs";

function legacyInput() {
  return {
    generateRequest: {
      prompt: "2027 年企业增长计划",
      slideCount: 12,
      language: "zh-CN",
      tone: "professional",
      textContent: "聚焦产品、渠道与客户成功三个增长支柱。",
      audience: "管理层",
      scenario: "年度经营会",
      generationAspectRatio: "16:9",
      theme: "technology",
      autoThemeEnabled: true,
      enableWebSearch: true,
      imageSource: "ai",
      textModel: "legacy-text-model",
      imageModel: "legacy-image-model",
      textInImage: false,
    },
    outline: {
      title: "2027 年企业增长计划",
      updatedAt: "2026-08-15T01:02:03Z",
      slides: [
        {
          page: 1,
          title: "2027 年企业增长计划",
          summary: "从共识到执行",
          bulletPoints: [],
          layout: "cover",
          slideType: "cover",
        },
        {
          page: 2,
          title: "三个增长支柱形成闭环",
          summary: "产品建立价值，渠道放大触达，客户成功驱动续费。",
          bulletPoints: ["产品：聚焦高价值场景", "渠道：建立可复制打法", "客户成功：提升续费与扩容"],
          layout: "content",
          slideType: "text_image",
        },
        {
          page: 3,
          title: "本页不进入 Phase 1",
          summary: "动态页数属于后续阶段。",
          bulletPoints: ["明确忽略，而不是静默丢失"],
          layout: "content",
          slideType: "text_image",
        },
      ],
    },
    taskContext: {
      taskId: "ppt_legacy_001",
      userId: "user_001",
      clientRequestId: "request_001",
      status: "success",
      title: "2027 年企业增长计划",
      speakerNotesByPage: {
        2: "说明三个支柱如何在同一经营节奏中协同。",
      },
    },
  };
}

test("migration adapters expose DeckSpec, Storyline, OutlinePlan, objectives, and generation context", () => {
  const input = legacyInput();
  const deckSpec = adaptLegacyGenerateRequestToDeckSpec(input.generateRequest, input.taskContext);
  const storyline = adaptLegacyOutlineToStoryline(input.outline, input.taskContext);
  const outlinePlan = adaptStorylineToOutlinePlan(storyline);
  const objective = adaptOutlineSlideToObjective(input.outline.slides[1], 2, input.taskContext);
  const generationContext = adaptLegacyTaskContextToGenerationContext(input.taskContext);

  assert.deepEqual(deckSpec, {
    title: "2027 年企业增长计划",
    language: "zh-CN",
    author: "Xianzhi AI",
    audience: "管理层",
    scenario: "年度经营会",
    source: {
      kind: "legacy-ppt-task",
      taskId: "ppt_legacy_001",
      clientRequestId: "request_001",
    },
  });
  assert.equal(storyline.beats.length, 2);
  assert.deepEqual(storyline.beats.map((beat) => beat.role), ["cover", "content"]);
  assert.deepEqual(outlinePlan.slideObjectives.map((item) => item.layoutId), [
    COVER_LAYOUT_ID,
    STANDARD_CONTENT_LAYOUT_ID,
  ]);
  assert.deepEqual(objective, {
    id: "objective_ppt_legacy_001_02",
    sequence: 2,
    role: "content",
    layoutId: STANDARD_CONTENT_LAYOUT_ID,
    title: "三个增长支柱形成闭环",
    message: "产品建立价值，渠道放大触达，客户成功驱动续费。",
    bulletPoints: ["产品：聚焦高价值场景", "渠道：建立可复制打法", "客户成功：提升续费与扩容"],
  });
  assert.deepEqual(generationContext, {
    legacyTaskId: "ppt_legacy_001",
    ownerUserId: "user_001",
    clientRequestId: "request_001",
    sourceStatus: "success",
  });
});

test("vertical slice is deterministic, uses stable IDs, and records every ignored legacy field", () => {
  const source = legacyInput();
  source.outline.legacyOutlineMode = "freeform";
  source.taskContext.speakerNotesByPage[3] = "Phase 1 must explicitly ignore this note.";
  const first = buildPhase1VerticalSlice(source);
  const second = buildPhase1VerticalSlice(structuredClone(source));

  assert.deepEqual(second, first);
  assert.equal(first.deck.deckId, "deck_ppt_legacy_001");
  assert.equal(first.deck.revision, 1);
  assert.deepEqual(first.deck.slides.map((slide) => slide.id), [
    "slide_ppt_legacy_001_cover",
    "slide_ppt_legacy_001_content",
  ]);
  assert.deepEqual(first.deck.slides.map((slide) => slide.role), ["cover", "content"]);
  assert.deepEqual(first.deck.migrationTrace.unmappedFields, []);
  assert.ok(first.deck.migrationTrace.consumedFields.includes("generateRequest.prompt"));
  assert.ok(first.deck.migrationTrace.consumedFields.includes("outline.slides[1].bulletPoints"));
  assert.ok(first.deck.migrationTrace.ignoredFields.some((item) =>
    item.field === "generateRequest.enableWebSearch" && /Phase 1/.test(item.reason)));
  assert.ok(first.deck.migrationTrace.ignoredFields.some((item) =>
    item.field === "outline.slides[2]" && /fixed two-slide/i.test(item.reason)));
  assert.ok(first.deck.migrationTrace.ignoredFields.some((item) =>
    item.field === "outline.legacyOutlineMode" && /Phase 1/.test(item.reason)));
  assert.ok(first.deck.migrationTrace.consumedFields.includes("taskContext.speakerNotesByPage[2]"));
  assert.ok(first.deck.migrationTrace.ignoredFields.some((item) =>
    item.field === "taskContext.speakerNotesByPage[3]" && /fixed two-slide/i.test(item.reason)));

  for (const slide of first.deck.slides) {
    assert.ok(slide.speakerNotes.length > 0);
    for (const element of slide.elements) {
      assert.equal(Object.hasOwn(element, "box"), false, `${element.id} leaked geometry into SlideIR`);
      assert.equal(Object.hasOwn(element, "resolvedStyle"), false, `${element.id} leaked resolved style into SlideIR`);
    }
  }
});

test("DeckRevision, SlideIR, LayoutResult, and RenderInput satisfy the 2.1 contract", () => {
  const slice = buildPhase1VerticalSlice(legacyInput());

  assert.equal(assertValidDeckRevision(slice.deck), slice.deck);
  assert.equal(assertValidLayoutResult(slice.layoutResult), slice.layoutResult);
  assert.equal(assertValidRenderInput(slice.renderInput), slice.renderInput);
});

test("cover and content layouts produce exact deterministic geometry with no illegal bounds", () => {
  const { deck, layoutResult } = buildPhase1VerticalSlice(legacyInput());

  assert.deepEqual(layoutResult.canvas, { unit: "pt", width: 960, height: 540 });
  assert.deepEqual(
    layoutResult.slides[0].elements.map(({ elementId, x, y, width, height, zIndex }) => ({
      elementId, x, y, width, height, zIndex,
    })),
    [
      { elementId: "element_ppt_legacy_001_cover_accent", x: 72, y: 90, width: 96, height: 14, zIndex: 0 },
      { elementId: "element_ppt_legacy_001_cover_title", x: 72, y: 150, width: 816, height: 96, zIndex: 1 },
      { elementId: "element_ppt_legacy_001_cover_subtitle", x: 72, y: 260, width: 744, height: 72, zIndex: 2 },
      { elementId: "element_ppt_legacy_001_cover_footer", x: 72, y: 450, width: 816, height: 24, zIndex: 3 },
    ],
  );
  assert.deepEqual(
    layoutResult.slides[1].elements.map(({ elementId, x, y, width, height, zIndex }) => ({
      elementId, x, y, width, height, zIndex,
    })),
    [
      { elementId: "element_ppt_legacy_001_content_panel", x: 72, y: 154, width: 816, height: 280, zIndex: 0 },
      { elementId: "element_ppt_legacy_001_content_title", x: 72, y: 54, width: 816, height: 64, zIndex: 1 },
      { elementId: "element_ppt_legacy_001_content_body", x: 108, y: 190, width: 744, height: 204, zIndex: 2 },
    ],
  );
  assert.deepEqual(layoutResult.diagnostics, []);

  const slideElementIds = new Set(deck.slides.flatMap((slide) => slide.elements.map((item) => item.id)));
  const layoutElementIds = new Set(layoutResult.slides.flatMap((slide) => slide.elements.map((item) => item.elementId)));
  assert.deepEqual(layoutElementIds, slideElementIds);
});

test("layout compiler reports every required minimal diagnostic and never silently renders errors", () => {
  const { deck } = buildPhase1VerticalSlice(legacyInput());
  const broken = structuredClone(deck);
  broken.slides[0].elements[1].slot = "missing-slot";
  broken.slides[1].elements[2].content.items = ["x".repeat(501)];

  const definitions = structuredClone(phase1LayoutDefinitions);
  definitions[COVER_LAYOUT_ID].slots.footer.width = -1;
  definitions[STANDARD_CONTENT_LAYOUT_ID].slots.panel.x = 20;
  definitions[STANDARD_CONTENT_LAYOUT_ID].slots.title.zIndex = -2;

  const result = compileDeckLayout(broken, { definitions });
  const diagnostics = result.diagnostics.map((item) => `${item.code}:${item.elementId ?? ""}`);

  assert.ok(diagnostics.includes("ELEMENT_MISSING_SLOT:element_ppt_legacy_001_cover_title"));
  assert.ok(diagnostics.includes("NEGATIVE_SIZE:element_ppt_legacy_001_cover_footer"));
  assert.ok(diagnostics.includes("BOUNDS_OUTSIDE_SAFE_AREA:element_ppt_legacy_001_content_panel"));
  assert.ok(diagnostics.includes("INVALID_Z_INDEX:element_ppt_legacy_001_content_title"));
  assert.ok(diagnostics.includes("TEXT_EXCEEDS_THRESHOLD:element_ppt_legacy_001_content_body"));
  assert.throws(() => assertValidLayoutResult(result), /layout result rejected/i);
});

test("RenderInput fails when a SlideIR element has no LayoutResult geometry", () => {
  const { renderInput } = buildPhase1VerticalSlice(legacyInput());
  const broken = structuredClone(renderInput);
  broken.layoutResults[0].elements.splice(1, 1);

  assert.throws(() => assertValidRenderInput(broken), /missing layout element/i);
});
