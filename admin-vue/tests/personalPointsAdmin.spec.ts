import { createPinia, setActivePinia } from "pinia";
import { flushPromises, mount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";
import { AdminApiError, apiClient } from "../src/api/client.ts";
import { personalPointsAdminApi } from "../src/api/personalPointsAdmin.ts";
import {
  buildPointMutationPayload,
  buildPolicyMutationPayload,
  buildPointLotSummaries,
  personalPointsErrorState,
  pointAdminActions
} from "../src/domain/personalPointsAdmin.ts";
import { canAccessAdminModule } from "../src/domain/pricePlanGovernance.ts";
import { usePersonalPointsAdminStore } from "../src/stores/personalPointsAdmin.ts";
import { adminModules } from "../src/stores/admin.ts";
import PersonalPointsGovernance from "../src/components/admin/PersonalPointsGovernance.vue";
import CustomerPointActions from "../src/components/admin/CustomerPointActions.vue";

afterEach(() => vi.restoreAllMocks());

const elementStubs = {
  "el-tag": { template: "<span><slot /></span>" },
  "el-button": { template: "<button><slot /></button>" },
  "el-alert": { props: ["title"], template: "<div><span>{{ title }}</span><slot /></div>" },
  "el-skeleton": { template: "<div><slot /></div>" },
  "el-card": { template: "<article><slot name='header' /><slot /></article>" }
};

describe("personal point administration boundaries", () => {
  it("keeps personal-point modules behind their exact permission", () => {
    expect(canAccessAdminModule(
      { role: "ADMIN", permissions: ["admin.full"] },
      "points:gift-policy:view"
    )).toBe(false);
    expect(canAccessAdminModule(
      { role: "POINTS_OPERATOR", permissions: ["points:gift-policy:view"] },
      "points:gift-policy:view"
    )).toBe(true);
    expect(canAccessAdminModule(
      { role: "SUPER_ADMIN", permissions: [] },
      "points:gift-policy:view"
    )).toBe(true);
  });

  it("registers a dedicated point-expiry governance route", () => {
    expect(adminModules).toContainEqual(expect.objectContaining({
      id: "personalPointsGovernance",
      path: "/admin/customers/point-expiry",
      permission: "points:gift-policy:view"
    }));
  });

  it("serializes only server-approved policy and point mutation fields", () => {
    expect(buildPolicyMutationPayload({
      revision: 7,
      enabled: true,
      durationValue: 3,
      changeReason: "  夏季活动策略  ",
      durationUnit: "DAY",
      timeZone: "UTC",
      sourceTypes: ["RECHARGE"]
    } as never)).toEqual({
      revision: 7,
      enabled: true,
      durationValue: 3,
      changeReason: "夏季活动策略"
    });

    expect(buildPointMutationPayload({
      points: 25,
      reason: "  客诉补偿  ",
      idempotencyKey: "  gift-case-25  ",
      source: "RECHARGE",
      expiresAt: "2099-01-01T00:00:00Z",
      tenantId: "tenant-1",
      amountCents: 1
    } as never, "GIFT")).toEqual({
      points: 25,
      reason: "客诉补偿",
      idempotencyKey: "gift-case-25"
    });

    expect(() => buildPointMutationPayload({ points: -1, reason: "x", idempotencyKey: "gift-negative" }, "GIFT")).toThrow("赠送积分必须大于 0");
    expect(() => buildPointMutationPayload({ points: 0, reason: "x", idempotencyKey: "correction-zero" }, "CORRECTION")).toThrow("余额纠正不能为 0");
  });

  it("derives explicitly labelled lot summaries without inventing an EXPIRE event", () => {
    const active = {
      id: "lot-active",
      account_id: "account-1",
      user_id: "user-1",
      source_type: "ADMIN_GIFT",
      reference_type: "ADMIN_GIFT",
      reference_id: "gift-1",
      original_points: 10,
      available_points: 10,
      reserved_points: 0,
      consumed_points: 0,
      expired_points: 0,
      reversed_points: 0,
      granted_at: "2026-08-04T01:00:00Z",
      expires_at: "2026-11-04T01:00:00Z",
      idempotency_key: "gift-1",
      status: "ACTIVE"
    };
    const expired = { ...active, id: "lot-expired", available_points: 0, expired_points: 10, status: "EXPIRED" };

    expect(buildPointLotSummaries([active])).toEqual([
      expect.objectContaining({ type: "GRANT", points: 10, summaryOnly: true })
    ]);
    expect(buildPointLotSummaries([expired])).toEqual([
      expect.objectContaining({ type: "EXPIRE", points: 10, summaryOnly: true }),
      expect.objectContaining({ type: "GRANT", points: 10, summaryOnly: true })
    ]);
  });

  it("keeps permissions separate and preserves 403/409 server messages", () => {
    expect(pointAdminActions({ role: "ADMIN", permissions: ["admin.full"] })).toEqual({
      canViewPolicy: false,
      canManagePolicy: false,
      canGift: false,
      canCorrect: false,
      canViewLots: false
    });
    expect(pointAdminActions({ role: "POINTS_OPERATOR", permissions: ["points:gift:grant", "points:lot:view"] })).toEqual(expect.objectContaining({
      canGift: true,
      canViewLots: true,
      canCorrect: false
    }));
    expect(personalPointsErrorState(new AdminApiError("策略已被其他管理员更新", 409, "POINT_POLICY_REVISION_CONFLICT"))).toEqual({
      message: "策略已被其他管理员更新",
      status: 409,
      code: "POINT_POLICY_REVISION_CONFLICT",
      forbidden: false,
      conflict: true
    });
    expect(personalPointsErrorState(new AdminApiError("暂无权限查看积分批次", 403, "ADMIN_PERMISSION_DENIED"))).toEqual(expect.objectContaining({
      message: "暂无权限查看积分批次",
      forbidden: true,
      conflict: false
    }));
  });

  it("uses the shared Axios client and keeps URLs and bodies narrow", async () => {
    const requests: Array<{ method?: string; url?: string; data?: unknown }> = [];
    apiClient.defaults.adapter = async (config) => {
      requests.push({ method: config.method, url: config.url, data: typeof config.data === "string" ? JSON.parse(config.data) : config.data });
      return { data: { item: {} }, status: 200, statusText: "OK", headers: {}, config };
    };

    await personalPointsAdminApi.updatePolicy({ revision: 2, enabled: true, durationValue: 3, changeReason: "publish" });
    await personalPointsAdminApi.grantGift("user / 1", { points: 5, reason: "gift", idempotencyKey: "gift-5" });
    await personalPointsAdminApi.correctBalance("user / 1", { points: -2, reason: "repair", idempotencyKey: "repair-2" });
    await personalPointsAdminApi.listLots("user / 1", { source: "ADMIN_GIFT", status: "ACTIVE", limit: 50, offset: 0 });

    expect(requests).toEqual([
      { method: "put", url: "/admin/points/expiry-policy", data: { revision: 2, enabled: true, durationValue: 3, changeReason: "publish" } },
      { method: "post", url: "/admin/customers/user%20%2F%201/point-gifts", data: { points: 5, reason: "gift", idempotencyKey: "gift-5" } },
      { method: "post", url: "/admin/customers/user%20%2F%201/point-corrections", data: { points: -2, reason: "repair", idempotencyKey: "repair-2" } },
      { method: "get", url: "/admin/customers/user%20%2F%201/point-lots", data: undefined }
    ]);
  });

  it("tracks policy loading and preserves conflict details in Pinia state", async () => {
    setActivePinia(createPinia());
    const store = usePersonalPointsAdminStore();
    const policy = {
      id: "policy-v2",
      version: 2,
      revision: 2,
      enabled: true,
      duration_value: 3,
      duration_unit: "CALENDAR_MONTH" as const,
      time_zone: "Asia/Shanghai",
      source_types: ["REGISTRATION_GIFT", "ACTIVITY_GIFT", "ADMIN_GIFT"],
      effective_from: "2026-08-04T00:00:00Z",
      status: "PUBLISHED",
      change_reason: "initial"
    };
    vi.spyOn(personalPointsAdminApi, "getPolicy").mockResolvedValue({ item: policy });
    await store.loadPolicy();
    expect(store.policy).toEqual(policy);
    expect(store.loading.policy).toBe(false);

    vi.spyOn(personalPointsAdminApi, "updatePolicy").mockRejectedValue(new AdminApiError(
      "当前 revision 已过期，请刷新策略",
      409,
      "POINT_POLICY_REVISION_CONFLICT"
    ));
    await expect(store.publishPolicy({ revision: 2, enabled: false, durationValue: 3, changeReason: "pause" })).rejects.toThrow("当前 revision 已过期，请刷新策略");
    expect(store.saving.policy).toBe(false);
    expect(store.errors.policy).toEqual(expect.objectContaining({
      message: "当前 revision 已过期，请刷新策略",
      conflict: true,
      status: 409
    }));
  });

  it("keeps customer mutations separate and refreshes point lots", async () => {
    setActivePinia(createPinia());
    const store = usePersonalPointsAdminStore();
    const lot = {
      id: "lot-1", account_id: "account-1", user_id: "user-1", source_type: "ADMIN_GIFT",
      reference_type: "ADMIN_GIFT", reference_id: "gift-1", original_points: 5, available_points: 5,
      reserved_points: 0, consumed_points: 0, expired_points: 0, reversed_points: 0,
      granted_at: "2026-08-04T00:00:00Z", expires_at: "2026-11-04T00:00:00Z", idempotency_key: "gift-1", status: "ACTIVE"
    };
    const gift = vi.spyOn(personalPointsAdminApi, "grantGift").mockResolvedValue({ item: lot, idempotent: false });
    const correction = vi.spyOn(personalPointsAdminApi, "correctBalance").mockResolvedValue({
      balance: { account_id: "account-1", user_id: "user-1", available: 3, frozen: 0, total: 3 },
      points: -2,
      idempotent: false
    });
    vi.spyOn(personalPointsAdminApi, "listLots").mockResolvedValue({ items: [lot] });

    await store.grantGift("user-1", { points: 5, reason: "campaign", idempotencyKey: "gift-1" });
    await store.correctBalance("user-1", { points: -2, reason: "repair", idempotencyKey: "repair-1" });

    expect(gift).toHaveBeenCalledWith("user-1", { points: 5, reason: "campaign", idempotencyKey: "gift-1" });
    expect(correction).toHaveBeenCalledWith("user-1", { points: -2, reason: "repair", idempotencyKey: "repair-1" });
    expect(store.lotsByUser["user-1"]).toEqual([lot]);
    expect(store.saving.gift).toBe(false);
    expect(store.saving.correction).toBe(false);
  });

  it("requires preview and confirmation before publishing the fixed calendar-month policy", async () => {
    const pinia = createPinia();
    setActivePinia(pinia);
    vi.spyOn(personalPointsAdminApi, "getPolicy").mockResolvedValue({ item: {
      id: "policy-v1", version: 1, revision: 1, enabled: true, duration_value: 3,
      duration_unit: "CALENDAR_MONTH", time_zone: "Asia/Shanghai",
      source_types: ["REGISTRATION_GIFT", "ACTIVITY_GIFT", "ADMIN_GIFT"],
      effective_from: "2026-08-04T00:00:00Z", status: "PUBLISHED", change_reason: "initial"
    } });
    const publish = vi.spyOn(personalPointsAdminApi, "updatePolicy").mockResolvedValue({ item: {
      id: "policy-v2", version: 2, revision: 2, enabled: false, duration_value: 3,
      duration_unit: "CALENDAR_MONTH", time_zone: "Asia/Shanghai",
      source_types: ["REGISTRATION_GIFT", "ACTIVITY_GIFT", "ADMIN_GIFT"],
      effective_from: "2026-08-04T01:00:00Z", status: "PUBLISHED", change_reason: "pause campaign"
    } });
    const wrapper = mount(PersonalPointsGovernance, {
      props: { role: "POINTS_OWNER", permissions: ["points:gift-policy:view", "points:gift-policy:manage"] },
      global: { plugins: [pinia], stubs: elementStubs }
    });
    await flushPromises();

    expect(wrapper.text()).toContain("CALENDAR_MONTH");
    expect(wrapper.text()).toContain("Asia/Shanghai");
    expect((wrapper.get('[data-testid="policy-duration"]').element as HTMLInputElement).value).toBe("3");
    expect(wrapper.get('[data-testid="policy-submit"]').attributes()).toHaveProperty("disabled");

    await wrapper.get('[data-testid="policy-reason"]').setValue("pause campaign");
    await wrapper.get('[data-testid="policy-enabled"]').setValue(false);
    await wrapper.get('[data-testid="policy-preview"]').trigger("click");
    expect(wrapper.text()).toContain("变更预览");
    await wrapper.get('[data-testid="policy-confirm"]').setValue(true);
    expect(wrapper.get('[data-testid="policy-submit"]').attributes("disabled")).toBeUndefined();
    await wrapper.get("form").trigger("submit");
    await flushPromises();

    expect(publish).toHaveBeenCalledWith({ revision: 1, enabled: false, durationValue: 3, changeReason: "pause campaign" });
  });

  it("renders only authorized customer point actions and an honest lot summary", async () => {
    const pinia = createPinia();
    setActivePinia(pinia);
    const activeLot = {
      id: "lot-active", account_id: "account-1", user_id: "user-1", source_type: "ADMIN_GIFT",
      reference_type: "ADMIN_GIFT", reference_id: "gift-1", original_points: 5, available_points: 5,
      reserved_points: 0, consumed_points: 0, expired_points: 0, reversed_points: 0,
      granted_at: "2026-08-04T00:00:00Z", expires_at: "2026-11-04T00:00:00Z", idempotency_key: "gift-1", status: "ACTIVE"
    };
    vi.spyOn(personalPointsAdminApi, "listLots").mockResolvedValue({ items: [activeLot] });
    const gift = vi.spyOn(personalPointsAdminApi, "grantGift").mockResolvedValue({ item: activeLot, idempotent: false });
    const wrapper = mount(CustomerPointActions, {
      props: { userId: "user-1", userName: "测试客户", role: "POINTS_OPERATOR", permissions: ["points:gift:grant", "points:lot:view"] },
      global: { plugins: [pinia], stubs: elementStubs }
    });
    await flushPromises();

    expect(wrapper.text()).toContain("赠送积分");
    expect(wrapper.text()).not.toContain("余额纠正");
    expect(wrapper.text()).toContain("批次变动摘要（非独立账本流水）");
    expect(wrapper.text()).toContain("GRANT");
    expect(wrapper.findAll(".point-summary-list b").map((item) => item.text())).not.toContain("EXPIRE");

    await wrapper.get('[data-testid="gift-points"]').setValue("5");
    await wrapper.get('[data-testid="gift-reason"]').setValue("campaign gift");
    await wrapper.get('[data-testid="gift-confirm"]').setValue(true);
    expect(wrapper.get('[data-testid="gift-submit"]').attributes("disabled")).toBeUndefined();
    await wrapper.get("form").trigger("submit");
    await flushPromises();
    const submitted = gift.mock.calls[0][1];
    expect(Object.keys(submitted).sort()).toEqual(["idempotencyKey", "points", "reason"]);
    expect(submitted).toEqual(expect.objectContaining({ points: 5, reason: "campaign gift" }));

    const denied = mount(CustomerPointActions, {
      props: { userId: "user-1", userName: "测试客户", role: "ADMIN", permissions: ["admin.full"] },
      global: { plugins: [createPinia()], stubs: elementStubs }
    });
    expect(denied.text()).toContain("暂无积分管理权限");
    expect(denied.text()).not.toContain("赠送积分");
  });
});
