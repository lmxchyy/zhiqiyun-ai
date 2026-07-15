import type { AppRole, CurrentContextRequest, EnterpriseContext, EnterpriseContextsResponse } from "../../types";
import { api, getApiBaseURL, getAuthToken } from "../../api/client";
import type {
  EnterpriseAIEmployee,
  EnterpriseAuditLog,
  EnterpriseBillingSummary,
  EnterpriseCertification,
  EnterpriseCreateResult,
  EnterpriseInvitation,
  EnterpriseJoinRequest,
  EnterpriseKnowledgeBase,
  EnterpriseMember,
  EnterpriseOrganization,
  EnterpriseOverview,
  EnterpriseRoleDefinition,
  ItemsResponse,
} from "./types";

export const enterpriseAPI = {
  contexts: () => api<EnterpriseContextsResponse>("/api/v1/user/enterprise-contexts"),
  switchContext: (input: CurrentContextRequest) => api<EnterpriseContext>("/api/v1/user/current-context", {
    method: "POST",
    body: JSON.stringify(input),
  }),
  create: (name: string) => api<EnterpriseCreateResult>("/api/v1/enterprises", {
    method: "POST",
    body: JSON.stringify({ name }),
  }),
  overview: () => api<EnterpriseOverview>("/api/v1/enterprise/overview"),
  members: () => api<ItemsResponse<EnterpriseMember>>("/api/v1/enterprise/members"),
  member: (id: string) => api<EnterpriseMember>(`/api/v1/enterprise/members/${encodeURIComponent(id)}`),
  updateMember: (id: string, input: { primaryOrganizationId?: string; roles?: AppRole[]; dataScope?: string }) => api<EnterpriseMember>(`/api/v1/enterprise/members/${encodeURIComponent(id)}`, {
    method: "PATCH",
    body: JSON.stringify(input),
  }),
  disableMember: (id: string) => api<EnterpriseMember>(`/api/v1/enterprise/members/${encodeURIComponent(id)}/disable`, { method: "POST" }),
  removeMember: (id: string) => api<void>(`/api/v1/enterprise/members/${encodeURIComponent(id)}`, { method: "DELETE" }),
  createInvitation: (input: { invitedUserId?: string; invitedEmail?: string; defaultOrganizationId?: string; defaultRole?: AppRole; expiresInHours?: number }) => api<EnterpriseInvitation>("/api/v1/enterprise/invitations", {
    method: "POST",
    body: JSON.stringify(input),
  }),
  acceptInvitation: (invitationCode: string) => api<EnterpriseContext>("/api/v1/enterprise/invitations/accept", {
    method: "POST",
    body: JSON.stringify({ invitationCode }),
  }),
  createJoinRequest: (input: { tenantId: string; requestedOrganizationId?: string; reason?: string }) => api<EnterpriseJoinRequest>("/api/v1/enterprise/join-requests", {
    method: "POST",
    body: JSON.stringify(input),
  }),
  joinRequests: () => api<ItemsResponse<EnterpriseJoinRequest>>("/api/v1/enterprise/join-requests"),
  reviewJoinRequest: (id: string, approved: boolean, comment = "") => api<EnterpriseJoinRequest>(`/api/v1/enterprise/join-requests/${encodeURIComponent(id)}/${approved ? "approve" : "reject"}`, {
    method: "POST",
    body: JSON.stringify({ comment }),
  }),
  organizations: () => api<ItemsResponse<EnterpriseOrganization>>("/api/v1/enterprise/organizations/tree"),
  createOrganization: (input: { parentId?: string; organizationType?: string; name: string; metadata?: Record<string, unknown> }) => api<EnterpriseOrganization>("/api/v1/enterprise/organizations", {
    method: "POST",
    body: JSON.stringify(input),
  }),
  updateOrganization: (id: string, input: { name?: string; organizationType?: string; status?: string; metadata?: Record<string, unknown> }) => api<EnterpriseOrganization>(`/api/v1/enterprise/organizations/${encodeURIComponent(id)}`, {
    method: "PATCH",
    body: JSON.stringify(input),
  }),
  moveOrganization: (id: string, parentId: string) => api<EnterpriseOrganization>(`/api/v1/enterprise/organizations/${encodeURIComponent(id)}/move`, {
    method: "POST",
    body: JSON.stringify({ parentId }),
  }),
  deleteOrganization: (id: string) => api<void>(`/api/v1/enterprise/organizations/${encodeURIComponent(id)}`, { method: "DELETE" }),
  roles: () => api<ItemsResponse<EnterpriseRoleDefinition>>("/api/v1/enterprise/roles"),
  billing: () => api<EnterpriseBillingSummary>("/api/v1/enterprise/billing/summary"),
  submitCertification: (input: { legalName: string; unifiedSocialCreditCode: string; legalRepresentativeName?: string; documentUrls: string[]; metadata?: Record<string, unknown> }) => api<EnterpriseCertification>("/api/v1/enterprise/certifications", {
    method: "POST",
    body: JSON.stringify(input),
  }),
  auditLogs: (limit = 100) => api<ItemsResponse<EnterpriseAuditLog>>(`/api/v1/enterprise/audit-logs?limit=${limit}`),
  aiEmployees: () => api<ItemsResponse<EnterpriseAIEmployee>>("/api/v1/knowledge-agents?limit=100"),
  aiEmployee: (id: string) => api<{ agent: EnterpriseAIEmployee; knowledgeBindings?: unknown[] }>(`/api/v1/knowledge-agents/${encodeURIComponent(id)}`),
  knowledgeBases: () => api<ItemsResponse<EnterpriseKnowledgeBase>>("/api/v1/knowledge-bases?limit=100"),
  createAIEmployee: (input: { name: string; description: string; modelName: string; systemPrompt: string; status: string; config: Record<string, unknown> }) => api<EnterpriseAIEmployee>("/api/v1/knowledge-agents", {
    method: "POST",
    body: JSON.stringify(input),
  }),
  replaceAIEmployeeBindings: (id: string, knowledgeBaseIds: string[]) => api<ItemsResponse<unknown>>(`/api/v1/knowledge-agents/${encodeURIComponent(id)}/knowledge-bindings`, {
    method: "PUT",
    body: JSON.stringify({ items: knowledgeBaseIds.map((knowledgeBaseId, index) => ({ knowledgeBaseId, priority: 100 - index, weight: 1, enabled: true, retrievalOverrides: {} })) }),
  }),
};

export function uploadEnterpriseDocument(filePath: string): Promise<string> {
  return new Promise((resolve, reject) => {
    uni.uploadFile({
      url: `${getApiBaseURL().replace(/\/+$/, "")}/api/v1/reference-images`,
      filePath,
      name: "file",
      header: { Authorization: `Bearer ${getAuthToken()}` },
      success(response) {
        if (response.statusCode < 200 || response.statusCode >= 300) {
          reject(new Error(`营业执照上传失败（${response.statusCode}）`));
          return;
        }
        try {
          const payload = JSON.parse(response.data || "{}") as Record<string, unknown>;
          const item = payload.item && typeof payload.item === "object" ? payload.item as Record<string, unknown> : payload;
          const value = String(item.url || item.path || "").trim();
          if (!value) throw new Error("上传接口未返回文件地址");
          resolve(value);
        } catch (error) {
          reject(error instanceof Error ? error : new Error("营业执照上传响应无效"));
        }
      },
      fail(error) {
        reject(new Error(error.errMsg || "营业执照上传失败"));
      },
    });
  });
}
