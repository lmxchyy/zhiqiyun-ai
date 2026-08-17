import { createHash } from "node:crypto";

export const COVER_LAYOUT_ID = "layout_cover_v1";
export const STANDARD_CONTENT_LAYOUT_ID = "layout_standard_content_v1";

function clean(value, fallback = "") {
  const text = String(value ?? "").trim();
  return text || fallback;
}

function stableStem(taskContext = {}) {
  const raw = clean(taskContext.taskId, clean(taskContext.clientRequestId));
  const sanitized = raw.replace(/[^A-Za-z0-9._-]+/g, "_").replace(/^[^A-Za-z]+/, "");
  if (sanitized) {
    return sanitized.slice(0, 56);
  }
  const digest = createHash("sha256").update(JSON.stringify(taskContext)).digest("hex").slice(0, 16);
  return `legacy_${digest}`;
}

function normalizedBullets(value) {
  return Array.isArray(value)
    ? value.map((item) => clean(item)).filter(Boolean).slice(0, 12)
    : [];
}

function fallbackOutlineSlide(sequence, title, message) {
  return {
    page: sequence,
    title,
    summary: message,
    bulletPoints: sequence === 1 ? [] : [message],
    layout: sequence === 1 ? "cover" : "content",
    slideType: sequence === 1 ? "cover" : "text_image",
  };
}

/** Migration-only, one-way adapter. Never write this result back to legacy JSON. */
export function adaptLegacyGenerateRequestToDeckSpec(generateRequest = {}, taskContext = {}) {
  return {
    title: clean(generateRequest.prompt, clean(taskContext.title, "Presentation")),
    language: clean(generateRequest.language, "zh-CN"),
    author: "Xianzhi AI",
    audience: clean(generateRequest.audience, "General audience"),
    scenario: clean(generateRequest.scenario, "Presentation"),
    source: {
      kind: "legacy-ppt-task",
      taskId: clean(taskContext.taskId, `ppt_${stableStem(taskContext)}`),
      clientRequestId: clean(taskContext.clientRequestId),
    },
  };
}

/** Migration-only, one-way adapter from a legacy outline slide. */
export function adaptOutlineSlideToObjective(outlineSlide = {}, sequence, taskContext = {}) {
  const stem = stableStem(taskContext);
  const role = sequence === 1 ? "cover" : "content";
  const title = clean(outlineSlide.title, sequence === 1 ? clean(taskContext.title, "Presentation") : "Key message");
  const message = clean(outlineSlide.summary, sequence === 1 ? "Presentation context" : title);
  return {
    id: `objective_${stem}_${String(sequence).padStart(2, "0")}`,
    sequence,
    role,
    layoutId: role === "cover" ? COVER_LAYOUT_ID : STANDARD_CONTENT_LAYOUT_ID,
    title,
    message,
    bulletPoints: role === "cover" ? [] : normalizedBullets(outlineSlide.bulletPoints),
  };
}

/** Migration-only adapter. Phase 1 deliberately emits exactly two beats. */
export function adaptLegacyOutlineToStoryline(outline = {}, taskContext = {}) {
  const title = clean(outline.title, clean(taskContext.title, "Presentation"));
  const sourceSlides = Array.isArray(outline.slides) ? outline.slides : [];
  const coverSource = sourceSlides[0] ?? fallbackOutlineSlide(1, title, "Presentation context");
  const contentSource = sourceSlides[1] ?? fallbackOutlineSlide(2, "Key message", clean(coverSource.summary, title));
  const objectives = [
    adaptOutlineSlideToObjective(coverSource, 1, taskContext),
    adaptOutlineSlideToObjective(contentSource, 2, taskContext),
  ];
  return {
    title,
    beats: objectives.map((objective) => ({
      id: `beat_${stableStem(taskContext)}_${String(objective.sequence).padStart(2, "0")}`,
      sequence: objective.sequence,
      role: objective.role,
      purpose: objective.message,
      objectiveId: objective.id,
      layoutId: objective.layoutId,
      title: objective.title,
      message: objective.message,
      bulletPoints: [...objective.bulletPoints],
    })),
  };
}

export function adaptStorylineToOutlinePlan(storyline) {
  return {
    slideObjectives: storyline.beats.map((beat) => ({
      id: beat.objectiveId,
      sequence: beat.sequence,
      role: beat.role,
      layoutId: beat.layoutId,
      title: beat.title,
      message: beat.message,
      bulletPoints: [...beat.bulletPoints],
    })),
  };
}

/** Migration-only adapter for owner/task identity; no Connector or billing fields. */
export function adaptLegacyTaskContextToGenerationContext(taskContext = {}) {
  return {
    legacyTaskId: clean(taskContext.taskId),
    ownerUserId: clean(taskContext.userId),
    clientRequestId: clean(taskContext.clientRequestId),
    sourceStatus: clean(taskContext.status),
  };
}

export function migrationStem(taskContext) {
  return stableStem(taskContext);
}
