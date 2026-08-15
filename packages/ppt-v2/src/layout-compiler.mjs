import {
  COVER_LAYOUT_ID,
  STANDARD_CONTENT_LAYOUT_ID,
} from "./migration-adapter.mjs";

export { COVER_LAYOUT_ID, STANDARD_CONTENT_LAYOUT_ID };

const CANVAS = Object.freeze({ unit: "pt", width: 960, height: 540 });
const SAFE_AREA = Object.freeze({ left: 48, top: 36, right: 48, bottom: 36 });

export const phase1LayoutDefinitions = {
  [COVER_LAYOUT_ID]: {
    safeArea: SAFE_AREA,
    slots: {
      accent: { x: 72, y: 90, width: 96, height: 14, zIndex: 0, characterThreshold: 0 },
      title: { x: 72, y: 150, width: 816, height: 96, zIndex: 1, characterThreshold: 80 },
      subtitle: { x: 72, y: 260, width: 744, height: 72, zIndex: 2, characterThreshold: 160 },
      footer: { x: 72, y: 450, width: 816, height: 24, zIndex: 3, characterThreshold: 100 },
    },
  },
  [STANDARD_CONTENT_LAYOUT_ID]: {
    safeArea: SAFE_AREA,
    slots: {
      panel: { x: 72, y: 154, width: 816, height: 280, zIndex: 0, characterThreshold: 0 },
      title: { x: 72, y: 54, width: 816, height: 64, zIndex: 1, characterThreshold: 100 },
      body: { x: 108, y: 190, width: 744, height: 204, zIndex: 2, characterThreshold: 500 },
    },
  },
};

function diagnostic(code, severity, message, slideId, elementId) {
  return {
    code,
    severity,
    message,
    ...(slideId ? { slideId } : {}),
    ...(elementId ? { elementId } : {}),
  };
}

function resolveColor(designSystem, token) {
  if (token === "none") {
    return "none";
  }
  return designSystem.colors[token];
}

function resolveStyle(element, designSystem) {
  if (element.type === "text") {
    const style = designSystem.textStyles[element.styleRole];
    if (!style) {
      return null;
    }
    return {
      kind: "text",
      fontFace: designSystem.fonts[style.fontRole],
      fontSizePt: style.fontSizePt,
      color: designSystem.colors[style.colorToken],
      bold: style.bold,
      italic: style.italic,
      align: style.align,
      verticalAlign: style.verticalAlign,
      marginPt: style.marginPt,
    };
  }
  const style = designSystem.shapeStyles[element.styleRole];
  if (!style) {
    return null;
  }
  return {
    kind: "shape",
    shapeType: element.shapeType,
    fillColor: resolveColor(designSystem, style.fillColorToken),
    lineColor: resolveColor(designSystem, style.lineColorToken),
    lineWidthPt: style.lineWidthPt,
    transparency: style.transparency,
  };
}

function textLength(element) {
  if (element.type !== "text") {
    return 0;
  }
  return element.content.kind === "bullets"
    ? element.content.items.join("").length
    : element.content.text.length;
}

function geometryDiagnostics(slide, element, slot, safeArea) {
  const diagnostics = [];
  if (slot.width <= 0 || slot.height <= 0) {
    diagnostics.push(diagnostic(
      "NEGATIVE_SIZE",
      "error",
      `Element ${element.id} must have positive width and height.`,
      slide.id,
      element.id,
    ));
  }
  if (!Number.isInteger(slot.zIndex) || slot.zIndex < 0) {
    diagnostics.push(diagnostic(
      "INVALID_Z_INDEX",
      "error",
      `Element ${element.id} has invalid z-index ${slot.zIndex}.`,
      slide.id,
      element.id,
    ));
  }
  if (
    slot.x < safeArea.left ||
    slot.y < safeArea.top ||
    slot.x + slot.width > CANVAS.width - safeArea.right ||
    slot.y + slot.height > CANVAS.height - safeArea.bottom
  ) {
    diagnostics.push(diagnostic(
      "BOUNDS_OUTSIDE_SAFE_AREA",
      "error",
      `Element ${element.id} lies outside the layout safe area.`,
      slide.id,
      element.id,
    ));
  }
  if (slot.characterThreshold > 0 && textLength(element) > slot.characterThreshold) {
    diagnostics.push(diagnostic(
      "TEXT_EXCEEDS_THRESHOLD",
      "warning",
      `Element ${element.id} exceeds the ${slot.characterThreshold} character threshold.`,
      slide.id,
      element.id,
    ));
  }
  return diagnostics;
}

export function compileDeckLayout(deck, options = {}) {
  const definitions = options.definitions ?? phase1LayoutDefinitions;
  const slides = [];
  const allDiagnostics = [];

  for (const slide of deck.slides) {
    const definition = definitions[slide.layoutId];
    const slideDiagnostics = [];
    const elements = [];
    if (!definition) {
      slideDiagnostics.push(diagnostic(
        "LAYOUT_DEFINITION_MISSING",
        "error",
        `Layout definition ${slide.layoutId} is missing.`,
        slide.id,
      ));
    } else {
      for (const element of slide.elements) {
        const slot = definition.slots[element.slot];
        if (!slot) {
          slideDiagnostics.push(diagnostic(
            "ELEMENT_MISSING_SLOT",
            "error",
            `Element ${element.id} references missing slot ${element.slot}.`,
            slide.id,
            element.id,
          ));
          continue;
        }
        const resolvedStyle = resolveStyle(element, deck.designSystem);
        if (!resolvedStyle) {
          slideDiagnostics.push(diagnostic(
            "STYLE_ROLE_MISSING",
            "error",
            `Element ${element.id} references missing style role ${element.styleRole}.`,
            slide.id,
            element.id,
          ));
          continue;
        }
        const elementDiagnostics = geometryDiagnostics(slide, element, slot, definition.safeArea);
        slideDiagnostics.push(...elementDiagnostics);
        elements.push({
          elementId: element.id,
          x: slot.x,
          y: slot.y,
          width: slot.width,
          height: slot.height,
          zIndex: slot.zIndex,
          resolvedStyle,
          diagnostics: elementDiagnostics,
        });
      }
    }
    allDiagnostics.push(...slideDiagnostics);
    slides.push({
      slideId: slide.id,
      layoutId: slide.layoutId,
      backgroundColor: deck.designSystem.colors[slide.backgroundToken],
      elements,
      diagnostics: slideDiagnostics,
    });
  }

  return {
    contractVersion: deck.contractVersion,
    deckId: deck.deckId,
    revision: deck.revision,
    canvas: { ...CANVAS },
    slides,
    diagnostics: allDiagnostics,
  };
}
