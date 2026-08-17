import {
  COVER_LAYOUT_ID,
  STANDARD_CONTENT_LAYOUT_ID,
} from "./migration-adapter.mjs";

export { COVER_LAYOUT_ID, STANDARD_CONTENT_LAYOUT_ID };

const CANVAS = Object.freeze({ unit: "pt", width: 960, height: 540 });
const SAFE_AREA = Object.freeze({ left: 48, top: 36, right: 48, bottom: 36 });

function slot(x, y, width, height, zIndex, characterThreshold = 0) {
  return { x, y, width, height, zIndex, characterThreshold };
}

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

export const professionalLayoutDefinitions = {
  layout_professional_cover_v1: {
    safeArea: SAFE_AREA,
    slots: {
      eyebrow: slot(72, 72, 816, 24, 0, 80),
      title: slot(72, 132, 816, 112, 1, 100),
      subtitle: slot(72, 270, 744, 74, 2, 180),
      footer: slot(72, 452, 816, 24, 3, 100),
    },
  },
  layout_professional_section_v1: {
    safeArea: SAFE_AREA,
    slots: {
      eyebrow: slot(72, 92, 816, 24, 0, 80),
      title: slot(72, 154, 816, 86, 1, 100),
      body: slot(72, 282, 720, 104, 2, 280),
    },
  },
  layout_professional_title_body_v1: {
    safeArea: SAFE_AREA,
    slots: {
      title: slot(72, 52, 816, 52, 0, 100),
      "key-message": slot(72, 116, 816, 42, 1, 180),
      body: slot(72, 188, 816, 278, 2, 700),
    },
  },
  layout_professional_title_bullets_v1: {
    safeArea: SAFE_AREA,
    slots: {
      title: slot(72, 52, 816, 52, 0, 100),
      "key-message": slot(72, 116, 816, 42, 1, 180),
      bullets: slot(90, 188, 780, 278, 2, 520),
    },
  },
  layout_professional_two_column_v1: {
    safeArea: SAFE_AREA,
    slots: {
      title: slot(72, 52, 816, 52, 0, 100),
      "key-message": slot(72, 116, 816, 42, 1, 180),
      left: slot(72, 188, 378, 278, 2, 340),
      right: slot(486, 188, 402, 278, 3, 340),
    },
  },
  layout_professional_text_image_v1: {
    safeArea: SAFE_AREA,
    slots: {
      title: slot(72, 52, 816, 52, 0, 100),
      "key-message": slot(72, 116, 816, 42, 1, 180),
      body: slot(72, 188, 372, 278, 2, 360),
      image: slot(480, 188, 408, 278, 3),
    },
  },
  layout_professional_image_text_v1: {
    safeArea: SAFE_AREA,
    slots: {
      title: slot(72, 52, 816, 52, 0, 100),
      "key-message": slot(72, 116, 816, 42, 1, 180),
      image: slot(72, 188, 408, 278, 2),
      body: slot(516, 188, 372, 278, 3, 360),
    },
  },
  layout_professional_key_metric_v1: {
    safeArea: SAFE_AREA,
    slots: {
      title: slot(72, 52, 816, 52, 0, 100),
      metric: slot(72, 166, 330, 104, 1, 24),
      "metric-label": slot(72, 282, 330, 38, 2, 100),
      body: slot(450, 166, 438, 222, 3, 420),
    },
  },
  layout_professional_closing_action_v1: {
    safeArea: SAFE_AREA,
    slots: {
      eyebrow: slot(72, 88, 816, 24, 0, 80),
      title: slot(72, 142, 816, 78, 1, 100),
      "key-message": slot(72, 246, 816, 54, 2, 180),
      body: slot(72, 342, 744, 112, 3, 260),
    },
  },
};

export const allLayoutDefinitions = {
  ...phase1LayoutDefinitions,
  ...professionalLayoutDefinitions,
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
  if (element.type === "image") {
    return { kind: "image", fit: element.fit };
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
  const definitions = options.definitions ?? allLayoutDefinitions;
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
