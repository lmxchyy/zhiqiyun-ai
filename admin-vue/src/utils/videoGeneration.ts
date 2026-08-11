import type { AdminRecord } from "../stores/admin";

export type VideoModelOption = { name: string; family: string; desc: string };

export type VideoModelParameterOption = {
  durations: number[];
  ratios?: string[];
  resolutions?: string[];
  maxReferenceImages?: number;
  requiresReferenceImage?: boolean;
  supportsAudio?: boolean;
};

export const videoModelOptions: VideoModelOption[] = [
  { name: "Mock Video", family: "tool", desc: "本地联调视频模型" },
  { name: "Grok Image Video", family: "grok", desc: "Grok 文生/图生视频" },
  { name: "Grok Imagine Video 1.5 Preview", family: "grok", desc: "100 积分/次 · 单图生视频 · 10/15 秒" },
  { name: "Grok Imagine Video 1.5", family: "grok", desc: "15 积分/秒 · 文生/多图生视频" },
  { name: "Veo 3", family: "veo", desc: "Google 视频生成" },
  { name: "Kling 2.1", family: "kling", desc: "可灵标准视频" },
  { name: "Seedance 2.0", family: "seedance", desc: "80 积分/秒 · 5/10/15 秒" },
  { name: "Doubao Seedance 2.0", family: "seedance", desc: "80 积分/秒 · 5/8/10/12/15 秒" },
  { name: "Wan 2.2", family: "wan", desc: "Wan 系列视频" },
  { name: "Sora 2", family: "sora", desc: "OpenAI 视频模型" }
];

export const videoToolOptions: VideoModelOption[] = [
  { name: "去水印", family: "tool", desc: "上传视频处理" },
  { name: "运动控制", family: "tool", desc: "视频 + 图片控制" }
];

export const videoDurationOptions = [4, 5, 6, 8, 10, 12, 14, 15, 16, 18, 20, 25];
export const videoRatioOptions = ["16:9", "9:16", "1:1", "4:3", "3:4", "21:9", "9:21"];
export const videoResolutionOptions = ["480p", "720p", "1080p"];

export const videoModelParameterOptions: Record<string, VideoModelParameterOption> = {
  "Mock Video": { durations: [4], ratios: ["16:9"], resolutions: ["480p"] },
  "Grok Image Video": { durations: [4, 6, 8, 10, 12, 15], ratios: ["16:9", "9:16", "1:1", "4:3", "3:4", "3:2", "2:3"], resolutions: ["480p", "720p"] },
  "Grok Imagine Video 1.5 Preview": { durations: [10, 15], ratios: ["16:9", "9:16"], resolutions: ["480p", "720p"], maxReferenceImages: 1, requiresReferenceImage: true, supportsAudio: false },
  "Grok Imagine Video 1.5": { durations: Array.from({ length: 25 }, (_, index) => index + 6), ratios: ["16:9", "9:16", "1:1", "3:2", "2:3"], resolutions: ["480p", "720p"], maxReferenceImages: 7, supportsAudio: false },
  "Veo 3": { durations: [8], ratios: ["16:9", "9:16"], resolutions: ["720p", "1080p"] },
  "Kling 2.1": { durations: [5, 10], ratios: ["16:9", "9:16", "1:1"], resolutions: ["720p", "1080p"] },
  "Seedance 2.0": { durations: [5, 10, 15], ratios: ["16:9", "9:16", "4:3", "3:4"], resolutions: ["480p", "720p", "1080p"], supportsAudio: false },
  "Doubao Seedance 2.0": { durations: [5, 8, 10, 12, 15], ratios: ["16:9", "9:16", "1:1", "4:3", "3:4", "21:9", "adaptive"], resolutions: ["480p", "720p", "1080p", "4k"], supportsAudio: false },
  "Wan 2.2": { durations: [5, 8], ratios: ["16:9", "9:16", "1:1"], resolutions: ["720p"], supportsAudio: false },
  "Sora 2": { durations: [10, 15, 25], ratios: ["16:9", "9:16"], resolutions: ["720p"], supportsAudio: false },
  去水印: { durations: [] },
  运动控制: { durations: [5, 10], ratios: ["16:9", "9:16", "1:1"], resolutions: ["720p", "1080p"] }
};

const videoModelIdMapping: Record<string, string> = {
  "Mock Video": "mock-video",
  "Grok Image Video": "grok-image-video",
  "Grok Imagine Video 1.5 Preview": "grok-imagine-video-1.5-preview",
  "Grok Imagine Video 1.5": "grok-imagine-1.5-video",
  "Veo 3": "veo3",
  "Kling 2.1": "kling-2.1",
  "Seedance 2.0": "seedance-fast-2.0",
  "Doubao Seedance 2.0": "doubao-seedance-2.0",
  "Wan 2.2": "wan-2.2",
  "Sora 2": "sora-2"
};

const videoModelIdAliases: Record<string, string> = {
  "seedance-2.0": "seedance-fast-2.0",
  "grok-video-image": "grok-image-video"
};

export function videoModelParameterOption(modelName: string) {
  const normalized = String(modelName || "").trim();
  if (!normalized) return undefined;
  if (videoModelParameterOptions[normalized]) {
    return videoModelParameterOptions[normalized];
  }
  const canonicalId = videoModelIdAliases[normalized] || normalized;
  const matchedName = Object.keys(videoModelIdMapping).find((name) => videoModelIdMapping[name] === canonicalId);
  return matchedName ? videoModelParameterOptions[matchedName] : undefined;
}

export function videoModelId(modelName: string) {
  const normalized = String(modelName || "").trim();
  if (videoModelIdMapping[normalized]) {
    return videoModelIdMapping[normalized];
  }
  return videoModelIdAliases[normalized] || normalized;
}

export function videoModelMaxReferenceImages(modelName: string) {
  return Math.max(1, Math.min(7, videoModelParameterOption(modelName)?.maxReferenceImages || 1));
}

export function videoModelRequiresReferenceImage(modelName: string) {
  return videoModelParameterOption(modelName)?.requiresReferenceImage === true;
}

export type VideoHistoryStatus = "success" | "generating" | "failed";

export type VideoHistoryEntry = {
  id: string;
  taskId?: string;
  backendTaskId?: string;
  url: string;
  prompt: string;
  model: string;
  mode: "text-to-video" | "image-to-video" | "video-to-video";
  aspect_ratio: string;
  duration: number | string;
  resolution: string;
  inputImageUrls: string[];
  inputVideoUrl: string;
  createdAt: string;
  timestamp: number;
  status: VideoHistoryStatus;
  errorMessage?: string;
  userId?: string;
};

export function videoTaskUrl(task: AdminRecord | null) {
  if (!task) return "";
  return String(task.outputUrl || task.resultUrl || task.imageUrl || "");
}

export function videoTaskParams(task: AdminRecord | null): Record<string, unknown> {
  if (!task) return {};
  const raw = task.params || task.paramsJson || task.params_json || task.metadata;
  if (!raw) return {};
  if (typeof raw === "string") {
    try {
      const parsed = JSON.parse(raw);
      return parsed && typeof parsed === "object" ? parsed as Record<string, unknown> : {};
    } catch {
      return {};
    }
  }
  return typeof raw === "object" ? raw as Record<string, unknown> : {};
}

export function videoStringValue(value: unknown, fallback = "") {
  if (value === undefined || value === null || value === "") return fallback;
  return String(value);
}

export function normalizeVideoErrorText(raw: string) {
  const text = raw.trim();
  if (!text) return "";
  const subscriptionMatch = text.match(/当前账号处未订购[^"\\\r\n]*/);
  if (subscriptionMatch?.[0]) return subscriptionMatch[0].trim();
  const jsonMessageMatch = text.match(/"(?:message|error|detail|reason)"\s*:\s*"([^"]+)"/i);
  if (jsonMessageMatch?.[1]) {
    try {
      return normalizeVideoErrorText(JSON.parse(`"${jsonMessageMatch[1]}"`));
    } catch {
      return normalizeVideoErrorText(jsonMessageMatch[1]);
    }
  }
  const lower = text.toLowerCase();
  if (lower.includes("create_video_generation_task returned empty task id") && lower.includes("seedance")) {
    return "移动云 Seedance 创建任务失败，请检查模型资费包、API Key 和模型权限";
  }
  if (lower.includes("input_reference") && lower.includes("unmarshal")) {
    return "视频参考图参数格式错误，请重新上传首帧图后重试";
  }
  if (lower.includes("cannot unmarshal") && lower.includes("seconds")) {
    return "视频时长参数格式错误，请重新选择时长后重试";
  }
  if (lower.includes("cannot unmarshal")) {
    return "上游视频接口返回格式异常，请稍后重试或切换模型";
  }
  if (lower.includes("video provider does not support parameter")) {
    return "当前视频通道不支持该参数，请调整参数后重试";
  }
  if (lower.includes("does not support model")) {
    return "当前视频通道不支持所选模型，请切换模型或通道";
  }
  if (lower.includes("requires exactly one reference image") || lower.includes("supports exactly one reference image")) {
    return "该视频模型需要且仅支持 1 张参考图";
  }
  if (lower.includes("supports at most seven reference images")) {
    return "该视频模型最多支持 7 张参考图";
  }
  if (lower.includes("video generation failed")) {
    return "视频生成失败，请稍后重试";
  }
  if (lower.includes("context deadline exceeded") || lower.includes("timeout")) {
    return "生成超时，请稍后重试";
  }
  if (lower.includes("connection refused") || lower.includes("dial tcp") || lower.includes("i/o timeout")) {
    return "无法连接视频上游服务，请检查网络或通道地址";
  }
  if (lower.includes("storage_master_key")) {
    return "对象存储密钥未配置，请检查 STORAGE_MASTER_KEY 后重试";
  }
  if (lower.includes("resolve generated artifact storage")) {
    return "生成结果入库失败，请检查对象存储配置后重试";
  }
  if (lower.includes("unrecognized message") || lower.includes("upstream returned unrecognized")) {
    return "上游视频通道返回无法识别的结果，请稍后重试或检查 Seedance 通道配置";
  }
  if (lower.includes("only http/https urls") || lower.includes("invalid format for image_urls") || lower.includes("asset://")) {
    return "参考图地址不被上游接受，请重新上传图片后重试";
  }
  if (/^[A-Za-z0-9]/.test(text) && !/[\u4e00-\u9fff]/.test(text) && /(json:|error|failed|invalid|provider|http|unmarshal)/i.test(text)) {
    return "生成失败，请稍后重试。若持续失败请检查模型、参数或上游通道配置";
  }
  const compact = text.replace(/\s+/g, " ");
  return compact.length > 180 ? `${compact.slice(0, 180)}...` : compact;
}

export function videoErrorMessage(value: unknown): string {
  if (value === undefined || value === null || value === "") return "";
  if (typeof value === "string" || typeof value === "number" || typeof value === "boolean") {
    return normalizeVideoErrorText(String(value));
  }
  if (typeof value === "object") {
    const record = value as Record<string, unknown>;
    for (const key of ["message", "error", "detail", "reason", "failureReason", "failReason", "errorMessage"]) {
      const nested = videoErrorMessage(record[key]);
      if (nested) return nested;
    }
    try {
      return normalizeVideoErrorText(JSON.stringify(value));
    } catch {
      return "生成失败";
    }
  }
  return normalizeVideoErrorText(String(value));
}

export function videoNumberOrString(value: unknown, fallback: number | string = "") {
  if (value === undefined || value === null || value === "") return fallback;
  const asNumber = Number(value);
  return Number.isFinite(asNumber) ? asNumber : String(value);
}

export function normalizeVideoTimestamp(value: unknown) {
  const parsed = value ? Date.parse(String(value)) : NaN;
  return Number.isFinite(parsed) ? parsed : Date.now();
}

export function videoStatusFromTask(task: AdminRecord): VideoHistoryStatus {
  const status = String(task.status || "").toUpperCase();
  if (["SUCCEEDED", "SUCCESS", "COMPLETED", "DONE"].includes(status)) return "success";
  if (["FAILED", "FAILURE", "ERROR", "CANCELED", "CANCELLED"].includes(status)) return "failed";
  return "generating";
}

export function videoModeFromTask(task: AdminRecord, params: Record<string, unknown>): VideoHistoryEntry["mode"] {
  const inputMode = String(params.inputMode || params.mode || task.mode || "").toLowerCase();
  const type = String(task.type || task.sourceType || "").toUpperCase();
  if (inputMode.includes("video") || type.includes("VIDEO_TO_VIDEO")) return "video-to-video";
  if (inputMode.includes("image") || type.includes("IMAGE_TO_VIDEO")) return "image-to-video";
  return "text-to-video";
}

export function videoInputImageUrlsFromTask(task: AdminRecord, params: Record<string, unknown>) {
  const candidates = [params.image_urls, params.imageUrls, params.inputImageUrls, params.reference_images, task.inputImageUrls];
  const urls = candidates.flatMap((value) => Array.isArray(value) ? value : value ? [value] : []);
  return urls.map((item) => String(item)).filter(Boolean);
}

export function isVideoGenerationTask(task: AdminRecord) {
  const type = String(task.type || task.sourceType || "").toUpperCase();
  const mediaType = String(task.mediaType || "").toLowerCase();
  const url = videoTaskUrl(task);
  return type.includes("VIDEO") || mediaType === "video" || /\.mp4(\?|$)/i.test(url);
}
