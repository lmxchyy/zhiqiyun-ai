import { flushPromises, mount } from "@vue/test-utils";
import { describe, expect, it, vi, beforeEach } from "vitest";
import { normalizeAnalyticsModelsResponse, normalizeAnalyticsProvidersResponse, analyticsMetricValue } from "../src/components/admin/analyticsContract";
import Dashboard, { type AnalyticsScopeInfo } from "../src/components/admin/AdminAnalyticsDashboard.vue";

let mockScopeResponse: AnalyticsScopeInfo = {
  level: "PLATFORM",
  scopeName: "全平台",
  capabilities: {
    canViewPlatformRevenue: true,
    canViewProviderCost: true,
    canViewRuntimeMetrics: true,
    canViewExceptionCenter: true,
    canViewTokens: true,
    canViewProviders: true,
    showRevenue: true,
    showCustomerRanking: true,
    showMemberRanking: false,
  },
};

const mockAdminRequest = vi.fn(async ({ url }: { url: string }) => {
  if (url.includes("scope")) {
    return mockScopeResponse;
  }
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
    frozenToday: 50,
    releasedToday: 0,
    netChangeToday: 920,
    totalAvailable: 5000,
    totalFrozen: 200,
    consumedTrend: [],
    rechargedTrend: [],
    byType: [],
  };
});

vi.mock("../src/api/client", () => ({
  adminRequest: (args: { url: string }) => mockAdminRequest(args),
}));

describe("admin analytics dashboard contract", () => {
  beforeEach(() => {
    mockAdminRequest.mockClear();
    mockScopeResponse = {
      level: "PLATFORM",
      scopeName: "全平台",
      capabilities: {
        canViewPlatformRevenue: true,
        canViewProviderCost: true,
        canViewRuntimeMetrics: true,
        canViewExceptionCenter: true,
        canViewTokens: true,
        canViewProviders: true,
        showRevenue: true,
        showCustomerRanking: true,
        showMemberRanking: false,
      },
    };
  });

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

  it("renders the 8 top KPI cards with accurate units and labels in PLATFORM mode", async () => {
    const wrapper = mount(Dashboard);
    await flushPromises();

    const text = wrapper.text();
    expect(text).toContain("平台运营驾驶舱");
    expect(text).toContain("今日经营概览");
    expect(text).toContain("今日新增用户");
    expect(text).toContain("今日活跃用户 (DAU)");
    expect(text).toContain("今日生图与视频生成量");
    expect(text).toContain("5次");
    expect(text).toContain("今日整体成功率");
    expect(text).toContain("今日积分消耗");
    expect(text).toContain("80点");
    expect(text).toContain("今日充值收入");
    expect(text).toContain("10.00元");
    expect(text).toContain("当前排队/处理中任务");
    expect(text).toContain("15个");
    expect(text).toContain("当前异常风险任务");
    expect(text).toContain("2条");
    expect(text).toContain("任务运行态透视");
  });

  it("keeps the range switch with detail analysis and updates only range-aware labels and requests", async () => {
    const wrapper = mount(Dashboard);
    await flushPromises();

    expect(wrapper.find(".dashboard-header .time-range-segmented").exists()).toBe(false);
    expect(wrapper.find(".analysis-toolbar .time-range-segmented").exists()).toBe(true);
    expect(wrapper.text()).toContain("近 7 日趋势分析");

    await wrapper.find(".analysis-toolbar .time-range-segmented button:nth-child(2)").trigger("click");
    await flushPromises();

    expect(wrapper.text()).toContain("近 30 日趋势分析");
    expect(wrapper.text()).toContain("今日新增用户");
    expect(wrapper.text()).toContain("今日积分消耗");

    const calledUrls = mockAdminRequest.mock.calls.map((call) => call[0].url);
    expect(calledUrls.some((url) => url.includes("/admin/analytics/trends?days=30"))).toBe(true);
    expect(calledUrls.some((url) => url.includes("/admin/analytics/models?days=30"))).toBe(true);
    expect(calledUrls.some((url) => url.includes("/admin/analytics/providers?days=30"))).toBe(true);
    expect(calledUrls.some((url) => url.includes("/admin/analytics/points?days=30"))).toBe(true);
    expect(calledUrls.some((url) => url.includes("/admin/analytics/overview?days="))).toBe(false);
  });

  it("renders OPERATION_CENTER scope dashboard correctly and hides provider cost / runtime panel", async () => {
    mockScopeResponse = {
      level: "OPERATION_CENTER",
      scopeName: "郑州运营中心",
      capabilities: {
        canViewPlatformRevenue: false,
        canViewProviderCost: false,
        canViewRuntimeMetrics: false,
        canViewExceptionCenter: false,
        canViewTokens: false,
        canViewProviders: false,
        showRevenue: true,
        showCustomerRanking: true,
        showMemberRanking: false,
      },
    };

    const wrapper = mount(Dashboard);
    await flushPromises();

    const text = wrapper.text();
    expect(text).toContain("运营中心驾驶舱");
    expect(text).toContain("郑州运营中心");
    expect(text).toContain("今日新增关联用户");
    expect(text).toContain("今日客户充值贡献");
    expect(text).not.toContain("任务运行态透视");
    expect(text).not.toContain("总成本");

    // Request minimization check: ensure /admin/analytics/providers is NOT fetched
    const calledUrls = mockAdminRequest.mock.calls.map((call) => call[0].url);
    expect(calledUrls.some((u) => u.includes("providers"))).toBe(false);
    expect(calledUrls.some((u) => u.includes("tokens"))).toBe(false);
  });

  it("renders AGENT scope dashboard correctly with direct customer and contribution metrics", async () => {
    mockScopeResponse = {
      level: "AGENT",
      scopeName: "代理商 A001",
      capabilities: {
        canViewPlatformRevenue: false,
        canViewProviderCost: false,
        canViewRuntimeMetrics: false,
        canViewExceptionCenter: false,
        canViewTokens: false,
        canViewProviders: false,
        showRevenue: true,
        showCustomerRanking: true,
        showMemberRanking: false,
      },
    };

    const wrapper = mount(Dashboard);
    await flushPromises();

    const text = wrapper.text();
    expect(text).toContain("代理经营驾驶舱");
    expect(text).toContain("代理商 A001");
    expect(text).toContain("今日新增直属客户");
    expect(text).toContain("今日客户充值贡献");
    expect(text).toContain("直属客户生成任务");
    expect(text).not.toContain("任务运行态透视");
  });

  it("renders TENANT scope dashboard correctly with enterprise member and quota metrics", async () => {
    mockScopeResponse = {
      level: "TENANT",
      scopeName: "先知创新科技有限公司",
      capabilities: {
        canViewPlatformRevenue: false,
        canViewProviderCost: false,
        canViewRuntimeMetrics: false,
        canViewExceptionCenter: false,
        canViewTokens: false,
        canViewProviders: false,
        showRevenue: false,
        showCustomerRanking: false,
        showMemberRanking: true,
      },
    };

    const wrapper = mount(Dashboard);
    await flushPromises();

    const text = wrapper.text();
    expect(text).toContain("企业使用驾驶舱");
    expect(text).toContain("先知创新科技有限公司");
    expect(text).toContain("企业成员数");
    expect(text).toContain("今日活跃成员");
    expect(text).toContain("企业可用积分");
    expect(text).toContain("企业冻结积分");
    expect(text).not.toContain("充值收入");
    expect(text).not.toContain("任务运行态透视");
  });
});
