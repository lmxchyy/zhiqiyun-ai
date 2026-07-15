import type { PromotionCode, PromotionOverview } from "./types";

const prefix = "zhiqiyun.promotion.v1";

function storageKey(scope: string, suffix: string) {
  return `${prefix}.${scope}.${suffix}`;
}

export function promotionScope(userId: string, tenantId: string) {
  return `${userId || "anonymous"}.${tenantId || "tenant_default"}`;
}

export function readPromotionOverviewCache(scope: string): PromotionOverview | null {
  try {
    const cached = uni.getStorageSync(storageKey(scope, "overview")) as { value?: PromotionOverview; expiresAt?: number } | undefined;
    if (!cached?.value || !cached.expiresAt || cached.expiresAt <= Date.now()) return null;
    return cached.value;
  } catch { return null; }
}

export function writePromotionOverviewCache(scope: string, value: PromotionOverview) {
  uni.setStorageSync(storageKey(scope, "overview"), { value, expiresAt: Date.now() + 30_000 });
}

export function readPromotionCodeCache(scope: string, templateId: string, activityId = ""): PromotionCode | null {
  try {
    const value = uni.getStorageSync(storageKey(scope, `code.${templateId}.${activityId || "none"}`)) as PromotionCode | undefined;
    if (!value?.imageDataUrl || !value.expiresAt || Date.parse(value.expiresAt) <= Date.now()) return null;
    return value;
  } catch { return null; }
}

export function writePromotionCodeCache(scope: string, templateId: string, activityId: string, value: PromotionCode) {
  uni.setStorageSync(storageKey(scope, `code.${templateId}.${activityId || "none"}`), value);
}

export function clearPromotionCache(scope?: string) {
  try {
    const info = uni.getStorageInfoSync();
    info.keys.filter(key => key.startsWith(scope ? `${prefix}.${scope}.` : `${prefix}.`)).forEach(key => uni.removeStorageSync(key));
  } catch { /* cache removal must never block the page */ }
}
