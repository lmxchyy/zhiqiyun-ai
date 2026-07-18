import { adminRequest } from "./client";

export type WorkspaceRecord = Record<string, any>;

export interface Customer360Response {
  profile: WorkspaceRecord;
  identity: WorkspaceRecord;
  wallet: WorkspaceRecord;
  summary: WorkspaceRecord;
  attribution: { item?: WorkspaceRecord };
  orders: WorkspaceRecord[];
  payments: WorkspaceRecord[];
  tokenRecords: WorkspaceRecord[];
  commissions: WorkspaceRecord[];
  billingEvents: WorkspaceRecord[];
  generationTasks: WorkspaceRecord[];
  mergeRequests: WorkspaceRecord[];
}

export interface OrderTimelineResponse {
  item: WorkspaceRecord;
  timeline: WorkspaceRecord[];
  payments: WorkspaceRecord[];
  paymentEvents: WorkspaceRecord[];
  tokenRecords: WorkspaceRecord[];
  commissions: WorkspaceRecord[];
}

export interface GlobalSearchItem {
  type: "customer" | "order" | "enterprise" | "generation_task" | "payment" | "invoice";
  recordId: string;
  title: string;
  description: string;
  module: "customers" | "orders" | "enterpriseList" | "aiCapabilityLogs" | "billingPayments" | "billingInvoices";
}

export interface ExceptionHistoryItem { action: string; actorId?: string; actorName?: string; from?: string; to?: string; note?: string; at: string }
export interface AdminExceptionCase {
  id: string; exceptionKey: string; title: string; description: string; module: string; severity: string; count: number; roles?: string[];
  assigneeId?: string; assigneeName?: string; status: "OPEN" | "IN_PROGRESS" | "RESOLVED" | "CLOSED"; slaDueAt?: string;
  firstDetectedAt: string; updatedAt: string; closedAt?: string; closeReason?: string; history?: ExceptionHistoryItem[];
}
export interface ExperienceAnalytics {
  days: number; totalEvents: number; eventCounts: Record<string, number>; moduleViews: Array<{ moduleId: string; count: number }>;
  lowFrequencyModules: Array<{ moduleId: string; count: number }>; taskCompletionRate: number; sampleReady: boolean;
  observedEvents: number; syntheticEvents: number; uniqueSessions: number; uniqueActors: number; activeDays: number;
  minimumEvents: number; minimumActiveDays: number;
}

const experienceSessionStorageKey = "xianzhi-admin-experience-session";

function experienceContext(metadata: Record<string, unknown>) {
  if (typeof window === "undefined") return metadata;
  let sessionId = window.sessionStorage.getItem(experienceSessionStorageKey) || "";
  if (!sessionId) {
    sessionId = typeof crypto !== "undefined" && typeof crypto.randomUUID === "function"
      ? crypto.randomUUID()
      : `session-${Date.now()}-${Math.random().toString(16).slice(2)}`;
    window.sessionStorage.setItem(experienceSessionStorageKey, sessionId);
  }
  const forcedMode = window.sessionStorage.getItem("xianzhi-admin-experience-mode");
  const synthetic = forcedMode === "synthetic" || window.navigator.webdriver === true;
  return {
    ...metadata,
    sessionId,
    clientSource: synthetic ? "automation" : "admin_ui",
    synthetic,
    pagePath: window.location.pathname
  };
}

export const adminWorkspaceApi = {
  customer360(userId: string) {
    return adminRequest<Customer360Response>({ method: "GET", url: `/admin/customers/${encodeURIComponent(userId)}/360` });
  },
  orderTimeline(orderId: string) {
    return adminRequest<OrderTimelineResponse>({ method: "GET", url: `/admin/orders/${encodeURIComponent(orderId)}/timeline` });
  },
  globalSearch(query: string) {
    return adminRequest<{ items: GlobalSearchItem[] }>({ method: "GET", url: "/admin/search", params: { q: query } });
  },
  updateException(id: string, data: Partial<AdminExceptionCase> & { note?: string }) {
    return adminRequest<{ item: AdminExceptionCase }>({ method: "PATCH", url: `/admin/exceptions/${encodeURIComponent(id)}`, data });
  },
  recordExperience(eventType: string, moduleId = "", targetId = "", metadata: Record<string, unknown> = {}) {
    return adminRequest<void>({ method: "POST", url: "/admin/experience-events", data: { eventType, moduleId, targetId, metadata: experienceContext(metadata) } });
  },
  experienceAnalytics(days = 30) {
    return adminRequest<ExperienceAnalytics>({ method: "GET", url: "/admin/experience-analytics", params: { days } });
  }
};
