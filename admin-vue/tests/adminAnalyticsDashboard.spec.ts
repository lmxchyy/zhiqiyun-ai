import { flushPromises, mount } from "@vue/test-utils";
import { describe, expect, it, vi } from "vitest";
import { normalizeAnalyticsModelsResponse, normalizeAnalyticsProvidersResponse, analyticsMetricValue } from "../src/components/admin/analyticsContract";
import Dashboard from "../src/components/admin/AdminAnalyticsDashboard.vue";

vi.mock("../src/api/client", () => ({
  adminRequest: vi.fn(async ({ url }: { url: string }) => {
    if (url.includes("overview")) return { newUsersToday: 7, dau: 5, wau: 9, mau: 12, aiUsersToday: 4, imagesGenerated: 3, videosGenerated: 2, pointsConsumed: 80, tokensUsed: 100, revenueTodayCents: 1000, costTodayCents: 400, failedTasksToday: 1, successRate: 90, avgLatencyMs: 120 };
    if (url.includes("trends")) { const point = { date: "2026-08-27", value: 1 }; return { newUsers: [point], dau: [point], wau: [point], mau: [point], aiUsers: [point], images: [], videos: [], points: [point], tokens: [point], revenue: [], cost: [], tasks: [], success: [], latency: [], failed: [] }; }
    if (url.includes("models")) return { models: [{ modelCode: "gpt-image-2", callCount: 3, successCount: 3, successRate: 100, avgLatencyMs: 20, totalCostCents: 50 }] };
    if (url.includes("providers")) return { providers: [{ providerCode: "openai", callCount: 3, successCount: 3, successRate: 100, avgLatencyMs: 20, totalCostCents: 50 }] };
    if (url.includes("tokens")) return { tokensToday: 1, tokens7d: 2, tokens30d: 3, byUser: [] };
    return { consumedToday: 1, rechargedToday: 2, grantedToday: 3, frozenToday: 0, releasedToday: 0, netChangeToday: 1, totalAvailable: 10, totalFrozen: 0, consumedTrend: [], rechargedTrend: [], byType: [] };
  })
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
});
