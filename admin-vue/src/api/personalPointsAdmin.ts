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
  async updatePolicy(input: UpdatePointExpiryPolicyRequest) {
    try {
      return await adminRequest<PointExpiryPolicyResponse>({ method: "PUT", url: "/admin/points/expiry-policy", data: buildPolicyMutationPayload(input), retryOnUnauthorized: false });
    } catch (error) {
      throw preservePointPolicyConflict(error);
    }
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
function preservePointPolicyConflict(error: unknown) {
  if (!(error instanceof AdminApiError) || error.status !== 409) return error;
  const payload = error.payload && typeof error.payload === "object" && !Array.isArray(error.payload)
    ? error.payload as { error?: unknown; message?: unknown }
    : {};
  const source = String(payload.error || payload.message || "").trim();
  if (source !== "point expiry policy revision conflict") return error;
  return new AdminApiError(source, error.status, error.code || "POINT_POLICY_REVISION_CONFLICT", error.payload);
}
import { AdminApiError, adminRequest } from "./client.ts";
import { buildPointMutationPayload, buildPolicyMutationPayload } from "../domain/personalPointsAdmin.ts";
