export type TaskStatus = "QUEUED" | "PROCESSING" | "RETRYING" | "SUCCEEDED" | "FAILED" | "CANCELLED";

export interface GenerationTask {
  id: string;
  type: "TEXT_TO_IMAGE" | "IMAGE_TO_IMAGE" | "TEXT_TO_VIDEO" | "IMAGE_TO_VIDEO";
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

export interface Asset {
  id: string;
  name: string;
  url: string;
  thumbnailUrl?: string;
  mediaType: "image" | "video" | "document";
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
}

export interface PointAccount {
  id: string;
  userId: string;
  available: number;
  frozen: number;
}

export interface PointAccountResponse {
  account: PointAccount;
  transactions?: unknown[];
}

export interface AuthUser {
  id: string;
  email: string;
  name: string;
  role: string;
  status: string;
  planId?: string;
}

export interface ChannelAgent {
  id: string;
  userId: string;
  name?: string;
  email?: string;
  parentId?: string;
  level: number;
  status: string;
  inviteCode: string;
  createdAt?: string;
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

export type WorkspaceRole = "user" | "agent" | "admin";

export interface AuthResponse {
  accessToken?: string;
  user: AuthUser;
  agent?: ChannelAgent;
  permissions: string[];
  defaultModule: string;
  workspace?: WorkspaceRole;
  defaultRoute?: string;
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
  commissions: ChannelCommission[];
  withdrawals: ChannelWithdrawal[];
  children: ChannelAgent[];
}

