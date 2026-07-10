import { adminRequest } from "./client";

export type KnowledgeAdminOverview = {
  tenantCount: number;
  knowledgeBaseCount: number;
  documentCount: number;
  chunkCount: number;
  readyDocumentCount: number;
  failedDocumentCount: number;
  agentCount: number;
  ragRunCount: number;
  completedRagRunCount: number;
  inputTokens: number;
  outputTokens: number;
  pointCost: number;
};

export type KnowledgeAdminRecord = Record<string, unknown>;

export const adminKnowledgeAPI = {
  overview: (tenantId = "") => adminRequest<KnowledgeAdminOverview>({ method: "GET", url: "/admin/knowledge/overview", params: tenantId ? { tenantId } : undefined }),
  records: (resource: string, tenantId = "", limit = 200) =>
    adminRequest<{ items: KnowledgeAdminRecord[] }>({ method: "GET", url: `/admin/knowledge/${resource}`, params: { tenantId: tenantId || undefined, limit } }),
  saveProfile: (resource: string, payload: KnowledgeAdminRecord, id = "") =>
    adminRequest<{ item: KnowledgeAdminRecord }>({ method: id ? "PATCH" : "POST", url: id ? `/admin/knowledge/${resource}/${id}` : `/admin/knowledge/${resource}`, data: payload })
};
