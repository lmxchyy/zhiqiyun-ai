import { api } from "./client";

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

const mockHistoryStorageKey = "xianzhi_ppt_generation_history";
const pptEndpoints = {
  create: "/api/ppt/generate",
  task: (taskId: string) => `/api/ppt/tasks/${encodeURIComponent(taskId)}`,
  history: "/api/ppt/history"
};

export async function createPptGenerationTask(request: PptGenerateRequest): Promise<PptGenerateResponse> {
  try {
    return await api<PptGenerateResponse>(pptEndpoints.create, {
      method: "POST",
      body: JSON.stringify(request)
    });
  } catch {
    return createMockPptTask(request);
  }
}

export async function getPptGenerationTask(taskId: string): Promise<PptTaskResponse> {
  try {
    return await api<PptTaskResponse>(pptEndpoints.task(taskId));
  } catch {
    return getMockPptTask(taskId);
  }
}

export async function listPptHistory(): Promise<PptHistoryItem[]> {
  try {
    return await api<PptHistoryItem[]>(pptEndpoints.history);
  } catch {
    return listMockPptHistory();
  }
}

export async function deletePptTask(taskId: string): Promise<void> {
  try {
    await api<{ ok: boolean }>(pptEndpoints.task(taskId), { method: "DELETE" });
  } catch {
    deleteMockPptTask(taskId);
  }
}

export async function requestPptDownload(taskId: string): Promise<{ url: string }> {
  const task = await getPptGenerationTask(taskId);
  if (!task.pptUrl) {
    throw new Error("PPT 下载接口已预留，当前任务暂无下载地址");
  }
  return { url: task.pptUrl };
}

function createMockPptTask(request: PptGenerateRequest): PptGenerateResponse {
  const now = new Date().toISOString();
  const taskId = `ppt_${Date.now()}_${Math.random().toString(36).slice(2, 8)}`;
  const item: PptHistoryItem = {
    taskId,
    status: "pending",
    title: normalizeTitle(request.prompt),
    prompt: request.prompt,
    slideCount: request.slideCount,
    language: request.language,
    theme: request.theme,
    enableWebSearch: request.enableWebSearch,
    pptUrl: "",
    pdfUrl: "",
    errorMessage: "",
    createdAt: now,
    updatedAt: now
  };
  writeMockHistory([item, ...readMockHistory().filter(record => record.taskId !== taskId)]);
  return { taskId, status: "pending" };
}

function getMockPptTask(taskId: string): PptTaskResponse {
  const history = readMockHistory();
  const index = history.findIndex(record => record.taskId === taskId);
  if (index < 0) {
    return {
      taskId,
      status: "failed",
      title: "未找到生成任务",
      pptUrl: "",
      pdfUrl: "",
      errorMessage: "任务不存在或已删除"
    };
  }
  const item = progressMockTask(history[index]);
  history[index] = item;
  writeMockHistory(history);
  return item;
}

function listMockPptHistory(): PptHistoryItem[] {
  const history = readMockHistory();
  const updated = history.map(progressMockTask);
  writeMockHistory(updated);
  return updated;
}

function deleteMockPptTask(taskId: string) {
  writeMockHistory(readMockHistory().filter(item => item.taskId !== taskId));
}

function progressMockTask(item: PptHistoryItem): PptHistoryItem {
  if (item.status === "success" || item.status === "failed") return item;
  const createdAt = item.createdAt ? Date.parse(item.createdAt) : Date.now();
  const elapsed = Date.now() - createdAt;
  const status: PptTaskStatus = elapsed > 2200 ? "success" : elapsed > 800 ? "processing" : "pending";
  return {
    ...item,
    status,
    updatedAt: new Date().toISOString()
  };
}

function readMockHistory(): PptHistoryItem[] {
  const raw = uni.getStorageSync(mockHistoryStorageKey);
  if (typeof raw !== "string" || !raw.trim()) {
    return seedMockHistory();
  }
  try {
    const parsed = JSON.parse(raw) as unknown;
    if (!Array.isArray(parsed)) return seedMockHistory();
    return parsed.filter(isPptHistoryItem);
  } catch {
    return seedMockHistory();
  }
}

function writeMockHistory(items: PptHistoryItem[]) {
  uni.setStorageSync(mockHistoryStorageKey, JSON.stringify(items.slice(0, 12)));
}

function seedMockHistory(): PptHistoryItem[] {
  const now = Date.now();
  return [
    {
      taskId: "mock_ppt_001",
      status: "success",
      title: "AI赋能企业营销增长方案",
      prompt: "AI赋能企业营销增长方案",
      slideCount: 10,
      language: "zh",
      theme: "business",
      enableWebSearch: false,
      pptUrl: "",
      pdfUrl: "",
      errorMessage: "",
      createdAt: new Date(now - 1000 * 60 * 46).toISOString(),
      updatedAt: new Date(now - 1000 * 60 * 43).toISOString()
    },
    {
      taskId: "mock_ppt_002",
      status: "processing",
      title: "短视频矩阵运营方案",
      prompt: "短视频矩阵运营方案",
      slideCount: 8,
      language: "zh",
      theme: "marketing",
      enableWebSearch: true,
      pptUrl: "",
      pdfUrl: "",
      errorMessage: "",
      createdAt: new Date(now - 1000 * 12).toISOString(),
      updatedAt: new Date(now - 1000 * 8).toISOString()
    },
    {
      taskId: "mock_ppt_003",
      status: "failed",
      title: "海外市场渠道复盘",
      prompt: "海外市场渠道复盘",
      slideCount: 15,
      language: "zh",
      theme: "pitch",
      enableWebSearch: false,
      pptUrl: "",
      pdfUrl: "",
      errorMessage: "mock 任务：等待后续接入真实生成服务",
      createdAt: new Date(now - 1000 * 60 * 120).toISOString(),
      updatedAt: new Date(now - 1000 * 60 * 118).toISOString()
    }
  ];
}

function normalizeTitle(prompt: string) {
  const title = prompt.trim().replace(/\s+/g, " ");
  return title ? title.slice(0, 60) : "未命名PPT";
}

function isPptHistoryItem(item: unknown): item is PptHistoryItem {
  if (!item || typeof item !== "object") return false;
  const record = item as Record<string, unknown>;
  return typeof record.taskId === "string" && typeof record.status === "string" && typeof record.title === "string";
}
