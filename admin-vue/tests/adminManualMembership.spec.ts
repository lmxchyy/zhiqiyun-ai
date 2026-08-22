import { createPinia, setActivePinia } from "pinia";
import { mount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";
import { apiClient } from "../src/api/client.ts";
import { personalPointsAdminApi } from "../src/api/personalPointsAdmin.ts";
import { buildPointMutationPayload } from "../src/domain/personalPointsAdmin.ts";
import CustomerPointActions from "../src/components/admin/CustomerPointActions.vue";

const originalAdapter = apiClient.defaults.adapter;

afterEach(() => {
  vi.restoreAllMocks();
  apiClient.defaults.adapter = originalAdapter;
});

describe("manual membership administration", () => {
  it("sends membership changes separately from point gifts", async () => {
    const requests: Array<{ method?: string; url?: string; data?: unknown }> = [];
    apiClient.defaults.adapter = async (config) => {
      requests.push({ method: config.method, url: config.url, data: typeof config.data === "string" ? JSON.parse(config.data) : config.data });
      return {
        data: { membership: { userId: "user-1", planId: "plan_ai_creator_996", memberLevel: "PRO", effectiveAt: "2026-08-23T00:00:00Z", expiresAt: "2027-08-23T00:00:00Z", durationDays: 365, idempotent: false }, idempotent: false },
        status: 200,
        statusText: "OK",
        headers: {},
        config
      };
    };

    await personalPointsAdminApi.grantMembership("user / 1", {
      planId: "plan_ai_creator_996",
      durationDays: 365,
      reason: "客户赠送年度会员",
      idempotencyKey: "member-1"
    });

    expect(requests).toEqual([{
      method: "post",
      url: "/admin/customers/user%20%2F%201/point-gifts",
      data: {
        points: 0,
        reason: "客户赠送年度会员",
        idempotencyKey: "member-1",
        membership: {
          planId: "plan_ai_creator_996",
          durationDays: 365,
          reason: "客户赠送年度会员",
          idempotencyKey: "member-1"
        }
      }
    }]);
  });

  it("allows a per-gift validity in days while keeping correction permanent", () => {
    expect(buildPointMutationPayload({ points: 1000, validityDays: 365, reason: "赠送", idempotencyKey: "gift-1" }, "GIFT")).toEqual({
      points: 1000,
      validityDays: 365,
      reason: "赠送",
      idempotencyKey: "gift-1"
    });
    expect(buildPointMutationPayload({ points: 1000, validityDays: 365, reason: "纠正", idempotencyKey: "correction-1" }, "CORRECTION")).toEqual({
      points: 1000,
      reason: "纠正",
      idempotencyKey: "correction-1"
    });
    expect(() => buildPointMutationPayload({ points: 1000, validityDays: 3651, reason: "赠送", idempotencyKey: "gift-2" }, "GIFT")).toThrow("0 到 3650 天");
  });

  it("shows membership grant and gift validity controls to super administrators", async () => {
    setActivePinia(createPinia());
    vi.spyOn(personalPointsAdminApi, "listLots").mockResolvedValue({ items: [] });
    const wrapper = mount(CustomerPointActions, {
      props: { userId: "user-1", userName: "测试客户", role: "SUPER_ADMIN", permissions: [] },
      global: {
        stubs: {
          "el-alert": { props: ["title"], template: "<div><span>{{ title }}</span><slot /></div>" }
        }
      }
    });
    await Promise.resolve();

    expect(wrapper.find('[data-testid="membership-plan"]').exists()).toBe(true);
    expect(wrapper.find('[data-testid="membership-days"]').exists()).toBe(true);
    expect(wrapper.find('[data-testid="membership-submit"]').exists()).toBe(true);
    expect(wrapper.find('[data-testid="gift-validity-days"]').exists()).toBe(true);
    expect(wrapper.text()).toContain("不会自动发放套餐默认 40,000 积分");
  });

  it("does not show the membership grant to ordinary point operators", () => {
    setActivePinia(createPinia());
    const wrapper = mount(CustomerPointActions, {
      props: { userId: "user-1", role: "POINTS_OPERATOR", permissions: ["points:gift:grant"] },
      global: {
        stubs: {
          "el-alert": { props: ["title"], template: "<div><span>{{ title }}</span><slot /></div>" }
        }
      }
    });
    expect(wrapper.find('[data-testid="membership-submit"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="gift-submit"]').exists()).toBe(true);
  });
});
