import type {
  AgentEntry,
  Asset,
  AuthResponse,
  AuthUser,
  ChannelAgent,
  ChannelCenterResponse,
  CreateDraft,
  FeatureEntry,
  GenerationTask,
  MembershipPlan,
  ModelInfo,
  PointAccount,
  PointAccountResponse,
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
    listTasks(): Promise<GenerationTask[]>;
  };
  assets: {
    list(): Promise<Asset[]>;
    getWorks(): Promise<WorkItem[]>;
  };
  models: {
    list(): Promise<ModelInfo[]>;
  };
  points: {
    account(): Promise<PointAccountResponse>;
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
