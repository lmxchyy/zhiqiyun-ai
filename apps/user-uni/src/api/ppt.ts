import { api, getApiBaseURL, getAuthToken } from "./client";

export type PptTaskStatus = "pending" | "processing" | "success" | "failed";
export type PptLanguage = "zh" | "en";
export type PptTheme =
  | "business"
  | "techBlue"
  | "education"
  | "marketing"
  | "pitch"
  | "medical";

export interface PptGenerateRequest {
  prompt: string;
  slideCount: number;
  language: PptLanguage;
  theme: PptTheme;
  enableWebSearch: boolean;
  tone?: string;
  textContent?: string;
  audience?: string;
  scenario?: string;
  generationAspectRatio?: string;
  autoThemeEnabled?: boolean;
  imageSource?: "ai" | "stock" | "none";
  textModel?: string;
  imageModel?: string;
}

export interface PptGenerateResponse {
  taskId: string;
  status: PptTaskStatus;
}

export interface PptTaskResponse {
  taskId: string;
  status: PptTaskStatus;
  title: string;
  prompt?: string;
  slideCount?: number;
  language?: PptLanguage;
  theme?: PptTheme;
  enableWebSearch?: boolean;
  pptUrl: string;
  pdfUrl: string;
  errorMessage: string;
  createdAt?: string;
  updatedAt?: string;
}

export type PptHistoryItem = PptTaskResponse;

const pptEndpoints = {
  create: "/api/v1/ppt/generate",
  task: (taskId: string) => `/api/v1/ppt/tasks/${encodeURIComponent(taskId)}`,
  history: "/api/v1/ppt/history",
  exportPptx: "/api/v1/ppt/export/pptx"
};

export async function createPptGenerationTask(request: PptGenerateRequest): Promise<PptGenerateResponse> {
  const response = await api<PptGenerateResponse>(pptEndpoints.create, {
    method: "POST",
    body: JSON.stringify({
      prompt: request.prompt,
      slideCount: request.slideCount,
      language: request.language,
      theme: request.theme,
      enableWebSearch: request.enableWebSearch,
      tone: request.tone || "professional",
      textContent: request.textContent || "concise",
      audience: request.audience || "auto",
      scenario: request.scenario || "auto",
      generationAspectRatio: request.generationAspectRatio || "dynamic",
      autoThemeEnabled: request.autoThemeEnabled ?? true,
      imageSource: request.imageSource || "ai",
      textModel: request.textModel,
      imageModel: request.imageModel || "default-image"
    })
  });
  return { taskId: response.taskId, status: normalizeStatus(response.status) };
}

export async function getPptGenerationTask(taskId: string): Promise<PptTaskResponse> {
  return normalizePptTask(await api<PptTaskResponse>(pptEndpoints.task(taskId)));
}

export async function listPptHistory(): Promise<PptHistoryItem[]> {
  const response = await api<PptHistoryItem[] | { items?: PptHistoryItem[]; rows?: PptHistoryItem[]; data?: PptHistoryItem[] }>(pptEndpoints.history);
  const items = Array.isArray(response) ? response : response.items || response.rows || response.data || [];
  return items.map(normalizePptTask);
}

export async function deletePptTask(taskId: string): Promise<void> {
  await api<{ ok: boolean }>(pptEndpoints.task(taskId), { method: "DELETE" });
}

export async function requestPptDownload(taskId: string): Promise<{ url: string }> {
  const task = await getPptGenerationTask(taskId);
  if (!task.pptUrl) {
    return { url: await exportPptxTask(taskId) };
  }
  return { url: task.pptUrl };
}

function normalizeStatus(value: unknown): PptTaskStatus {
  const status = String(value || "pending").toLowerCase();
  if (status === "processing" || status === "success" || status === "failed") return status;
  return "pending";
}

function normalizePptTask(task: PptHistoryItem): PptHistoryItem {
  return {
    ...task,
    taskId: String(task.taskId || ""),
    status: normalizeStatus(task.status),
    title: task.title || task.prompt || "未命名PPT",
    pptUrl: task.pptUrl || "",
    pdfUrl: task.pdfUrl || "",
    errorMessage: task.errorMessage || ""
  };
}

async function exportPptxTask(taskId: string) {
  if (typeof fetch !== "function" || typeof URL === "undefined") {
    throw new Error("当前运行环境不支持直接导出 PPT，请在 H5 工作台下载。");
  }
  const base = getApiBaseURL();
  const response = await fetch(`${base}${pptEndpoints.exportPptx}`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Authorization: `Bearer ${getAuthToken()}`
    },
    body: JSON.stringify({ taskId })
  });
  if (!response.ok) {
    let message = "PPT 导出失败";
    try {
      const payload = await response.json() as { error?: string; message?: string };
      message = payload.error || payload.message || message;
    } catch {
      const text = await response.text().catch(() => "");
      if (text) message = text;
    }
    throw new Error(message);
  }
  const blob = await response.blob();
  return URL.createObjectURL(blob);
}
