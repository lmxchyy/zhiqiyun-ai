import { flushPromises, mount } from "@vue/test-utils";
import { describe, expect, it, vi } from "vitest";
import { normalizeAnalyticsModelsResponse, normalizeAnalyticsProvidersResponse, analyticsMetricValue } from "../src/components/admin/analyticsContract";
import Dashboard from "../src/components/admin/AdminAnalyticsDashboard.vue";

vi.mock("../src/api/client", () => ({
  adminRequest: vi.fn(async ({ url }: { url: string }) => {
    if (url.includes("overview")) {
      return {
        newUsersToday: 7,
        dau: 5,
        wau: 9,
        mau: 12,
        aiUsersToday: 4,
        imagesGenerated: 3,
        videosGenerated: 2,
        pointsConsumed: 80,
        tokensUsed: 100,
        revenueTodayCents: 1000,
        costTodayCents: 400,
        failedTasksToday: 1,
        processingTasks: 15,
        exceptionCount: 2,
        successRate: 90,
        avgLatencyMs: 120,
      };
    }
    if (url.includes("trends")) {
      const point = { date: "2026-08-27", value: 1 };
      return {
        newUsers: [point],
        dau: [point],
        wau: [point],
        mau: [point],
        aiUsers: [point],
        images: [],
        videos: [],
        points: [point],
        tokens: [point],
        revenue: [],
        cost: [],
        tasks: [],
        success: [],
        latency: [],
        failed: [],
      };
    }
    if (url.includes("generation")) {
      return {
        imagesToday: 3,
        videosToday: 2,
        totalTasksToday: 5,
        successRate: 90,
        avgLatencyMs: 120,
        failedTasks: 1,
        byType: [
          { type: "TEXT_TO_IMAGE", count: 3, rate: 60 },
          { type: "TEXT_TO_VIDEO", count: 2, rate: 40 },
        ],
      };
    }
    if (url.includes("models")) {
      return {
        models: [
          { modelCode: "gpt-image-2", callCount: 3, successCount: 3, successRate: 100, avgLatencyMs: 20, totalCostCents: 50 },
        ],
      };
    }
    if (url.includes("providers")) {
      return {
        providers: [
          { providerCode: "openai", callCount: 3, successCount: 3, successRate: 100, avgLatencyMs: 20, totalCostCents: 50 },
        ],
      };
    }
    if (url.includes("tokens")) {
      return { tokensToday: 1, tokens7d: 2, tokens30d: 3, byUser: [] };
    }
    return {
      consumedToday: 80,
      rechargedToday: 1000,
      grantedToday: 0,
      frozenToday: 0,
      releasedToday: 0,
      netChangeToday: 920,
      totalAvailable: 5000,
      totalFrozen: 200,
      consumedTrend: [],
      rechargedTrend: [],
      byType: [],
    };
  }),
}));

describe("admin analytics dashboard contract", () => {
  it("consumes camelCase overview metrics without legacy aliases", () => {
    expect(analyticsMetricValue({ newUsersToday: 7 }, "newUsersToday")).toBe(7);
    expect(analyticsMetricValue({ NewUsersToday: 99 }, "newUsersToday")).toBe(0);
  });

  it("unwraps models and providers response objects", () => {
    expect(normalizeAnalyticsModelsResponse({ models: [{ modelCode: "gpt-image-2" }] })).toEqual([{ modelCode: "gpt-image-2" }]);
    expect(normalizeAnalyticsProvidersResponse({ providers: [{ providerCode: "openai" }] })).toEqual([{ providerCode: "openai" }]);
  });

  it("renders camelCase overview data from the backend contract", async () => {
    const wrapper = mount(Dashboard);
    await flushPromises();
    expect(wrapper.text()).toContain("7");
    expect(wrapper.text()).toContain("90.0%");
  });

  it("renders five ECharts trend containers for the required metrics", async () => {
    const wrapper = mount(Dashboard);
    await flushPromises();
    await wrapper.get(".tab:nth-child(2)").trigger("click");
    await flushPromises();
    expect(wrapper.findAll(".chart-content")).toHaveLength(5);
  });

  it("renders the 8 top KPI cards with accurate units and labels", async () => {
    const wrapper = mount(Dashboard);
    await flushPromises();

    const text = wrapper.text();
    // 1. New users
    expect(text).toContain("今日新增用户");
    // 2. DAU
    expect(text).toContain("今日活跃用户 (DAU)");
    // 3. AI Tasks Total
    expect(text).toContain("今日 AI 生成量");
    expect(text).toContain("5次");
    // 4. Success rate
    expect(text).toContain("今日整体成功率");
    // 5. Points
    expect(text).toContain("今日积分消耗");
    expect(text).toContain("80点");
    // 6. Revenue
    expect(text).toContain("今日充值收入");
    expect(text).toContain("10.00元");
    // 7. Processing tasks
    expect(text).toContain("当前处理中任务");
    expect(text).toContain("15个");
    // 8. Exception tasks
    expect(text).toContain("当前异常风险任务");
    expect(text).toContain("2条");
  });

  it("renders AI application types distribution and format Chinese names", async () => {
    const wrapper = mount(Dashboard);
    await flushPromises();

    const text = wrapper.text();
    expect(text).toContain("AI 应用类型分布");
    expect(text).toContain("文生图");
    expect(text).toContain("文生视频");
  });

  it("renders system runtime panel with operational indicators", async () => {
    const wrapper = mount(Dashboard);
    await flushPromises();

    const text = wrapper.text();
    expect(text).toContain("系统运行状态");
    expect(text).toContain("处理中任务 (PROCESSING)");
    expect(text).toContain("异常处置工单");
  });
});
