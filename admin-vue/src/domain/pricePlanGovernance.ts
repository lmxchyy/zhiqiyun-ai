import type {
  BusinessPlan,
  PricingHealthBusinessPlan,
  PricingHealthDefaultSummary,
  PricingHealthSummary
} from "../types/pricePlanAdmin.ts";

export interface AdminModulePrincipal {
  role: string;
  permissions: readonly string[];
}

export interface AuthorizedAdminModuleInput {
  requestedModuleId: string;
  fallbackModuleId: string;
  allowedModuleIds: readonly string[];
  modulePermissions: Record<string, string>;
  principal: AdminModulePrincipal;
}

export interface BusinessPlanFilters {
  keyword?: string;
  businessType?: "ALL" | "MEMBER" | "AGENT";
  status?: "ALL" | "HEALTHY" | "DEGRADED" | "BLOCKED";
}

export interface BusinessPlanListRow extends BusinessPlan {
  codeReadOnly: true;
  activeVersionId: string;
  pricePlanCount: number;
  healthStatus: string;
  issueCodes: string[];
  productionDefault: PricingHealthDefaultSummary | null;
  sandboxDefault: PricingHealthDefaultSummary | null;
}

export type PricingHealthCardTone = "primary" | "success" | "info" | "danger";

export interface PricingHealthCard {
  key: "businessPlans" | "pricePlans" | "wechatGoods" | "issues";
  label: string;
  value: number;
  tone: PricingHealthCardTone;
  detail: string;
}

export type BusinessPlanListDisplayState = "LOADING" | "ERROR" | "TABLE" | "EMPTY";

export interface PricingHealthDisplayState {
  showCards: boolean;
  showError: boolean;
  stale: boolean;
}

export interface LegacyPlanSaveContext {
  saveSequence: number;
  currentSequence: number;
  dialogOpen: boolean;
  expectedPlanId: string;
  currentPlanId: string;
  gate: LegacyPlanEditorGate;
}

export const PRICE_PLAN_DETAIL_TABS = [
  { id: "basic", label: "基本信息", ready: true },
  { id: "entitlements", label: "权益版本", ready: true },
  { id: "pricePlans", label: "价格方案", ready: true },
  { id: "wechatGoods", label: "微信商品", ready: true },
  { id: "testWhitelist", label: "测试白名单", ready: true },
  { id: "audit", label: "审计日志", ready: true }
] as const;

export type LegacyPlanEditorGateStatus = "CHECKING" | "LEGACY" | "MANAGED" | "BLOCKED";

export interface LegacyPlanEditorGate {
  status: LegacyPlanEditorGateStatus;
  planId: string;
  message: string;
  managedPlan?: Record<string, unknown>;
}

type BusinessPlanLookup = (planId: string) => Promise<unknown>;

export function canAccessAdminModule(principal: AdminModulePrincipal, permission: string) {
  if (!permission) return true;
  const role = String(principal.role || "").trim().toUpperCase();
  if (role === "SUPER_ADMIN") return true;
  const permissions = Array.isArray(principal.permissions) ? principal.permissions : [];
  if (permission.startsWith("pricing:") || permission.startsWith("points:")) return permissions.includes(permission);
  return permissions.includes(permission) || permissions.includes("admin.full");
}

export function resolveAuthorizedAdminModule(input: AuthorizedAdminModuleInput) {
  const allowed = new Set(input.allowedModuleIds);
  const accessible = (moduleId: string) => allowed.has(moduleId)
    && canAccessAdminModule(input.principal, input.modulePermissions[moduleId] || "");
  if (accessible(input.requestedModuleId)) return input.requestedModuleId;
  return accessible(input.fallbackModuleId) ? input.fallbackModuleId : "";
}

export function filterBusinessPlanRows(
  plans: readonly BusinessPlan[],
  healthPlans: readonly PricingHealthBusinessPlan[],
  filters: BusinessPlanFilters = {}
): BusinessPlanListRow[] {
  const keyword = String(filters.keyword || "").trim().toLowerCase();
  const businessType = String(filters.businessType || "ALL").toUpperCase();
  const status = String(filters.status || "ALL").toUpperCase();
  const healthByPlanId = new Map(healthPlans.map((item) => [item.planId, item]));

  return plans
    .filter((plan) => plan.businessType === "MEMBER" || plan.businessType === "AGENT")
    .map((plan) => {
      const health = healthByPlanId.get(plan.id);
      return {
        ...plan,
        codeReadOnly: true as const,
        activeVersionId: health?.activeVersionId || plan.activeVersionId || "",
        pricePlanCount: Number(health?.pricePlanCount || 0),
        healthStatus: health?.status || "UNKNOWN",
        issueCodes: Array.isArray(health?.issueCodes) ? [...health.issueCodes] : [],
        productionDefault: health?.defaults.production || null,
        sandboxDefault: health?.defaults.sandbox || null
      };
    })
    .filter((plan) => businessType === "ALL" || plan.businessType === businessType)
    .filter((plan) => status === "ALL" || plan.healthStatus === status)
    .filter((plan) => !keyword || [plan.id, plan.code, plan.name, plan.businessType]
      .some((value) => String(value || "").toLowerCase().includes(keyword)));
}

export function businessPlanListDisplayState(input: { initialLoading: boolean; error?: string; rowCount: number }): BusinessPlanListDisplayState {
  if (input.initialLoading && input.rowCount === 0) return "LOADING";
  if (input.rowCount > 0) return "TABLE";
  if (String(input.error || "").trim()) return "ERROR";
  return "EMPTY";
}

export function buildPricingHealthCards(summary: PricingHealthSummary): PricingHealthCard[] {
  return [
    { key: "businessPlans", label: "业务套餐", value: Number(summary.businessPlanCount || 0), tone: "primary", detail: "仅统计 V2 会员与代理商套餐" },
    { key: "pricePlans", label: "价格方案", value: Number(summary.pricePlanCount || 0), tone: "success", detail: "由服务端价格方案汇总" },
    { key: "wechatGoods", label: "微信商品", value: Number(summary.wechatGoodCount || 0), tone: "info", detail: "本地微信虚拟商品记录" },
    { key: "issues", label: "健康问题", value: Number(summary.issueCount || 0), tone: "danger", detail: `阻断 ${Number(summary.blockedIssueCount || 0)} · 关注 ${Number(summary.degradedIssueCount || 0)}` }
  ];
}

export function pricingHealthDisplayState(input: { hasCachedHealth: boolean; error?: string }): PricingHealthDisplayState {
  const showError = Boolean(String(input.error || "").trim());
  return {
    showCards: input.hasCachedHealth,
    showError,
    stale: input.hasCachedHealth && showError
  };
}

export async function resolveLegacyPlanEditorGate(planId: string, lookup: BusinessPlanLookup): Promise<LegacyPlanEditorGate> {
  const normalizedPlanId = String(planId || "").trim();
  if (!normalizedPlanId) return blockedLegacyPlanGate("", "套餐 ID 缺失，无法确认是否由 V2 托管。");
  try {
    const response = await lookup(normalizedPlanId);
    if (!response || typeof response !== "object" || Array.isArray(response)) {
      return blockedLegacyPlanGate(normalizedPlanId, "服务端返回异常，无法确认套餐托管状态。");
    }
    const item = (response as { item?: unknown }).item;
    if (!item || typeof item !== "object" || Array.isArray(item)) {
      return blockedLegacyPlanGate(normalizedPlanId, "服务端未返回有效套餐信息，旧编辑入口已阻断。");
    }
    const plan = item as Record<string, unknown>;
    if (String(plan.id || "").trim() !== normalizedPlanId) {
      return blockedLegacyPlanGate(normalizedPlanId, "服务端套餐标识不一致，旧编辑入口已阻断。");
    }
    const businessType = String(plan.businessType || "").trim().toUpperCase();
    if (businessType === "MEMBER" || businessType === "AGENT") {
      return {
        status: "MANAGED",
        planId: normalizedPlanId,
        message: "该套餐已由 V2 权益版本和价格方案托管，旧编辑器不会加载或保存价格与权益。",
        managedPlan: plan
      };
    }
    if (businessType === "OPERATION_CENTER_PACKAGE" || businessType === "TOKEN_RECHARGE") {
      return { status: "LEGACY", planId: normalizedPlanId, message: "该套餐不属于本阶段 V2 会员或代理商托管范围。" };
    }
    return blockedLegacyPlanGate(normalizedPlanId, "套餐业务类型未知，无法证明旧编辑入口安全。");
  } catch (error) {
    const record = error && typeof error === "object" ? error as Record<string, unknown> : {};
    const status = Number(record.status || 0);
    const code = String(record.code || "").trim();
    if (status === 404 && code === "BUSINESS_PLAN_NOT_FOUND") {
      return { status: "LEGACY", planId: normalizedPlanId, message: "服务端确认该套餐未由 V2 会员或代理商套餐托管。" };
    }
    return blockedLegacyPlanGate(normalizedPlanId, "无法可靠确认套餐托管状态，旧编辑入口已安全阻断，请检查权限或稍后重试。");
  }
}

export function legacyPlanEditorAllowsIO(status: LegacyPlanEditorGateStatus) {
  return status === "LEGACY";
}

export async function revalidateLegacyPlanEditorForSave(
  currentGate: LegacyPlanEditorGate,
  planId: string,
  lookup: BusinessPlanLookup
): Promise<LegacyPlanEditorGate> {
  const normalizedPlanId = String(planId || "").trim();
  if (!legacyPlanEditorAllowsIO(currentGate.status) || currentGate.planId !== normalizedPlanId) {
    return blockedLegacyPlanGate(normalizedPlanId, "旧编辑入口状态已变化，保存前安全复核已阻断。");
  }
  return resolveLegacyPlanEditorGate(normalizedPlanId, lookup);
}

export function legacyPlanSaveContextIsCurrent(input: LegacyPlanSaveContext) {
  const expectedPlanId = String(input.expectedPlanId || "").trim();
  return input.saveSequence === input.currentSequence
    && input.dialogOpen
    && Boolean(expectedPlanId)
    && String(input.currentPlanId || "").trim() === expectedPlanId
    && input.gate.planId === expectedPlanId
    && legacyPlanEditorAllowsIO(input.gate.status);
}

export function managedPlanHandoff(gate: LegacyPlanEditorGate) {
  return gate.status === "MANAGED"
    ? { moduleId: "pricePlanGovernance", planId: gate.planId }
    : null;
}

function blockedLegacyPlanGate(planId: string, message: string): LegacyPlanEditorGate {
  return { status: "BLOCKED", planId, message };
}
