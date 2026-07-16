import { adminRequest } from "./client";

export type BillingRuleSource = "DATABASE" | "CODE_DEFAULT" | "PLAN_OVERRIDE" | "TENANT_OVERRIDE";

export interface BillingRuleValidationIssue {
  code: string;
  field?: string;
  severity: "ERROR" | "WARNING" | string;
  message: string;
}

export interface BillingRuleValidation {
  valid: boolean;
  validatedAt?: string;
  issues: BillingRuleValidationIssue[];
}

export interface BillingRuleVersion {
  id: string;
  ruleKey: string;
  legacyRuleId?: string;
  modelName: string;
  modelCode: string;
  moduleCode: string;
  billingUnit: string;
  basePrice: number;
  minimumCharge: number;
  parameterRules: Record<string, unknown>;
  ruleSource: BillingRuleSource;
  tenantId?: string;
  planId?: string;
  version: number;
  status: string;
  effectiveFrom?: string;
  effectiveTo?: string;
  validationResult: BillingRuleValidation;
  updatedAt: string;
  publishedAt?: string;
}

export interface ProviderCost {
  id: string;
  provider: string;
  channel: string;
  platformModelCode: string;
  upstreamModelName: string;
  billingUnit: string;
  parameterRange: Record<string, unknown>;
  unitCost: number;
  currency: string;
  effectiveFrom: string;
  effectiveTo?: string;
  status: string;
  updatedAt: string;
}

export interface BillingLifecycleEvent {
  id: string;
  taskId: string;
  userId?: string;
  tenantId?: string;
  modelCode?: string;
  eventType: string;
  billingStatus: string;
  points: number;
  ruleVersionId?: string;
  providerChannel?: string;
  idempotencyKey: string;
  metadata: Record<string, unknown>;
  createdAt: string;
}

export interface BillingReconciliationItem {
  taskId: string;
  userId: string;
  tenantId?: string;
  modelCode: string;
  taskStatus: string;
  billingStatus: string;
  quotedPoints: number;
  reservedPoints: number;
  capturedPoints: number;
  releasedPoints: number;
  refundedPoints: number;
  supplierCost?: number | null;
  estimatedMargin?: number | null;
  providerChannel?: string;
  ruleVersionId?: string;
  clientRequestId?: string;
  billingEventCount: number;
  walletLedgerCount: number;
  anomalies: string[];
  createdAt: string;
}

export interface WalletLedgerEntry {
  id: string;
  accountId: string;
  userId?: string;
  tenantId?: string;
  taskId?: string;
  billingEventId?: string;
  entryType: string;
  points: number;
  availableBefore: number;
  availableAfter: number;
  frozenBefore: number;
  frozenAfter: number;
  idempotencyKey: string;
  referenceType?: string;
  referenceId?: string;
  remark?: string;
  metadata: Record<string, unknown>;
  createdAt: string;
}

export interface BillingOverview {
  summary: {
    publishedRules: number;
    draftRules: number;
    providerCosts: number;
    tasks: number;
    abnormalTasks: number;
    walletEntries: number;
    estimatedMargin: number;
    marginTasks: number;
  };
  recentTasks: BillingReconciliationItem[];
  recentLedger: WalletLedgerEntry[];
}

export const billingApi = {
  overview: () => adminRequest<BillingOverview>({ method: "GET", url: "/admin/billing/overview" }),
  rules: () => adminRequest<{ items: BillingRuleVersion[]; total: number }>({ method: "GET", url: "/admin/billing/rules" }),
  rule: (id: string) => adminRequest<{ item: BillingRuleVersion }>({ method: "GET", url: `/admin/billing/rules/${id}` }),
  createRuleDraft: (id: string, payload: Record<string, unknown>) => adminRequest<{ item: BillingRuleVersion }>({ method: "PATCH", url: `/admin/billing/rules/${id}`, data: payload }),
  validateRule: (id: string) => adminRequest<{ validation: BillingRuleValidation }>({ method: "POST", url: `/admin/billing/rules/${id}/validate` }),
  publishRule: (id: string) => adminRequest<{ item: BillingRuleVersion }>({ method: "POST", url: `/admin/billing/rules/${id}/publish` }),
  providerCosts: () => adminRequest<{ items: ProviderCost[]; total: number }>({ method: "GET", url: "/admin/billing/provider-costs" }),
  updateProviderCost: (id: string, payload: Partial<ProviderCost>) => adminRequest<{ item: ProviderCost }>({ method: "PATCH", url: `/admin/billing/provider-costs/${id}`, data: payload }),
  events: () => adminRequest<{ summary: Record<string, unknown>; items: BillingLifecycleEvent[] }>({ method: "GET", url: "/admin/billing/events" }),
  reconciliation: () => adminRequest<{ summary: Record<string, unknown>; items: BillingReconciliationItem[] }>({ method: "GET", url: "/admin/billing/reconciliation" }),
  walletLedger: () => adminRequest<{ summary: Record<string, unknown>; items: WalletLedgerEntry[] }>({ method: "GET", url: "/admin/billing/wallet-ledger" })
};
