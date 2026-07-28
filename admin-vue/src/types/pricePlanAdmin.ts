export type PricingBusinessType = "MEMBER" | "AGENT";
export type PlanVersionStatus = "DRAFT" | "ACTIVE" | "RETIRED";
export type PricePlanKind = "NORMAL" | "PROMOTION" | "TEST";
export type PricePlanStatus = "DRAFT" | "ACTIVE" | "INACTIVE" | "EXPIRED";
export type PaymentEnvironment = "PRODUCTION" | "SANDBOX";
export type PaymentChannel = "WECHAT_VIRTUAL" | string;
export type PricingCurrency = "CNY" | string;
export type PricePlanAudienceType = "PUBLIC" | "RULE" | "WHITELIST" | "INVITE" | "TEST" | string;
export type WhitelistStatus = "PENDING" | "ACTIVE" | "EXPIRED" | "DISABLED";
export type PricingHealthStatus = "HEALTHY" | "DEGRADED" | "BLOCKED";

export interface ItemResponse<T> {
  item: T;
}

export interface ListResponse<T> {
  items: T[];
  total: number;
}

export interface BusinessPlan {
  id: string;
  code: string;
  name: string;
  businessType: PricingBusinessType;
  legacyCode: boolean;
  codeReadOnly: boolean;
  active: boolean;
  activeVersionId?: string;
}

export interface PlanVersion {
  id: string;
  planId: string;
  versionNo: number;
  businessType: PricingBusinessType;
  rightsSnapshot: Record<string, unknown>;
  memberLevel?: string;
  agentLevel?: string;
  tokenAmount: number;
  pointsAmount: number;
  durationDays: number;
  commissionRuleVersion: string;
  commissionSnapshot: Record<string, unknown>;
  status: PlanVersionStatus;
  revision: number;
  effectiveAt?: string;
  expiresAt?: string;
  createdBy?: string;
  updatedBy?: string;
  activatedBy?: string;
  activatedAt?: string;
  retiredBy?: string;
  retiredAt?: string;
  changeReason?: string;
  createdAt: string;
  updatedAt: string;
}

export interface PlanVersionDraftInput {
  memberLevel?: string;
  agentLevel?: string;
  tokenAmount?: number;
  pointsAmount?: number;
  durationDays?: number;
  rightsSnapshot?: Record<string, unknown>;
  commissionRuleVersion?: string;
  commissionSnapshot?: Record<string, unknown>;
  effectiveAt?: string;
  expiresAt?: string;
  changeReason: string;
}

export type PlanVersionCreateInput = PlanVersionDraftInput;

export interface PlanVersionUpdateInput extends PlanVersionDraftInput {
  revision: number;
}

export interface RevisionReasonInput {
  revision: number;
  changeReason: string;
}

export interface PricePlan {
  pricePlanId: string;
  planId: string;
  planVersionId: string;
  code: string;
  name: string;
  kind: PricePlanKind;
  channel: PaymentChannel;
  environment: PaymentEnvironment;
  currency: PricingCurrency;
  salePriceCents: number;
  listPriceCents: number;
  giftPoints: number;
  giftTokens: number;
  validFrom?: string;
  validUntil?: string;
  audienceType: PricePlanAudienceType;
  audienceRule: Record<string, unknown>;
  isVisible: boolean;
  isDefault: boolean;
  isEnabled: boolean;
  status: PricePlanStatus;
  revision: number;
  changeReason?: string;
  createdBy?: string;
  updatedBy?: string;
  enabledBy?: string;
  enabledAt?: string;
  disabledBy?: string;
  disabledAt?: string;
  createdAt: string;
  updatedAt: string;
  hasQuote: boolean;
  hasOrder: boolean;
  economicFieldsLocked: boolean;
}

export interface PricePlanCreateInput {
  revision: number;
  planVersionId: string;
  code: string;
  name: string;
  kind: PricePlanKind;
  channel: PaymentChannel;
  environment: PaymentEnvironment;
  currency: PricingCurrency;
  salePriceCents: number;
  listPriceCents: number;
  giftPoints: number;
  giftTokens: number;
  validFrom?: string;
  validUntil?: string;
  audienceType: PricePlanAudienceType;
  audienceRule: Record<string, unknown>;
  isVisible: boolean;
  changeReason: string;
}

export interface PricePlanUpdateInput {
  revision: number;
  name?: string;
  planVersionId?: string;
  kind?: PricePlanKind;
  channel?: PaymentChannel;
  environment?: PaymentEnvironment;
  currency?: PricingCurrency;
  salePriceCents?: number;
  listPriceCents?: number;
  giftPoints?: number;
  giftTokens?: number;
  validFrom?: string;
  validUntil?: string;
  clearValidFrom?: boolean;
  clearValidUntil?: boolean;
  audienceType?: PricePlanAudienceType;
  audienceRule?: Record<string, unknown>;
  isVisible?: boolean;
  changeReason: string;
}

export interface PricePlanCloneInput extends RevisionReasonInput {
  code: string;
  name: string;
}

export interface PricePlanValidationCheck {
  code: string;
  passed: boolean;
  message?: string;
}

export interface PricePlanValidation {
  pricePlanId: string;
  valid: boolean;
  checkedAt: string;
  paymentBindingId?: string;
  wechatGoodId?: string;
  wechatProductId?: string;
  pricePlanPriceCents: number;
  bindingPriceCents?: number;
  wechatGoodPriceCents?: number;
  checks: PricePlanValidationCheck[];
}

export interface PricePlanDefaultResponse extends ItemResponse<PricePlan> {
  alreadyDefault: boolean;
}

export interface WechatVirtualGood {
  id: string;
  channel: PaymentChannel;
  environment: PaymentEnvironment;
  offerId: string;
  productId: string;
  goodsName: string;
  platformPriceCents: number;
  mode: string;
  published: boolean;
  enabled: boolean;
  status: string;
  verificationStatus: string;
  verificationSource: string;
  platformRealtimeVerified: boolean;
  verifiedBy?: string;
  verifiedAt?: string;
  verificationReason?: string;
  verificationEvidence?: string;
  verificationSnapshot: Record<string, unknown>;
  verificationExpiresAt?: string;
  revision: number;
  createdBy?: string;
  updatedBy?: string;
  createdAt: string;
  updatedAt: string;
}

export interface WechatVirtualGoodCreateInput {
  channel: PaymentChannel;
  environment: PaymentEnvironment;
  offerId: string;
  productId: string;
  goodsName: string;
  platformPriceCents: number;
  mode: string;
  changeReason: string;
}

export interface WechatVirtualGoodUpdateInput extends RevisionReasonInput {
  channel?: PaymentChannel;
  environment?: PaymentEnvironment;
  offerId?: string;
  productId?: string;
  goodsName?: string;
  platformPriceCents?: number;
  mode?: string;
}

export interface WechatVirtualGoodConfirmationInput extends RevisionReasonInput {
  verificationReason: string;
  evidence?: string;
  verificationExpiresAt?: string;
}

export interface WechatVirtualGoodsResponse extends ListResponse<WechatVirtualGood> {
  verificationSource: string;
}

export interface WechatVirtualGoodConfirmationResponse extends ItemResponse<WechatVirtualGood> {
  confirmation: "LOCAL_MANUAL_ONLY" | string;
  wechatRealtimeVerified: false;
}

export interface WechatGoodReference {
  bindingId: string;
  pricePlanId: string;
  pricePlanCode: string;
  pricePlanName: string;
  planId: string;
  planName: string;
  isDefault: boolean;
  bindingStatus: string;
  bindingEnabled: boolean;
  salePriceCents: number;
  providerPriceSnapshotCents: number;
  channel: PaymentChannel;
  environment: PaymentEnvironment;
  wechatGoodId: string;
  quoteCount: number;
  orderCount: number;
}

export interface WechatGoodReferencesResponse extends ListResponse<WechatGoodReference> {}

export interface PricePlanPaymentBinding {
  id: string;
  pricePlanId: string;
  wechatGoodId: string;
  channel: PaymentChannel;
  environment: PaymentEnvironment;
  providerPriceSnapshotCents: number;
  enabled: boolean;
  status: string;
  revision: number;
  createdBy?: string;
  updatedBy?: string;
  enabledBy?: string;
  enabledAt?: string;
  disabledBy?: string;
  disabledAt?: string;
  createdAt: string;
  updatedAt: string;
  pricePlanSalePriceCents: number;
  wechatGoodPriceCents: number;
  wechatProductId: string;
  verificationStatus: string;
  priceConsistent: boolean;
  environmentConsistent: boolean;
}

export interface PaymentBindingCreateInput {
  wechatGoodId: string;
  changeReason: string;
}

export interface PaymentBindingRebindInput extends RevisionReasonInput {
  wechatGoodId: string;
}

export interface PaymentBindingTransitionInput extends RevisionReasonInput {
  enabled: boolean;
}

export interface PricePlanWhitelistEntry {
  whitelistEntryId: string;
  planId: string;
  pricePlanId: string;
  userId: string;
  status: WhitelistStatus;
  validFrom?: string;
  validUntil?: string;
  reason: string;
  revision: number;
  createdBy: string;
  updatedBy?: string;
  disabledBy?: string;
  disabledAt?: string;
  createdAt: string;
  updatedAt: string;
}

export interface WhitelistCreateInput {
  revision: 0;
  userId: string;
  reason: string;
  validFrom?: string;
  validUntil?: string;
  changeReason: string;
}

export interface WhitelistUpdateInput extends RevisionReasonInput {
  reason?: string;
  validFrom?: string;
  validUntil?: string;
  clearValidFrom?: boolean;
  clearValidUntil?: boolean;
}

export interface WhitelistDisableResponse extends ItemResponse<PricePlanWhitelistEntry> {
  alreadyDisabled: boolean;
}

export interface WhitelistFilters {
  status?: WhitelistStatus;
  userId?: string;
  page?: number;
  pageSize?: number;
}

export interface WhitelistPage extends ListResponse<PricePlanWhitelistEntry> {
  page: number;
  pageSize: number;
}

export type PricingAuditResult = "SUCCEEDED" | "FAILED";

export interface PricingAuditFilters {
  planId?: string;
  planVersionId?: string;
  pricePlanId?: string;
  wechatGoodId?: string;
  bindingId?: string;
  whitelistEntryId?: string;
  action?: string;
  operatorId?: string;
  operatorRole?: string;
  startTime?: string;
  endTime?: string;
  result?: PricingAuditResult;
  page?: number;
  pageSize?: number;
}

export interface PricingAuditLog {
  auditLogId: string;
  operatorId: string;
  operatorRole: string;
  operationTime: string;
  action: string;
  entityType: string;
  entityId: string;
  changeReason: string;
  beforeSnapshot: unknown | null;
  afterSnapshot: unknown | null;
  revisionBefore: number | null;
  revisionAfter: number | null;
  requestId: string;
  result: "SUCCEEDED" | "FAILED";
  errorCode?: string;
  planId?: string;
  planVersionId?: string;
  pricePlanId?: string;
  wechatGoodId?: string;
  bindingId?: string;
  whitelistEntryId?: string;
  environment?: string;
  metadata: Record<string, unknown>;
}

export interface PricingAuditPage extends ListResponse<PricingAuditLog> {
  page: number;
  pageSize: number;
}

export interface PricingHealthSummary {
  businessPlanCount: number;
  pricePlanCount: number;
  wechatGoodCount: number;
  issueCount: number;
  blockedIssueCount: number;
  degradedIssueCount: number;
  healthyResourceCount: number;
}

export interface PricingHealthIssue {
  code: string;
  severity: "WARNING" | "BLOCKING" | string;
  scope: string;
  planId?: string;
  pricePlanId?: string;
  paymentBindingId?: string;
  wechatGoodId?: string;
  environment?: PaymentEnvironment | string;
  message: string;
}

export interface PricingHealthDefaultSummary {
  pricePlanId: string;
  salePriceCents: number;
  currency: string;
  wechatGoodId?: string;
  wechatProductId?: string;
}

export interface PricingHealthBusinessPlan {
  planId: string;
  name: string;
  status: PricingHealthStatus;
  issueCodes: string[];
  activeVersionId?: string;
  pricePlanCount: number;
  defaults: {
    production?: PricingHealthDefaultSummary | null;
    sandbox?: PricingHealthDefaultSummary | null;
  };
}

export interface PricingHealthPricePlan {
  pricePlanId: string;
  planId: string;
  planVersionId: string;
  name: string;
  priceType: string;
  channel: string;
  environment: string;
  status: PricingHealthStatus;
  issueCodes: string[];
  salePriceCents: number;
  currency: string;
  paymentBindingId?: string;
  wechatGoodId?: string;
  wechatProductId?: string;
  quoteCount: number;
  orderCount: number;
}

export interface PricingHealthWechatGood {
  wechatGoodId: string;
  wechatProductId: string;
  environment: string;
  referenceCount: number;
}

export interface PricingHealthRuntime {
  pricePlanCreationEnabled: boolean;
  pricePlanTestEntryEnabled: boolean;
  snapshotV2FulfillmentEnabled: boolean;
  v132Blocked: boolean;
  v132Scope: string;
  v132AffectedTenantCount: number;
  v132AffectedTenantIds: string[];
}

export interface PricingHealth {
  checkedAt: string;
  status: PricingHealthStatus;
  summary: PricingHealthSummary;
  issues: PricingHealthIssue[];
  businessPlans: PricingHealthBusinessPlan[];
  pricePlans: PricingHealthPricePlan[];
  wechatGoods: PricingHealthWechatGood[];
  runtime: PricingHealthRuntime;
}
