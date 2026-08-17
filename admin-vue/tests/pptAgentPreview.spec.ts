import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";
import PptAgentDeckPreview from "../src/components/ppt/PptAgentDeckPreview.vue";
import { previewCanvasTransform } from "../src/components/ppt/pptAgentPreview";
import type { OutlinePlan, PptAgentPreviewProjection } from "../src/types/pptAgent";

function previewFixture(pageCount = 8): PptAgentPreviewProjection {
  const slides = Array.from({ length: pageCount }, (_, index) => {
    const slideId = `slide_${index + 1}`;
    const elements: Array<Record<string, unknown>> = [
      { id: `${slideId}_title`, type: "text", slot: "title", content: { kind: "plain", text: `Slide ${index + 1}` }, styleRole: "professionalTitle" },
      { id: `${slideId}_bullets`, type: "text", slot: "bullets", content: { kind: "bullets", items: ["Verified evidence", "Management action"] }, styleRole: "professionalBody" }
    ];
    if (index === 0) {
      elements.push({ id: `${slideId}_shape`, type: "shape", slot: "visual", shapeType: "roundRect", styleRole: "professionalCard" });
    }
    if (index === 1) {
      elements.push({ id: `${slideId}_image`, type: "image", slot: "image", assetRef: "asset_market", fit: "cover", altText: "Electric vehicle market", citationRefs: ["claim_market"] });
    }
    return {
      id: slideId,
      sequence: index + 1,
      role: index === 0 ? "cover" : index === pageCount - 1 ? "closing" : "content",
      layoutId: "layout_professional_title_bullets_v1",
      backgroundToken: "background",
      speakerNotes: "Source-backed notes",
      objectiveId: slideId,
      keyMessage: `Key message ${index + 1}`,
      evidenceRequired: index === 1,
      citationRefs: index === 1 ? ["claim_market"] : [],
      elements
    };
  });
  const layoutSlides = slides.map((slide, index) => ({
    slideId: slide.id,
    layoutId: slide.layoutId,
    backgroundColor: index === 0 ? "#172033" : "#FFFFFF",
    elements: [
      { elementId: `${slide.id}_title`, x: 72, y: 64, width: 816, height: 54, zIndex: 3, resolvedStyle: { kind: "text", fontFace: "Aptos", fontSizePt: 28, color: index === 0 ? "#FFFFFF" : "#172033", bold: true, italic: false, align: "left", verticalAlign: "middle", marginPt: 0 } },
      { elementId: `${slide.id}_bullets`, x: 72, y: 160, width: 420, height: 220, zIndex: 2, resolvedStyle: { kind: "text", fontFace: "Aptos", fontSizePt: 18, color: "#344054", bold: false, italic: false, align: "left", verticalAlign: "top", marginPt: 8 } },
      ...(index === 0 ? [{ elementId: `${slide.id}_shape`, x: 520, y: 160, width: 368, height: 220, zIndex: 1, resolvedStyle: { kind: "shape", shapeType: "roundRect", fillColor: "#E8EEF8", lineColor: "#B5C4DD", lineWidthPt: 1, transparency: 0 } }] : []),
      ...(index === 1 ? [{ elementId: `${slide.id}_image`, x: 520, y: 160, width: 368, height: 220, zIndex: 1, resolvedStyle: { kind: "image", fit: "cover" } }] : [])
    ]
  }));
  return {
    deckId: "deck_preview",
    revision: 3,
    deck: {
      contractVersion: "2.1",
      deckId: "deck_preview",
      revision: 3,
      deckSpec: { title: "EV market", language: "en-US" },
      assetManifest: [{ id: "asset_market", type: "image", mimeType: "image/png", uri: "asset://ppt-v2/market", sha256: "a".repeat(64) }],
      provenance: {
        sources: [{ id: "source_market", title: "EV Market Report", type: "industry_report", locator: "https://example.test/market" }],
        citations: [{ id: "citation_market", sourceId: "source_market", locator: "https://example.test/market#claim" }],
        claims: [{ id: "claim_market", sourceId: "source_market", citationRefs: ["citation_market"], text: "EV demand is growing.", verificationStatus: "SOURCE_SUPPORTED" }]
      },
      slides
    },
    layoutResult: { contractVersion: "2.1", deckId: "deck_preview", revision: 3, canvas: { unit: "pt", width: 960, height: 540 }, slides: layoutSlides },
    assets: [{ assetId: "asset_market", url: "https://storage.example/download/asset", expiresIn: 600, mimeType: "image/png", altText: "Electric vehicle market" }]
  } as PptAgentPreviewProjection;
}

function approvedOutline(pageCount = 8): OutlinePlan {
  return {
    id: "outline_preview",
    revision: 3,
    topic: "EV market",
    language: "en-US",
    pageCount,
    nextSlideSequence: pageCount + 1,
    createdAt: "2026-08-16T00:00:00Z",
    approvedAt: "2026-08-16T00:01:00Z",
    provenance: { mode: "AI", provider: "chat", model: "model" },
    slides: Array.from({ length: pageCount }, (_, index) => ({
      slideId: `slide_${index + 1}`,
      title: `Slide ${index + 1}`,
      purpose: "Support a management decision",
      keyMessage: `Key message ${index + 1}`,
      evidenceRequired: index === 1,
      evidenceRefs: index === 1 ? ["claim_market"] : [],
      evidence: index === 1 ? [{ claimId: "claim_market", rationale: "This verified market signal supports the slide conclusion." }] : [],
      visualIntent: "Professional evidence summary",
      expectedElementTypes: index === 1 ? ["TEXT", "IMAGE"] : ["TEXT", "SHAPE"]
    }))
  };
}

describe("PPT Agent geometry-authoritative preview", () => {
  for (const pageCount of [6, 8, 10, 12]) {
    it(`previews and navigates all ${pageCount} approved pages`, async () => {
      const wrapper = mount(PptAgentDeckPreview, { props: { projection: previewFixture(pageCount), approvedOutline: approvedOutline(pageCount), loading: false, error: "", busy: false } });
      expect(wrapper.findAll('[data-preview-thumbnail]')).toHaveLength(pageCount);
      await wrapper.find(`[data-preview-thumbnail="slide_${pageCount}"]`).trigger("click");
      expect(wrapper.text()).toContain(`${pageCount} / ${pageCount}`);
    });
  }

  it("renders the exact LayoutResult geometry with one uniform canvas scale", () => {
    const wrapper = mount(PptAgentDeckPreview, { props: { projection: previewFixture(), approvedOutline: approvedOutline(), loading: false, error: "", busy: false } });
    const element = wrapper.find('[data-preview-element-id="slide_1_title"]');
    expect(element.attributes("style")).toContain("left: 72px");
    expect(element.attributes("style")).toContain("top: 64px");
    expect(element.attributes("style")).toContain("width: 816px");
    expect(element.attributes("style")).toContain("height: 54px");
    expect(element.attributes("style")).toContain("z-index: 3");
    expect(element.attributes("style")).toContain("font-family: Aptos");
    expect(element.attributes("style")).toContain("font-size: 28px");
    expect(element.attributes("style")).toContain("font-weight: 700");
    expect(element.attributes("style")).toContain("text-align: left");
    expect(wrapper.findAll('.ppt-agent-main-canvas [data-preview-element-id="slide_1_bullets"] li')).toHaveLength(2);
    const shape = wrapper.find('[data-preview-shape="slide_1_shape"]');
    expect(shape.attributes("style")).toContain("border-radius: 18px");
    expect(shape.attributes("style")).toContain("opacity: 1");
    expect(wrapper.findAll(".ppt-agent-slide-canvas")).toHaveLength(9);

    const transform = previewCanvasTransform({ width: 960, height: 540 }, 480);
    expect(transform).toEqual({ scale: 0.5, width: 480, height: 270, transform: "scale(0.5)" });
    expect(72 * transform.scale / transform.scale).toBe(72);
  });

  it("shows all slides, keyboard navigation, images, and current-slide evidence", async () => {
    const wrapper = mount(PptAgentDeckPreview, { props: { projection: previewFixture(), approvedOutline: approvedOutline(), loading: false, error: "", busy: false } });
    expect(wrapper.findAll('[data-preview-thumbnail]')).toHaveLength(8);
    expect(wrapper.text()).toContain("1 / 8");
    expect(wrapper.find('[data-preview-shape="slide_1_shape"]').exists()).toBe(true);

    await wrapper.find('[data-preview-thumbnail="slide_2"]').trigger("click");
    expect(wrapper.text()).toContain("2 / 8");
    expect(wrapper.find('[data-preview-thumbnail="slide_2"]').attributes("aria-current")).toBe("page");
    expect(wrapper.find('img[alt="Electric vehicle market"]').attributes("src")).toBe("https://storage.example/download/asset");
    expect(wrapper.find('img[alt="Electric vehicle market"]').attributes("style")).toContain("object-fit: cover");
    expect(wrapper.text()).toContain("EV Market Report");
    expect(wrapper.text()).toContain("industry_report");
    expect(wrapper.text()).toContain("EV demand is growing.");
    expect(wrapper.text()).toContain("https://example.test/market#claim");
    expect(wrapper.text()).toContain("SOURCE_SUPPORTED");
    expect(wrapper.text()).toContain("This verified market signal supports the slide conclusion.");

    await wrapper.find('[data-preview-workspace]').trigger("keydown", { key: "ArrowRight" });
    expect(wrapper.text()).toContain("3 / 8");
    await wrapper.find('[data-action="previous-slide"]').trigger("click");
    expect(wrapper.text()).toContain("2 / 8");
    await wrapper.find('img[alt="Electric vehicle market"]').trigger("error");
    expect(wrapper.emitted("asset-expired")).toHaveLength(1);
    await wrapper.find('[data-action="download-pptx"]').trigger("click");
    expect(wrapper.emitted("download")).toHaveLength(1);
  });

  it("surfaces preview loading and recoverable API errors instead of a blank canvas", async () => {
    const loading = mount(PptAgentDeckPreview, { props: { projection: null, approvedOutline: approvedOutline(), loading: true, error: "", busy: false } });
    expect(loading.text()).toContain("正在加载演示文稿预览");

    const failed = mount(PptAgentDeckPreview, { props: { projection: null, approvedOutline: approvedOutline(), loading: false, error: "预览暂时不可用", busy: false } });
    expect(failed.text()).toContain("预览暂时不可用");
    await failed.find('[data-action="retry-preview"]').trigger("click");
    expect(failed.emitted("retry")).toHaveLength(1);
  });
});
