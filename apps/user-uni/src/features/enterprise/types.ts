import type { AppRole, EnterpriseContext, EnterpriseDataScope, EnterpriseWalletSummary } from "../../types";

export interface EnterpriseTenant {
  id: string;
  name: string;
  ownerUserId: string;
  status: string;
  certificationStatus: string;
  config?: Record<string, unknown>;
  createdAt: string;
  updatedAt: string;
}

export interface EnterpriseSubscriptionSummary {
  id: string;
  planId?: string;
  planCode: string;
  status: string;
  trialExpiresAt?: string;
  entitlements: Record<string, unknown>;
}

export interface EnterpriseOverview {
  tenant: EnterpriseTenant;
  memberCount: number;
  activeMembers: number;
  organizationCount: number;
  pendingJoinRequests: number;
  wallet: EnterpriseWalletSummary;
  subscription: EnterpriseSubscriptionSummary;
  currentContext: EnterpriseContext;
}

export interface EnterpriseMember {
  id: string;
  tenantId: string;
  userId: string;
  name: string;
  email: string;
  primaryOrganizationId: string;
  organizationName: string;
  memberStatus: string;
  certificationStatus: string;
  dataScope: EnterpriseDataScope;
  roles: AppRole[];
  joinedAt: string;
  invitedBy?: string;
  createdAt: string;
  updatedAt: string;
}

export interface EnterpriseOrganization {
  id: string;
  tenantId: string;
  parentId?: string;
  organizationType: string;
  name: string;
  status: string;
  metadata?: Record<string, unknown>;
  memberCount: number;
  children?: EnterpriseOrganization[];
  createdAt: string;
  updatedAt: string;
}

export interface EnterpriseInvitation {
  id: string;
  tenantId: string;
  invitationCode: string;
  invitedUserId?: string;
  invitedEmail?: string;
  defaultOrganizationId: string;
  defaultRole: AppRole;
  expiresAt: string;
  status: string;
  createdBy: string;
  acceptedBy?: string;
  acceptedAt?: string;
  createdAt: string;
  updatedAt: string;
}

export interface EnterpriseJoinRequest {
  id: string;
  tenantId: string;
  applicantUserId: string;
  applicantName?: string;
  applicantEmail?: string;
  requestedOrganizationId: string;
  requestedRole: AppRole;
  reason: string;
  status: string;
  reviewedBy?: string;
  reviewedAt?: string;
  reviewComment?: string;
  createdAt: string;
  updatedAt: string;
}

export interface EnterpriseRoleDefinition {
  code: AppRole;
  permissions: string[];
  assignable: boolean;
}

export interface EnterpriseBillingSummary {
  wallet: EnterpriseWalletSummary;
  subscription: EnterpriseSubscriptionSummary;
}

export interface EnterpriseAuditLog {
  id: string;
  tenantId: string;
  actorUserId: string;
  actorRole: string;
  organizationId?: string;
  action: string;
  resourceType: string;
  resourceId?: string;
  targetUserId?: string;
  status: string;
  metadata?: Record<string, unknown>;
  createdAt: string;
}

export interface EnterpriseCertification {
  id: string;
  tenantId: string;
  legalName: string;
  unifiedSocialCreditCode: string;
  legalRepresentativeName: string;
  documentUrls: string[];
  status: string;
  submittedBy: string;
  reviewedBy?: string;
  reviewedAt?: string;
  reviewComment?: string;
  metadata?: Record<string, unknown>;
  createdAt: string;
  updatedAt: string;
}

export interface EnterpriseCreateResult {
  tenant: EnterpriseTenant;
  context: EnterpriseContext;
  invitation: EnterpriseInvitation;
  organization: EnterpriseOrganization;
}

export interface EnterpriseAIEmployee {
  id: string;
  tenantId: string;
  ownerUserId: string;
  name: string;
  description: string;
  modelName: string;
  systemPrompt?: string;
  status: string;
  version: number;
  config?: Record<string, unknown>;
  createdAt: string;
  updatedAt: string;
}

export interface EnterpriseKnowledgeBase {
  id: string;
  tenantId: string;
  organizationId?: string;
  name: string;
  description: string;
  visibility: string;
  status: string;
  documentCount: number;
}

export interface ItemsResponse<T> {
  items: T[];
  total?: number;
  nextCursor?: string;
}

export interface EnterpriseConnectorConfig {
  aiImageEnabled: boolean;
  defaultImageModel: string;
  defaultSize: string;
  defaultImageCount: number;
  memberDailyQuota: number;
  allowGroupChat: boolean;
  groupRequireMention: boolean;
}

export interface EnterpriseConnector {
  id: string;
  enterpriseId: string;
  connectorType: "feishu";
  connectorName: string;
  connectorKey: string;
  appId: string;
  externalTenantKey?: string;
  botOpenId?: string;
  status: "disabled" | "active" | "error";
  config: EnterpriseConnectorConfig;
  secretsConfigured: { appSecret: boolean; verificationToken: boolean; encryptKey: boolean };
  callbackUrl: string;
  lastConnectedAt?: string;
  lastErrorMessage?: string;
  createdAt: string;
  updatedAt: string;
}

export interface ConnectorUserBinding {
  id: string;
  enterpriseId: string;
  connectorId: string;
  platform: "feishu";
  externalUserId: string;
  externalUnionId?: string;
  externalName: string;
  internalUserId?: string;
  internalUserName?: string;
  organizationName?: string;
  permission: { imageGenerate?: boolean; dailyQuota?: number };
  status: "active" | "disabled";
  dailyUsage: number;
  dailyQuota: number;
  lastActiveAt?: string;
  createdAt: string;
  updatedAt: string;
}

export interface ConnectorMessageLog {
  id: string;
  externalMessageId: string;
  externalUserId: string;
  direction: "inbound" | "outbound";
  messageType: string;
  processingStatus: string;
  errorMessage?: string;
  content: Record<string, unknown>;
  createdAt: string;
}

export interface ConnectorAITask {
  id: string;
  bindingId?: string;
  externalUserId?: string;
  externalUserName?: string;
  externalMessageId: string;
  originalText: string;
  intent: string;
  modelId: string;
  platformTaskId?: string;
  status: string;
  progress: number;
  pointsCost: number;
  errorCode?: string;
  errorMessage?: string;
  createdAt: string;
}
