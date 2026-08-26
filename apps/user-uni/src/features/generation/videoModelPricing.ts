/** Video model presentation helpers. Pricing values are API-owned. */

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

export function videoModelListPricePoints(item: VideoModelPriceSortable) {
  const fromApi = Number(item.listPricePoints);
  return Number.isFinite(fromApi) && fromApi > 0 ? fromApi : Number.MAX_SAFE_INTEGER;
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
  const parts = [videoModelCapabilityHint(item)].filter(Boolean);
  if (parts.length) return parts.join(" · ");
  return "";
}

export function sortVideoModelsByListPrice<T extends VideoModelPriceSortable>(items: T[]) {
  return [...items].sort((left, right) => {
    const priceDiff = videoModelListPricePoints(left) - videoModelListPricePoints(right);
    if (priceDiff !== 0) return priceDiff;
    return String(left.code || "").localeCompare(String(right.code || ""));
  });
}

export const DEFAULT_VIDEO_MODEL_CODE = "grok-imagine-1.5-video";

export function pickDefaultVideoModelCode(
  codes: Array<string | null | undefined>,
  preferred = DEFAULT_VIDEO_MODEL_CODE,
) {
  const available = codes.map((code) => String(code || "").trim()).filter(Boolean);
  if (preferred && available.includes(preferred)) return preferred;
  return available[0] || "";
}

/** Kept only for source compatibility; callers must use the quote API. */
export function estimateFormalVideoPoints(model: string, duration: number, resolution: string) {
  void model;
  void duration;
  void resolution;
  return 0;
}
