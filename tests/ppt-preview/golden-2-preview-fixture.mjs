import { buildProfessionalDeck } from "../../packages/ppt-v2/src/professional-deck.mjs";

const PNG_1X1 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=";
const PNG_SHA256 = "dfe7badda145ff43e5c49ea6e49e40559fe904a7876c78cbaf8a056b8084e5fe";

export function golden2PreviewFixture() {
  const definitions = [
    ["Executive EV Market Outlook", "Frame the management decision", "The EV market now requires a focused response.", "cover"],
    ["Market context", "Establish the external context", "Verified demand signals are reshaping priorities.", "section"],
    ["Three management signals", "Summarize the evidence", "Growth, competition, and policy now move together.", "title-bullets"],
    ["Demand is becoming visible", "Connect evidence to market demand", "Verified demand data supports selective expansion.", "text-image"],
    ["Opportunity and risk", "Compare the two sides of the decision", "Upside is real, but execution risk remains material.", "two-column"],
    ["42%", "Highlight the decision metric", "A single metric clarifies the priority window.", "key-metric"],
    ["Translate insight into action", "Show the operating implication", "Teams should focus resources on the highest-confidence segment.", "image-text"],
    ["Decide the next move", "Close with a concrete action", "Assign an owner and validate the priority market.", "closing-action"]
  ];
  const slides = definitions.map(([title, purpose, keyMessage, layoutHint], index) => ({
    slideId: `slide_golden_2_${index + 1}`,
    title,
    purpose,
    keyMessage,
    evidenceRequired: index > 0 && index < definitions.length - 1,
    evidenceRefs: index > 0 && index < definitions.length - 1 ? ["claim_market"] : [],
    evidence: index > 0 && index < definitions.length - 1 ? [{ claimId: "claim_market", rationale: "The verified market signal directly supports this page's management conclusion." }] : [],
    visualIntent: layoutHint.includes("image") ? "Professional evidence with a market image" : "Professional evidence summary",
    expectedElementTypes: layoutHint.includes("image") ? ["TEXT", "IMAGE"] : ["TEXT", "SHAPE"]
  }));
  const contents = definitions.map(([title, , keyMessage, layoutHint], index) => {
    let bodyBlocks = [{ heading: "Finding", text: keyMessage }];
    let bullets = ["Verified evidence", "Management implication", "Focused next step"];
    if (layoutHint === "cover") bodyBlocks = [];
    if (layoutHint === "two-column") bodyBlocks = [{ heading: "Opportunity", text: "Demand creates a focused growth window." }, { heading: "Risk", text: "Competition raises execution requirements." }];
    if (layoutHint === "key-metric") bodyBlocks = [{ heading: "42%", text: "Illustrative verified market signal" }];
    const needsImage = layoutHint.includes("image");
    return {
      slideId: slides[index].slideId,
      language: "en-US",
      title,
      subtitle: layoutHint === "cover" ? "Professional Research Deck" : undefined,
      bodyBlocks,
      bullets,
      supportingText: keyMessage,
      speakerNotes: `${keyMessage}\nSource: https://example.test/market#claim`,
      assetIntents: needsImage ? [{ id: `provider_image_${index + 1}`, stableId: `asset_intent_${index + 1}`, prompt: "Professional electric vehicle market photograph", altText: "Electric vehicle market" }] : [],
      citationRefs: [...slides[index].evidenceRefs],
      layoutHint
    };
  });
  const assets = contents.flatMap(content => content.assetIntents.map(intent => ({
    id: `asset_${intent.stableId}`,
    intentId: intent.stableId,
    slideId: content.slideId,
    type: "image",
    mimeType: "image/png",
    uri: `asset://ppt-v2/${intent.stableId}.png`,
    sha256: PNG_SHA256,
    altText: intent.altText
  })));
  const approvedOutline = {
    id: "outline_golden_2",
    revision: 3,
    topic: "Electric vehicle market",
    language: "en-US",
    pageCount: 8,
    nextSlideSequence: 9,
    createdAt: "2026-08-16T00:00:00Z",
    approvedAt: "2026-08-16T00:01:00Z",
    provenance: { mode: "AI", provider: "chat", model: "model" },
    slides
  };
  const input = {
    generationJobId: "pptv2_job_golden_2_preview",
    revision: 3,
    intent: { topic: "Electric vehicle market", goal: "industry-analysis", audience: "company management", scenario: "management-report", language: "en-US", professionalStyle: "professional-business" },
    research: {
      sources: [{ id: "source_market", title: "EV Market Report", type: "industry_report", locator: "https://example.test/market" }],
      citations: [{ id: "citation_market", sourceId: "source_market", locator: "https://example.test/market#claim" }],
      claims: [{ id: "claim_market", sourceId: "source_market", citationRefs: ["citation_market"], text: "EV demand is growing across priority segments.", verificationStatus: "SOURCE_SUPPORTED" }]
    },
    storyline: { thesis: "Market growth requires a management decision now.", audienceTakeaway: "Focus on high-confidence opportunities.", narrativeArc: ["context", "evidence", "action"], sections: [], closingAction: "Assign an owner and next step." },
    approvedOutline,
    slideContents: contents,
    assets
  };
  const compiled = buildProfessionalDeck(input);
  return {
    projection: {
      deckId: compiled.deck.deckId,
      revision: compiled.deck.revision,
      deck: compiled.deck,
      layoutResult: compiled.layoutResult,
      assets: assets.map(asset => ({ assetId: asset.id, url: `data:image/png;base64,${PNG_1X1}`, expiresIn: 600, mimeType: asset.mimeType, altText: asset.altText }))
    },
    approvedOutline
  };
}
