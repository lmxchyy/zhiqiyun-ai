import { getAuthToken } from "../../api/client";
import { promotionAPI } from "./api";
import { trackPromotion } from "./analytics";
import { promotionTemplateById, promotionTemplateIdBySceneIndex } from "./templates";
import type { PromotionReferral, PromotionTemplateId } from "./types";

const pendingKey = "zhiqiyun.promotion.pending-referral.v1";
const inviteTokenPattern = /^inv_[aou][0-9a-f]{16}$/i;

function normalizeInviteToken(value: unknown): string {
  const token = String(value || "").trim().toLowerCase();
  return inviteTokenPattern.test(token) ? token : "";
}

function decodeScene(scene?: string) {
  const value = decodeURIComponent(String(scene || "").replace(/\+/g, "%20"));
  const result: Record<string, string> = {};
  const token = normalizeInviteToken(value);
  if (token) return { inviteToken: token };
  const compact = /^C([A-Z0-9]+)T(\d{2})(?:A([A-F0-9]{6}))?$/i.exec(value);
  if (compact) return { c: compact[1], t: String(Number(compact[2]) || 1), a: compact[3] || "" };
  value.split("&").forEach(part => {
    const [key, ...rest] = part.split("=");
    if (key) result[key] = rest.join("=");
  });
  if (!result.inviteToken) {
    const nested = normalizeInviteToken(result.inviteToken || result.token || result.scene);
    if (nested) result.inviteToken = nested;
  }
  return result;
}

export function capturePromotionReferral(query?: Record<string, unknown>) {
  if (!query) return null;
  const scene = decodeScene(String(query.scene || ""));
  const inviteToken = normalizeInviteToken(query.inviteToken || query.token || scene.inviteToken || query.scene);
  const inviteCode = String(query.invite || query.inviteCode || scene.c || scene.inviteCode || "").trim().toUpperCase();
  if (!inviteToken && !inviteCode) return null;
  const templateCandidate = String(query.templateId || "");
  const templateId = (templateCandidate ? promotionTemplateById(templateCandidate).id : promotionTemplateIdBySceneIndex(scene.t || 1)) as PromotionTemplateId;
  const referral: PromotionReferral = {
    inviteCode: inviteCode || undefined,
    inviteToken: inviteToken || undefined,
    templateId,
    activityId: String(query.activityId || scene.a || ""),
    source: String(query.source || "wechat_friend"),
    capturedAt: Date.now(),
  };
  uni.setStorageSync(pendingKey, referral);
  return referral;
}

export function pendingPromotionReferral(): PromotionReferral | null {
  try {
    const value = uni.getStorageSync(pendingKey) as PromotionReferral | undefined;
    if ((!value?.inviteCode && !value?.inviteToken) || Date.now() - (value?.capturedAt || 0) > 30 * 24 * 60 * 60 * 1000) return null;
    return value;
  } catch { return null; }
}

export async function syncPendingPromotionReferral() {
  const referral = pendingPromotionReferral();
  if (!referral || !getAuthToken()) return false;
  try {
    await promotionAPI.visit(referral);
    // 归属绑定只允许在新用户注册事务内由后端完成。这里仅记录访问，避免老用户登录后被重新绑定。
    uni.removeStorageSync(pendingKey);
    trackPromotion("promotion_referral_visit", { templateId: referral.templateId, source: referral.source });
    return true;
  } catch (error) {
    const message = error instanceof Error ? error.message.toLowerCase() : "";
    if (message.includes("invalid or expired") || message.includes("邀请链接无效") || message.includes("邀请链接已过期")) {
      uni.removeStorageSync(pendingKey);
    }
    return false;
  }
}
