import { GENERATION_QUEUED_LABEL, generationDisplayStatus, isGenerationQueued, videoGenerationFailureMessage } from "@xianzhi/shared-types";
import type { AdminRecord } from "../stores/admin";

export type VideoModelOption = {
  name: string;
  family: string;
  desc: string;
  code?: string;
  listPricePoints?: number;
};

export type VideoModelParameterOption = {
  durations: number[];
  ratios?: string[];
  resolutions?: string[];
  maxReferenceImages?: number;
  requiresReferenceImage?: boolean;
  supportsAudio?: boolean;
};

/** Static catalog kept for parameter metadata / fallback labels. Prefer API-wired options for the picker. */
export const videoModelOptions: VideoModelOption[] = [
  { name: "Mock Video", family: "tool", desc: "本地联调视频模型", code: "mock-video" },
  { name: "Grok Image Video", family: "grok", desc: "Grok 文生/图生视频", code: "grok-image-video" },
  { name: "Grok Imagine Video 1.5 Preview", family: "grok", desc: "100 积分/次 · 单图生视频 · 10/15 秒", code: "grok-imagine-video-1.5-preview" },
  { name: "Grok Imagine Video 1.5", family: "grok", desc: "15 积分/秒 · 文生/多图生视频", code: "grok-imagine-1.5-video" },
  { name: "Veo 3", family: "veo", desc: "Google 视频生成", code: "veo3" },
  { name: "Kling 2.1", family: "kling", desc: "可灵标准视频", code: "kling-2.1" },
  { name: "Seedance 2.0", family: "seedance", desc: "80 积分/秒 · 5/10/15 秒", code: "seedance-fast-2.0" },
  { name: "Doubao Seedance 2.0", family: "seedance", desc: "80 积分/秒 · 5/8/10/12/15 秒", code: "doubao-seedance-2.0" },
  { name: "Wan 2.2", family: "wan", desc: "Wan 系列视频", code: "wan-2.2" },
  { name: "Sora 2", family: "sora", desc: "OpenAI 视频模型", code: "sora-2" }
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

const videoModelCodeToName = Object.fromEntries(
  Object.entries(videoModelIdMapping).map(([name, code]) => [code, name])
) as Record<string, string>;

export type PublicVideoModelRow = {
  code?: string;
  name?: string;
  displayName?: string;
  capabilities?: string[];
  videoCapabilities?: unknown;
  video_capabilities?: unknown;
  listPricePoints?: number;
  priceHint?: string;
  capabilityHint?: string;
  priceLabel?: string;
  description?: string;
  pointCost?: number;
};

function isPublicVideoModelRow(item: PublicVideoModelRow) {
  const capabilities = Array.isArray(item.capabilities)
    ? item.capabilities.map((value) => String(value).toUpperCase())
    : [];
  return capabilities.includes("TEXT_TO_VIDEO")
    || capabilities.includes("IMAGE_TO_VIDEO")
    || Boolean(item.videoCapabilities || item.video_capabilities);
}

function familyForVideoModelName(name: string) {
  const matched = videoModelOptions.find((item) => item.name === name);
  if (matched) return matched.family;
  const lower = name.toLowerCase();
  if (lower.includes("grok")) return "grok";
  if (lower.includes("seedance") || lower.includes("doubao")) return "seedance";
  if (lower.includes("mock")) return "tool";
  return "video";
}

function publicVideoModelSubtitle(item: PublicVideoModelRow) {
  const parts = [item.priceHint, item.capabilityHint]
    .map((value) => String(value || "").trim())
    .filter(Boolean);
  if (parts.length) return parts.join(" · ");
  return String(item.priceLabel || item.description || "").trim();
}

/** Build picker options from `/api/v1/models` (already price-sorted by backend). */
export function videoModelOptionsFromPublicModels(items: PublicVideoModelRow[]): VideoModelOption[] {
  return (Array.isArray(items) ? items : [])
    .filter(isPublicVideoModelRow)
    .map((item) => {
      const code = String(item.code || "").trim();
      const name = String(item.displayName || item.name || videoModelCodeToName[code] || code).trim() || code;
      const desc = publicVideoModelSubtitle(item);
      const listPricePoints = Number(item.listPricePoints ?? item.pointCost ?? 0);
      return {
        name,
        family: familyForVideoModelName(name),
        desc,
        code,
        listPricePoints: Number.isFinite(listPricePoints) ? listPricePoints : undefined
      };
    });
}

/** Default picker selection — independent of price-asc list order. */
export const DEFAULT_VIDEO_MODEL_CODE = "grok-imagine-1.5-video";
export const DEFAULT_VIDEO_MODEL_NAME = "Grok Imagine Video 1.5";

export function pickDefaultVideoModelOption(options: VideoModelOption[]) {
  const preferred = options.find((item) => {
    const code = String(item.code || videoModelId(item.name) || "").trim();
    return code === DEFAULT_VIDEO_MODEL_CODE || item.name === DEFAULT_VIDEO_MODEL_NAME;
  });
  return preferred || options[0] || null;
}

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
  assetId?: string;
  resultIds?: string[];
  url: string;
  posterUrl?: string;
  thumbnailUrl?: string;
  downloadUrl?: string;
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
  taskStatus?: string;
  availability?: string;
  availabilityReason?: string;
  errorMessage?: string;
  billingStatus?: string;
  releasedPoints?: number;
  refundedPoints?: number;
  userId?: string;
};

export function videoCardPlaceholderText(entry: Partial<VideoHistoryEntry> | null | undefined): string {
  if (!entry) return "生成中";
  if (entry.status !== "success" && entry.status !== "failed" && isGenerationQueued(entry)) return GENERATION_QUEUED_LABEL;
  if (entry.status === "failed") return "生成失败";
  if (entry.status === "success") {
    if (entry.url && entry.url.trim()) return "悬停预览";
    const avail = String(entry.availability || "").toUpperCase();
    if (avail === "EXPIRED") return "已完成 · 视频源已过期";
    return "已完成";
  }
  return "生成中";
}

export function isSameVideoHistoryEntry(left: Partial<VideoHistoryEntry> | null | undefined, right: Partial<VideoHistoryEntry> | null | undefined): boolean {
  if (!left || !right) return false;
  if (left.id && right.id && left.id === right.id) return true;
  if (left.backendTaskId && right.backendTaskId && left.backendTaskId === right.backendTaskId) return true;
  if (left.backendTaskId && right.id && left.backendTaskId === right.id) return true;
  if (left.id && right.backendTaskId && left.id === right.backendTaskId) return true;
  if (left.taskId && right.taskId && left.taskId === right.taskId) return true;
  if (left.taskId && right.id && left.taskId === right.id) return true;
  if (left.id && right.taskId && left.id === right.taskId) return true;
  return false;
}

export function mergeVideoHistoryEntry(existing: VideoHistoryEntry, incoming: Partial<VideoHistoryEntry>): VideoHistoryEntry {
  let status = existing.status;
  if (incoming.status) {
    if (existing.status === "success" && incoming.status === "generating") {
      status = "success";
    } else if (existing.status === "failed" && incoming.status === "generating") {
      status = "failed";
    } else {
      status = incoming.status;
    }
  }

  const url = (incoming.url && incoming.url.trim()) ? incoming.url : existing.url;
  const posterUrl = (incoming.posterUrl && incoming.posterUrl.trim()) ? incoming.posterUrl : existing.posterUrl;
  const thumbnailUrl = (incoming.thumbnailUrl && incoming.thumbnailUrl.trim()) ? incoming.thumbnailUrl : existing.thumbnailUrl;
  const downloadUrl = (incoming.downloadUrl && incoming.downloadUrl.trim()) ? incoming.downloadUrl : existing.downloadUrl;

  const resultIds = (Array.isArray(incoming.resultIds) && incoming.resultIds.length)
    ? incoming.resultIds
    : existing.resultIds;
  const assetId = (incoming.assetId && incoming.assetId.trim())
    ? incoming.assetId
    : (existing.assetId || (resultIds && resultIds[0]) || undefined);

  const inputImageUrls = (Array.isArray(incoming.inputImageUrls) && incoming.inputImageUrls.length)
    ? incoming.inputImageUrls
    : existing.inputImageUrls;
  const inputVideoUrl = (incoming.inputVideoUrl && incoming.inputVideoUrl.trim())
    ? incoming.inputVideoUrl
    : existing.inputVideoUrl;

  const availability = (incoming.availability && incoming.availability.trim())
    ? incoming.availability
    : existing.availability;
  const availabilityReason = (incoming.availabilityReason && incoming.availabilityReason.trim())
    ? incoming.availabilityReason
    : existing.availabilityReason;

  return {
    ...existing,
    ...incoming,
    id: existing.id || incoming.id || "",
    taskId: incoming.taskId || existing.taskId,
    backendTaskId: incoming.backendTaskId || existing.backendTaskId,
    assetId,
    resultIds,
    url,
    posterUrl,
    thumbnailUrl: thumbnailUrl || posterUrl,
    downloadUrl,
    status,
    availability,
    availabilityReason,
    errorMessage: (incoming.errorMessage && incoming.errorMessage.trim())
      ? incoming.errorMessage
      : (status === "failed" ? existing.errorMessage : ""),
    inputImageUrls,
    inputVideoUrl
  };
}

export function normalizeVideoHistoryEntry(
  entry: Partial<VideoHistoryEntry> | null | undefined,
  fallbacks?: { model?: string; ratio?: string; duration?: number | string; resolution?: string }
): VideoHistoryEntry | null {
  if (!entry) return null;
  const timestamp = normalizeVideoTimestamp(entry.createdAt || entry.timestamp);
  const id = String(entry.id || entry.taskId || entry.backendTaskId || `video-${timestamp}`).trim();
  if (!id) return null;
  const status: VideoHistoryStatus = entry.status
    ? entry.status
    : (entry.url ? "success" : "generating");
  const resultIds = Array.isArray(entry.resultIds) && entry.resultIds.length
    ? entry.resultIds.map(String).filter(Boolean)
    : entry.assetId
      ? [String(entry.assetId)]
      : undefined;
  const assetId = entry.assetId ? String(entry.assetId) : (resultIds ? resultIds[0] : undefined);
  const posterUrl = entry.posterUrl ? String(entry.posterUrl) : undefined;
  const thumbnailUrl = entry.thumbnailUrl ? String(entry.thumbnailUrl) : posterUrl;
  const downloadUrl = entry.downloadUrl ? String(entry.downloadUrl) : undefined;
  const availability = entry.availability ? String(entry.availability).trim().toUpperCase() : undefined;
  const availabilityReason = entry.availabilityReason ? String(entry.availabilityReason).trim() : undefined;

  return {
    id,
    taskId: entry.taskId ? String(entry.taskId) : undefined,
    backendTaskId: entry.backendTaskId ? String(entry.backendTaskId) : undefined,
    assetId,
    resultIds,
    url: String(entry.url || ""),
    posterUrl,
    thumbnailUrl,
    downloadUrl,
    prompt: String(entry.prompt || ""),
    model: String(entry.model || fallbacks?.model || DEFAULT_VIDEO_MODEL_CODE),
    mode: entry.mode || "text-to-video",
    aspect_ratio: String(entry.aspect_ratio || fallbacks?.ratio || ""),
    duration: entry.duration || fallbacks?.duration || "",
    resolution: String(entry.resolution || fallbacks?.resolution || ""),
    inputImageUrls: Array.isArray(entry.inputImageUrls) ? entry.inputImageUrls.map(String).filter(Boolean) : [],
    inputVideoUrl: String(entry.inputVideoUrl || ""),
    createdAt: entry.createdAt || new Date(timestamp).toISOString(),
    timestamp,
    status,
    availability,
    availabilityReason,
    errorMessage: entry.errorMessage ? videoErrorMessage(entry.errorMessage) : "",
    taskStatus: entry.taskStatus,
    billingStatus: entry.billingStatus ? String(entry.billingStatus) : undefined,
    releasedPoints: Number.isFinite(Number(entry.releasedPoints)) ? Number(entry.releasedPoints) : undefined,
    refundedPoints: Number.isFinite(Number(entry.refundedPoints)) ? Number(entry.refundedPoints) : undefined,
    userId: entry.userId ? String(entry.userId) : undefined
  };
}

export function taskToVideoHistoryEntry(
  task: AdminRecord,
  fallbacks?: { model?: string; ratio?: string; duration?: number | string; resolution?: string }
): VideoHistoryEntry | null {
  if (!isVideoGenerationTask(task)) return null;
  const params = videoTaskParams(task);
  const createdAt = String(task.createdAt || task.created_at || task.updatedAt || new Date().toISOString());
  const status = videoStatusFromTask(task);
  const resultIds = Array.isArray(task.resultIds)
    ? task.resultIds.map(String).filter(Boolean)
    : Array.isArray(task.result_ids)
      ? task.result_ids.map(String).filter(Boolean)
      : Array.isArray(params.resultIds)
        ? (params.resultIds as unknown[]).map(String).filter(Boolean)
        : [];
  const assetId = String(task.assetId || task.asset_id || params.assetId || params.asset_id || resultIds[0] || "").trim() || undefined;
  const posterUrl = String(task.thumbnailUrl || task.posterUrl || task.coverUrl || params.posterUrl || params.thumbnailUrl || "").trim() || undefined;
  const downloadUrl = String(task.downloadUrl || params.downloadUrl || "").trim() || undefined;
  const availability = String(task.availability || task.videoStatus || params.availability || "").trim().toUpperCase() || undefined;
  const availabilityReason = String(task.availabilityReason || task.message || params.availabilityReason || "").trim() || undefined;

  return normalizeVideoHistoryEntry({
    id: String(task.id || task.taskId || `video-${Date.now()}`),
    taskId: String(task.taskId || task.providerTaskId || task.id || ""),
    backendTaskId: String(task.id || ""),
    taskStatus: generationDisplayStatus(task),
    assetId,
    resultIds: resultIds.length ? resultIds : undefined,
    url: videoTaskUrl(task),
    posterUrl,
    thumbnailUrl: posterUrl,
    downloadUrl,
    prompt: String(task.prompt || params.prompt || ""),
    model: String(task.model || params.model || fallbacks?.model || DEFAULT_VIDEO_MODEL_CODE),
    mode: videoModeFromTask(task, params),
    aspect_ratio: videoStringValue(params.ratio ?? params.aspect_ratio ?? task.aspect_ratio, fallbacks?.ratio || ""),
    duration: videoNumberOrString(params.duration ?? params.seconds ?? task.duration, fallbacks?.duration || ""),
    resolution: videoStringValue(params.resolution ?? task.resolution, fallbacks?.resolution || ""),
    inputImageUrls: videoInputImageUrlsFromTask(task, params),
    inputVideoUrl: videoStringValue(params.inputVideoUrl ?? params.video_url ?? params.videoUrl ?? task.inputVideoUrl),
    createdAt,
    status,
    availability,
    availabilityReason,
    errorMessage: videoGenerationFailureMessage({
      errorCode: task.errorCode ?? task.error_code,
      code: task.code,
      failureReason: task.failureReason ?? task.errorMessage ?? task.failReason,
      error: task.error,
      billingStatus: task.billingStatus ?? task.billing_status,
      releasedPoints: task.releasedPoints ?? task.released_points,
      refundedPoints: task.refundedPoints ?? task.refunded_points,
    }, videoErrorMessage(task.failureReason ?? task.errorMessage ?? task.error ?? task.failReason)),
    billingStatus: videoStringValue(task.billingStatus ?? task.billing_status),
    releasedPoints: Number(task.releasedPoints ?? task.released_points) || undefined,
    refundedPoints: Number(task.refundedPoints ?? task.refunded_points) || undefined,
    userId: videoStringValue(task.userId)
  }, fallbacks);
}

export function mergeVideoHistoryList(
  currentList: VideoHistoryEntry[],
  incomingList: Array<Partial<VideoHistoryEntry> | null>,
  hiddenIds: string[] = []
): VideoHistoryEntry[] {
  const result: VideoHistoryEntry[] = [];
  const hiddenSet = new Set(hiddenIds);

  for (const entry of currentList) {
    if (!hiddenSet.has(entry.id)) {
      result.push({ ...entry });
    }
  }

  for (const raw of incomingList) {
    if (!raw) continue;
    const normalized = normalizeVideoHistoryEntry(raw);
    if (!normalized || hiddenSet.has(normalized.id)) continue;

    const matchIndex = result.findIndex((item) => isSameVideoHistoryEntry(item, normalized));
    if (matchIndex >= 0) {
      result[matchIndex] = mergeVideoHistoryEntry(result[matchIndex], normalized);
    } else {
      result.push(normalized);
    }
  }

  return result;
}

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
