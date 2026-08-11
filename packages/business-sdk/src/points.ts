import type { ApiClient } from "@xianzhi/api-client";
import type {
  AdminPointCorrectionResponse,
  AdminPointGiftResponse,
  PersonalPointLotsResponse,
  PointAccountResponse,
  PointExpiryPolicyResponse
} from "@xianzhi/shared-types";
import type { BusinessSdk } from "./types";

export function createPointsSdk(api: ApiClient): BusinessSdk["points"] {
  return {
    account: () => api.request<PointAccountResponse>("/api/v1/member/wallet"),
    expirySummary: () => api.request<PointAccountResponse>("/api/v1/points/account"),
    adminPolicy: () => api.request<PointExpiryPolicyResponse>("/api/v1/admin/points/expiry-policy"),
    updateAdminPolicy: (input) => api.request<PointExpiryPolicyResponse>("/api/v1/admin/points/expiry-policy", { method: "PUT", body: input }),
    adminLots: (userId, options = {}) => {
      const query = new URLSearchParams();
      if (options.source) query.set("source", options.source);
      if (options.status) query.set("status", options.status);
      if (options.limit !== undefined) query.set("limit", String(options.limit));
      if (options.offset !== undefined) query.set("offset", String(options.offset));
      const suffix = query.toString();
      return api.request<PersonalPointLotsResponse>(`/api/v1/admin/customers/${encodeURIComponent(userId)}/point-lots${suffix ? `?${suffix}` : ""}`);
    },
    grantAdminGift: (userId, input) => api.request<AdminPointGiftResponse>(`/api/v1/admin/customers/${encodeURIComponent(userId)}/point-gifts`, { method: "POST", body: input }),
    correctAdminBalance: (userId, input) => api.request<AdminPointCorrectionResponse>(`/api/v1/admin/customers/${encodeURIComponent(userId)}/point-corrections`, { method: "POST", body: input })
  };
}
