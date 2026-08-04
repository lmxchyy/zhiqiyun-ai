import type {
  AdminPointCorrectionResponse,
  AdminPointGiftResponse,
  AdminPointMutationRequest,
  PersonalPointLot,
  PersonalPointLotsResponse,
  PointExpiryPolicy,
  PointExpiryPolicyResponse,
  UpdatePointExpiryPolicyRequest
} from "@xianzhi/shared-types";

export type {
  AdminPointCorrectionResponse,
  AdminPointGiftResponse,
  AdminPointMutationRequest,
  PersonalPointLot,
  PersonalPointLotsResponse,
  PointExpiryPolicy,
  PointExpiryPolicyResponse,
  UpdatePointExpiryPolicyRequest
};

export interface PointAdminPrincipal {
  role: string;
  permissions: readonly string[];
}

export interface PointAdminActions {
  canViewPolicy: boolean;
  canManagePolicy: boolean;
  canGift: boolean;
  canCorrect: boolean;
  canViewLots: boolean;
}

export interface PointLotSummary {
  id: string;
  lotId: string;
  type: "GRANT" | "EXPIRE";
  points: number;
  occurredAt: string;
  sourceType: string;
  referenceId: string;
  summaryOnly: true;
}

export interface PersonalPointsErrorState {
  message: string;
  status: number;
  code: string;
  forbidden: boolean;
  conflict: boolean;
}

export interface PointLotFilters {
  source?: string;
  status?: string;
  limit?: number;
  offset?: number;
}
