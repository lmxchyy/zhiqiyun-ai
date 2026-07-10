import { adminRequest } from "./client";

export type KnowledgeBase = {
  id: string;
  name: string;
  description: string;
  knowledgeType: "ENTERPRISE" | "DEPARTMENT" | "PERSONAL" | "AGENT";
  visibility: string;
  status: string;
  documentCount: number;
  chunkCount: number;
  ingestionProfileId?: string;
  retrievalProfileId?: string;
  version: number;
  categoryId?: string;
  logoObjectKey?: string;
  tags?: KnowledgeTag[];
  updatedAt: string;
};

export type KnowledgeDocument = {
  id: string;
  knowledgeBaseId: string;
  name: string;
  documentType: string;
  mimeType: string;
  status: string;
  updatedAt: string;
};

export type KnowledgeAgent = {
  id: string;
  name: string;
  description: string;
  modelName: string;
  systemPrompt: string;
  status: string;
  updatedAt: string;
};

export type KnowledgeBinding = {
  id: string;
  agentId: string;
  knowledgeBaseId: string;
  retrievalProfileId?: string;
  priority: number;
  weight: number;
  enabled: boolean;
};

export type KnowledgeConversation = {
  id: string;
  agentId: string;
  title: string;
  status: string;
};

export type KnowledgeCitation = {
  id: string;
  ragRunId: string;
  documentId: string;
  documentVersionId: string;
  chunkId: string;
  order: number;
  documentName: string;
  quote: string;
  locator?: Record<string, unknown>;
  similarityScore?: number;
};

export type KnowledgeRunResult = {
  run: { id: string; status: string; retrievalLatencyMs: number; generationLatencyMs: number };
  message: { id: string; role: string; content: string };
  citations: KnowledgeCitation[];
};

export type KnowledgeStreamEvent = {
  id?: string;
  ragRunId?: string;
  sequence?: number;
  event?: string;
  data?: Record<string, unknown>;
  error?: string;
};

export type KnowledgeProfile = { id: string; name: string; status: string; [key: string]: unknown };
export type KnowledgeTag = { id: string; name: string; color?: string };
export type KnowledgeCategory = { id: string; name: string; parentId?: string; sortOrder: number };

type ListResponse<T> = { items: T[]; nextCursor?: string };

export const knowledgeAPI = {
  listBases: () => adminRequest<ListResponse<KnowledgeBase>>({ method: "GET", url: "/knowledge-bases", params: { limit: 100 } }),
  createBase: (payload: { name: string; description?: string; knowledgeType: KnowledgeBase["knowledgeType"]; visibility?: string; categoryId?: string; logoObjectKey?: string; tagIds?: string[] }) =>
    adminRequest<KnowledgeBase>({ method: "POST", url: "/knowledge-bases", data: payload }),
  updateBase: (knowledgeBaseId: string, payload: Partial<KnowledgeBase> & { expectedVersion: number }) =>
    adminRequest<KnowledgeBase>({ method: "PATCH", url: `/knowledge-bases/${knowledgeBaseId}`, data: payload }),
  listProfiles: (resource: "ingestion-profiles" | "retrieval-profiles") =>
    adminRequest<ListResponse<KnowledgeProfile>>({ method: "GET", url: `/knowledge/profiles/${resource}` }),
  listTags: () => adminRequest<ListResponse<KnowledgeTag>>({ method: "GET", url: "/knowledge/tags" }),
  createTag: (name: string, color = "") => adminRequest<KnowledgeTag>({ method: "POST", url: "/knowledge/tags", data: { name, color } }),
  listCategories: () => adminRequest<ListResponse<KnowledgeCategory>>({ method: "GET", url: "/knowledge/categories" }),
  createCategory: (name: string) => adminRequest<KnowledgeCategory>({ method: "POST", url: "/knowledge/categories", data: { name, sortOrder: 100 } }),
  listDocuments: (knowledgeBaseId: string) =>
    adminRequest<ListResponse<KnowledgeDocument>>({ method: "GET", url: `/knowledge-bases/${knowledgeBaseId}/documents`, params: { limit: 100 } }),
  deleteDocument: (documentId: string) =>
    adminRequest<void>({ method: "DELETE", url: `/knowledge-documents/${documentId}` }),
  uploadDocument: (knowledgeBaseId: string, file: File, chunkerKey = "heading") => {
    const body = new FormData();
    body.append("file", file);
    body.append("name", file.name);
    body.append("mimeType", file.type || mimeTypeForFile(file.name));
    body.append("chunkerKey", chunkerKey);
    body.append("chunkSize", "800");
    body.append("overlap", "100");
    return adminRequest<{ document: KnowledgeDocument }>({ method: "POST", url: `/knowledge-bases/${knowledgeBaseId}/documents:ingest`, data: body });
  },
  listAgents: () => adminRequest<ListResponse<KnowledgeAgent>>({ method: "GET", url: "/knowledge-agents", params: { limit: 100 } }),
  getAgent: (agentId: string) =>
    adminRequest<{ agent: KnowledgeAgent; knowledgeBindings: KnowledgeBinding[] }>({ method: "GET", url: `/knowledge-agents/${agentId}` }),
  createAgent: (payload: { name: string; description?: string; modelName?: string; systemPrompt?: string; status?: string }) =>
    adminRequest<KnowledgeAgent>({ method: "POST", url: "/knowledge-agents", data: payload }),
  replaceBindings: (agentId: string, bindings: Array<string | { knowledgeBaseId: string; priority?: number; weight?: number; enabled?: boolean; retrievalProfileId?: string; retrievalOverrides?: Record<string, unknown> }>) =>
    adminRequest<ListResponse<KnowledgeBinding>>({
      method: "PUT",
      url: `/knowledge-agents/${agentId}/knowledge-bindings`,
      data: { items: bindings.map((item, index) => typeof item === "string" ? { knowledgeBaseId: item, priority: 100 - index, weight: 1, enabled: true } : item) }
    }),
  createConversation: (agentId: string, title: string) =>
    adminRequest<KnowledgeConversation>({ method: "POST", url: "/knowledge-conversations", data: { agentId, title } }),
  cancelRun: (runId: string) => adminRequest({ method: "POST", url: `/knowledge-runs/${runId}/cancel` })
};

export async function streamKnowledgeRun(
  conversationId: string,
  payload: { question: string; topK?: number; threshold?: number; mode?: string },
  onEvent: (eventName: string, value: KnowledgeStreamEvent | KnowledgeRunResult) => void,
  signal?: AbortSignal
) {
  const token = localStorage.getItem("token") || sessionStorage.getItem("token");
  const response = await fetch(`/api/v1/knowledge-conversations/${conversationId}/runs:stream`, {
    method: "POST",
    headers: {
      Accept: "text/event-stream",
      "Content-Type": "application/json",
      ...(token ? { Authorization: `Bearer ${token}` } : {})
    },
    body: JSON.stringify(payload),
    signal
  });
  if (!response.ok || !response.body) {
    let message = `请求失败 (${response.status})`;
    try {
      const body = await response.json();
      message = body.error || body.message || message;
    } catch {
      // Keep the status-based message when the body is not JSON.
    }
    throw new Error(message);
  }
  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  while (true) {
    const { value, done } = await reader.read();
    buffer += decoder.decode(value, { stream: !done });
    let boundary = buffer.indexOf("\n\n");
    while (boundary >= 0) {
      const block = buffer.slice(0, boundary);
      buffer = buffer.slice(boundary + 2);
      const eventName = block.split("\n").find((line) => line.startsWith("event:"))?.slice(6).trim() || "message";
      const data = block.split("\n").find((line) => line.startsWith("data:"))?.slice(5).trim();
      if (data) onEvent(eventName, JSON.parse(data));
      boundary = buffer.indexOf("\n\n");
    }
    if (done) break;
  }
}

function mimeTypeForFile(name: string) {
  const extension = name.split(".").pop()?.toLowerCase();
  if (extension === "md" || extension === "markdown") return "text/markdown";
  if (extension === "txt") return "text/plain";
  if (extension === "csv") return "text/csv";
  if (extension === "html" || extension === "htm") return "text/html";
  return "application/octet-stream";
}
