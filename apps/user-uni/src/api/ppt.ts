import { api, apiFetchResponse } from "./client";
import { downloadTemporaryFile } from "./files";

export type PptTaskStatus = "pending" | "processing" | "success" | "failed";
export type PptGenerationStage = "analyzing" | "outline" | "content" | "visual" | "layout" | "preview" | "export" | "completed" | "failed" | "cancelled";
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
  outline?: PptOutline;
}

export interface PptCreateForm {
  topic: string;
  description?: string;
  pageCount: number;
  language: PptLanguage;
  scenario: string;
  style: string;
  templateId?: string;
  referenceFileIds: string[];
  knowledgeBaseIds: string[];
  generateSpeakerNotes: boolean;
  generateVisuals: boolean;
}

export interface PptOutlineItem {
  id: string;
  slideIndex: number;
  slideType: string;
  title: string;
  description?: string;
  keyMessage?: string;
  bulletPoints: string[];
  layout: string;
}

export interface PptOutline {
  title: string;
  slides: Array<{
    page: number;
    title: string;
    summary: string;
    bulletPoints: string[];
    layout?: string;
    slideType?: string;
  }>;
  updatedAt?: string;
}

export interface PptCostEstimate {
  pointCost: number;
  slideCount: number;
  model: string;
  sufficient?: boolean;
  availablePoints?: number;
}

export interface PptGenerateResponse {
  taskId: string;
  status: PptTaskStatus;
}

export interface PptVisualPlan {
  visualType: string;
  imageRequired: boolean;
  chartRequired: boolean;
  diagramRequired: boolean;
  textInImage: false;
  subject: string;
  scene: string;
  action: string;
  objects: string[];
  mood: string;
  composition: string;
  style: string;
  prompt: string;
  negativePrompt: string;
}

export interface PptVisualAsset {
  url: string;
  storageRef?: string;
  taskId?: string;
  modelName?: string;
  createdAt: string;
}

export interface PptSlide {
  id: string;
  page: number;
  title: string;
  content: string;
  bulletPoints: string[];
  imageUrl?: string;
  visualStorageRef?: string;
  layout: string;
  slideType?: string;
  visualPlan?: PptVisualPlan;
  visualHistory?: PptVisualAsset[];
  visualTaskId?: string;
  visualModelName?: string;
  visualCreatedAt?: string;
  visualStatus?: string;
  visualError?: string;
  speakerNotes?: string;
  version?: number;
}

export interface PptRegenerateVisualRequest {
  visualType: string;
  style: string;
  composition: string;
  customInstruction: string;
  keepCurrentContent: true;
}

export interface PptRegenerateVisualResponse {
  taskId?: string;
  status: string;
  slide: PptSlide;
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
  slides?: PptSlide[];
  pptUrl: string;
  pdfUrl: string;
  errorMessage: string;
  createdAt?: string;
  updatedAt?: string;
  progress?: number;
  currentPage?: number;
  stage?: PptGenerationStage;
  outline?: PptOutline;
  version?: number;
}

export type PptHistoryItem = PptTaskResponse;

const pptEndpoints = {
  create: "/api/v1/ppt/generate",
  estimate: "/api/v1/ppt/estimate",
  outlineGenerate: "/api/v1/ppt/outline/generate",
  outlineSave: "/api/v1/ppt/outline/save",
  task: (taskId: string) => `/api/v1/ppt/tasks/${encodeURIComponent(taskId)}`,
  history: "/api/v1/ppt/history",
  exportPptx: "/api/v1/ppt/export/pptx",
  downloadPptx: (taskId: string) => `/api/v1/ppt/tasks/${encodeURIComponent(taskId)}/export/pptx`,
  exportPdf: "/api/v1/ppt/export/pdf",
  updateSlide: (taskId: string, slideId: string) => `/api/v1/presentations/${encodeURIComponent(taskId)}/slides/${encodeURIComponent(slideId)}`,
  updateSlideImage: (taskId: string, slideId: string) => `/api/v1/presentations/${encodeURIComponent(taskId)}/slides/${encodeURIComponent(slideId)}/visual`,
  regenerateVisual: (taskId: string, slideId: string) => `/api/v1/presentations/${encodeURIComponent(taskId)}/slides/${encodeURIComponent(slideId)}/regenerate-visual`,
  deleteVisual: (taskId: string, slideId: string) => `/api/v1/presentations/${encodeURIComponent(taskId)}/slides/${encodeURIComponent(slideId)}/visual`,
  restoreVisual: (taskId: string, slideId: string) => `/api/v1/presentations/${encodeURIComponent(taskId)}/slides/${encodeURIComponent(slideId)}/visual/restore`
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
      imageModel: request.imageModel || "default-image",
      outline: request.outline
    })
  });
  return { taskId: response.taskId, status: normalizeStatus(response.status) };
}

export async function estimatePptCost(request: Pick<PptGenerateRequest, "prompt" | "slideCount" | "textModel" | "imageSource">): Promise<PptCostEstimate> {
  return api<PptCostEstimate>(pptEndpoints.estimate, {
    method: "POST",
    body: JSON.stringify(request)
  });
}

export async function generatePptOutline(request: PptGenerateRequest): Promise<PptOutline> {
  return api<PptOutline>(pptEndpoints.outlineGenerate, {
    method: "POST",
    body: JSON.stringify({
      prompt: request.prompt,
      slideCount: request.slideCount,
      language: request.language,
      tone: request.tone || "professional",
      textContent: request.textContent || "concise",
      audience: request.audience || "auto",
      scenario: request.scenario || "auto",
      generationAspectRatio: request.generationAspectRatio || "dynamic",
      autoThemeEnabled: request.autoThemeEnabled ?? true,
      enableWebSearch: request.enableWebSearch,
      imageSource: request.imageSource || "ai",
      textModel: request.textModel,
      imageModel: request.imageModel || "default-image"
    })
  });
}

export async function savePptOutline(outline: PptOutline): Promise<PptOutline> {
  return api<PptOutline>(pptEndpoints.outlineSave, { method: "POST", body: JSON.stringify(outline) });
}

export async function updatePptSlide(taskId: string, slideId: string, slide: PptSlide): Promise<PptSlide> {
  return api<PptSlide>(pptEndpoints.updateSlide(taskId, slideId), { method: "PATCH", body: JSON.stringify(slide) });
}

export async function updatePptSlideImage(taskId: string, slideId: string, imageUrl: string): Promise<PptSlide> {
  return api<PptSlide>(pptEndpoints.updateSlideImage(taskId, slideId), { method: "PATCH", body: JSON.stringify({ imageUrl }) });
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

export async function regeneratePptSlideVisual(taskId: string, slideId: string, request: PptRegenerateVisualRequest): Promise<PptRegenerateVisualResponse> {
  return api<PptRegenerateVisualResponse>(pptEndpoints.regenerateVisual(taskId, slideId), {
    method: "POST",
    body: JSON.stringify(request)
  });
}

export async function deletePptSlideVisual(taskId: string, slideId: string): Promise<PptRegenerateVisualResponse> {
  return api<PptRegenerateVisualResponse>(pptEndpoints.deleteVisual(taskId, slideId), { method: "DELETE" });
}

export async function restorePptSlideVisual(taskId: string, slideId: string, createdAt: string, url: string, storageRef?: string): Promise<PptRegenerateVisualResponse> {
  return api<PptRegenerateVisualResponse>(pptEndpoints.restoreVisual(taskId, slideId), {
    method: "POST",
    body: JSON.stringify({ createdAt, url, storageRef })
  });
}

export async function requestPptDownload(taskId: string): Promise<{ url: string }> {
  const task = await getPptGenerationTask(taskId);
  if (!task.pptUrl) {
    return { url: await exportPptxTask(taskId) };
  }
  return { url: task.pptUrl };
}

export async function downloadPptExport(taskId: string, format: "pptx" | "pdf") {
  const endpoint = format === "pdf" ? pptEndpoints.exportPdf : pptEndpoints.exportPptx;
  // MP-WEIXIN cannot consume browser Blob URLs; the backend should return a temporary URL there.
  // #ifdef MP-WEIXIN || APP-PLUS
  if (format === "pptx") return downloadTemporaryFile(pptEndpoints.downloadPptx(taskId));
  const payload = await api<{ url?: string; downloadUrl?: string }>(endpoint, { method: "POST", body: JSON.stringify({ taskId, responseMode: "url" }) });
  const url = String(payload.url || payload.downloadUrl || "");
  if (!url) throw new Error("导出服务尚未返回小程序临时下载链接");
  return downloadTemporaryFile(url);
  // #endif
  // #ifndef MP-WEIXIN
  const response = await apiFetchResponse(endpoint, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ taskId }) });
  const blob = await response.blob();
  return URL.createObjectURL(blob);
  // #endif
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
    slides: Array.isArray(task.slides) ? task.slides : [],
    pptUrl: task.pptUrl || "",
    pdfUrl: task.pdfUrl || "",
    errorMessage: task.errorMessage || ""
  };
}

async function exportPptxTask(taskId: string) {
  if (typeof fetch !== "function" || typeof URL === "undefined") {
    throw new Error("当前运行环境不支持直接导出 PPT，请在 H5 工作台下载。");
  }
  const response = await apiFetchResponse(pptEndpoints.exportPptx, {
    method: "POST",
    headers: {
      "Content-Type": "application/json"
    },
    body: JSON.stringify({ taskId })
  });
  const blob = await response.blob();
  return URL.createObjectURL(blob);
}
