import type {
  AdminPointCorrectionResponse,
  AdminPointGiftResponse,
  AdminPointMutationRequest,
  PersonalPointLotsResponse,
  PointExpiryPolicyResponse,
  PointLotFilters,
  UpdatePointExpiryPolicyRequest
} from "../types/personalPointsAdmin.ts";

export const personalPointsAdminApi = {
  getPolicy() {
    return adminRequest<PointExpiryPolicyResponse>({ method: "GET", url: "/admin/points/expiry-policy" });
  },
  updatePolicy(input: UpdatePointExpiryPolicyRequest) {
    return adminRequest<PointExpiryPolicyResponse>({ method: "PUT", url: "/admin/points/expiry-policy", data: buildPolicyMutationPayload(input), retryOnUnauthorized: false });
  },
  listLots(userId: string, filters: PointLotFilters = {}) {
    const params: PointLotFilters = {};
    const source = String(filters.source || "").trim();
    const status = String(filters.status || "").trim();
    if (source) params.source = source;
    if (status) params.status = status;
    if (Number.isSafeInteger(filters.limit) && Number(filters.limit) >= 0) params.limit = Number(filters.limit);
    if (Number.isSafeInteger(filters.offset) && Number(filters.offset) >= 0) params.offset = Number(filters.offset);
    return adminRequest<PersonalPointLotsResponse>({ method: "GET", url: customerPointPath(userId, "point-lots"), params });
  },
  grantGift(userId: string, input: AdminPointMutationRequest) {
    return adminRequest<AdminPointGiftResponse>({ method: "POST", url: customerPointPath(userId, "point-gifts"), data: buildPointMutationPayload(input, "GIFT"), retryOnUnauthorized: false });
  },
  correctBalance(userId: string, input: AdminPointMutationRequest) {
    return adminRequest<AdminPointCorrectionResponse>({ method: "POST", url: customerPointPath(userId, "point-corrections"), data: buildPointMutationPayload(input, "CORRECTION"), retryOnUnauthorized: false });
  }
};

function customerPointPath(userId: string, suffix: string) {
  const id = String(userId || "").trim();
  if (!id) throw new Error("客户 ID 不能为空");
  return `/admin/customers/${encodeURIComponent(id)}/${suffix}`;
}
import { adminRequest } from "./client.ts";
import { buildPointMutationPayload, buildPolicyMutationPayload } from "../domain/personalPointsAdmin.ts";
