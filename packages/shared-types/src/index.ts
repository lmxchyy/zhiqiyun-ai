export type WorkspaceRole = "user" | "agent" | "admin";

export type AppRole =
  | "USER"
  | "AGENT"
  | "OPERATION"
  | "ENTERPRISE_ADMIN"
  | "AI_ADMIN"
  | "FINANCE"
  | "CUSTOMER_SERVICE"
  | "ENTERPRISE_MEMBER";

export interface UserAccessProfile {
  userId: string;
  tenantId: string;
  organizationId: string;
  roles: AppRole[];
  currentRole: AppRole;
  permissions: string[];
}

export type UserContextType = "PERSONAL" | "ENTERPRISE" | "AGENT" | "OPERATION";

export type EnterpriseDataScope = "TENANT_ALL" | "ORG_AND_CHILDREN" | "ORG_SELF" | "OWNER" | "SELF";

export interface EnterpriseWalletSummary {
  pointBalance: number;
  frozenPoints: number;
  cashBalanceCents: number;
  status: string;
}

export interface EnterpriseContext {
  type: UserContextType;
  tenantId: string;
  tenantName: string;
  organizationId: string;
  organizationName: string;
  memberStatus: string;
  certificationStatus: string;
  roles: AppRole[];
  currentRole: AppRole;
  permissions: string[];
  dataScope: EnterpriseDataScope;
  entitlements: Record<string, unknown>;
  wallet: EnterpriseWalletSummary;
  current: boolean;
}

export interface EnterpriseContextsResponse {
  contexts: EnterpriseContext[];
  current: EnterpriseContext;
}

export interface CurrentContextRequest {
  type: UserContextType;
  tenantId?: string;
  organizationId?: string;
  role?: AppRole;
}

export interface EnterpriseCertificationSubmitRequest {
  legalName: string;
  unifiedSocialCreditCode: string;
  legalRepresentativeName?: string;
  documentUrls: string[];
  metadata?: Record<string, unknown>;
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

export type TaskStatus =
  | "PENDING"
  | "QUEUED"
  | "RUNNING"
  | "PROCESSING"
  | "RETRYING"
  | "SUCCEEDED"
  | "COMPLETED"
  | "FAILED"
  | "ERROR"
  | "CANCELLED";

export type GenerationTaskType =
  | "TEXT_TO_IMAGE"
  | "IMAGE_TO_IMAGE"
  | "TEXT_TO_VIDEO"
  | "IMAGE_TO_VIDEO"
  | "PPT_GENERATION"
  | string;

export type VideoGenerationMode = "TEXT_TO_VIDEO" | "IMAGE_TO_VIDEO";

export interface VideoModelCapabilities {
  supportsTextToVideo: boolean;
  supportsImageToVideo: boolean;
  supportsFirstFrame: boolean;
  supportsLastFrame: boolean;
  maxReferenceImages: number;
  supportedDurations: number[];
  supportedResolutions: string[];
  supportedAspectRatios: string[];
}

export interface ApiEnvelope<T> {
  code?: number | string;
  message?: string;
  error?: string;
  data?: T;
}

export type { components as OpenApiComponents, operations as OpenApiOperations, paths as OpenApiPaths } from "./generated/openapi";

export interface AuthUser {
  id: string;
  tenantId?: string;
  organizationId?: string;
  email: string;
  mobileMasked?: string;
  passwordSet?: boolean;
  wechatLinked?: boolean;
  name: string;
  role: string;
  status: string;
  planId?: string;
  memberLevel?: string;
  agentStatus?: string;
  operationCenterStatus?: string;
  referredBy?: string;
  subscriptionExpiresAt?: string;
}

export interface ChannelAgent {
  id: string;
  userId: string;
  name?: string;
  email?: string;
  parentId?: string;
  operationCenterId?: string;
  level: number;
  status: string;
  inviteCode: string;
  inviteLink?: string;
  createdAt?: string;
}

export interface AuthResponse {
  accessToken?: string;
  refreshToken?: string;
  user: AuthUser;
  agent?: ChannelAgent;
  permissions?: string[];
  tenantId?: string;
  organizationId?: string;
  roles?: AppRole[];
  currentRole?: AppRole;
  defaultModule?: string;
  workspace?: WorkspaceRole;
  defaultRoute?: string;
  isNewUser?: boolean;
  registrationStatus?: string;
  inviteBindStatus?: string;
  newcomerBenefits?: Array<{ title?: string; description?: string; status?: string }>;
  nextAction?: string;
  expiresIn?: number;
}

export interface GenerationTask {
  id: string;
  type: GenerationTaskType;
  status: TaskStatus;
  progress?: number;
  prompt: string;
  model: string;
  pointCost: number;
  resultIds?: string[];
  params?: Record<string, unknown>;
  createdAt?: string;
  updatedAt?: string;
  workerFinishedAt?: string;
}

export interface GeneratedImage {
  url: string;
  thumbnailUrl?: string;
  contentType?: string;
  width?: number;
  height?: number;
  source?: string;
}

export interface CreateGenerationTaskRequest {
  type: GenerationTaskType;
  clientRequestId?: string;
  module_code?: string;
  moduleCode?: string;
  prompt: string;
  model: string;
  params?: Record<string, unknown>;
}

export type AssetMediaType = "image" | "video" | "document" | string;

export interface Asset {
  id: string;
  name: string;
  url: string;
  thumbnailUrl?: string;
  mediaType: AssetMediaType;
  taskId?: string;
  metadata?: Record<string, unknown>;
  createdAt?: string;
  updatedAt?: string;
}

export interface ReferenceImage {
  path: string;
  name: string;
  sourceAssetId?: string;
}

export interface ModelInfo {
  code: string;
  name: string;
  capabilities?: string[];
  pointCost?: number;
  fixedQuota?: number;
  providerId?: string;
  providerName?: string;
  online?: boolean;
  videoCapabilities?: VideoModelCapabilities;
}

export interface PointAccount {
  id: string;
  userId: string;
  available: number;
  frozen: number;
  total?: number;
  permanentAvailable?: number;
  expiringAvailable?: number;
  nextExpiryAt?: string;
  nextExpiryPoints?: number;
}

export type PersonalPointSource =
  | "REGISTRATION_GIFT"
  | "ACTIVITY_GIFT"
  | "ADMIN_GIFT"
  | "ADMIN_CORRECTION"
  | "RECHARGE"
  | "MEMBERSHIP_GRANT"
  | "MEMBER_PACKAGE_GRANT"
  | "AGENT_GRANT"
  | "AGENT_JOIN_GRANT"
  | "OPERATION_CENTER_GRANT"
  | "ORDER_GRANT"
  | "COMMERCE_ORDER"
  | "UNIFIED_PAYMENT_GRANT"
  | "WECHAT_VIRTUAL_ORDER"
  | "WECHAT_VIRTUAL_COUPON"
  | "COUPON_GRANT"
  | "LEGACY"
  | string;

export interface PointExpiryPolicy {
  id: string;
  version: number;
  revision: number;
  enabled: boolean;
  duration_value: number;
  duration_unit: "CALENDAR_MONTH";
  time_zone: string;
  source_types: PersonalPointSource[];
  effective_from: string;
  effective_to?: string;
  status: string;
  created_by?: string;
  change_reason?: string;
}

export interface PointExpiryPolicyResponse { item: PointExpiryPolicy; }
export interface UpdatePointExpiryPolicyRequest {
  revision: number;
  enabled: boolean;
  durationValue: number;
  changeReason: string;
}

export interface PersonalPointLot {
  id: string;
  account_id: string;
  user_id: string;
  source_type: PersonalPointSource;
  reference_type: string;
  reference_id: string;
  original_points: number;
  available_points: number;
  reserved_points: number;
  consumed_points: number;
  expired_points: number;
  reversed_points: number;
  granted_at: string;
  expires_at?: string;
  policy_version_id?: string;
  idempotency_key: string;
  status: string;
}

export interface PersonalPointLotsResponse { items: PersonalPointLot[]; }
export interface AdminPointMutationRequest {
  points: number;
  reason: string;
  idempotencyKey: string;
}
export interface AdminPointGiftResponse { item: PersonalPointLot; idempotent: boolean; }
export interface AdminPointCorrectionResponse {
  balance: { account_id: string; user_id: string; available: number; frozen: number; total: number };
  lot?: PersonalPointLot;
  points: number;
  idempotent: boolean;
}

export interface PointAccountResponse {
  account: PointAccount;
  transactions?: unknown[];
  orders?: unknown[];
}

export interface UserDashboardSummary {
  availablePoints: number;
  todayGenerations: number;
  succeededGenerations?: number;
  queueTasks?: number;
  apiPlatforms?: number;
  assets: number;
  totalPointCost: number;
}

export interface UserDashboardResponse {
  summary: UserDashboardSummary;
  metrics?: Array<{ label: string; value: number | string }>;
  recentTasks?: GenerationTask[];
  recentAssets?: Asset[];
}

export interface CommercePlan {
  id: string;
  name: string;
  priceCents?: number;
  amountCents?: number;
  pointAmount?: number;
  tokenAmount?: number;
  rechargePoints?: number;
  benefits?: string[];
  recommended?: boolean;
  metadata?: Record<string, unknown>;
}

export type WorkType = "image" | "video" | "ppt" | "document";
export type WorkStatus = "queued" | "processing" | "succeeded" | "failed";
export type CreateMode = "image" | "video" | "ppt" | "agent";

export interface UserProfile {
  id: string;
  name: string;
  avatarText: string;
  memberLevel: string;
  points: number;
  agentEnabled: boolean;
}

export interface FeatureEntry {
  id: string;
  title: string;
  subtitle: string;
  icon: string;
  tone: "primary" | "accent" | "dark" | "green";
  path?: string;
}

export interface WorkItem {
  id: string;
  title: string;
  type: WorkType;
  status: WorkStatus;
  model: string;
  prompt: string;
  createdAt: string;
  thumbnailUrl?: string;
  url?: string;
}

export interface AgentEntry {
  id: string;
  title: string;
  description: string;
  tags: string[];
  tone: string;
}

export interface MembershipPlan {
  id: string;
  name: string;
  price: string;
  points: number;
  benefits: string[];
  recommended?: boolean;
}

export interface CreateDraft {
  mode: CreateMode;
  prompt: string;
  model: string;
  style: string;
  size: string;
  quality: string;
  count: number;
  referenceImages: string[];
  videoMode?: VideoGenerationMode;
  firstFrame?: string;
  lastFrame?: string;
  videoCapabilities?: VideoModelCapabilities;
  clientRequestId?: string;
  negativePrompt?: string;
  duration?: number;
  parameters?: Record<string, unknown>;
}

export interface ChannelCommission {
  id: string;
  orderId: string;
  agentId: string;
  amountCents: number;
  rate: number;
  status: string;
  createdAt?: string;
}

export interface ChannelWithdrawal {
  id: string;
  agentId: string;
  amountCents: number;
  status: string;
  createdAt?: string;
  reviewedAt?: string;
}

export interface ChannelOrder {
  id: string;
  orderId?: string;
  customer?: string;
  plan?: string;
  amountCents?: number;
  status: string;
  createdAt?: string;
}

export interface ChannelUsageEvent {
  id: string;
  transactionId: string;
  userId: string;
  customerId?: string;
  agentId?: string;
  taskId: string;
  assetIds?: string[];
  metricCode: string;
  quantity: number;
  unitAmountCents: number;
  amountCents: number;
  pointCost: number;
  balanceBefore: number;
  balanceAfter: number;
  model: string;
  status: string;
  occurredAt: string;
  metadata?: Record<string, unknown>;
}

export interface ChannelCenterSummary {
  directCustomers: number;
  childAgents: number;
  totalCommission: number;
  settledCommission: number;
  pendingCommission: number;
  withdrawn: number;
  pendingWithdrawal: number;
  availableToWithdraw: number;
}

export interface ChannelCenterResponse {
  user: AuthUser;
  agent: ChannelAgent;
  summary: ChannelCenterSummary;
  customers: AuthUser[];
  orders?: ChannelOrder[];
  usageEvents?: ChannelUsageEvent[];
  commissions: ChannelCommission[];
  withdrawals: ChannelWithdrawal[];
  children: ChannelAgent[];
}

