import { createHash } from "node:crypto";

import {
  assertValidDeckRevision,
  assertValidLayoutResult,
  assertValidRenderInput,
} from "./contract.mjs";
import { professionalBusinessDesignSystem } from "./design-system.mjs";
import { compileDeckLayout, professionalLayoutDefinitions } from "./layout-compiler.mjs";

const CONTRACT_VERSION = "2.1";
const MIN_PAGES = 6;
const MAX_PAGES = 12;

function items(value) {
	return Array.isArray(value) ? value : [];
}

function requiredText(value, label) {
  const text = String(value ?? "").trim();
  if (!text) {
    throw new Error(`PPT V2 Slice B requires ${label}`);
  }
  return text;
}

function stableElementId(slideId, slot) {
  const candidate = `element_${slideId}_${slot}`;
  if (candidate.length <= 96) {
    return candidate;
  }
  const digest = createHash("sha256").update(candidate).digest("hex").slice(0, 16);
  return `element_${slideId.slice(0, 64)}_${digest}`;
}

function stableID(prefix, value) {
  const candidate = `${prefix}_${value}`;
  if (candidate.length <= 96) return candidate;
  const digest = createHash("sha256").update(candidate).digest("hex").slice(0, 16);
  return `${prefix}_${String(value).slice(0, 64)}_${digest}`;
}

function textElement(slideId, slot, content, styleRole) {
  return {
    id: stableElementId(slideId, slot),
    type: "text",
    slot,
    content: Array.isArray(content)
      ? { kind: "bullets", items: content.map((item) => requiredText(item, `${slot} bullet`)) }
      : { kind: "plain", text: requiredText(content, slot) },
    styleRole,
  };
}

function plainBody(content) {
  const blocks = Array.isArray(content.bodyBlocks) ? content.bodyBlocks : [];
  if (blocks.length > 0) {
    return blocks.map((block) => {
      const heading = String(block.heading ?? "").trim();
      const text = requiredText(block.text, "body block text");
      return heading ? `${heading}\n${text}` : text;
    }).join("\n\n");
  }
  return requiredText(content.supportingText, "supportingText");
}

function layoutID(hint) {
  const normalized = String(hint ?? "").trim().toLowerCase();
  const supported = new Set([
    "cover", "section", "title-body", "title-bullets", "two-column",
    "text-image", "image-text", "key-metric", "closing-action",
  ]);
  if (!supported.has(normalized)) {
    throw new Error(`PPT V2 Slice B unsupported professional layout ${hint}`);
  }
  return `layout_professional_${normalized.replaceAll("-", "_")}_v1`;
}

function roleFor(layoutHint) {
  if (layoutHint === "cover") return "cover";
  if (layoutHint === "section") return "section";
  if (layoutHint === "closing-action") return "closing";
  return "content";
}

function assetForIntent(input, slideId, intent) {
	const intentId = requiredText(intent.stableId || intent.id, "asset intent stable identity");
	const asset = items(input.assets).find((item) => item.slideId === slideId && item.intentId === intentId);
	if (!asset) {
		throw new Error(`PPT V2 Slice B asset intent ${intentId} is unresolved`);
	}
	return asset;
}

function contentElements(input, objective, content) {
  const hint = String(content.layoutHint).trim().toLowerCase();
  const requiresImage = items(objective.expectedElementTypes).some((type) => String(type).trim().toUpperCase() === "IMAGE");
  const usesImageLayout = hint === "text-image" || hint === "image-text";
  if (requiresImage !== usesImageLayout) {
    throw new Error(`PPT V2 Slice B image layout does not match approved objective ${objective.slideId}`);
  }
  const elements = [];
  const title = requiredText(content.title, "slide title");
  const keyMessage = requiredText(objective.keyMessage, "slide keyMessage");
  if (hint === "cover") {
    elements.push(textElement(objective.slideId, "eyebrow", input.intent.scenario, "professionalEyebrow"));
    elements.push(textElement(objective.slideId, "title", title, "coverTitle"));
    elements.push(textElement(objective.slideId, "subtitle", content.subtitle || keyMessage, "coverSubtitle"));
    elements.push(textElement(objective.slideId, "footer", "Xianzhi AI", "coverFooter"));
    return elements;
  }
  if (hint === "section") {
    elements.push(textElement(objective.slideId, "eyebrow", input.intent.topic, "professionalEyebrow"));
    elements.push(textElement(objective.slideId, "title", title, "professionalTitle"));
    elements.push(textElement(objective.slideId, "body", plainBody(content), "professionalBody"));
    return elements;
  }
  if (hint === "closing-action") {
    elements.push(textElement(objective.slideId, "eyebrow", input.storyline.closingAction, "professionalEyebrow"));
    elements.push(textElement(objective.slideId, "title", title, "professionalTitle"));
    elements.push(textElement(objective.slideId, "key-message", keyMessage, "professionalKeyMessage"));
    elements.push(textElement(objective.slideId, "body", plainBody(content), "professionalBody"));
    return elements;
  }

  elements.push(textElement(objective.slideId, "title", title, "professionalTitle"));
  if (hint !== "key-metric") {
    elements.push(textElement(objective.slideId, "key-message", keyMessage, "professionalKeyMessage"));
  }
  if (hint === "title-bullets") {
    elements.push(textElement(objective.slideId, "bullets", content.bullets, "professionalBody"));
  } else if (hint === "two-column") {
    if (!Array.isArray(content.bodyBlocks) || content.bodyBlocks.length !== 2) {
      throw new Error("PPT V2 Slice B two-column layout requires exactly two body blocks");
    }
    elements.push(textElement(objective.slideId, "left", `${content.bodyBlocks[0].heading}\n${content.bodyBlocks[0].text}`, "professionalBody"));
    elements.push(textElement(objective.slideId, "right", `${content.bodyBlocks[1].heading}\n${content.bodyBlocks[1].text}`, "professionalBody"));
  } else if (hint === "key-metric") {
    const metric = requiredText(content.bodyBlocks?.[0]?.heading, "key metric");
    elements.push(textElement(objective.slideId, "metric", metric, "professionalMetric"));
    elements.push(textElement(objective.slideId, "metric-label", content.bodyBlocks?.[0]?.text, "professionalMetricLabel"));
    elements.push(textElement(objective.slideId, "body", content.supportingText, "professionalBody"));
  } else {
    elements.push(textElement(objective.slideId, "body", plainBody(content), "professionalBody"));
  }
  if (hint === "text-image" || hint === "image-text") {
    if (!Array.isArray(content.assetIntents) || content.assetIntents.length !== 1) {
      throw new Error(`PPT V2 Slice B ${hint} layout requires one asset intent`);
    }
    const intent = content.assetIntents[0];
    const asset = assetForIntent(input, objective.slideId, intent);
    elements.push({
      id: stableElementId(objective.slideId, "image"), type: "image", slot: "image",
      assetRef: asset.id, fit: "cover", altText: requiredText(intent.altText, "image altText"),
		citationRefs: [...items(objective.evidenceRefs)],
    });
  }
  return elements;
}

function buildSlides(input) {
	const objectives = items(input.approvedOutline.slides);
	const contentBySlide = new Map(items(input.slideContents).map((item) => [item.slideId, item]));
  if (contentBySlide.size !== objectives.length) {
    throw new Error("PPT V2 Slice B requires one content result per approved slide objective");
  }
  return objectives.map((objective, index) => {
    const content = contentBySlide.get(objective.slideId);
    if (!content) {
      throw new Error(`PPT V2 Slice B content missing for ${objective.slideId}`);
    }
    const sequence = index + 1;
    const hint = String(content.layoutHint).trim().toLowerCase();
		const contentCitationRefs = items(content.citationRefs);
		const objectiveEvidenceRefs = items(objective.evidenceRefs);
		if (contentCitationRefs.length !== objectiveEvidenceRefs.length || contentCitationRefs.some((ref) => !objectiveEvidenceRefs.includes(ref))) {
      throw new Error(`PPT V2 Slice B content citation refs do not match approved evidence for ${objective.slideId}`);
    }
    return {
      id: objective.slideId,
      sequence,
      role: roleFor(hint),
      layoutId: layoutID(hint),
      backgroundToken: hint === "cover" ? "primary" : "background",
		speakerNotes: speakerNotesWithSources(content.speakerNotes, contentCitationRefs, input.research),
      objectiveId: objective.slideId,
      keyMessage: requiredText(objective.keyMessage, "keyMessage"),
      evidenceRequired: Boolean(objective.evidenceRequired),
      citationRefs: [...contentCitationRefs],
      elements: contentElements(input, objective, content),
    };
  });
}

function legacyStoryline(input, slides) {
	const objectiveBySlide = new Map(items(input.approvedOutline.slides).map((item) => [item.slideId, item]));
	const contentBySlide = new Map(items(input.slideContents).map((item) => [item.slideId, item]));
  return {
    title: input.intent.topic,
    beats: slides.map((slide) => {
      const objective = objectiveBySlide.get(slide.id);
      const content = contentBySlide.get(slide.id);
      return {
        id: stableID("beat", slide.id),
        sequence: slide.sequence,
        role: slide.role,
        purpose: objective.purpose,
        objectiveId: objective.slideId,
        layoutId: slide.layoutId,
        title: content.title,
        message: objective.keyMessage,
				bulletPoints: [...items(content.bullets)],
      };
    }),
  };
}

function legacyOutline(input, slides) {
	const objectiveBySlide = new Map(items(input.approvedOutline.slides).map((item) => [item.slideId, item]));
	const contentBySlide = new Map(items(input.slideContents).map((item) => [item.slideId, item]));
  return {
    slideObjectives: slides.map((slide) => ({
      id: objectiveBySlide.get(slide.id).slideId,
      sequence: slide.sequence,
      role: slide.role,
      layoutId: slide.layoutId,
      title: contentBySlide.get(slide.id).title,
      message: objectiveBySlide.get(slide.id).keyMessage,
			bulletPoints: [...items(contentBySlide.get(slide.id).bullets)],
    })),
  };
}

function manifest(input) {
	return items(input.assets).map((asset) => ({
    id: asset.id,
    type: "image",
    mimeType: asset.mimeType,
    uri: asset.uri,
    sha256: asset.sha256,
  }));
}

function provenance(research) {
  return {
		sources: items(research.sources).map(({ id, title, type, locator }) => ({ id, title, type, locator })),
		citations: items(research.citations).map(({ id, sourceId, locator }) => ({ id, sourceId, locator })),
		claims: items(research.claims).map(({ id, sourceId, citationRefs, text, verificationStatus }) => ({ id, sourceId, citationRefs: [...items(citationRefs)], text, verificationStatus })),
  };
}

function speakerNotesWithSources(notes, claimIDs, research) {
	const base = requiredText(notes, "speakerNotes");
	if (claimIDs.length === 0) return base;
	const claims = new Map(items(research.claims).map((claim) => [claim.id, claim]));
	const citations = new Map(items(research.citations).map((citation) => [citation.id, citation]));
	const sources = new Map(items(research.sources).map((source) => [source.id, source]));
	const lines = claimIDs.map((claimID) => {
		const claim = claims.get(claimID);
		const citation = citations.get(items(claim?.citationRefs)[0]);
		const source = sources.get(claim?.sourceId);
		if (!claim || !citation || !source || citation.sourceId !== source.id) {
			throw new Error(`PPT V2 Slice B claim ${claimID} has incomplete source provenance`);
		}
		return `- [${claimID}] ${requiredText(source.title, "source title")} — ${requiredText(citation.locator || source.locator, "citation locator")}`;
	});
	return `${base}\n\nSources:\n${lines.join("\n")}`;
}

function diagnostic(code, message, slideId, elementId) {
  return { code, severity: "error", message, ...(slideId ? { slideId } : {}), ...(elementId ? { elementId } : {}) };
}

function boxesOverlap(left, right) {
  return left.x < right.x + right.width && left.x + left.width > right.x && left.y < right.y + right.height && left.y + left.height > right.y;
}

function elementTextLength(element) {
  if (element.type !== "text") return 0;
  return element.content.kind === "bullets" ? element.content.items.join("").length : element.content.text.length;
}

export function runProfessionalQualityGate(deck, layoutResult) {
  const diagnostics = [];
  const assetIDs = new Set(deck.assetManifest.map((item) => item.id));
  const slideIDs = new Set();
  if (deck.slides.length !== deck.outlinePlan.slideObjectives.length) {
    diagnostics.push(diagnostic("PAGE_COUNT_MISMATCH", "Slide count does not match the approved outline."));
  }
  deck.slides.forEach((slide, index) => {
    if (slideIDs.has(slide.id)) diagnostics.push(diagnostic("DUPLICATE_SLIDE_IDENTITY", `Duplicate slide ${slide.id}.`, slide.id));
    slideIDs.add(slide.id);
    if (slide.sequence !== index + 1) diagnostics.push(diagnostic("INVALID_PAGE_ORDER", `Slide ${slide.id} is out of order.`, slide.id));
    const title = slide.elements.find((item) => item.type === "text" && item.slot === "title");
    if (!title) diagnostics.push(diagnostic("MISSING_TITLE", `Slide ${slide.id} has no title.`, slide.id));
    if (!String(slide.keyMessage ?? "").trim()) diagnostics.push(diagnostic("MISSING_KEY_MESSAGE", `Slide ${slide.id} has no key message.`, slide.id));
    if (slide.evidenceRequired && (!Array.isArray(slide.citationRefs) || slide.citationRefs.length === 0)) {
      diagnostics.push(diagnostic("MISSING_CITATION", `Factual slide ${slide.id} has no claim citation.`, slide.id));
    }
    const definition = professionalLayoutDefinitions[slide.layoutId];
    const expectsImage = Boolean(definition?.slots.image);
    const images = slide.elements.filter((item) => item.type === "image");
    if (expectsImage && images.length === 0) diagnostics.push(diagnostic("MISSING_ASSET", `Slide ${slide.id} requires an image.`, slide.id));
    for (const image of images) {
      if (!assetIDs.has(image.assetRef)) diagnostics.push(diagnostic("BROKEN_ASSET_REFERENCE", `Image ${image.id} references unknown asset ${image.assetRef}.`, slide.id, image.id));
    }
    for (const element of slide.elements) {
      const threshold = definition?.slots[element.slot]?.characterThreshold ?? 0;
      if (threshold > 0 && elementTextLength(element) > threshold) {
        diagnostics.push(diagnostic("TEXT_OVERFLOW", `Element ${element.id} exceeds its text capacity.`, slide.id, element.id));
      }
    }
  });
  for (const layout of layoutResult.slides) {
    for (const element of layout.elements) {
      if (element.x < 0 || element.y < 0 || element.width <= 0 || element.height <= 0 || element.x + element.width > layoutResult.canvas.width || element.y + element.height > layoutResult.canvas.height) {
        diagnostics.push(diagnostic("ILLEGAL_BOUNDS", `Element ${element.elementId} has illegal bounds.`, layout.slideId, element.elementId));
      }
    }
    for (let left = 0; left < layout.elements.length; left += 1) {
      for (let right = left + 1; right < layout.elements.length; right += 1) {
        if (boxesOverlap(layout.elements[left], layout.elements[right])) {
          diagnostics.push(diagnostic("ELEMENT_OVERLAP", `Elements ${layout.elements[left].elementId} and ${layout.elements[right].elementId} overlap.`, layout.slideId));
        }
      }
    }
  }
  return { valid: diagnostics.length === 0, diagnostics };
}

export function buildProfessionalDeck(input) {
  if (!input?.approvedOutline || !input.approvedOutline.revision) {
    throw new Error("PPT V2 Slice B requires an approved OutlinePlan revision");
  }
  const pageCount = input.approvedOutline.slides?.length ?? 0;
  if (pageCount < MIN_PAGES || pageCount > MAX_PAGES || input.approvedOutline.pageCount !== pageCount) {
    throw new Error("PPT V2 Slice B approved OutlinePlan must contain 6-12 matching pages");
  }
  const slides = buildSlides(input);
  const deckID = stableID("deck", requiredText(input.generationJobId, "generationJobId"));
  const deck = {
    contractVersion: CONTRACT_VERSION,
    deckId: deckID,
    revision: input.revision,
    deckSpec: {
      title: input.intent.topic,
      language: input.intent.language,
      author: "Xianzhi AI",
      audience: input.intent.audience,
      scenario: input.intent.scenario,
      source: { kind: "agent-approved-outline", taskId: input.generationJobId, clientRequestId: input.approvedOutline.id },
    },
    storyline: legacyStoryline(input, slides),
    outlinePlan: legacyOutline(input, slides),
    designSystem: professionalBusinessDesignSystem(),
    assetManifest: manifest(input),
    provenance: provenance(input.research),
    migrationTrace: {
      adapter: "agent-approved-outline-to-v2-slice-b",
      consumedFields: ["intent", "research", "storyline", "approvedOutline", "slideContents", "assets"],
      ignoredFields: [],
      unmappedFields: [],
    },
    slides,
  };
  assertValidDeckRevision(deck);
  const layoutResult = compileDeckLayout(deck, { definitions: professionalLayoutDefinitions });
  assertValidLayoutResult(layoutResult);
  const quality = runProfessionalQualityGate(deck, layoutResult);
  if (!quality.valid) {
    throw new Error(`PPT V2 quality gate rejected:\n${quality.diagnostics.map((item) => `- ${item.code}: ${item.message}`).join("\n")}`);
  }
  const renderInput = {
    contractVersion: CONTRACT_VERSION,
    deckRevision: { deckId: deck.deckId, revision: deck.revision, title: deck.deckSpec.title, language: deck.deckSpec.language, author: deck.deckSpec.author },
    slides: deck.slides,
    layoutResults: layoutResult.slides,
    designSystem: deck.designSystem,
    assetManifest: deck.assetManifest,
    options: { layout: "wide", deterministic: true },
  };
  assertValidRenderInput(renderInput);
  return { deck, layoutResult, quality, renderInput };
}
