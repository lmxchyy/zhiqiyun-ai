export interface EnterpriseFilterOption {
  value: string;
  label: string;
}

export interface EnterprisePlanSummary {
  id?: string;
  code: string;
  name: string;
  status: string;
  expiresAt?: string;
}

export interface EnterpriseComputeSummary {
  balance: number;
  frozen: number;
  unit: "POINT" | string;
  balanceText?: string;
}

export interface EnterpriseRelationSummary {
  id?: string;
  name?: string;
}

export interface AdminEnterpriseListItem {
  id: string;
  enterpriseCode: string;
  name: string;
  certificationStatus: string;
  plan: EnterprisePlanSummary;
  memberCount: number;
  activeMemberCount: number;
  seatLimit: number;
  compute: EnterpriseComputeSummary;
  sourceAgent: EnterpriseRelationSummary;
  operationCenter: EnterpriseRelationSummary;
  status: string;
  createdAt: string;
  updatedAt: string;
}

export interface AdminEnterpriseStats {
  total: number;
  certified: number;
  createdThisMonth: number;
  abnormal: number;
}

export interface AdminEnterpriseFilters {
  plans: EnterpriseFilterOption[];
  sourceAgents: EnterpriseFilterOption[];
  operationCenters: EnterpriseFilterOption[];
}

export interface AdminEnterpriseListResult {
  items: AdminEnterpriseListItem[];
  total: number;
  page: number;
  pageSize: number;
  stats: AdminEnterpriseStats;
  filters: AdminEnterpriseFilters;
}

export interface AdminEnterpriseProfile {
  legalName?: string;
  unifiedSocialCreditCode?: string;
  legalRepresentativeName?: string;
  industry?: string;
  companySize?: string;
  ownerUserId?: string;
}

export interface AdminEnterpriseOperation {
  id: string;
  actor: string;
  action: string;
  summary: string;
  createdAt: string;
}

export interface AdminEnterprisePrivacyBoundary {
  summaryOnly: boolean;
  message: string;
  restrictedFields: string[];
}

export interface AdminEnterpriseDetail {
  enterprise: AdminEnterpriseListItem;
  profile: AdminEnterpriseProfile;
  organizationCount: number;
  recentOperations: AdminEnterpriseOperation[];
  privacy: AdminEnterprisePrivacyBoundary;
}

export interface AdminEnterpriseListQuery {
  page: number;
  pageSize: number;
  keyword?: string;
  certificationStatus?: string;
  planCode?: string;
  status?: string;
  sourceAgentId?: string;
  operationCenterId?: string;
  createdFrom?: string;
  createdTo?: string;
}

export interface AdminEnterpriseCreateRequest {
  name: string;
  enterpriseCode?: string;
  ownerUserId?: string;
  planId?: string;
  planCode?: string;
  seatLimit: number;
  industry?: string;
  companySize?: string;
  sourceAgentId?: string;
  operationCenterId?: string;
}

export interface AdminEnterpriseSectionResult {
  section: string;
  enterprise?: EnterpriseRelationSummary;
  summary: Record<string, unknown>;
  items: Array<Record<string, unknown>>;
  total: number;
  unit?: string;
  privacy: AdminEnterprisePrivacyBoundary;
}

export interface AdminEnterpriseMutationRequest {
  requestId: string;
  reason: string;
  status?: string;
  reviewComment?: string;
  planId?: string;
  planCode?: string;
  expiresAt?: string;
  seatLimit?: number;
  pointDelta?: number;
  sourceAgentId?: string;
  operationCenterId?: string;
  name?: string;
  industry?: string;
  companySize?: string;
  moduleCode?: string;
  modelName?: string;
  limits?: Record<string, unknown>;
}

export interface AdminEnterpriseMutationResult {
  requestId: string;
  action: string;
  status: string;
  enterpriseId: string;
  before: Record<string, unknown>;
  after: Record<string, unknown>;
  auditId?: string;
  message: string;
}

export const enterprisePermissions = {
  list: "enterprise:list",
  detail: "enterprise:detail",
  create: "enterprise:create",
  update: "enterprise:update",
  certificationReview: "enterprise:certification:review",
  memberView: "enterprise:member:view",
  packageView: "enterprise:package:view",
  packageAdjust: "enterprise:package:adjust",
  seatAdjust: "enterprise:seat:adjust",
  computeView: "enterprise:compute:view",
  computeAdjust: "enterprise:compute:adjust",
  transactionView: "enterprise:transaction:view",
  orderView: "enterprise:order:view",
  aiView: "enterprise:ai:view",
  aiConfigure: "enterprise:ai:configure",
  employeeView: "enterprise:employee:view",
  knowledgeView: "enterprise:knowledge:view",
  attributionView: "enterprise:attribution:view",
  attributionChange: "enterprise:attribution:change",
  riskView: "enterprise:risk:view",
  riskDisable: "enterprise:risk:disable",
  riskRestore: "enterprise:risk:restore",
  auditView: "enterprise:audit:view",
  export: "enterprise:export"
} as const;

export type EnterprisePermission = typeof enterprisePermissions[keyof typeof enterprisePermissions];
