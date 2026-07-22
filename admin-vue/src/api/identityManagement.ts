import { adminRequest } from "./client";

export type BusinessIdentityType = "USER" | "AGENT" | "OPERATION_CENTER";
export type IdentityStatus = "PENDING" | "ACTIVE" | "FROZEN" | "TERMINATED";
export type IdentityChangeMethod = "ONLY_IDENTITY" | "OFFLINE_ORDER" | "SPECIAL_GRANT" | "PACKAGE_CONVERSION";
export type IdentityChangeAction = "UPGRADE" | "FREEZE" | "RESTORE" | "TERMINATE" | "ADJUST_PARENT_AGENT" | "ADJUST_OPERATION_CENTER";

export interface BusinessIdentity {
  id: string;
  userId: string;
  identityType: BusinessIdentityType;
  identityStatus: IdentityStatus;
  commissionEnabled: boolean;
  sourceType: string;
  sourceOrderId?: string;
  effectiveAt: string;
  expiresAt?: string;
  endedAt?: string;
  statusReason?: string;
  identityVersion: number;
  createdBy: string;
  createdAt: string;
  updatedAt: string;
}

export interface IdentityProfile {
  userId: string;
  accountStatus: string;
  legacyRole: string;
  accountRoles: string[];
  primaryIdentity: BusinessIdentityType;
  identities: BusinessIdentity[];
}

export interface UserRelationship {
  id?: string;
  userId: string;
  parentAgentId?: string;
  parentAgentUserId?: string;
  parentAgentName?: string;
  operationCenterId?: string;
  operationCenterName?: string;
  effectiveAt?: string;
  endedAt?: string;
  status?: string;
  sourceType?: string;
  createdBy?: string;
  createdAt?: string;
  updatedAt?: string;
}

export interface IdentityChangeRecord {
  id: string;
  oldIdentity?: Record<string, unknown>;
  newIdentity?: Record<string, unknown>;
  changeType: string;
  sourceType: string;
  sourceOrderId?: string;
  oldParentAgentId?: string;
  newParentAgentId?: string;
  oldOperationCenterId?: string;
  newOperationCenterId?: string;
  reason?: string;
  remark?: string;
  operatorId: string;
  requestId?: string;
  createdAt: string;
}

export interface IdentityFinancialOverview {
  userId: string;
  membership: Record<string, any>;
  wallet: Record<string, any>;
  token: Record<string, any>;
  commission: Record<string, any>;
}

export interface IdentityCommissionPreview {
  beneficiaryType: string;
  beneficiaryId: string;
  ruleId: string;
  ruleCode: string;
  amountCents: number;
}

export interface IdentityChangePreviewRequest {
  action: IdentityChangeAction;
  method: IdentityChangeMethod;
  targetIdentity?: "AGENT" | "OPERATION_CENTER";
  planId?: string;
  parentAgentId?: string;
  operationCenterId?: string;
  paidAmountCents?: number;
  grantPackageToken?: boolean;
  giftTokenAmount?: number;
  conversionTokenPolicy?: "KEEP_EXISTING" | "ADJUST_DIFFERENCE";
  paymentProof?: { reference?: string; url?: string; note?: string };
  reason: string;
  remark?: string;
}

export interface IdentityChangePreview {
  previewToken: string;
  previewId: string;
  userId: string;
  oldIdentity: BusinessIdentityType;
  targetIdentity: BusinessIdentityType;
  method: IdentityChangeMethod;
  action: IdentityChangeAction;
  relationshipBefore: Record<string, string>;
  relationshipAfter: Record<string, string>;
  paymentRequired: boolean;
  paidAmountCents: number;
  tokenDelta: number;
  tokenChangeType?: string;
  commissionGenerated: boolean;
  estimatedCommissions: IdentityCommissionPreview[];
  riskWarnings: string[];
  blockers: string[];
  highRisk: boolean;
  reviewRequired: boolean;
  status: "READY" | "BLOCKED" | "REVIEW_REQUIRED" | "APPROVED" | "REJECTED" | "CONSUMED";
  expiresAt: string;
  effectiveAt?: string;
  sourceMembershipOrderId?: string;
}

export interface IdentityOption { id: string; userId?: string; name?: string; owner?: string; status?: string; level?: number }
export interface IdentityPlanOption { id: string; code: string; name: string; priceCents: number; grantPoints: number; active: boolean; entitlements?: Record<string, any> }

export type IdentityDowngradeStrategy = "TRANSFER_TO_AGENT" | "DIRECT_OPERATION_CENTER" | "PRESERVE_HISTORY";
export interface IdentityDowngradeRequest { targetIdentity?: "AGENT"; childStrategy: IdentityDowngradeStrategy; targetAgentId?: string; targetOperationCenterId?: string; waitForSettlement: boolean; effectiveAt?: string; reason: string; remark?: string }
export interface IdentityDowngradeCheck { code: string; label: string; count: number; amountCents: number; blocking: boolean }
export interface IdentityDowngradePreview { previewToken: string; previewId: string; userId: string; currentIdentity: "AGENT" | "OPERATION_CENTER"; targetIdentity?: "AGENT"; childStrategy: IdentityDowngradeStrategy; effectiveAt: string; waitForSettlement: boolean; checks: IdentityDowngradeCheck[]; downlineMembers: number; downlineAgents: number; migrationCount: number; relationshipBefore: Record<string, unknown>; relationshipAfter: Record<string, unknown>; blockers: string[]; riskWarnings: string[]; status: "READY" | "BLOCKED" | "WAITING" | "SCHEDULED" | "CONSUMED" | "EXPIRED"; expiresAt: string }
export interface IdentityDowngradeResult { requestId: string; userId: string; status: "WAITING" | "SCHEDULED" | "PROCESSING" | "SUCCEEDED" | "FAILED" | "CANCELLED"; effectiveAt: string; migratedMembers: number; migratedAgents: number; migratedRelationships: number; idempotent: boolean }

function userPath(userId: string, suffix: string) {
  return `/admin/users/${encodeURIComponent(userId)}/identity-change/${suffix}`;
}

export const identityManagementApi = {
  async profile(userId: string) {
    const response = await adminRequest<{ item: IdentityProfile }>({ method: "GET", url: `/admin/customers/${encodeURIComponent(userId)}/identity-profile` });
    return response.item;
  },
  history(userId: string) {
    return adminRequest<{ userId: string; identities: BusinessIdentity[]; changeRecords: IdentityChangeRecord[] }>({ method: "GET", url: `/admin/customers/${encodeURIComponent(userId)}/identity-history` });
  },
  async relationship(userId: string) {
    const response = await adminRequest<{ item: UserRelationship | null }>({ method: "GET", url: `/admin/customers/${encodeURIComponent(userId)}/relationship` });
    return response.item;
  },
  async relationshipHistory(userId: string) {
    const response = await adminRequest<{ items: UserRelationship[] }>({ method: "GET", url: `/admin/customers/${encodeURIComponent(userId)}/relationship-history` });
    return response.items;
  },
  async financialOverview(userId: string) {
    const response = await adminRequest<{ item: IdentityFinancialOverview }>({ method: "GET", url: `/admin/customers/${encodeURIComponent(userId)}/identity-financial-overview` });
    return response.item;
  },
  async agents() {
    const response = await adminRequest<{ items: IdentityOption[] }>({ method: "GET", url: "/admin/channel-agents" });
    return response.items;
  },
  async operationCenters() {
    const response = await adminRequest<{ items: IdentityOption[] }>({ method: "GET", url: "/admin/operation-centers" });
    return response.items;
  },
  async plans() {
    const response = await adminRequest<{ items: IdentityPlanOption[] }>({ method: "GET", url: "/admin/plans" });
    return response.items;
  },
  async preview(userId: string, data: IdentityChangePreviewRequest) {
    const response = await adminRequest<{ item: IdentityChangePreview }>({ method: "POST", url: userPath(userId, "preview"), data, retryOnUnauthorized: false });
    return response.item;
  },
  async confirm(userId: string, previewToken: string, highRiskConfirmed: boolean) {
    const response = await adminRequest<{ item: { executionId: string; status: string; orderId?: string; idempotent: boolean } }>({ method: "POST", url: userPath(userId, "confirm"), data: { previewToken, highRiskConfirmed }, retryOnUnauthorized: false });
    return response.item;
  },
  async review(userId: string, previewToken: string, decision: "APPROVED" | "REJECTED", reason: string) {
    const response = await adminRequest<{ item: IdentityChangePreview }>({ method: "POST", url: userPath(userId, "review"), data: { previewToken, decision, reason }, retryOnUnauthorized: false });
    return response.item;
  },
  async downgradePreview(userId: string, data: IdentityDowngradeRequest) {
    const response = await adminRequest<{ item: IdentityDowngradePreview }>({ method: "POST", url: `/admin/users/${encodeURIComponent(userId)}/identity-downgrade/preview`, data, retryOnUnauthorized: false });
    return response.item;
  },
  async downgradeConfirm(userId: string, previewToken: string) {
    const response = await adminRequest<{ item: IdentityDowngradeResult }>({ method: "POST", url: `/admin/users/${encodeURIComponent(userId)}/identity-downgrade/confirm`, data: { previewToken, highRiskConfirmed: true }, retryOnUnauthorized: false });
    return response.item;
  },
  async downgradeRequests(userId: string) {
    const response = await adminRequest<{ items: IdentityDowngradeResult[] }>({ method: "GET", url: `/admin/users/${encodeURIComponent(userId)}/identity-downgrade/requests` });
    return response.items;
  }
};
