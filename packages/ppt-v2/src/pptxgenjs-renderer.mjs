import PptxGenJS from "pptxgenjs";
import JSZip from "jszip";

import { assertValidRenderInput } from "./contract.mjs";
import { PptxRenderer } from "./renderer-port.mjs";

const POINTS_PER_INCH = 72;
const DETERMINISTIC_DATE = new Date("2000-01-01T00:00:00.000Z");

function inches(points) {
  return points / POINTS_PER_INCH;
}

function position(layout) {
  return {
    x: inches(layout.x),
    y: inches(layout.y),
    w: inches(layout.width),
    h: inches(layout.height),
  };
}

function verticalAlign(value) {
  return value === "middle" ? "mid" : value;
}

function transparentPaint() {
  return { color: "FFFFFF", transparency: 100 };
}

function renderShape(slide, presentation, layout) {
  const style = layout.resolvedStyle;
  const shapeType = {
    rect: presentation.ShapeType.rect,
    roundRect: presentation.ShapeType.roundRect,
    ellipse: presentation.ShapeType.ellipse,
  }[style.shapeType];
  slide.addShape(shapeType, {
    ...position(layout),
    objectName: layout.elementId,
    fill: style.fillColor === "none"
      ? transparentPaint()
      : { color: style.fillColor, transparency: style.transparency },
    line: style.lineColor === "none"
      ? transparentPaint()
      : { color: style.lineColor, width: style.lineWidthPt },
  });
}

function textOptions(layout) {
  const style = layout.resolvedStyle;
  return {
    ...position(layout),
    objectName: layout.elementId,
    fontFace: style.fontFace,
    fontSize: style.fontSizePt,
    color: style.color,
    bold: style.bold,
    italic: style.italic,
    align: style.align,
    valign: verticalAlign(style.verticalAlign),
    margin: style.marginPt,
    breakLine: false,
    isTextBox: true,
  };
}

function renderText(slide, element, layout) {
  const content = element.content;
  if (content.kind === "bullets") {
    const runs = content.items.map((text, index) => ({
      text,
      options: {
        bullet: { indent: 18 },
        breakLine: index < content.items.length - 1,
      },
    }));
    slide.addText(runs, textOptions(layout));
    return;
  }
  slide.addText(content.text, textOptions(layout));
}

function normalizePresentationXml(xml) {
  const notesMaster = xml.match(/<p:notesMasterIdLst>[\s\S]*?<\/p:notesMasterIdLst>/)?.[0];
  if (!notesMaster) {
    return xml;
  }
  const withoutNotesMaster = xml.replace(notesMaster, "");
  const slideListOffset = withoutNotesMaster.indexOf("<p:sldIdLst>");
  if (slideListOffset < 0) {
    throw new Error("presentation.xml has notes but no slide id list");
  }
  return `${withoutNotesMaster.slice(0, slideListOffset)}${notesMaster}${withoutNotesMaster.slice(slideListOffset)}`;
}

function normalizeCoreProperties(xml) {
  return xml
    .replace(/<dcterms:created[^>]*>[\s\S]*?<\/dcterms:created>/, '<dcterms:created xsi:type="dcterms:W3CDTF">2000-01-01T00:00:00Z</dcterms:created>')
    .replace(/<dcterms:modified[^>]*>[\s\S]*?<\/dcterms:modified>/, '<dcterms:modified xsi:type="dcterms:W3CDTF">2000-01-01T00:00:00Z</dcterms:modified>');
}

async function normalizeOpenXmlPackage(buffer) {
  const zip = await JSZip.loadAsync(buffer);
  const presentationPath = "ppt/presentation.xml";
  const presentationXml = await zip.file(presentationPath).async("string");
  zip.file(presentationPath, normalizePresentationXml(presentationXml), { date: DETERMINISTIC_DATE });

  const corePath = "docProps/core.xml";
  const coreXml = await zip.file(corePath).async("string");
  zip.file(corePath, normalizeCoreProperties(coreXml), { date: DETERMINISTIC_DATE });
  for (const entry of Object.values(zip.files)) {
    entry.date = DETERMINISTIC_DATE;
  }
  return zip.generateAsync({
    type: "nodebuffer",
    compression: "DEFLATE",
    compressionOptions: { level: 9 },
    platform: "DOS",
  });
}

export class PptxGenJSRenderer extends PptxRenderer {
  async render(renderInput) {
    assertValidRenderInput(renderInput);
    const presentation = new PptxGenJS();
    const layoutName = "XIANZHI_PPT_V2_WIDE";
    presentation.defineLayout({ name: layoutName, width: 960 / POINTS_PER_INCH, height: 540 / POINTS_PER_INCH });
    presentation.layout = layoutName;
    presentation.author = renderInput.deckRevision.author;
    presentation.company = "Xianzhi AI";
    presentation.subject = "PPT Generation V2 Phase 1";
    presentation.title = renderInput.deckRevision.title;
    presentation.lang = renderInput.deckRevision.language;
    presentation.theme = {
      headFontFace: renderInput.designSystem.fonts.heading,
      bodyFontFace: renderInput.designSystem.fonts.body,
      lang: renderInput.deckRevision.language,
    };

    const layoutBySlide = new Map(renderInput.layoutResults.map((item) => [item.slideId, item]));
    for (const slideIR of renderInput.slides) {
      const slideLayout = layoutBySlide.get(slideIR.id);
      const slide = presentation.addSlide();
      slide.background = { color: slideLayout.backgroundColor };
      slide.addNotes(slideIR.speakerNotes);
      const elementById = new Map(slideIR.elements.map((item) => [item.id, item]));
      const orderedLayout = slideLayout.elements
        .map((item, sourceOrder) => ({ item, sourceOrder }))
        .sort((left, right) => left.item.zIndex - right.item.zIndex || left.sourceOrder - right.sourceOrder)
        .map(({ item }) => item);
      for (const elementLayout of orderedLayout) {
        const element = elementById.get(elementLayout.elementId);
        if (element.type === "shape") {
          renderShape(slide, presentation, elementLayout);
        } else {
          renderText(slide, element, elementLayout);
        }
      }
    }

    const output = await presentation.write({ outputType: "nodebuffer", compression: true });
    return normalizeOpenXmlPackage(Buffer.from(output));
  }
}
