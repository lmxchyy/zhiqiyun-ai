import { createPinia, setActivePinia } from "pinia";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { AgentPlanningState, PptAgentPreviewProjection } from "../src/types/pptAgent";

const api = vi.hoisted(() => ({
  getState: vi.fn(),
  getPreview: vi.fn(),
  guide: vi.fn(),
  approve: vi.fn(),
  update: vi.fn(),
  retry: vi.fn(),
  download: vi.fn()
}));

vi.mock("../src/api/ppt", () => ({
  getPptAgentState: api.getState,
  getPptAgentPreview: api.getPreview,
  guidePptAgent: api.guide,
  approvePptAgentOutline: api.approve,
  updatePptAgentOutline: api.update,
  retryPptAgentPlanning: api.retry,
  downloadPptAgentDeck: api.download
}));

import { usePptAgentStore } from "../src/stores/pptAgent";

const completedState = {
  job: {
    id: "job_preview", workflowType: "AGENT_OUTLINE", tenantId: "tenant_1", userId: "user_1", organizationId: "org_1",
    status: "SUCCEEDED", stage: "COMPLETED", completedWorkUnits: 12, totalWorkUnits: 12, slideCount: 8,
    runAfter: "2026-08-16T00:00:00Z", updatedAt: "2026-08-16T00:00:00Z", deckId: "deck_preview", revision: 3
  },
  intent: { topic: "EV", goal: "analysis", audience: "management", scenario: "report", language: "en-US", pageCount: { min: 8, max: 8, preferred: 8, explicit: true }, professionalStyle: "professional", researchRequired: true },
  research: { sources: [], claims: [], citations: [], datasets: [], verificationStatus: "SOURCE_SUPPORTED" },
  storyline: { id: "story", language: "en-US", thesis: "thesis", audienceTakeaway: "takeaway", narrativeArc: ["context"], sections: [], closingAction: "act", provenance: { mode: "AI", provider: "chat", model: "model" } },
  outline: { id: "outline", revision: 3, topic: "EV", language: "en-US", pageCount: 8, nextSlideSequence: 9, slides: [], createdAt: "2026-08-16T00:00:00Z", approvedAt: "2026-08-16T00:00:00Z", provenance: { mode: "AI", provider: "chat", model: "model" } },
  researchExecutionCount: 1
} as AgentPlanningState;

const projection = {
  deckId: "deck_preview",
  revision: 3,
  deck: { contractVersion: "2.1", deckId: "deck_preview", revision: 3, deckSpec: { title: "EV", language: "en-US" }, assetManifest: [], provenance: { sources: [], citations: [], claims: [] }, slides: [] },
  layoutResult: { contractVersion: "2.1", deckId: "deck_preview", revision: 3, canvas: { unit: "pt", width: 960, height: 540 }, slides: [] },
  assets: []
} as PptAgentPreviewProjection;

describe("PPT Agent preview recovery", () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    localStorage.clear();
    vi.clearAllMocks();
    api.getState.mockResolvedValue(completedState);
    api.getPreview.mockResolvedValue(projection);
  });

  it("restores a completed durable job directly into preview after page refresh", async () => {
    localStorage.setItem("xianzhi_ppt_agent_planning_job", JSON.stringify({ jobId: "job_preview", prompt: "Create an EV deck" }));
    const store = usePptAgentStore();
    await store.restoreOrStart({ idempotencyKey: "request", text: "Create an EV deck", language: "en" });

    expect(api.guide).not.toHaveBeenCalled();
    expect(api.getState).toHaveBeenCalledWith("job_preview");
    expect(api.getPreview).toHaveBeenCalledWith("job_preview", 3);
    expect(store.preview).toEqual(projection);
  });

  it("renews preview asset URLs without restarting generation", async () => {
    const store = usePptAgentStore();
    store.state = completedState;
    store.preview = projection;
    await store.refreshPreviewAssets();

    expect(api.getPreview).toHaveBeenCalledTimes(1);
    expect(api.getPreview).toHaveBeenCalledWith("job_preview", 3);
    expect(api.guide).not.toHaveBeenCalled();
  });

  it("does not expose a previous job preview while a different request starts", async () => {
    const store = usePptAgentStore();
    store.state = completedState;
    store.preview = projection;
    api.guide.mockRejectedValue(new Error("generation unavailable"));

    await store.start({ idempotencyKey: "new-request", text: "Create a different deck", language: "en" });

    expect(store.state).toBeNull();
    expect(store.preview).toBeNull();
    expect(store.requestError).toBe("generation unavailable");
  });
});
