export type WorkspaceRole = "user" | "agent" | "admin";

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

export interface ApiEnvelope<T> {
  code?: number | string;
  message?: string;
  error?: string;
  data?: T;
}

export type { components as OpenApiComponents, operations as OpenApiOperations, paths as OpenApiPaths } from "./generated/openapi";

export interface AuthUser {
  id: string;
  email: string;
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
  defaultModule?: string;
  workspace?: WorkspaceRole;
  defaultRoute?: string;
}

export interface GenerationTask {
  id: string;
  type: GenerationTaskType;
  status: TaskStatus;
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
}

export interface PointAccount {
  id: string;
  userId: string;
  available: number;
  frozen: number;
  total?: number;
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

