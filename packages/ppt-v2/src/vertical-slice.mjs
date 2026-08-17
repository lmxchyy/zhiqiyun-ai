import {
  assertValidDeckRevision,
  assertValidLayoutResult,
  assertValidRenderInput,
} from "./contract.mjs";
import { professionalBusinessDesignSystem } from "./design-system.mjs";
import { compileDeckLayout } from "./layout-compiler.mjs";
import {
  adaptLegacyGenerateRequestToDeckSpec,
  adaptLegacyOutlineToStoryline,
  adaptLegacyTaskContextToGenerationContext,
  adaptStorylineToOutlinePlan,
  migrationStem,
} from "./migration-adapter.mjs";

const CONTRACT_VERSION = "2.1";

function clean(value, fallback = "") {
  const text = String(value ?? "").trim();
  return text || fallback;
}

function traceMigration(input) {
  const consumedFields = [];
  const ignoredFields = [];
  const consumedRequestKeys = new Set(["prompt", "language", "audience", "scenario"]);
  for (const key of Object.keys(input.generateRequest ?? {}).sort()) {
    const field = `generateRequest.${key}`;
    if (consumedRequestKeys.has(key)) {
      consumedFields.push(field);
    } else {
      ignoredFields.push({
        field,
        reason: "Phase 1 uses a fixed two-slide, no-provider vertical slice and does not consume this legacy option.",
      });
    }
  }

  for (const key of Object.keys(input.outline ?? {}).sort()) {
    if (key === "title") {
      consumedFields.push("outline.title");
    } else if (key === "updatedAt") {
      ignoredFields.push({
        field: "outline.updatedAt",
        reason: "Legacy outline timestamps are not a canonical DeckRevision identity.",
      });
    } else if (key !== "slides") {
      ignoredFields.push({
        field: `outline.${key}`,
        reason: "Phase 1 only consumes the legacy outline title and fixed first two slide sources.",
      });
    }
  }
  const outlineSlides = Array.isArray(input.outline?.slides) ? input.outline.slides : [];
  for (let index = 0; index < Math.min(outlineSlides.length, 2); index += 1) {
    const slide = outlineSlides[index];
    for (const key of ["title", "summary", "bulletPoints"]) {
      if (Object.hasOwn(slide, key)) {
        if (index === 0 && key === "bulletPoints") {
          ignoredFields.push({
            field: `outline.slides[${index}].${key}`,
            reason: "The fixed Cover layout has no bullet slot.",
          });
        } else {
          consumedFields.push(`outline.slides[${index}].${key}`);
        }
      }
    }
    for (const key of Object.keys(slide).sort()) {
      if (["title", "summary", "bulletPoints"].includes(key)) {
        continue;
      }
      ignoredFields.push({
        field: `outline.slides[${index}].${key}`,
        reason: "Phase 1 selects its canonical role and layout from the fixed page sequence.",
      });
    }
  }
  for (let index = 2; index < outlineSlides.length; index += 1) {
    ignoredFields.push({
      field: `outline.slides[${index}]`,
      reason: "Phase 1 fixed two-slide content plan excludes dynamic page planning.",
    });
  }

  const consumedTaskKeys = new Set(["taskId", "userId", "clientRequestId", "status", "title", "speakerNotesByPage"]);
  for (const key of Object.keys(input.taskContext ?? {}).sort()) {
    const field = `taskContext.${key}`;
    if (consumedTaskKeys.has(key)) {
      consumedFields.push(field);
    } else {
      ignoredFields.push({
        field,
        reason: "Phase 1 task context only carries owner, source identity, status, title fallback, and speaker notes.",
      });
    }
  }
  for (const page of Object.keys(input.taskContext?.speakerNotesByPage ?? {}).sort((left, right) => Number(left) - Number(right))) {
    const field = `taskContext.speakerNotesByPage[${page}]`;
    if (page === "1" || page === "2") {
      consumedFields.push(field);
    } else {
      ignoredFields.push({
        field,
        reason: "Phase 1 fixed two-slide content plan excludes notes for later pages.",
      });
    }
  }

  return {
    adapter: "legacy-ppt-to-v2-phase1",
    consumedFields: [...new Set(consumedFields)].sort(),
    ignoredFields,
    unmappedFields: [],
  };
}

function buildSlides(outlinePlan, taskContext) {
  const stem = migrationStem(taskContext);
  const [cover, content] = outlinePlan.slideObjectives;
  const notesByPage = taskContext.speakerNotesByPage ?? {};
  return [
    {
      id: `slide_${stem}_cover`,
      sequence: 1,
      role: "cover",
      layoutId: cover.layoutId,
      backgroundToken: "primary",
      speakerNotes: clean(notesByPage[1], `Introduce ${cover.title} and establish the presentation context.`),
      elements: [
        {
          id: `element_${stem}_cover_accent`, type: "shape", slot: "accent",
          shapeType: "rect", styleRole: "coverAccent",
        },
        {
          id: `element_${stem}_cover_title`, type: "text", slot: "title",
          content: { kind: "plain", text: cover.title }, styleRole: "coverTitle",
        },
        {
          id: `element_${stem}_cover_subtitle`, type: "text", slot: "subtitle",
          content: { kind: "plain", text: cover.message }, styleRole: "coverSubtitle",
        },
        {
          id: `element_${stem}_cover_footer`, type: "text", slot: "footer",
          content: { kind: "plain", text: "Xianzhi AI · Phase 1" }, styleRole: "coverFooter",
        },
      ],
    },
    {
      id: `slide_${stem}_content`,
      sequence: 2,
      role: "content",
      layoutId: content.layoutId,
      backgroundToken: "background",
      speakerNotes: clean(notesByPage[2], `Explain ${content.title} and connect it to the deck objective.`),
      elements: [
        {
          id: `element_${stem}_content_panel`, type: "shape", slot: "panel",
          shapeType: "roundRect", styleRole: "contentPanel",
        },
        {
          id: `element_${stem}_content_title`, type: "text", slot: "title",
          content: { kind: "plain", text: content.title }, styleRole: "contentTitle",
        },
        {
          id: `element_${stem}_content_body`, type: "text", slot: "body",
          content: content.bulletPoints.length > 0
            ? { kind: "bullets", items: [...content.bulletPoints] }
            : { kind: "plain", text: content.message },
          styleRole: "contentBody",
        },
      ],
    },
  ];
}

export function buildPhase1VerticalSlice(input) {
  const generateRequest = input?.generateRequest ?? {};
  const taskContext = input?.taskContext ?? {};
  const outline = input?.outline ?? { title: clean(generateRequest.prompt, clean(taskContext.title, "Presentation")), slides: [] };
  const deckSpec = adaptLegacyGenerateRequestToDeckSpec(generateRequest, taskContext);
  const storyline = adaptLegacyOutlineToStoryline(outline, taskContext);
  const outlinePlan = adaptStorylineToOutlinePlan(storyline);
  const designSystem = professionalBusinessDesignSystem();
  const stem = migrationStem(taskContext);
  const deck = {
    contractVersion: CONTRACT_VERSION,
    deckId: `deck_${stem}`,
    revision: 1,
    deckSpec,
    storyline,
    outlinePlan,
    designSystem,
    assetManifest: [],
    migrationTrace: traceMigration({ generateRequest, outline, taskContext }),
    slides: buildSlides(outlinePlan, taskContext),
  };
  assertValidDeckRevision(deck);

  const layoutResult = compileDeckLayout(deck);
  assertValidLayoutResult(layoutResult);
  const renderInput = {
    contractVersion: CONTRACT_VERSION,
    deckRevision: {
      deckId: deck.deckId,
      revision: deck.revision,
      title: deck.deckSpec.title,
      language: deck.deckSpec.language,
      author: deck.deckSpec.author,
    },
    slides: deck.slides,
    layoutResults: layoutResult.slides,
    designSystem: deck.designSystem,
    assetManifest: deck.assetManifest,
    options: { layout: "wide", deterministic: true },
  };
  assertValidRenderInput(renderInput);

  return {
    contentPlan: {
      kind: "fixed-two-slide",
      pages: [
        { sequence: 1, role: "cover" },
        { sequence: 2, role: "content" },
      ],
    },
    generationContext: adaptLegacyTaskContextToGenerationContext(taskContext),
    deck,
    layoutResult,
    renderInput,
  };
}
