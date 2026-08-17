import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";
import PptAgentOutlineReview from "../src/components/ppt/PptAgentOutlineReview.vue";
import { planningProductMessage, planningStageLabel } from "../src/stores/pptAgent";
import type { AgentPlanningState } from "../src/types/pptAgent";

function planningState(): AgentPlanningState {
  return {
    job: {
      id: "job_1", workflowType: "AGENT_OUTLINE", tenantId: "tenant_1", userId: "user_1", organizationId: "org_1",
      status: "WAITING_FOR_OUTLINE_APPROVAL", stage: "OUTLINE_PLANNED", completedWorkUnits: 3, totalWorkUnits: 3,
      slideCount: 10, runAfter: "2026-08-16T00:00:00Z", updatedAt: "2026-08-16T00:00:00Z"
    },
    intent: {
      topic: "新能源汽车行业", goal: "支持管理层决策", audience: "公司管理层", scenario: "行业分析",
      language: "zh-CN", pageCount: { min: 10, max: 10, preferred: 10, explicit: true }, professionalStyle: "professional", researchRequired: true
    },
    research: {
      sources: [{
        id: "source_1", provider: "research", providerIdentity: "ev-market", title: "新能源汽车市场年度报告",
        type: "industry_report", locator: "https://example.test/report", retrievedAt: "2026-08-16T00:00:00Z"
      }],
      citations: [{ id: "citation_1", sourceId: "source_1", locator: "https://example.test/report#sales", retrievedAt: "2026-08-16T00:00:00Z" }],
      claims: [{
        id: "claim_1", sourceId: "source_1", citationRefs: ["citation_1"], text: "新能源汽车销量持续增长。", verificationStatus: "SOURCE_SUPPORTED"
      }],
      verificationStatus: "SOURCE_SUPPORTED"
    },
    storyline: {
      id: "storyline_1", language: "zh-CN", thesis: "市场变化需要管理层现在做出选择。", audienceTakeaway: "聚焦高置信度机会。",
      narrativeArc: ["context"], sections: [{ id: "context", title: "市场变化", objective: "建立决策背景", evidenceRefs: ["claim_1"] }],
      closingAction: "明确优先市场和负责人。", provenance: { mode: "AI", provider: "chat", model: "model" }
    },
    outline: {
      id: "job_1_outline", revision: 1, topic: "新能源汽车行业", language: "zh-CN", pageCount: 10, nextSlideSequence: 11,
      createdAt: "2026-08-16T00:00:00Z", provenance: { mode: "AI", provider: "chat", model: "model" },
      slides: Array.from({ length: 10 }, (_, index) => ({
        slideId: `slide_${index + 1}`, title: `第 ${index + 1} 页`, purpose: "支持管理层决策", keyMessage: "需要聚焦高置信度机会。",
        evidenceRequired: index === 1, evidenceRefs: index === 1 ? ["claim_1"] : [],
        evidence: index === 1 ? [{ claimId: "claim_1", rationale: "销量趋势直接支持本页的市场判断。" }] : [],
        visualIntent: "清晰的商务信息层级", expectedElementTypes: ["TEXT", "SHAPE"]
      }))
    },
    researchExecutionCount: 1
  };
}

describe("PPT Agent durable planning UX", () => {
  it("maps persisted stages to honest user-facing status", () => {
    expect(planningStageLabel("CREATED")).toBe("正在理解需求");
    expect(planningStageLabel("INTENT_RESOLVED")).toBe("正在研究资料");
    expect(planningStageLabel("RESEARCHED")).toBe("正在规划叙事");
    expect(planningStageLabel("STORYLINE_PLANNED")).toBe("正在生成大纲");
    expect(planningStageLabel("OUTLINE_PLANNED")).toBe("大纲已生成，请确认");
    expect(planningProductMessage("OUTLINE_APPROVED")).toBe("正在生成内容");
    expect(planningStageLabel("CONTENT_READY")).toBe("正在准备图片");
    expect(planningStageLabel("ASSETS_READY")).toBe("正在排版");
    expect(planningStageLabel("LAYOUT_COMPILED")).toBe("正在检查");
    expect(planningStageLabel("QUALITY_CHECKED")).toBe("正在生成 PPTX");
    expect(planningStageLabel("COMPLETED")).toBe("演示文稿已完成");
  });

  it("shows complete evidence provenance and the slide it supports", () => {
    const wrapper = mount(PptAgentOutlineReview, { props: { state: planningState(), busy: false } });
    expect(wrapper.text()).toContain("新能源汽车市场年度报告");
    expect(wrapper.text()).toContain("industry_report");
    expect(wrapper.text()).toContain("新能源汽车销量持续增长");
    expect(wrapper.text()).toContain("https://example.test/report#sales");
    expect(wrapper.text()).toContain("SOURCE_SUPPORTED");
    expect(wrapper.text()).toContain("销量趋势直接支持本页的市场判断");
    expect(wrapper.text()).toContain("支持第 2 页");
    expect(wrapper.text()).toContain("清晰的商务信息层级");
  });

  it("emits deterministic P0 outline commands without replacing evidence", async () => {
    const state = planningState();
    const wrapper = mount(PptAgentOutlineReview, { props: { state, busy: false } });
    const title = wrapper.find('[data-slide-id="slide_2"] [data-field="title"]');
    await title.setValue("更新后的市场判断");
    await title.trigger("change");
    const update = wrapper.emitted("commands")?.at(-1)?.[0] as Array<Record<string, unknown>>;
    expect(update[0]).toMatchObject({ type: "UPDATE_SLIDE_OBJECTIVE", slideId: "slide_2" });
    expect(update[0].objective).toMatchObject({ title: "更新后的市场判断", evidenceRefs: ["claim_1"] });

    await wrapper.find('[data-slide-id="slide_2"] [data-action="move-down"]').trigger("click");
    expect(wrapper.emitted("commands")?.at(-1)?.[0]).toEqual([{ type: "MOVE_SLIDE", slideId: "slide_2", toIndex: 3 }]);
    await wrapper.find('[data-slide-id="slide_2"] [data-action="delete"]').trigger("click");
    expect(wrapper.emitted("commands")?.at(-1)?.[0]).toEqual([{ type: "DELETE_SLIDE", slideId: "slide_2" }]);
    await wrapper.find('[data-action="add-slide"]').trigger("click");
    expect(wrapper.emitted("commands")?.at(-1)?.[0]).toMatchObject([{ type: "ADD_SLIDE", afterSlideId: "slide_10" }]);
  });
});
