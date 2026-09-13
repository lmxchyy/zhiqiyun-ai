import type { BillingRuleValidationIssue, BillingRuleVersion } from "../api/billing";

export interface GptImageMultipliers {
  basePrice: number;
  minimumCharge: number;
  tier1k: number;
  tier2k: number;
  tier4k: number;
  qualityLow: number;
  qualityNormal: number;
  qualityHigh: number;
}

export interface GptImageQuoteRow {
  tierName: string;
  tierKey: string;
  sizeMultiplier: number;
  lowPoints: number;
  normalPoints: number;
  highPoints: number;
}

export interface ValidationIssueClassification {
  hardBlockers: BillingRuleValidationIssue[];
  negativeMarginWarnings: BillingRuleValidationIssue[];
  hasHardBlockers: boolean;
  hasNegativeMargin: boolean;
  canOverridePublish: boolean;
}

export function isGPTImageRule(rule?: { ruleKey?: string; legacyRuleId?: string; modelCode?: string; modelName?: string } | null): boolean {
  if (!rule) return false;
  const key = String(rule.ruleKey || "").trim();
  const legacyId = String(rule.legacyRuleId || "").trim();
  const modelCode = String(rule.modelCode || "").trim().toLowerCase();
  const modelName = String(rule.modelName || "").trim().toLowerCase();
  return (
    key === "billing_rule_image_gpt" ||
    legacyId === "billing_rule_image_gpt" ||
    modelCode === "gpt-image-2" ||
    modelCode === "gpt_image_2" ||
    modelCode === "gpt-image" ||
    modelName === "gpt-image-2"
  );
}

export function extractGptImageMultipliers(rule?: BillingRuleVersion | null): GptImageMultipliers {
  const defaultValues: GptImageMultipliers = {
    basePrice: 10,
    minimumCharge: 1,
    tier1k: 1.0,
    tier2k: 1.5,
    tier4k: 2.0,
    qualityLow: 1.0,
    qualityNormal: 1.2,
    qualityHigh: 1.5,
  };

  if (!rule) return defaultValues;

  const basePrice = Number(rule.basePrice);
  const minimumCharge = Number(rule.minimumCharge);
  const paramRules = (rule.parameterRules || {}) as Record<string, unknown>;

  const sizeMap = (paramRules.size || {}) as Record<string, unknown>;
  const qualityMap = (paramRules.quality || {}) as Record<string, unknown>;

  const tier1k = Number(sizeMap.tier_1k ?? defaultValues.tier1k);
  const tier2k = Number(sizeMap.tier_2k ?? defaultValues.tier2k);
  const tier4k = Number(sizeMap.tier_4k ?? defaultValues.tier4k);

  const qualityLow = Number(qualityMap.low ?? defaultValues.qualityLow);
  const qualityNormal = Number(qualityMap.normal ?? qualityMap.medium ?? defaultValues.qualityNormal);
  const qualityHigh = Number(qualityMap.high ?? defaultValues.qualityHigh);

  return {
    basePrice: Number.isFinite(basePrice) && basePrice > 0 ? basePrice : defaultValues.basePrice,
    minimumCharge: Number.isFinite(minimumCharge) && minimumCharge >= 0 ? minimumCharge : defaultValues.minimumCharge,
    tier1k: Number.isFinite(tier1k) && tier1k > 0 ? tier1k : defaultValues.tier1k,
    tier2k: Number.isFinite(tier2k) && tier2k > 0 ? tier2k : defaultValues.tier2k,
    tier4k: Number.isFinite(tier4k) && tier4k > 0 ? tier4k : defaultValues.tier4k,
    qualityLow: Number.isFinite(qualityLow) && qualityLow > 0 ? qualityLow : defaultValues.qualityLow,
    qualityNormal: Number.isFinite(qualityNormal) && qualityNormal > 0 ? qualityNormal : defaultValues.qualityNormal,
    qualityHigh: Number.isFinite(qualityHigh) && qualityHigh > 0 ? qualityHigh : defaultValues.qualityHigh,
  };
}

export function buildGptImageParameterRules(multipliers: GptImageMultipliers): Record<string, unknown> {
  const t1 = multipliers.tier1k > 0 ? multipliers.tier1k : 1.0;
  const t2 = multipliers.tier2k > 0 ? multipliers.tier2k : 1.5;
  const t4 = multipliers.tier4k > 0 ? multipliers.tier4k : 2.0;

  const qLow = multipliers.qualityLow > 0 ? multipliers.qualityLow : 1.0;
  const qNormal = multipliers.qualityNormal > 0 ? multipliers.qualityNormal : 1.2;
  const qHigh = multipliers.qualityHigh > 0 ? multipliers.qualityHigh : 1.5;

  return {
    quality: {
      auto: qLow,
      low: qLow,
      normal: qNormal,
      medium: qNormal,
      high: qHigh,
      standard: qLow,
    },
    size: {
      auto: t1,
      tier_720p: t1,
      tier_1k: t1,
      tier_2k: t2,
      tier_4k: t4,
      "1024x1024": t1,
      "1024x1536": t1,
      "1536x1024": t1,
      "1280x720": t1,
      "720x1280": t1,
      "2048x2048": t2,
      "2048x1152": t2,
      "3840x2160": t4,
      "2160x3840": t4,
    },
  };
}

export function calculateGptImageQuotePreview(
  basePrice: number,
  multipliers: Pick<GptImageMultipliers, "tier1k" | "tier2k" | "tier4k" | "qualityLow" | "qualityNormal" | "qualityHigh">,
): GptImageQuoteRow[] {
  const base = Math.max(1, Number(basePrice) || 1);
  const tiers = [
    { name: "1K 基础尺寸 (1024x1024等)", key: "tier_1k", mult: multipliers.tier1k || 1.0 },
    { name: "2K 高清尺寸 (2048x2048等)", key: "tier_2k", mult: multipliers.tier2k || 1.5 },
    { name: "4K 超清尺寸 (3840x2160等)", key: "tier_4k", mult: multipliers.tier4k || 2.0 },
  ];

  const qLow = multipliers.qualityLow || 1.0;
  const qNormal = multipliers.qualityNormal || 1.2;
  const qHigh = multipliers.qualityHigh || 1.5;

  return tiers.map((tier) => ({
    tierName: tier.name,
    tierKey: tier.key,
    sizeMultiplier: tier.mult,
    lowPoints: Math.ceil(base * tier.mult * qLow),
    normalPoints: Math.ceil(base * tier.mult * qNormal),
    highPoints: Math.ceil(base * tier.mult * qHigh),
  }));
}

export function classifyValidationIssues(issues?: BillingRuleValidationIssue[]): ValidationIssueClassification {
  const safeIssues = Array.isArray(issues) ? issues : [];
  const hardBlockers: BillingRuleValidationIssue[] = [];
  const negativeMarginWarnings: BillingRuleValidationIssue[] = [];

  for (const issue of safeIssues) {
    const severity = String(issue.severity || "").toUpperCase();
    const code = String(issue.code || "").toUpperCase();
    if (severity === "ERROR") {
      if (code === "NEGATIVE_MARGIN") {
        negativeMarginWarnings.push(issue);
      } else {
        hardBlockers.push(issue);
      }
    }
  }

  const hasHardBlockers = hardBlockers.length > 0;
  const hasNegativeMargin = negativeMarginWarnings.length > 0;

  return {
    hardBlockers,
    negativeMarginWarnings,
    hasHardBlockers,
    hasNegativeMargin,
    canOverridePublish: !hasHardBlockers && hasNegativeMargin,
  };
}

export function formatBillingErrorMessage(error: unknown): string {
  if (!error) return "未知错误";

  const errObj = error as {
    code?: string;
    status?: number;
    message?: string;
    payload?: { code?: string; message?: string; error?: string };
  };

  const code = errObj.code || errObj.payload?.code || "";
  const status = errObj.status || 0;
  const rawMsg = errObj.payload?.message || errObj.payload?.error || errObj.message || "";

  if (code === "BILLING_RULE_VALIDATION_FAILED") {
    return "规则校验未通过：存在硬性阻断错误（如售价或倍率非正数、缺少关键尺寸阶梯等），无法发布。";
  }
  if (code === "NEGATIVE_MARGIN_CONFIRMATION_REQUIRED") {
    return "负毛利风险拦截：该版本折算售价低于供应商成本，必须显式确认负毛利商业风险后方可发布。";
  }
  if (code === "INVALID_BILLING_RULE_STATUS") {
    return "规则状态无效：只有草稿 (DRAFT) 状态的计费版本才允许发布。";
  }
  if (status === 401) {
    return "登录状态已失效，请重新登录。";
  }
  if (status === 403) {
    return "暂无权限执行此计费规则修改或发布操作，请联系系统管理员。";
  }

  const lower = String(rawMsg).toLowerCase();
  if (lower.includes("network error") || lower.includes("failed to fetch") || lower.includes("econnrefused")) {
    return "网络连接失败，未能连接到计费服务，请检查网络。";
  }
  if (lower.includes("timeout") || lower.includes("timed out")) {
    return "计费服务请求超时，请稍后重试。";
  }

  return rawMsg || "计费操作处理失败，请稍后重试。";
}
