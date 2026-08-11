/** Shared video model list pricing helpers for mini/App when API fields are missing. */

export type VideoModelPriceSortable = {
  code?: string;
  listPricePoints?: number;
  priceHint?: string;
  capabilityHint?: string;
  priceLabel?: string;
  description?: string;
  videoCapabilities?: {
    supportsTextToVideo?: boolean;
    supportsImageToVideo?: boolean;
    maxReferenceImages?: number;
    supportedDurations?: number[];
  } | null;
};

const FORMAL_VIDEO_LIST_PRICE: Record<string, number> = {
  "mock-video": 5,
  "grok-imagine-1.5-video": 90,
  "grok-imagine-video-1.5-preview": 100,
  "seedance-fast-2.0": 480,
  "doubao-seedance-2.0": 480,
};

const FORMAL_VIDEO_PRICE_HINT: Record<string, string> = {
  "mock-video": "1 积分/秒",
  "grok-imagine-1.5-video": "15 积分/秒",
  "grok-imagine-video-1.5-preview": "100 积分/次",
  "seedance-fast-2.0": "80 积分/秒",
  "doubao-seedance-2.0": "80 积分/秒",
};

function resolutionMultiplier(model: string, resolution: string) {
  const normalized = String(resolution || "").toLowerCase();
  if (model === "seedance-fast-2.0" || model === "doubao-seedance-2.0") {
    if (normalized === "480p") return 1;
    if (normalized === "720p") return 1.5;
    if (normalized === "1080p") return 2;
    if (normalized === "4k") return 4;
  }
  return 1;
}

export function videoModelListPricePoints(item: VideoModelPriceSortable) {
  const code = String(item.code || "").trim();
  const fromApi = Number(item.listPricePoints);
  if (Number.isFinite(fromApi) && fromApi > 0) return fromApi;
  return FORMAL_VIDEO_LIST_PRICE[code] || Number.MAX_SAFE_INTEGER;
}

export function videoModelPriceHint(item: VideoModelPriceSortable) {
  const fromApi = String(item.priceHint || "").trim();
  if (fromApi) return fromApi;
  const code = String(item.code || "").trim();
  return FORMAL_VIDEO_PRICE_HINT[code] || "";
}

export function videoModelCapabilityHint(item: VideoModelPriceSortable) {
  const fromApi = String(item.capabilityHint || "").trim();
  if (fromApi) return fromApi;
  const caps = item.videoCapabilities || {};
  const parts: string[] = [];
  if (caps.supportsTextToVideo && caps.supportsImageToVideo) parts.push("文生/图生");
  else if (caps.supportsImageToVideo) parts.push("仅图生");
  else if (caps.supportsTextToVideo) parts.push("仅文生");
  const durations = Array.isArray(caps.supportedDurations)
    ? caps.supportedDurations.map(Number).filter(value => Number.isFinite(value) && value > 0)
    : [];
  if (durations.length === 1) parts.push(`${durations[0]}s`);
  else if (durations.length > 1 && durations.length <= 3) parts.push(`${durations.join("/")}s`);
  else if (durations.length > 3) {
    parts.push(`${Math.min(...durations)}–${Math.max(...durations)}s`);
  }
  if (caps.supportsImageToVideo && Number(caps.maxReferenceImages || 0) > 1) {
    parts.push(`最多${caps.maxReferenceImages}图`);
  } else if (caps.supportsImageToVideo && !caps.supportsTextToVideo && Number(caps.maxReferenceImages || 0) === 1) {
    parts.push("需1张参考图");
  }
  return parts.join(" · ");
}

export function videoModelSubtitle(item: VideoModelPriceSortable) {
  const parts = [videoModelPriceHint(item), videoModelCapabilityHint(item)].filter(Boolean);
  if (parts.length) return parts.join(" · ");
  return String(item.priceLabel || item.description || "").trim();
}

export function sortVideoModelsByListPrice<T extends VideoModelPriceSortable>(items: T[]) {
  return [...items].sort((left, right) => {
    const priceDiff = videoModelListPricePoints(left) - videoModelListPricePoints(right);
    if (priceDiff !== 0) return priceDiff;
    return String(left.code || "").localeCompare(String(right.code || ""));
  });
}

/** Local fallback estimate aligned with published formal rules. */
export function estimateFormalVideoPoints(model: string, duration: number, resolution: string) {
  const code = String(model || "").trim();
  const seconds = Number(duration);
  const safeSeconds = Number.isFinite(seconds) && seconds > 0 ? seconds : 0;
  if (code === "grok-imagine-video-1.5-preview") return 100;
  if (code === "grok-imagine-1.5-video") return Math.max(15, Math.ceil(15 * safeSeconds));
  if (code === "seedance-fast-2.0" || code === "doubao-seedance-2.0") {
    return Math.max(1, Math.ceil(80 * safeSeconds * resolutionMultiplier(code, resolution)));
  }
  if (code === "mock-video") {
    return Math.max(1, Math.ceil(1 * safeSeconds * (String(resolution).toLowerCase() === "720p" ? 1.2 : 1)));
  }
  return 0;
}
