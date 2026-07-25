import { api } from "../../api/client";
import { promotionScope, readPromotionCodeCache, readPromotionOverviewCache, writePromotionCodeCache, writePromotionOverviewCache } from "./cache";
import type { AgentInvitePoster, AgentInviteProfile, PromotionAnalytics, PromotionCode, PromotionOverview, PromotionProfile, PromotionRecordsResponse, PromotionShareCopy, PromotionTemplate, PromotionTemplateId } from "./types";

const inflight = new Map<string, Promise<unknown>>();

function once<T>(key: string, request: () => Promise<T>): Promise<T> {
  const existing = inflight.get(key) as Promise<T> | undefined;
  if (existing) return existing;
  const pending = request().finally(() => inflight.delete(key));
  inflight.set(key, pending);
  return pending;
}

export const promotionAPI = {
  async overview(userId: string, tenantId: string, currentRole: string, force = false) {
    const scope = `${promotionScope(userId, tenantId)}.${currentRole}`;
    if (!force) {
      const cached = readPromotionOverviewCache(scope);
      if (cached) return cached;
    }
    return once(`overview.${scope}`, async () => {
      const payload = await api<PromotionOverview>("/api/v1/promotion/overview");
      writePromotionOverviewCache(scope, payload);
      return payload;
    });
  },
  profile: () => api<PromotionProfile>("/api/v1/promotion/profile"),
  templates: async () => (await api<{ items: PromotionTemplate[]; total: number; defaultTemplateId: PromotionTemplateId }>("/api/v1/promotion/poster-templates")).items,
  async code(input: { userId: string; tenantId: string; currentRole: string; templateId: PromotionTemplateId; activityId?: string; invalidate?: boolean }) {
    const scope = `${promotionScope(input.userId, input.tenantId)}.${input.currentRole}`;
    if (!input.invalidate) {
      const cached = readPromotionCodeCache(scope, input.templateId, input.activityId);
      if (cached) return cached;
    }
    const key = `code.${scope}.${input.templateId}.${input.activityId || "none"}`;
    return once(key, async () => {
      const payload = await api<PromotionCode>("/api/v1/promotion/miniprogram-code", { method: "POST", body: JSON.stringify({ templateId: input.templateId, activityId: input.activityId || "", invalidate: Boolean(input.invalidate) }) });
      writePromotionCodeCache(scope, input.templateId, input.activityId || "", payload);
      return payload;
    });
  },
  records: (input: { page: number; pageSize: number; status?: string }) => api<PromotionRecordsResponse>(`/api/v1/promotion/records?page=${input.page}&pageSize=${input.pageSize}&status=${encodeURIComponent(input.status || "all")}`),
  analytics: (days = 7) => api<PromotionAnalytics>(`/api/v1/promotion/analytics?days=${days}`),
  shareCopy: (templateId: PromotionTemplateId) => api<PromotionShareCopy>(`/api/v1/promotion/share-copy?templateId=${encodeURIComponent(templateId)}`),
  renderConfig: (templateId: PromotionTemplateId, activityId = "") => api<Record<string, unknown>>("/api/v1/promotion/poster/render", { method: "POST", body: JSON.stringify({ templateId, activityId }) }),
  visit: (input: { inviteCode: string; templateId: PromotionTemplateId; activityId?: string; source?: string }) => api("/api/v1/promotion/visit", { method: "POST", body: JSON.stringify(input) }),
  bind: (input: { inviteCode: string; templateId: PromotionTemplateId; activityId?: string; source?: string }) => api("/api/v1/promotion/bind", { method: "POST", body: JSON.stringify(input) }),
  agentInviteProfile: () => api<AgentInviteProfile>("/api/v1/agent/invite/profile"),
  agentPoster: () => api<AgentInvitePoster>("/api/v1/agent/invite/poster", { method: "POST", body: JSON.stringify({}) }),
};
