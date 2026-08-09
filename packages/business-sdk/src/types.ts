import type {
  AgentEntry,
  Asset,
  AuthResponse,
  AuthUser,
  ChannelAgent,
  ChannelCenterResponse,
  CurrentContextRequest,
  EnterpriseContext,
  EnterpriseContextsResponse,
  EnterpriseCertification,
  EnterpriseCertificationSubmitRequest,
  CreateDraft,
  CreateGenerationTaskRequest,
  FeatureEntry,
  GenerationTask,
  MembershipPlan,
  ModelInfo,
  PointAccount,
  PointAccountResponse,
  PointExpiryPolicyResponse,
  UpdatePointExpiryPolicyRequest,
  PersonalPointLotsResponse,
  AdminPointMutationRequest,
  AdminPointGiftResponse,
  AdminPointCorrectionResponse,
  UserProfile,
  WorkItem
} from "@xianzhi/shared-types";

export type AnyRecord = Record<string, unknown>;

export interface HomeOverview {
  user: UserProfile;
  features: FeatureEntry[];
  works: WorkItem[];
  plans: MembershipPlan[];
}

export interface RoleWalletResponse {
  account?: PointAccount;
  tokenRecords?: AnyRecord[];
  orders?: AnyRecord[];
  transactions?: AnyRecord[];
}

export interface MemberProfileResponse {
  user?: AuthUser;
  account?: PointAccount;
  plan?: AnyRecord;
  agent?: (ChannelAgent & AnyRecord) | null;
  operationCenter?: AnyRecord | null;
}

export interface OperationProfileResponse {
  user?: AuthUser;
  operationCenter?: AnyRecord | null;
  joinPlan?: AnyRecord;
  summary?: AnyRecord;
}

export interface ItemsResponse<T = AnyRecord> {
  items?: T[];
  rows?: T[];
  data?: T[];
  summary?: AnyRecord;
}

export interface PageOptions {
  limit?: number;
  offset?: number;
}

export interface TaskPageOptions extends PageOptions {
  prioritizeActive?: boolean;
}

export interface VideoGenerationEstimate {
  model: string;
  estimatedPoints: number;
  billingType: string;
  quantityField: string;
  quantity: number;
  note: string;
}

export interface PagedItems<T> {
  items: T[];
  total: number;
  limit: number;
  offset: number;
  hasMore: boolean;
  summary?: {
    total?: number;
    monthTotal?: number;
    favoriteTotal?: number;
    storageBytes?: number;
  };
}

export interface BillingOrderInput {
  planId?: string;
  amountCents?: number;
  paymentMethod?: string;
  idempotencyKey?: string;
}

export interface RechargeOrderInput {
  amountCents?: number;
  rechargePackageId?: string;
  paymentMethod?: string;
}

export interface SubscriptionOrderInput {
  planId: string;
  amountCents: number;
  paymentMethod?: string;
}

export interface BillingOrderResponse extends AnyRecord {
  item?: AnyRecord;
  order?: AnyRecord;
  plan?: AnyRecord;
  rechargePoints?: number;
  message?: string;
}

export interface BusinessSdk {
  auth: {
    me(): Promise<AuthResponse>;
  };
  dashboard: {
    getHomeOverview(): Promise<HomeOverview>;
  };
  generation: {
    createTask(draft: CreateDraft): Promise<GenerationTask>;
    estimateVideo(request: CreateGenerationTaskRequest): Promise<VideoGenerationEstimate>;
    listTasks(): Promise<GenerationTask[]>;
    listTaskPage(options?: TaskPageOptions): Promise<PagedItems<GenerationTask>>;
  };
  assets: {
    list(): Promise<Asset[]>;
    listPage(options?: PageOptions): Promise<PagedItems<Asset>>;
    getWorks(): Promise<WorkItem[]>;
  };
  models: {
    list(): Promise<ModelInfo[]>;
  };
  points: {
    account(): Promise<PointAccountResponse>;
    expirySummary(): Promise<PointAccountResponse>;
    adminPolicy(): Promise<PointExpiryPolicyResponse>;
    updateAdminPolicy(input: UpdatePointExpiryPolicyRequest): Promise<PointExpiryPolicyResponse>;
    adminLots(userId: string, options?: { source?: string; status?: string; limit?: number; offset?: number }): Promise<PersonalPointLotsResponse>;
    grantAdminGift(userId: string, input: AdminPointMutationRequest): Promise<AdminPointGiftResponse>;
    correctAdminBalance(userId: string, input: AdminPointMutationRequest): Promise<AdminPointCorrectionResponse>;
  };
  membership: {
    plans(): Promise<MembershipPlan[]>;
  };
  billing: {
    wallet(): Promise<RoleWalletResponse>;
    pointsAccount(): Promise<PointAccountResponse>;
    plans(planType?: string): Promise<MembershipPlan[]>;
    createOrder(input: BillingOrderInput): Promise<BillingOrderResponse>;
    createRechargeOrder(input: RechargeOrderInput): Promise<BillingOrderResponse>;
    createSubscriptionOrder(input: SubscriptionOrderInput): Promise<BillingOrderResponse>;
    createAgentJoinOrder(input?: BillingOrderInput): Promise<BillingOrderResponse>;
    createOperationCenterJoinOrder(input?: BillingOrderInput): Promise<BillingOrderResponse>;
  };
  agents: {
    list(): Promise<AgentEntry[]>;
    center(): Promise<ChannelCenterResponse>;
  };
  enterprise: {
    contexts(): Promise<EnterpriseContextsResponse>;
    switchContext(input: CurrentContextRequest): Promise<EnterpriseContext>;
    create(name: string): Promise<AnyRecord>;
    overview(): Promise<AnyRecord>;
    members(): Promise<AnyRecord>;
    organizationTree(): Promise<AnyRecord>;
    submitCertification(input: EnterpriseCertificationSubmitRequest): Promise<EnterpriseCertification>;
  };
  roleWorkbench: {
    memberProfile(): Promise<MemberProfileResponse>;
    wallet(): Promise<RoleWalletResponse>;
    pointsAccount(): Promise<RoleWalletResponse>;
    recentAssets(limit?: number): Promise<Asset[]>;
    channelCenter(): Promise<ChannelCenterResponse>;
    operationProfile(): Promise<OperationProfileResponse>;
    operationAgents(): Promise<ItemsResponse>;
    operationOrders(): Promise<ItemsResponse>;
    operationCommissions(): Promise<ItemsResponse>;
  };
}
