import type {
  AdminPointMutationRequest,
  PersonalPointLot,
  PointAdminActions,
  PointAdminPrincipal,
  PointLotSummary,
  PersonalPointsErrorState,
  UpdatePointExpiryPolicyRequest
} from "../types/personalPointsAdmin.ts";
import { AdminApiError } from "../api/client.ts";

export const PERSONAL_POINT_ADMIN_PERMISSION = {
  viewPolicy: "points:gift-policy:view",
  managePolicy: "points:gift-policy:manage",
  grantGift: "points:gift:grant",
  correctBalance: "points:balance:correct",
  viewLots: "points:lot:view"
} as const;

export const PERSONAL_POINTS_ADMIN_PERMISSIONS = [
  PERSONAL_POINT_ADMIN_PERMISSION.viewPolicy,
  PERSONAL_POINT_ADMIN_PERMISSION.managePolicy,
  PERSONAL_POINT_ADMIN_PERMISSION.grantGift,
  PERSONAL_POINT_ADMIN_PERMISSION.correctBalance,
  PERSONAL_POINT_ADMIN_PERMISSION.viewLots
] as const;

export const CUSTOMER_POINT_ACTION_PERMISSIONS = [
  PERSONAL_POINT_ADMIN_PERMISSION.grantGift,
  PERSONAL_POINT_ADMIN_PERMISSION.correctBalance,
  PERSONAL_POINT_ADMIN_PERMISSION.viewLots
] as const;

export interface AdminPointMutationInput extends AdminPointMutationRequest {
  validityDays?: number;
}

export function hasAnyPersonalPointsAdminPermission(permissions: readonly string[]) {
  return PERSONAL_POINTS_ADMIN_PERMISSIONS.some((permission) => permissions.includes(permission));
}

export function canAccessCustomerPointActions(principal: PointAdminPrincipal) {
  const role = String(principal.role || "").trim().toUpperCase();
  const permissions = Array.isArray(principal.permissions) ? principal.permissions : [];
  return role === "SUPER_ADMIN" || CUSTOMER_POINT_ACTION_PERMISSIONS.some((permission) => permissions.includes(permission));
}

function validationError(message: string, code: string) {
  return new AdminApiError(message, 400, code, { code });
}

export function buildPolicyMutationPayload(input: UpdatePointExpiryPolicyRequest): UpdatePointExpiryPolicyRequest {
  const revision = Number(input?.revision);
  const durationValue = Number(input?.durationValue);
  const changeReason = String(input?.changeReason || "").trim();
  if (!Number.isSafeInteger(revision) || revision <= 0) {
    throw validationError("策略 revision 无效，请刷新后重试", "POINT_POLICY_REVISION_INVALID");
  }
  if (!Number.isSafeInteger(durationValue) || durationValue <= 0) {
    throw validationError("到期月数必须为大于 0 的整数", "POINT_POLICY_DURATION_INVALID");
  }
  if (!changeReason) {
    throw validationError("请输入策略变更原因", "POINT_POLICY_REASON_REQUIRED");
  }
  return { revision, enabled: input.enabled === true, durationValue, changeReason };
}

export function buildPointMutationPayload(input: AdminPointMutationInput, kind: "GIFT" | "CORRECTION"): AdminPointMutationInput {
  const points = Number(input?.points);
  const reason = String(input?.reason || "").trim();
  const idempotencyKey = String(input?.idempotencyKey || "").trim();
  if (!Number.isSafeInteger(points) || (kind === "GIFT" ? points <= 0 : points === 0)) {
    throw validationError(kind === "GIFT" ? "赠送积分必须大于 0" : "余额纠正不能为 0", "POINT_MUTATION_POINTS_INVALID");
  }
  if (!reason) throw validationError("请输入操作原因", "POINT_MUTATION_REASON_REQUIRED");
  if (!idempotencyKey) throw validationError("缺少幂等键，请重新打开操作窗口", "POINT_MUTATION_IDEMPOTENCY_REQUIRED");
  if (kind === "CORRECTION") return { points, reason, idempotencyKey };
  const validityDays = Number(input?.validityDays || 0);
  if (!Number.isSafeInteger(validityDays) || validityDays < 0 || validityDays > 3650) {
    throw validationError("赠送积分有效期必须是 0 到 3650 天的整数", "POINT_MUTATION_VALIDITY_INVALID");
  }
  return { points, reason, idempotencyKey, ...(validityDays > 0 ? { validityDays } : {}) };
}

export function buildPointLotSummaries(lots: readonly PersonalPointLot[]): PointLotSummary[] {
  return lots.flatMap((lot) => {
    const summaries: PointLotSummary[] = [{
      id: `${lot.id}:grant`,
      lotId: lot.id,
      type: "GRANT",
      points: Number(lot.original_points || 0),
      occurredAt: lot.granted_at,
      sourceType: String(lot.source_type || ""),
      referenceId: String(lot.reference_id || ""),
      summaryOnly: true
    }];
    const expiredPoints = Number(lot.expired_points || 0);
    if (expiredPoints > 0) {
      summaries.push({
        id: `${lot.id}:expire`,
        lotId: lot.id,
        type: "EXPIRE",
        points: expiredPoints,
        occurredAt: String(lot.expires_at || ""),
        sourceType: String(lot.source_type || ""),
        referenceId: String(lot.reference_id || ""),
        summaryOnly: true
      });
    }
    return summaries;
  }).sort((left, right) => right.occurredAt.localeCompare(left.occurredAt));
}

export function pointAdminActions(principal: PointAdminPrincipal): PointAdminActions {
  const role = String(principal.role || "").trim().toUpperCase();
  const permissions = Array.isArray(principal.permissions) ? principal.permissions : [];
  const has = (permission: string) => role === "SUPER_ADMIN" || permissions.includes(permission);
  return {
    canViewPolicy: has(PERSONAL_POINT_ADMIN_PERMISSION.viewPolicy),
    canManagePolicy: has(PERSONAL_POINT_ADMIN_PERMISSION.managePolicy),
    canGift: has(PERSONAL_POINT_ADMIN_PERMISSION.grantGift),
    canCorrect: has(PERSONAL_POINT_ADMIN_PERMISSION.correctBalance),
    canViewLots: has(PERSONAL_POINT_ADMIN_PERMISSION.viewLots)
  };
}

export function personalPointsErrorState(error: unknown): PersonalPointsErrorState {
  if (error instanceof AdminApiError) {
    return {
      message: error.message,
      status: error.status,
      code: error.code,
      forbidden: error.status === 403,
      conflict: error.status === 409
    };
  }
  return {
    message: error instanceof Error ? error.message : "积分管理请求失败",
    status: 0,
    code: "",
    forbidden: false,
    conflict: false
  };
}
