import { AdminApiError, adminRequest } from "./client.ts";
import {
  buildPaymentBindingCreatePayload,
  buildPaymentBindingRebindPayload,
  buildPaymentBindingTransitionPayload,
  buildPlanVersionCreatePayload,
  buildPlanVersionPayload,
  buildPricePlanClonePayload,
  buildPricePlanCreatePayload,
  buildPricePlanTransitionPayload,
  buildPricePlanUpdatePayload,
  buildVersionTransitionPayload,
  buildWechatGoodConfirmationPayload,
  buildWechatGoodCreatePayload,
  buildWechatGoodDisablePayload,
  buildWechatGoodUpdatePayload,
  buildWhitelistCreatePayload,
  buildWhitelistDisablePayload,
  buildWhitelistUpdatePayload
} from "../domain/pricePlanAdmin.ts";
import type {
  BusinessPlan,
  ItemResponse,
  ListResponse,
  PaymentBindingCreateInput,
  PaymentBindingRebindInput,
  PaymentBindingTransitionInput,
  PlanVersion,
  PlanVersionCreateInput,
  PlanVersionUpdateInput,
  PricePlan,
  PricePlanCloneInput,
  PricePlanCreateInput,
  PricePlanDefaultResponse,
  PricePlanPaymentBinding,
  PricePlanValidation,
  PricePlanUpdateInput,
  PricePlanWhitelistEntry,
  PricingAuditFilters,
  PricingAuditPage,
  PricingHealth,
  RevisionReasonInput,
  WechatVirtualGood,
  WechatGoodReferencesResponse,
  WechatVirtualGoodConfirmationInput,
  WechatVirtualGoodConfirmationResponse,
  WechatVirtualGoodCreateInput,
  WechatVirtualGoodsResponse,
  WechatVirtualGoodUpdateInput,
  WhitelistCreateInput,
  WhitelistDisableResponse,
  WhitelistFilters,
  WhitelistPage,
  WhitelistUpdateInput
} from "../types/pricePlanAdmin.ts";

function pathID(value: string): string {
  return encodeURIComponent(String(value || "").trim());
}

const PRICING_AUDIT_STRING_FILTERS = [
  "planId",
  "planVersionId",
  "pricePlanId",
  "wechatGoodId",
  "bindingId",
  "whitelistEntryId",
  "action",
  "operatorId",
  "operatorRole"
] as const;
const PRICING_AUDIT_QUERY_KEYS = [
  ...PRICING_AUDIT_STRING_FILTERS,
  "startTime",
  "endTime",
  "result",
  "page",
  "pageSize"
] as const;
const PRICING_AUDIT_MAX_PAGE = 1_000_000;
const RFC3339_WITH_OFFSET = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.\d{1,9})?(?:Z|([+-])(\d{2}):(\d{2}))$/;

function pricingAuditQueryError(code: string, message: string): AdminApiError {
  return new AdminApiError(message, 400, code, { code });
}

function isValidPricingAuditRFC3339(value: string): boolean {
  const match = RFC3339_WITH_OFFSET.exec(value);
  if (!match) return false;
  const [, yearText, monthText, dayText, hourText, minuteText, secondText, , offsetHourText, offsetMinuteText] = match;
  const year = Number(yearText);
  const month = Number(monthText);
  const day = Number(dayText);
  const hour = Number(hourText);
  const minute = Number(minuteText);
  const second = Number(secondText);
  const leapYear = year % 4 === 0 && (year % 100 !== 0 || year % 400 === 0);
  const daysInMonth = [31, leapYear ? 29 : 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31];
  if (month < 1 || month > 12 || day < 1 || day > daysInMonth[month - 1]
    || hour > 23 || minute > 59 || second > 59) return false;
  if (offsetHourText !== undefined
    && (Number(offsetHourText) > 23 || Number(offsetMinuteText) > 59)) return false;
  return Number.isFinite(Date.parse(value));
}

export function normalizePricingAuditFilters(input: PricingAuditFilters | Record<string, unknown> = {}): PricingAuditFilters {
  const source = input as Record<string, unknown>;
  const normalized: PricingAuditFilters = {};
  for (const key of PRICING_AUDIT_STRING_FILTERS) {
    const value = typeof source[key] === "string" ? String(source[key]).trim() : "";
    if (value.length > 256) {
      throw pricingAuditQueryError("PRICING_AUDIT_FILTER_INVALID", "pricing audit filter exceeds the maximum length");
    }
    if (value) normalized[key] = value;
  }

  for (const key of ["startTime", "endTime"] as const) {
    const value = typeof source[key] === "string" ? String(source[key]).trim() : "";
    if (!value) continue;
    if (!isValidPricingAuditRFC3339(value)) {
      throw pricingAuditQueryError("PRICING_AUDIT_TIME_INVALID", "pricing audit time must use RFC3339 with an offset");
    }
    normalized[key] = value;
  }
  if (normalized.startTime && normalized.endTime
    && Date.parse(normalized.startTime) > Date.parse(normalized.endTime)) {
    throw pricingAuditQueryError("PRICING_AUDIT_TIME_RANGE_INVALID", "pricing audit endTime must not be before startTime");
  }

  const rawResult = typeof source.result === "string" ? source.result.trim().toUpperCase() : "";
  if (rawResult && rawResult !== "SUCCEEDED" && rawResult !== "FAILED") {
    throw pricingAuditQueryError("PRICING_AUDIT_RESULT_INVALID", "pricing audit result must be SUCCEEDED or FAILED");
  }
  if (rawResult === "SUCCEEDED" || rawResult === "FAILED") normalized.result = rawResult;

  const page = source.page === undefined || source.page === null || source.page === "" ? 1 : Number(source.page);
  if (!Number.isInteger(page) || page < 1 || page > PRICING_AUDIT_MAX_PAGE) {
    throw pricingAuditQueryError("PRICING_AUDIT_PAGE_INVALID", "pricing audit page must be within the supported range");
  }
  const pageSize = source.pageSize === undefined || source.pageSize === null || source.pageSize === "" ? 50 : Number(source.pageSize);
  if (!Number.isInteger(pageSize) || pageSize < 1 || pageSize > 200) {
    throw pricingAuditQueryError("PRICING_AUDIT_PAGE_SIZE_INVALID", "pricing audit pageSize must be between 1 and 200");
  }
  normalized.page = page;
  normalized.pageSize = pageSize;
  return normalized;
}

export function buildPricingAuditQuery(filters: PricingAuditFilters | Record<string, unknown> = {}): string {
  const normalized = normalizePricingAuditFilters(filters);
  const query = new URLSearchParams();
  for (const key of PRICING_AUDIT_QUERY_KEYS) {
    const value = normalized[key];
    if (value !== undefined && value !== null && value !== "") query.set(key, String(value));
  }
  return `?${query.toString()}`;
}

export function buildWhitelistQuery(filters: WhitelistFilters = {}): string {
  const query = new URLSearchParams();
  const status = String(filters.status || "").trim();
  const userId = String(filters.userId || "").trim();
  if (status) query.set("status", status);
  if (userId) query.set("userId", userId);
  query.set("page", String(filters.page ?? 1));
  query.set("pageSize", String(filters.pageSize ?? 50));
  const serialized = query.toString();
  return serialized ? `?${serialized}` : "";
}

export const pricePlanAdminApi = {
  listBusinessPlans() {
    return adminRequest<ListResponse<BusinessPlan>>({ method: "GET", url: "/admin/business-plans" });
  },
  getBusinessPlan(planId: string) {
    return adminRequest<ItemResponse<BusinessPlan>>({ method: "GET", url: `/admin/business-plans/${pathID(planId)}` });
  },
  listPlanVersions(planId: string) {
    return adminRequest<ListResponse<PlanVersion>>({ method: "GET", url: `/admin/business-plans/${pathID(planId)}/versions` });
  },
  createPlanVersion(planId: string, input: PlanVersionCreateInput) {
    return adminRequest<ItemResponse<PlanVersion>>({
      method: "POST",
      url: `/admin/business-plans/${pathID(planId)}/versions`,
      data: buildPlanVersionCreatePayload(input as unknown as Record<string, unknown>)
    });
  },
  updatePlanVersion(versionId: string, input: PlanVersionUpdateInput) {
    return adminRequest<ItemResponse<PlanVersion>>({
      method: "PATCH",
      url: `/admin/plan-versions/${pathID(versionId)}`,
      data: buildPlanVersionPayload(input as unknown as Record<string, unknown>)
    });
  },
  activatePlanVersion(versionId: string, input: RevisionReasonInput) {
    return adminRequest<ItemResponse<PlanVersion>>({
      method: "POST",
      url: `/admin/plan-versions/${pathID(versionId)}/activate`,
      data: buildVersionTransitionPayload(input as unknown as Record<string, unknown>)
    });
  },
  retirePlanVersion(versionId: string, input: RevisionReasonInput) {
    return adminRequest<ItemResponse<PlanVersion>>({
      method: "POST",
      url: `/admin/plan-versions/${pathID(versionId)}/retire`,
      data: buildVersionTransitionPayload(input as unknown as Record<string, unknown>)
    });
  },
  listPricePlans(planId: string) {
    return adminRequest<ListResponse<PricePlan>>({ method: "GET", url: `/admin/business-plans/${pathID(planId)}/price-plans` });
  },
  createPricePlan(planId: string, input: PricePlanCreateInput) {
    return adminRequest<ItemResponse<PricePlan>>({
      method: "POST",
      url: `/admin/business-plans/${pathID(planId)}/price-plans`,
      data: buildPricePlanCreatePayload(input as unknown as Record<string, unknown>)
    });
  },
  getPricePlan(pricePlanId: string) {
    return adminRequest<ItemResponse<PricePlan>>({ method: "GET", url: `/admin/price-plans/${pathID(pricePlanId)}` });
  },
  updatePricePlan(pricePlanId: string, input: PricePlanUpdateInput) {
    return adminRequest<ItemResponse<PricePlan>>({
      method: "PATCH",
      url: `/admin/price-plans/${pathID(pricePlanId)}`,
      data: buildPricePlanUpdatePayload(input as unknown as Record<string, unknown>)
    });
  },
  clonePricePlan(pricePlanId: string, input: PricePlanCloneInput) {
    return adminRequest<ItemResponse<PricePlan>>({
      method: "POST",
      url: `/admin/price-plans/${pathID(pricePlanId)}/clone`,
      data: buildPricePlanClonePayload(input as unknown as Record<string, unknown>)
    });
  },
  validatePricePlan(pricePlanId: string) {
    return adminRequest<PricePlanValidation>({ method: "GET", url: `/admin/price-plans/${pathID(pricePlanId)}/validation` });
  },
  enablePricePlan(pricePlanId: string, input: RevisionReasonInput) {
    return adminRequest<ItemResponse<PricePlan>>({
      method: "POST",
      url: `/admin/price-plans/${pathID(pricePlanId)}/enable`,
      data: buildPricePlanTransitionPayload(input as unknown as Record<string, unknown>)
    });
  },
  disablePricePlan(pricePlanId: string, input: RevisionReasonInput) {
    return adminRequest<ItemResponse<PricePlan>>({
      method: "POST",
      url: `/admin/price-plans/${pathID(pricePlanId)}/disable`,
      data: buildPricePlanTransitionPayload(input as unknown as Record<string, unknown>)
    });
  },
  makeDefaultPricePlan(pricePlanId: string, input: RevisionReasonInput) {
    return adminRequest<PricePlanDefaultResponse>({
      method: "POST",
      url: `/admin/price-plans/${pathID(pricePlanId)}/make-default`,
      data: buildPricePlanTransitionPayload(input as unknown as Record<string, unknown>)
    });
  },
  listWechatVirtualGoods() {
    return adminRequest<WechatVirtualGoodsResponse>({ method: "GET", url: "/admin/wechat-virtual-goods" });
  },
  getWechatVirtualGood(goodId: string) {
    return adminRequest<ItemResponse<WechatVirtualGood>>({ method: "GET", url: `/admin/wechat-virtual-goods/${pathID(goodId)}` });
  },
  listWechatVirtualGoodReferences(goodId: string) {
    return adminRequest<WechatGoodReferencesResponse>({ method: "GET", url: `/admin/wechat-virtual-goods/${pathID(goodId)}/references` });
  },
  createWechatVirtualGood(input: WechatVirtualGoodCreateInput) {
    return adminRequest<ItemResponse<WechatVirtualGood>>({
      method: "POST",
      url: "/admin/wechat-virtual-goods",
      data: buildWechatGoodCreatePayload(input as unknown as Record<string, unknown>)
    });
  },
  updateWechatVirtualGood(goodId: string, input: WechatVirtualGoodUpdateInput) {
    return adminRequest<ItemResponse<WechatVirtualGood>>({
      method: "PATCH",
      url: `/admin/wechat-virtual-goods/${pathID(goodId)}`,
      data: buildWechatGoodUpdatePayload(input as unknown as Record<string, unknown>)
    });
  },
  confirmWechatVirtualGood(goodId: string, input: WechatVirtualGoodConfirmationInput) {
    return adminRequest<WechatVirtualGoodConfirmationResponse>({
      method: "POST",
      url: `/admin/wechat-virtual-goods/${pathID(goodId)}/confirm-published`,
      data: buildWechatGoodConfirmationPayload(input as unknown as Record<string, unknown>)
    });
  },
  disableWechatVirtualGood(goodId: string, input: RevisionReasonInput) {
    return adminRequest<ItemResponse<WechatVirtualGood>>({
      method: "POST",
      url: `/admin/wechat-virtual-goods/${pathID(goodId)}/disable`,
      data: buildWechatGoodDisablePayload(input as unknown as Record<string, unknown>)
    });
  },
  listPaymentBindings(pricePlanId: string) {
    return adminRequest<ListResponse<PricePlanPaymentBinding>>({ method: "GET", url: `/admin/price-plans/${pathID(pricePlanId)}/payment-bindings` });
  },
  createPaymentBinding(pricePlanId: string, input: PaymentBindingCreateInput) {
    return adminRequest<ItemResponse<PricePlanPaymentBinding>>({
      method: "POST",
      url: `/admin/price-plans/${pathID(pricePlanId)}/payment-bindings`,
      data: buildPaymentBindingCreatePayload(input as unknown as Record<string, unknown>)
    });
  },
  rebindPaymentBinding(bindingId: string, input: PaymentBindingRebindInput) {
    return adminRequest<ItemResponse<PricePlanPaymentBinding>>({
      method: "PATCH",
      url: `/admin/payment-bindings/${pathID(bindingId)}`,
      data: buildPaymentBindingRebindPayload(input as unknown as Record<string, unknown>)
    });
  },
  transitionPaymentBinding(bindingId: string, input: PaymentBindingTransitionInput) {
    return adminRequest<ItemResponse<PricePlanPaymentBinding>>({
      method: "PATCH",
      url: `/admin/payment-bindings/${pathID(bindingId)}`,
      data: buildPaymentBindingTransitionPayload(input as unknown as Record<string, unknown>)
    });
  },
  listWhitelist(pricePlanId: string, filters: WhitelistFilters = {}) {
    return adminRequest<WhitelistPage>({
      method: "GET",
      url: `/admin/price-plans/${pathID(pricePlanId)}/whitelist${buildWhitelistQuery(filters)}`
    });
  },
  createWhitelistEntry(pricePlanId: string, input: WhitelistCreateInput) {
    return adminRequest<ItemResponse<PricePlanWhitelistEntry>>({
      method: "POST",
      url: `/admin/price-plans/${pathID(pricePlanId)}/whitelist`,
      data: buildWhitelistCreatePayload(input as unknown as Record<string, unknown>)
    });
  },
  updateWhitelistEntry(pricePlanId: string, entryId: string, input: WhitelistUpdateInput) {
    return adminRequest<ItemResponse<PricePlanWhitelistEntry>>({
      method: "PATCH",
      url: `/admin/price-plans/${pathID(pricePlanId)}/whitelist/${pathID(entryId)}`,
      data: buildWhitelistUpdatePayload(input as unknown as Record<string, unknown>)
    });
  },
  disableWhitelistEntry(pricePlanId: string, entryId: string, input: RevisionReasonInput) {
    return adminRequest<WhitelistDisableResponse>({
      method: "POST",
      url: `/admin/price-plans/${pathID(pricePlanId)}/whitelist/${pathID(entryId)}/disable`,
      data: buildWhitelistDisablePayload(input as unknown as Record<string, unknown>)
    });
  },
  listPricingAuditLogs(filters: PricingAuditFilters = {}) {
    return adminRequest<PricingAuditPage>({ method: "GET", url: `/admin/pricing-audit-logs${buildPricingAuditQuery(filters)}` });
  },
  getPricingHealth() {
    return adminRequest<PricingHealth>({ method: "GET", url: "/admin/pricing-health" });
  }
};
