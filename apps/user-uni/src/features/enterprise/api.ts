import type { AppRole, CurrentContextRequest, EnterpriseContext, EnterpriseContextsResponse } from "../../types";
import { api } from "../../api/client";
import { uploadReferenceImage } from "../../api/files";
import type {
  EnterpriseAIEmployee,
  EnterpriseAuditLog,
  EnterpriseBillingSummary,
  EnterpriseCertification,
  EnterpriseConnector,
  EnterpriseConnectorConfig,
  EnterpriseCreateResult,
  EnterpriseInvitation,
  EnterpriseJoinRequest,
  EnterpriseKnowledgeBase,
  EnterpriseMember,
  EnterpriseOrganization,
  EnterpriseOverview,
  EnterpriseRoleDefinition,
  ConnectorAITask,
  ConnectorMessageLog,
  ConnectorUserBinding,
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
  feishuConnector: () => api<EnterpriseConnector | { configured: false; config: EnterpriseConnectorConfig }>("/api/v1/enterprise/connectors/feishu"),
  saveFeishuConnector: (configured: boolean, input: { connectorName: string; appId: string; appSecret?: string; verificationToken?: string; encryptKey?: string; config: EnterpriseConnectorConfig }) => api<EnterpriseConnector>("/api/v1/enterprise/connectors/feishu", {
    method: configured ? "PUT" : "POST",
    body: JSON.stringify(input),
  }),
  testFeishuConnector: () => api<{ ok: boolean; connector: EnterpriseConnector }>("/api/v1/enterprise/connectors/feishu/test", { method: "POST" }),
  setFeishuConnectorEnabled: (enabled: boolean) => api<{ ok: boolean; connector: EnterpriseConnector }>(`/api/v1/enterprise/connectors/feishu/${enabled ? "enable" : "disable"}`, { method: "POST" }),
  feishuUsers: () => api<ItemsResponse<ConnectorUserBinding>>("/api/v1/enterprise/connectors/feishu/users?limit=200"),
  updateFeishuUser: (id: string, input: { internalUserId?: string; permission: Record<string, unknown>; status: "active" | "disabled" }) => api<ConnectorUserBinding>(`/api/v1/enterprise/connectors/feishu/users/${encodeURIComponent(id)}`, {
    method: "PUT",
    body: JSON.stringify(input),
  }),
  feishuTasks: () => api<ItemsResponse<ConnectorAITask>>("/api/v1/enterprise/connectors/feishu/tasks?limit=200"),
  feishuLogs: () => api<ItemsResponse<ConnectorMessageLog>>("/api/v1/enterprise/connectors/feishu/logs?limit=100"),
};

export async function uploadEnterpriseDocument(filePath: string): Promise<string> {
  try {
    return await uploadReferenceImage(filePath);
  } catch (error) {
    throw new Error(error instanceof Error ? error.message : "营业执照上传失败");
  }
}
