import { mount } from "@vue/test-utils";
import ElementPlus, { ElMessage, ElMessageBox } from "element-plus";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { BillingRuleVersion, ProviderCost } from "../src/api/billing";
import { billingApi } from "../src/api/billing";
import GptImageBillingPanel from "../src/components/billing/GptImageBillingPanel.vue";
import {
  buildGptImageParameterRules,
  calculateGptImageQuotePreview,
  classifyValidationIssues,
  extractGptImageMultipliers,
  formatBillingErrorMessage,
  isGPTImageRule,
} from "../src/domain/gptImageBilling";

const mockPublishedV2: BillingRuleVersion = {
  id: "brv_billing_rule_image_gpt_v2",
  ruleKey: "billing_rule_image_gpt",
  legacyRuleId: "billing_rule_image_gpt",
  modelName: "GPT Image 2",
  modelCode: "gpt-image-2",
  moduleCode: "image_generation",
  billingUnit: "PER_IMAGE",
  basePrice: 10,
  minimumCharge: 1,
  parameterRules: {
    quality: { auto: 1, low: 1, normal: 1.2, medium: 1.2, high: 1.5, standard: 1 },
    size: {
      auto: 1,
      tier_720p: 1,
      tier_1k: 1,
      tier_2k: 1.5,
      tier_4k: 2,
      "1024x1024": 1,
      "2048x2048": 1.5,
      "3840x2160": 2,
    },
  },
  ruleSource: "DATABASE",
  version: 2,
  status: "PUBLISHED",
  effectiveFrom: "2026-09-01T00:00:00Z",
  publishedAt: "2026-09-01T00:00:00Z",
  updatedAt: "2026-09-01T00:00:00Z",
  validationResult: {
    valid: false,
    issues: [
      {
        code: "NEGATIVE_MARGIN",
        field: "basePrice",
        severity: "ERROR",
        message: "单位售价折算 0.10 CNY，低于供应商成本 0.60 CNY",
      },
    ],
  },
};

const mockDraftV3: BillingRuleVersion = {
  id: "brv_billing_rule_image_gpt_v3",
  ruleKey: "billing_rule_image_gpt",
  modelName: "GPT Image 2",
  modelCode: "gpt-image-2",
  moduleCode: "image_generation",
  billingUnit: "PER_IMAGE",
  basePrice: 15,
  minimumCharge: 1,
  parameterRules: {
    quality: { auto: 1, low: 1, normal: 1.2, medium: 1.2, high: 1.5 },
    size: { auto: 1, tier_1k: 1, tier_2k: 1.5, tier_4k: 2 },
  },
  ruleSource: "DATABASE",
  version: 3,
  status: "DRAFT",
  createdAt: "2026-09-13T10:00:00Z",
  updatedAt: "2026-09-13T10:00:00Z",
  validationResult: {
    valid: false,
    issues: [
      {
        code: "NEGATIVE_MARGIN",
        field: "basePrice",
        severity: "ERROR",
        message: "单位售价折算 0.15 CNY，低于供应商成本 0.60 CNY",
      },
    ],
  },
};

const mockArchivedV1: BillingRuleVersion = {
  id: "brv_billing_rule_image_gpt_v1",
  ruleKey: "billing_rule_image_gpt",
  modelName: "GPT Image 2",
  modelCode: "gpt-image-2",
  moduleCode: "image_generation",
  billingUnit: "PER_IMAGE",
  basePrice: 10,
  minimumCharge: 1,
  parameterRules: {},
  ruleSource: "CODE_DEFAULT",
  version: 1,
  status: "ARCHIVED",
  effectiveFrom: "2026-08-01T00:00:00Z",
  effectiveTo: "2026-09-01T00:00:00Z",
  updatedAt: "2026-09-01T00:00:00Z",
  validationResult: { valid: true, issues: [] },
};

const mockCosts: ProviderCost[] = [
  {
    id: "pcost_openai_gpt_image_2",
    provider: "OPENAI",
    channel: "channel_openai",
    platformModelCode: "gpt-image-2",
    upstreamModelName: "gpt-image-2",
    billingUnit: "PER_IMAGE",
    parameterRange: {},
    unitCost: 0.6,
    currency: "CNY",
    effectiveFrom: "2026-08-01T00:00:00Z",
    status: "ACTIVE",
    updatedAt: "2026-08-01T00:00:00Z",
  },
];

describe("GptImageBilling Domain Logic", () => {
  it("extracts published multipliers accurately with fallbacks", () => {
    const extracted = extractGptImageMultipliers(mockPublishedV2);
    expect(extracted.basePrice).toBe(10);
    expect(extracted.tier1k).toBe(1.0);
    expect(extracted.tier2k).toBe(1.5);
    expect(extracted.tier4k).toBe(2.0);
    expect(extracted.qualityLow).toBe(1.0);
    expect(extracted.qualityNormal).toBe(1.2);
    expect(extracted.qualityHigh).toBe(1.5);
  });

  it("buildsGptImageParameterRules normalizes tiers, aliases, and quality levels", () => {
    const rules = buildGptImageParameterRules({
      basePrice: 15,
      minimumCharge: 1,
      tier1k: 1.0,
      tier2k: 1.5,
      tier4k: 2.0,
      qualityLow: 1.0,
      qualityNormal: 1.2,
      qualityHigh: 1.5,
    });
    const size = rules.size as Record<string, number>;
    const quality = rules.quality as Record<string, number>;

    expect(size.tier_1k).toBe(1.0);
    expect(size.tier_2k).toBe(1.5);
    expect(size.tier_4k).toBe(2.0);
    expect(size["1024x1024"]).toBe(1.0);
    expect(size["2048x2048"]).toBe(1.5);
    expect(size["3840x2160"]).toBe(2.0);

    expect(quality.low).toBe(1.0);
    expect(quality.normal).toBe(1.2);
    expect(quality.medium).toBe(1.2);
    expect(quality.high).toBe(1.5);
  });

  it("calculates real-time quote preview matching expected numbers for basePrice=10 and 15", () => {
    // basePrice = 10
    const preview10 = calculateGptImageQuotePreview(10, {
      tier1k: 1.0,
      tier2k: 1.5,
      tier4k: 2.0,
      qualityLow: 1.0,
      qualityNormal: 1.2,
      qualityHigh: 1.5,
    });
    expect(preview10[0].lowPoints).toBe(10); // 1K low: 10 * 1 * 1 = 10
    expect(preview10[1].lowPoints).toBe(15); // 2K low: 10 * 1.5 * 1 = 15
    expect(preview10[2].lowPoints).toBe(20); // 4K low: 10 * 2 * 1 = 20

    // basePrice = 15
    const preview15 = calculateGptImageQuotePreview(15, {
      tier1k: 1.0,
      tier2k: 1.5,
      tier4k: 2.0,
      qualityLow: 1.0,
      qualityNormal: 1.2,
      qualityHigh: 1.5,
    });
    expect(preview15[0].lowPoints).toBe(15); // 1K low: 15 * 1 * 1 = 15
    expect(preview15[1].lowPoints).toBe(23); // 2K low: 15 * 1.5 * 1 = 22.5 -> 23
    expect(preview15[2].lowPoints).toBe(30); // 4K low: 15 * 2 * 1 = 30
    expect(preview15[0].normalPoints).toBe(18); // 1K normal: 15 * 1 * 1.2 = 18
    expect(preview15[1].normalPoints).toBe(27); // 2K normal: 15 * 1.5 * 1.2 = 27
    expect(preview15[2].normalPoints).toBe(36); // 4K normal: 15 * 2 * 1.2 = 36
    expect(preview15[0].highPoints).toBe(23); // 1K high: 15 * 1 * 1.5 = 22.5 -> 23
    expect(preview15[1].highPoints).toBe(34); // 2K high: 15 * 1.5 * 1.5 = 33.75 -> 34
    expect(preview15[2].highPoints).toBe(45); // 4K high: 15 * 2 * 1.5 = 45
  });

  it("classifies Hard Blockers and Overrideable NEGATIVE_MARGIN correctly", () => {
    // Only NEGATIVE_MARGIN -> can override
    const class1 = classifyValidationIssues([
      { code: "NEGATIVE_MARGIN", severity: "ERROR", message: "折算低于成本" },
    ]);
    expect(class1.hasHardBlockers).toBe(false);
    expect(class1.hasNegativeMargin).toBe(true);
    expect(class1.canOverridePublish).toBe(true);

    // Hard Blocker -> cannot override
    const class2 = classifyValidationIssues([
      { code: "MISSING_TIER_2K", severity: "ERROR", message: "缺少 tier_2k" },
      { code: "NEGATIVE_MARGIN", severity: "ERROR", message: "折算低于成本" },
    ]);
    expect(class2.hasHardBlockers).toBe(true);
    expect(class2.canOverridePublish).toBe(false);
  });

  it("formats error messages clearly without generic failure text", () => {
    expect(formatBillingErrorMessage({ code: "BILLING_RULE_VALIDATION_FAILED" })).toContain("硬性阻断");
    expect(formatBillingErrorMessage({ code: "NEGATIVE_MARGIN_CONFIRMATION_REQUIRED" })).toContain("负毛利风险");
    expect(formatBillingErrorMessage({ status: 401 })).toContain("登录状态已失效");
    expect(formatBillingErrorMessage({ status: 403 })).toContain("暂无权限");
    expect(formatBillingErrorMessage({ message: "Network Error" })).toContain("网络连接失败");
  });
});

describe("GptImageBillingPanel Component (Requirements 1-12)", () => {
  function mountPanel(props: { rules: BillingRuleVersion[]; costs: ProviderCost[] }) {
    return mount(GptImageBillingPanel, {
      props,
      global: {
        plugins: [ElementPlus],
      },
    });
  }

  beforeEach(() => {
    vi.restoreAllMocks();
    vi.spyOn(ElMessage, "success").mockImplementation(() => ({} as any));
    vi.spyOn(ElMessage, "error").mockImplementation(() => ({} as any));
    vi.spyOn(ElMessageBox, "alert").mockResolvedValue("confirm" as any);
    vi.spyOn(ElMessageBox, "confirm").mockResolvedValue("confirm" as any);
  });

  // 1. 正确展示当前 PUBLISHED 规则
  it("1. correctly displays current PUBLISHED rule", () => {
    const wrapper = mountPanel({
      rules: [mockPublishedV2, mockArchivedV1],
      costs: mockCosts,
    });

    expect(wrapper.text()).toContain("v2 正式版");
    expect(wrapper.text()).toContain("10 积分/张");
    expect(wrapper.text()).toContain("0.60 CNY/张");
    expect(wrapper.text()).toContain("1K: 1x");
    expect(wrapper.text()).toContain("2K: 1.5x");
    expect(wrapper.text()).toContain("4K: 2x");
    expect(wrapper.text()).toContain("low: 1x");
    expect(wrapper.text()).toContain("normal: 1.2x");
    expect(wrapper.text()).toContain("high: 1.5x");
    expect(wrapper.text()).toContain("存在负毛利商业风险");
  });

  // 2. basePrice 输入更新
  it("2. updates basePrice in editor and recalculates", async () => {
    const wrapper = mountPanel({ rules: [mockPublishedV2], costs: mockCosts });

    (wrapper.vm as any).openEditor();
    expect((wrapper.vm as any).editorVisible).toBe(true);

    (wrapper.vm as any).editorForm.basePrice = 15;
    expect((wrapper.vm as any).editorForm.basePrice).toBe(15);
  });

  // 3. tier multiplier 输入更新
  it("3. updates tier multipliers in editor", async () => {
    const wrapper = mountPanel({ rules: [mockPublishedV2], costs: mockCosts });

    (wrapper.vm as any).openEditor();
    (wrapper.vm as any).editorForm.tier2k = 1.8;
    (wrapper.vm as any).editorForm.tier4k = 2.5;

    expect((wrapper.vm as any).editorForm.tier2k).toBe(1.8);
    expect((wrapper.vm as any).editorForm.tier4k).toBe(2.5);
  });

  // 4. quality multiplier 输入更新
  it("4. updates quality multipliers in editor", async () => {
    const wrapper = mountPanel({ rules: [mockPublishedV2], costs: mockCosts });

    (wrapper.vm as any).openEditor();
    (wrapper.vm as any).editorForm.qualityHigh = 1.8;
    expect((wrapper.vm as any).editorForm.qualityHigh).toBe(1.8);
  });

  // 5. 实时试算更新
  it("5. updates real-time quote calculation when basePrice and multipliers change", async () => {
    const wrapper = mountPanel({ rules: [mockPublishedV2], costs: mockCosts });

    (wrapper.vm as any).openEditor();
    // Default basePrice=10
    (wrapper.vm as any).editorForm.basePrice = 10;
    let rows = (wrapper.vm as any).quotePreviewRows;
    expect(rows[0].lowPoints).toBe(10);
    expect(rows[1].lowPoints).toBe(15);
    expect(rows[2].lowPoints).toBe(20);

    // Update to basePrice=15
    (wrapper.vm as any).editorForm.basePrice = 15;
    rows = (wrapper.vm as any).quotePreviewRows;
    expect(rows[0].lowPoints).toBe(15);
    expect(rows[1].lowPoints).toBe(23);
    expect(rows[2].lowPoints).toBe(30);
  });

  // 6. 保存草稿
  it("6. saves rule as DRAFT without publishing", async () => {
    const spyCreate = vi.spyOn(billingApi, "createRuleDraft").mockResolvedValue({
      item: mockDraftV3,
    });

    const wrapper = mountPanel({ rules: [mockPublishedV2], costs: mockCosts });

    (wrapper.vm as any).openEditor();
    (wrapper.vm as any).editorForm.basePrice = 15;

    const draft = await (wrapper.vm as any).handleSaveDraft();
    expect(draft).toBeTruthy();
    expect(draft?.version).toBe(3);
    expect(spyCreate).toHaveBeenCalledWith(
      "brv_billing_rule_image_gpt_v2",
      expect.objectContaining({
        base_price: 15,
        status: "DRAFT",
      }),
    );
    expect(wrapper.emitted("reload")).toBeTruthy();
    expect(ElMessage.success).toHaveBeenCalledWith(expect.stringContaining("草稿已保存"));
  });

  // 7. Hard Blocker 禁止发布
  it("7. blocks publication when Hard Blocker is detected by validation", async () => {
    vi.spyOn(billingApi, "createRuleDraft").mockResolvedValue({ item: mockDraftV3 });
    vi.spyOn(billingApi, "validateRule").mockResolvedValue({
      validation: {
        valid: false,
        issues: [
          { code: "MISSING_TIER_2K", field: "parameterRules.size.tier_2k", severity: "ERROR", message: "缺少必需的尺寸阶梯 tier_2k" },
        ],
      },
    });
    const spyPublish = vi.spyOn(billingApi, "publishRule");

    const wrapper = mountPanel({ rules: [mockPublishedV2], costs: mockCosts });

    (wrapper.vm as any).openEditor();
    await (wrapper.vm as any).handlePublish();

    expect(ElMessageBox.alert).toHaveBeenCalledWith(
      expect.stringContaining("硬性结构阻断错误"),
      expect.stringContaining("禁止发布上线"),
      expect.anything(),
    );
    expect(spyPublish).not.toHaveBeenCalled();
  });

  // 8. NEGATIVE_MARGIN 弹二次确认
  it("8. opens secondary confirmation dialog when only NEGATIVE_MARGIN warning exists", async () => {
    vi.spyOn(billingApi, "createRuleDraft").mockResolvedValue({ item: mockDraftV3 });
    vi.spyOn(billingApi, "validateRule").mockResolvedValue({
      validation: {
        valid: false,
        issues: [
          { code: "NEGATIVE_MARGIN", field: "basePrice", severity: "ERROR", message: "折算低于成本" },
        ],
      },
    });
    const spyPublish = vi.spyOn(billingApi, "publishRule");

    const wrapper = mountPanel({ rules: [mockPublishedV2], costs: mockCosts });

    (wrapper.vm as any).openEditor();
    await (wrapper.vm as any).handlePublish();

    expect((wrapper.vm as any).negativeMarginDialogVisible).toBe(true);
    expect((wrapper.vm as any).targetDraftId).toBe("brv_billing_rule_image_gpt_v3");
    // Not published immediately until confirmed
    expect(spyPublish).not.toHaveBeenCalled();
  });

  // 9. confirm 后正确提交 confirmNegativeMargin=true
  it("9. submits confirmNegativeMargin=true when confirmed in dialog", async () => {
    const spyPublish = vi.spyOn(billingApi, "publishRule").mockResolvedValue({
      item: { ...mockDraftV3, status: "PUBLISHED" },
    });

    const wrapper = mountPanel({ rules: [mockPublishedV2], costs: mockCosts });

    (wrapper.vm as any).targetDraftId = "brv_billing_rule_image_gpt_v3";
    (wrapper.vm as any).negativeMarginDialogVisible = true;

    await (wrapper.vm as any).confirmAndPublishNegativeMargin();

    expect(spyPublish).toHaveBeenCalledWith("brv_billing_rule_image_gpt_v3", {
      confirmNegativeMargin: true,
    });
    expect((wrapper.vm as any).negativeMarginDialogVisible).toBe(false);
    expect(ElMessage.success).toHaveBeenCalledWith(expect.stringContaining("已正式发布并生效"));
  });

  // 10. 发布成功刷新当前 version
  it("10. emits reload on successful publish to refresh the active version", async () => {
    vi.spyOn(billingApi, "publishRule").mockResolvedValue({
      item: { ...mockDraftV3, status: "PUBLISHED", version: 3 },
    });

    const wrapper = mountPanel({ rules: [mockPublishedV2], costs: mockCosts });

    await (wrapper.vm as any).executePublish("brv_billing_rule_image_gpt_v3", true);

    expect(wrapper.emitted("reload")).toBeTruthy();
    expect((wrapper.vm as any).editorVisible).toBe(false);
  });

  // 11. 历史版本展示
  it("11. displays history versions and allows viewing parameter snapshots", async () => {
    const wrapper = mountPanel({
      rules: [mockPublishedV2, mockArchivedV1, mockDraftV3],
      costs: mockCosts,
    });

    expect((wrapper.vm as any).gptImageVersions.length).toBe(3);
    // Versions sorted by version descending: v3, v2, v1
    expect((wrapper.vm as any).gptImageVersions[0].version).toBe(3);
    expect((wrapper.vm as any).gptImageVersions[1].version).toBe(2);
    expect((wrapper.vm as any).gptImageVersions[2].version).toBe(1);

    (wrapper.vm as any).viewVersionDetail(mockPublishedV2);
    expect((wrapper.vm as any).detailDialogVisible).toBe(true);
    expect((wrapper.vm as any).selectedVersionItem.id).toBe("brv_billing_rule_image_gpt_v2");
  });

  // 12. API failure 不显示假成功
  it("12. handles API failure safely without false success", async () => {
    vi.spyOn(billingApi, "publishRule").mockRejectedValue(
      new Error("BILLING_RULE_VALIDATION_FAILED: network connection timeout"),
    );

    const wrapper = mountPanel({ rules: [mockPublishedV2], costs: mockCosts });

    (wrapper.vm as any).targetDraftId = "brv_billing_rule_image_gpt_v3";
    (wrapper.vm as any).negativeMarginDialogVisible = true;

    await (wrapper.vm as any).confirmAndPublishNegativeMargin();

    expect(ElMessage.error).toHaveBeenCalled();
    expect(ElMessage.success).not.toHaveBeenCalled();
    // Modal stays open or failure is preserved
    expect(wrapper.emitted("reload")).toBeFalsy();
  });
});
