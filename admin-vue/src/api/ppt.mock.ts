import type {
  PptGenerateOutlineRequest,
  PptGenerateRequest,
  PptGenerateResponse,
  PptHistoryItem,
  PptImageOption,
  PptModelOption,
  PptOutline,
  PptOutlineSlide,
  PptSlide,
  PptSlideLayout,
  PptTaskResponse,
  PptTaskStatus,
  PptTheme
} from "../types/ppt";

const mockHistoryStorageKey = "xianzhi_admin_ppt_generation_history";

const titleSeeds = ["行业背景", "痛点分析", "解决方案", "落地路径", "价值收益", "案例展示", "行动计划", "总结展望"];
const mockImagePalettes = [
  ["#08111f", "#2563eb", "#38bdf8"],
  ["#0f172a", "#16a34a", "#86efac"],
  ["#111827", "#8b5cf6", "#22d3ee"],
  ["#fff7ed", "#f97316", "#be123c"],
  ["#0e7490", "#ecfeff", "#22c55e"]
];

export const mockTextModels: PptModelOption[] = [
  { label: "Kimi K2.6", value: "kimi-k2.6", provider: "Moonshot/Kimi", providerType: "openai", group: "当前后台配置", description: "通过后台 OpenAI 兼容网关生成 PPT 大纲" },
  { label: "GPT-4o-mini", value: "gpt-4o-mini", provider: "OpenAI", providerType: "openai", group: "云模型", description: "快速生成日常演示草稿" },
  { label: "GPT-4o", value: "gpt-4o", provider: "OpenAI", providerType: "openai", group: "云模型", description: "兼顾速度和质量的云模型" },
  { label: "GPT-4.1-mini", value: "gpt-4.1-mini", provider: "OpenAI", providerType: "openai", group: "云模型", description: "适合结构化生成的轻量模型" },
  { label: "GPT-4.1-nano", value: "gpt-4.1-nano", provider: "OpenAI", providerType: "openai", group: "云模型", description: "更快、更低成本的 GPT-4.1 模型" },
  { label: "GPT-4.1", value: "gpt-4.1", provider: "OpenAI", providerType: "openai", group: "云模型", description: "复杂演示的高质量生成模型" },
  { label: "GPT-5.2", value: "gpt-5.2", provider: "OpenAI", providerType: "openai", group: "云模型", description: "旗舰 GPT 模型" },
  { label: "GPT-5.2 Chat", value: "gpt-5.2-chat-latest", provider: "OpenAI", providerType: "openai", group: "云模型", description: "ChatGPT 风格的 GPT-5.2 模型" },
  { label: "GPT-5.2 Pro", value: "gpt-5.2-pro", provider: "OpenAI", providerType: "openai", group: "云模型", description: "适合更高难度任务的高算力模型" },
  { label: "GPT-5.1", value: "gpt-5.1", provider: "OpenAI", providerType: "openai", group: "云模型", description: "支持复杂推理的旗舰模型" },
  { label: "GPT-5", value: "gpt-5", provider: "OpenAI", providerType: "openai", group: "云模型", description: "上一代 GPT-5 推理模型" },
  { label: "GPT-5-mini", value: "gpt-5-mini", provider: "OpenAI", providerType: "openai", group: "云模型", description: "更快、更经济的 GPT-5 模型" },
  { label: "GPT-5-nano", value: "gpt-5-nano", provider: "OpenAI", providerType: "openai", group: "云模型", description: "最快、最低成本的 GPT-5 模型" },
  { label: "DeepSeek-V3", value: "deepseek-v3", provider: "NewAPI", providerType: "newapi", group: "平台模型", description: "通过当前平台网关接入的 DeepSeek 文本模型" },
  { label: "Qwen-Max", value: "qwen-max", provider: "NewAPI", providerType: "newapi", group: "平台模型", description: "通过当前平台网关接入的通义文本模型" },
  { label: "Doubao Pro", value: "doubao-pro", provider: "NewAPI", providerType: "newapi", group: "平台模型", description: "通过当前平台网关接入的火山文本模型" },
  { label: "llama3.1:8b", value: "ollama:llama3.1:8b", provider: "Ollama", providerType: "ollama", group: "可下载 Ollama 模型", description: "参考项目的本地模型建议，首次使用时可由后端拉取", downloadable: true },
  { label: "llama3.1:70b", value: "ollama:llama3.1:70b", provider: "Ollama", providerType: "ollama", group: "可下载 Ollama 模型", description: "参考项目的本地模型建议，适合更强本地推理", downloadable: true },
  { label: "llama3.2:3b", value: "ollama:llama3.2:3b", provider: "Ollama", providerType: "ollama", group: "可下载 Ollama 模型", description: "参考项目的轻量本地模型建议", downloadable: true },
  { label: "llama3.2:8b", value: "ollama:llama3.2:8b", provider: "Ollama", providerType: "ollama", group: "可下载 Ollama 模型", description: "参考项目的通用本地模型建议", downloadable: true },
  { label: "mistral:7b", value: "ollama:mistral:7b", provider: "Ollama", providerType: "ollama", group: "可下载 Ollama 模型", description: "参考项目的本地模型建议", downloadable: true },
  { label: "codellama:7b", value: "ollama:codellama:7b", provider: "Ollama", providerType: "ollama", group: "可下载 Ollama 模型", description: "参考项目的代码和结构化内容本地模型建议", downloadable: true },
  { label: "qwen2.5:7b", value: "ollama:qwen2.5:7b", provider: "Ollama", providerType: "ollama", group: "可下载 Ollama 模型", description: "参考项目的中文友好本地模型建议", downloadable: true },
  { label: "gemma2:9b", value: "ollama:gemma2:9b", provider: "Ollama", providerType: "ollama", group: "可下载 Ollama 模型", description: "参考项目的高性价比本地模型建议", downloadable: true },
  { label: "phi3:3.8b", value: "ollama:phi3:3.8b", provider: "Ollama", providerType: "ollama", group: "可下载 Ollama 模型", description: "参考项目的小尺寸本地模型建议", downloadable: true },
  { label: "neural-chat:7b", value: "ollama:neural-chat:7b", provider: "Ollama", providerType: "ollama", group: "可下载 Ollama 模型", description: "参考项目的对话式本地模型建议", downloadable: true },
  { label: "LM Studio 本地模型", value: "lmstudio:local-model", provider: "LM Studio", providerType: "lmstudio", group: "本地 LM Studio 模型", description: "后端接入本地模型发现接口后会替换为真实模型列表" }
];

export const mockImageModels: PptModelOption[] = [
  { label: "系统默认图片模型", value: "default-image", provider: "system" },
  { label: "gpt-image-2", value: "gpt-image-2", provider: "OpenAI" },
  { label: "ComfyUI 工作流", value: "comfyui-ppt", provider: "ComfyUI" }
];

export async function mockGeneratePptOutline(request: PptGenerateOutlineRequest): Promise<PptOutline> {
  await wait(650);
  return createOutline(request.prompt, request.slideCount, request.imageSource);
}

export async function mockSavePptOutline(outline: PptOutline): Promise<PptOutline> {
  await wait(180);
  return { ...outline, updatedAt: new Date().toISOString() };
}

export async function mockCreatePptTask(request: PptGenerateRequest): Promise<PptGenerateResponse> {
  await wait(260);
  const taskId = `ppt_${Date.now()}_${Math.random().toString(36).slice(2, 8)}`;
  const now = new Date().toISOString();
  const outline = request.outline || createOutline(request.prompt, request.slideCount, request.imageSource);
  const item: PptHistoryItem = {
    taskId,
    type: "ppt",
    mediaType: "ppt",
    status: "pending",
    title: outline.title || normalizeTitle(request.prompt),
    prompt: request.prompt,
    slideCount: request.slideCount,
    language: request.language,
    tone: request.tone,
    textContent: request.textContent,
    audience: request.audience,
    scenario: request.scenario,
    generationAspectRatio: request.generationAspectRatio,
    theme: request.theme,
    autoThemeEnabled: request.autoThemeEnabled,
    imageSource: request.imageSource,
    textModel: request.textModel,
    imageModel: request.imageModel,
    enableWebSearch: request.enableWebSearch,
    progress: 0,
    currentPage: 0,
    outline,
    slides: outlineToSlides(outline, request.theme, request.imageSource),
    pptUrl: "",
    pdfUrl: "",
    errorMessage: "",
    createdAt: now,
    updatedAt: now
  };
  writeMockHistory([item, ...readMockHistory().filter(record => record.taskId !== taskId)]);
  return { taskId, status: "pending" };
}

export async function mockGetPptTaskStatus(taskId: string): Promise<PptTaskResponse> {
  await wait(220);
  const history = readMockHistory();
  const index = history.findIndex(record => record.taskId === taskId);
  if (index < 0) {
    return {
      taskId,
      type: "ppt",
      mediaType: "ppt",
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

export async function mockGetPptHistory(): Promise<PptHistoryItem[]> {
  await wait(180);
  const history = readMockHistory();
  const updated = (history.length ? history : seedMockHistory()).map(progressMockTask);
  writeMockHistory(updated);
  return updated;
}

export function mockGetPptDraftHistory(): PptHistoryItem[] {
  return readMockHistory().filter(item => item.status === "draft");
}

export async function mockUpsertPptDraft(item: PptHistoryItem): Promise<PptHistoryItem> {
  await wait(80);
  const now = new Date().toISOString();
  const history = readMockHistory();
  const previous = history.find(record => record.taskId === item.taskId);
  const draft: PptHistoryItem = {
    ...previous,
    ...item,
    status: "draft",
    type: "ppt",
    mediaType: "ppt",
    title: item.title || item.prompt || previous?.title || "无标题演示文稿",
    pptUrl: item.pptUrl || previous?.pptUrl || "",
    pdfUrl: item.pdfUrl || previous?.pdfUrl || "",
    errorMessage: "",
    createdAt: previous?.createdAt || item.createdAt || now,
    updatedAt: now
  };
  writeMockHistory([draft, ...history.filter(record => record.taskId !== draft.taskId)]);
  return draft;
}

export async function mockDeletePptTask(taskId: string): Promise<void> {
  await wait(120);
  writeMockHistory(readMockHistory().filter(item => item.taskId !== taskId));
}

export async function mockRegeneratePptSlide(slide: PptSlide): Promise<PptSlide> {
  await wait(420);
  return {
    ...slide,
    title: `${slide.title.replace(/（已优化）$/, "")}（已优化）`,
    content: `${slide.content}\n\n已根据当前主题重新整理表达，后续可接入真实单页重写接口。`
  };
}

export async function mockGeneratePptImage(slide: PptSlide): Promise<PptImageOption> {
  await wait(520);
  return {
    id: `ai_img_${slide.id}`,
    title: `${slide.title} 配图`,
    source: "ai",
    url: mockPptImageUrl(`${slide.title} AI配图`, mockImagePalettes[slide.page % mockImagePalettes.length])
  };
}

export async function mockSearchPptImages(keyword: string): Promise<PptImageOption[]> {
  await wait(360);
  return [1, 2, 3].map(index => ({
    id: `stock_${index}_${keyword}`,
    title: `${keyword || "PPT"} 图库图片 ${index}`,
    source: "stock",
    url: mockPptImageUrl(`${keyword || "PPT"} 图库 ${index}`, mockImagePalettes[(index + keyword.length) % mockImagePalettes.length])
  }));
}

function mockPptImageUrl(label: string, palette: string[]) {
  const [background, accent, highlight] = palette;
  const safeLabel = label.slice(0, 24);
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="960" height="540" viewBox="0 0 960 540">
    <defs>
      <linearGradient id="bg" x1="0" y1="0" x2="1" y2="1">
        <stop offset="0%" stop-color="${background}"/>
        <stop offset="58%" stop-color="${accent}"/>
        <stop offset="100%" stop-color="${highlight}"/>
      </linearGradient>
      <filter id="shadow" x="-20%" y="-20%" width="140%" height="140%">
        <feDropShadow dx="0" dy="22" stdDeviation="22" flood-color="#000" flood-opacity=".28"/>
      </filter>
    </defs>
    <rect width="960" height="540" rx="38" fill="url(#bg)"/>
    <circle cx="792" cy="92" r="108" fill="#fff" opacity=".16"/>
    <circle cx="116" cy="440" r="156" fill="#fff" opacity=".12"/>
    <rect x="92" y="106" width="454" height="292" rx="26" fill="#fff" opacity=".18" filter="url(#shadow)"/>
    <path d="M128 345 246 232l86 77 65-58 108 94Z" fill="#fff" opacity=".56"/>
    <circle cx="426" cy="176" r="36" fill="#fff" opacity=".62"/>
    <text x="92" y="456" fill="#fff" font-family="Arial, sans-serif" font-size="34" font-weight="700">${escapeSvgText(safeLabel)}</text>
    <text x="92" y="494" fill="#fff" opacity=".72" font-family="Arial, sans-serif" font-size="20">Zhiqiyun AI Mock Image</text>
  </svg>`;
  return `data:image/svg+xml;utf8,${encodeURIComponent(svg)}`;
}

function escapeSvgText(value: string) {
  return value.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
}

export function mockExportPpt(): never {
  throw new Error("PPTX 导出能力已预留，真实文件生成服务接入后可用");
}

export function mockExportPdf(): never {
  throw new Error("PDF 导出能力已预留，真实文件生成服务接入后可用");
}

function progressMockTask(item: PptHistoryItem): PptHistoryItem {
  if (item.status === "draft") return item;
  if (item.status === "success" || item.status === "failed") return item;
  const createdAt = item.createdAt ? Date.parse(item.createdAt) : Date.now();
  const elapsed = Date.now() - createdAt;
  let status: PptTaskStatus = "pending";
  let progress = 8;
  if (elapsed > 5200) {
    status = "success";
    progress = 100;
  } else if (elapsed > 3600) {
    status = "processing";
    progress = 90;
  } else if (elapsed > 1800) {
    status = "processing";
    progress = 70;
  } else if (elapsed > 700) {
    status = "processing";
    progress = 40;
  }
  const slideCount = item.slideCount || item.outline?.slides.length || 5;
  return {
    ...item,
    status,
    progress,
    currentPage: Math.min(slideCount, Math.max(1, Math.round((progress / 100) * slideCount))),
    updatedAt: new Date().toISOString()
  };
}

function readMockHistory(): PptHistoryItem[] {
  if (typeof window === "undefined") return seedMockHistory();
  const raw = window.localStorage.getItem(mockHistoryStorageKey);
  if (!raw) return seedMockHistory();
  try {
    const parsed = JSON.parse(raw) as unknown;
    if (!Array.isArray(parsed)) return seedMockHistory();
    const items = parsed.filter(isPptHistoryItem);
    return items.length ? items : seedMockHistory();
  } catch {
    return seedMockHistory();
  }
}

function writeMockHistory(items: PptHistoryItem[]) {
  if (typeof window === "undefined") return;
  window.localStorage.setItem(mockHistoryStorageKey, JSON.stringify(items.slice(0, 18)));
}

function seedMockHistory(): PptHistoryItem[] {
  const now = Date.now();
  const firstOutline = createOutline("生成端午由来的ppt", 5);
  const secondOutline = createOutline("无标题演示文稿", 5);
  return [
    seededHistoryItem("mock_reference_001", "生成端午由来的ppt", firstOutline, now - 1000 * 60 * 58),
    seededHistoryItem("mock_reference_002", "无标题演示文稿", secondOutline, now - 1000 * 60 * 63)
  ];
}

function seededHistoryItem(taskId: string, title: string, outline: PptOutline, createdAt: number): PptHistoryItem {
  return {
    taskId,
    type: "ppt",
    mediaType: "ppt",
    status: "success",
    title,
    prompt: title,
    slideCount: outline.slides.length,
    language: "zh",
    tone: "professional",
    textContent: "concise",
    audience: "auto",
    scenario: "auto",
    generationAspectRatio: "dynamic",
    theme: "business",
    autoThemeEnabled: true,
    imageSource: "ai",
    textModel: "gpt-4o-mini",
    imageModel: "default-image",
    enableWebSearch: false,
    progress: 100,
    currentPage: outline.slides.length,
    outline,
    slides: outlineToSlides(outline, "business"),
    pptUrl: "",
    pdfUrl: "",
    errorMessage: "",
    createdAt: new Date(createdAt).toISOString(),
    updatedAt: new Date(createdAt + 1000 * 60 * 2).toISOString()
  };
}

function createOutline(prompt: string, slideCount: number, imageSource: PptGenerateOutlineRequest["imageSource"] = "ai"): PptOutline {
  const title = normalizeTitle(prompt);
  const count = Math.max(1, slideCount);
  const slides: PptOutlineSlide[] = Array.from({ length: count }, (_, index) => {
    const page = index + 1;
    if (page === 1) {
      return {
        page,
        title,
        summary: "封面页，呈现主题、汇报对象和核心价值主张。",
        bulletPoints: [],
        layout: "cover"
      };
    }
    if (page === count) {
      return {
        page,
        title: "总结与下一步行动",
        summary: "收束核心观点，并给出可执行的落地计划。",
        bulletPoints: ["明确优先级", "制定时间表", "确认资源和负责人"],
        layout: "summary"
      };
    }
    const seed = titleSeeds[(page - 2) % titleSeeds.length];
    return {
      page,
      title: seed,
      summary: `围绕「${title}」展开${seed}，帮助听众理解关键背景和方案价值。`,
      bulletPoints: ["核心问题拆解", "关键数据和案例", "可执行建议"],
      layout: requestLayoutForPage(page, count, imageSource)
    };
  });
  return { title, slides, updatedAt: new Date().toISOString() };
}

function outlineToSlides(outline: PptOutline, theme: PptTheme, imageSource: PptGenerateOutlineRequest["imageSource"] = "ai"): PptSlide[] {
  return outline.slides.map((slide, index) => ({
    id: `slide_${index + 1}_${Date.now()}`,
    page: index + 1,
    title: slide.title,
    content: slide.summary,
    bulletPoints: [...slide.bulletPoints],
    imageUrl: "",
    layout: slide.layout || (index === 0 ? "cover" : index === outline.slides.length - 1 ? "summary" : imageSource === "none" ? "content" : theme === "techBlue" ? "imageText" : "content"),
    speakerNotes: `第 ${index + 1} 页讲稿可在后续接入模型后自动生成。`
  }));
}

function requestLayoutForPage(page: number, total: number, imageSource: PptGenerateOutlineRequest["imageSource"] = "ai"): PptSlideLayout {
  if (page === 1) return "cover";
  if (page === total) return "summary";
  if (imageSource === "none") return "content";
  return page % 2 === 0 ? "imageText" : "content";
}

function normalizeTitle(prompt: string) {
  const title = prompt.trim().replace(/\s+/g, " ");
  return title ? title.slice(0, 60) : "无标题演示文稿";
}

function isPptHistoryItem(item: unknown): item is PptHistoryItem {
  if (!item || typeof item !== "object") return false;
  const record = item as Record<string, unknown>;
  return typeof record.taskId === "string" && typeof record.status === "string" && typeof record.title === "string";
}

function wait(ms: number) {
  return new Promise(resolve => window.setTimeout(resolve, ms));
}
