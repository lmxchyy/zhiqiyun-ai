import { api, getApiBaseURL, getAuthToken } from "./client";

export type MiniKnowledgeAgent = { id: string; name: string; description: string; status: string; modelName: string };
export type MiniKnowledgeConversation = { id: string; agentId: string; title: string; status: string; updatedAt: string };
export type MiniKnowledgeMessage = { id: string; role: "user" | "assistant"; content: string; createdAt: string; metadata?: Record<string, unknown> };
export type MiniKnowledgeCitation = { id: string; order: number; documentName: string; documentId: string; chunkId: string; quote: string; locator?: Record<string, unknown>; similarityScore?: number };
export type MiniKnowledgeRunResult = { run: { id: string; status: string }; message: MiniKnowledgeMessage; citations: MiniKnowledgeCitation[] };
export type MiniKnowledgeStreamEvent = { ragRunId?: string; event?: string; data?: Record<string, unknown>; error?: string };
export type KnowledgeRunHandle = { promise: Promise<MiniKnowledgeRunResult>; abort: () => void };

type ListResponse<T> = { items: T[] };

export const miniKnowledgeAPI = {
  agents: () => api<ListResponse<MiniKnowledgeAgent>>("/api/v1/knowledge-agents?limit=100"),
  conversations: (agentId = "") => api<ListResponse<MiniKnowledgeConversation>>(`/api/v1/knowledge-conversations?limit=100${agentId ? `&agentId=${encodeURIComponent(agentId)}` : ""}`),
  messages: (conversationId: string) => api<ListResponse<MiniKnowledgeMessage>>(`/api/v1/knowledge-conversations/${conversationId}/messages?limit=200`),
  createConversation: (agentId: string, title: string) => api<MiniKnowledgeConversation>("/api/v1/knowledge-conversations", { method: "POST", body: JSON.stringify({ agentId, title }) }),
  citations: (runId: string) => api<ListResponse<MiniKnowledgeCitation>>(`/api/v1/knowledge-runs/${runId}/citations`),
  cancel: (runId: string) => api(`/api/v1/knowledge-runs/${runId}/cancel`, { method: "POST" }),
  retry: (runId: string) => api<MiniKnowledgeRunResult>(`/api/v1/knowledge-runs/${runId}/retry`, { method: "POST" })
};

export function startMiniKnowledgeRun(
  conversationId: string,
  question: string,
  onEvent: (name: string, value: MiniKnowledgeStreamEvent | MiniKnowledgeRunResult) => void
): KnowledgeRunHandle {
  // #ifdef H5
  return startH5KnowledgeRun(conversationId, question, onEvent);
  // #endif
  // #ifndef H5
  return startUniKnowledgeRun(conversationId, question, onEvent);
  // #endif
}

function startH5KnowledgeRun(
  conversationId: string,
  question: string,
  onEvent: (name: string, value: MiniKnowledgeStreamEvent | MiniKnowledgeRunResult) => void
): KnowledgeRunHandle {
  const controller = new AbortController();
  const baseURL = getApiBaseURL().replace(/\/+$/, "");
  const promise = (async () => {
    const token = getAuthToken();
    const response = await fetch(`${baseURL}/api/v1/knowledge-conversations/${conversationId}/runs:stream`, {
      method: "POST",
      headers: { Accept: "text/event-stream", "Content-Type": "application/json", ...(token ? { Authorization: `Bearer ${token}` } : {}) },
      body: JSON.stringify({ question, mode: "HYBRID", topK: 8, threshold: 0.2 }),
      signal: controller.signal
    });
    if (!response.ok || !response.body) throw new Error(`知识问答请求失败 (${response.status})`);
    const reader = response.body.getReader();
    const decoder = new TextDecoder();
    let buffer = "";
    let result: MiniKnowledgeRunResult | null = null;
    while (true) {
      const { value, done } = await reader.read();
      buffer += decoder.decode(value, { stream: !done });
      let boundary = buffer.indexOf("\n\n");
      while (boundary >= 0) {
        const block = buffer.slice(0, boundary);
        buffer = buffer.slice(boundary + 2);
        const name = block.split("\n").find(line => line.startsWith("event:"))?.slice(6).trim() || "message";
        const raw = block.split("\n").find(line => line.startsWith("data:"))?.slice(5).trim();
        if (raw) {
          const parsed = JSON.parse(raw) as MiniKnowledgeStreamEvent | MiniKnowledgeRunResult;
          onEvent(name, parsed);
          if (name === "result") result = parsed as MiniKnowledgeRunResult;
          if (name === "error") throw new Error((parsed as MiniKnowledgeStreamEvent).error || "知识问答生成失败");
        }
        boundary = buffer.indexOf("\n\n");
      }
      if (done) break;
    }
    if (!result) throw new Error("知识问答未返回结果");
    return result;
  })();
  return { promise, abort: () => controller.abort() };
}

function startUniKnowledgeRun(
  conversationId: string,
  question: string,
  onEvent: (name: string, value: MiniKnowledgeStreamEvent | MiniKnowledgeRunResult) => void
): KnowledgeRunHandle {
  let requestTask: UniApp.RequestTask | null = null;
  const baseURL = getApiBaseURL().replace(/\/+$/, "");
  const promise = new Promise<MiniKnowledgeRunResult>((resolve, reject) => {
    requestTask = uni.request({
      url: `${baseURL}/api/v1/knowledge-conversations/${conversationId}/runs`,
      method: "POST",
      header: { Accept: "application/json", Authorization: `Bearer ${getAuthToken()}` },
      data: { question, mode: "HYBRID", topK: 8, threshold: 0.2 },
      timeout: 600000,
      success(response) {
        if (response.statusCode < 200 || response.statusCode >= 300) {
          const body = response.data as { error?: string; message?: string } | undefined;
          reject(new Error(body?.error || body?.message || `知识问答请求失败 (${response.statusCode})`));
          return;
        }
        const result = response.data as MiniKnowledgeRunResult;
        onEvent("result", result);
        resolve(result);
      },
      fail(error) { reject(new Error(error.errMsg || "知识问答请求失败")); }
    });
  });
  return { promise, abort: () => requestTask?.abort() };
}
