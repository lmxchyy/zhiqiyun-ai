import { api, apiRequestTask, getApiBaseURL, type ApiRequestTaskHandle } from "../../api/client";
import { v531SlotsByPage } from "../../config/v531";
import { beginWorksPerformanceStep } from "./performance";
import type {
  AssetBatchPayload,
  AssetDetail,
  AssetFilter,
  AssetItem,
  AssetOverview,
  AssetPageResponse,
  AssetSort,
  AssetType,
  GenerationTask,
  ProjectOption,
  TaskPageResponse,
} from "./types";

type AnyRecord = Record<string, unknown>;

function record(value: unknown): AnyRecord {
  return value && typeof value === "object" && !Array.isArray(value) ? value as AnyRecord : {};
}

function stringValue(value: unknown): string {
  return value === undefined || value === null ? "" : String(value);
}

function numberValue(...values: unknown[]): number {
  for (const value of values) {
    const parsed = Number(value);
    if (Number.isFinite(parsed)) return parsed;
  }
  return 0;
}

function booleanValue(...values: unknown[]): boolean {
  for (const value of values) {
    if (typeof value === "boolean") return value;
    if (value === 1 || String(value).toLowerCase() === "true") return true;
  }
  return false;
}

function stringArray(value: unknown): string[] {
  if (Array.isArray(value)) return value.map(item => stringValue(item)).filter(Boolean);
  if (typeof value === "string") return value.split(/[,，]/).map(item => item.trim()).filter(Boolean);
  return [];
}

export function normalizeAssetType(value: unknown): Exclude<AssetType, "all"> {
  const type = stringValue(value).toLowerCase();
  if (type.includes("video")) return "video";
  if (type.includes("image")) return "image";
  if (type.includes("ppt") || type.includes("presentation")) return "ppt";
  if (type.includes("agent")) return "agent";
  if (type.includes("infographic") || type.includes("long_image") || type.includes("long-image")) return "infographic";
  if (type.includes("knowledge")) return "knowledge";
  if (type.includes("prompt")) return "prompt";
  if (type.includes("template")) return "template";
  if (type.includes("document") || type.includes("doc") || type.includes("pdf") || type.includes("text")) return "document";
  return "image";
}

function normalizeAssetStatus(raw: AnyRecord, metadata: AnyRecord, deletedAt: string): AssetItem["status"] {
  if (deletedAt) return "recycled";
  if (booleanValue(metadata.archived)) return "archived";
  const value = stringValue(raw.status || metadata.status || metadata.taskStatus || "COMPLETED").toUpperCase();
  if (["PENDING", "QUEUED"].some(item => value.includes(item))) return "queued";
  if (["RUNNING", "PROCESSING", "RETRYING", "GENERATING"].some(item => value.includes(item))) return "generating";
  if (["FAILED", "ERROR"].some(item => value.includes(item))) return "failed";
  return "completed";
}

function fallbackFor(type: AssetItem["type"]): string {
  const slotType = type === "video" ? "video" : type === "ppt" ? "ppt" : type === "infographic" ? "long_image" : "image";
  return v531SlotsByPage.assets[`assets.cover.${slotType}`]?.fallbackUrl || "/static/fallbacks/default-cover.jpg";
}

export function normalizeAsset(value: unknown): AssetItem {
  const raw = record(value);
  const metadata = record(raw.metadata);
  const type = normalizeAssetType(raw.mediaType || raw.type || metadata.mediaType || metadata.type);
  const deletedAt = stringValue(raw.deletedAt || metadata.deletedAt);
  const remoteUrl = stringValue(raw.url || raw.remoteUrl || raw.outputUrl || raw.fileUrl || metadata.remote_url || metadata.remoteUrl);
  const fallbackUrl = stringValue(raw.fallbackUrl || metadata.fallback_url || metadata.fallbackUrl) || fallbackFor(type);
  return {
    id: stringValue(raw.id || raw.assetId),
    taskId: stringValue(raw.taskId || metadata.taskId) || undefined,
    name: stringValue(raw.name || raw.title || metadata.title) || "未命名作品",
    type,
    status: normalizeAssetStatus(raw, metadata, deletedAt),
    remoteUrl,
    fallbackUrl,
    thumbnailUrl: stringValue(raw.thumbnailUrl || raw.previewUrl || metadata.thumbnailUrl || metadata.coverUrl) || remoteUrl,
    projectId: stringValue(raw.projectId || metadata.projectId),
    projectName: stringValue(raw.projectName || metadata.projectName),
    favorite: booleanValue(raw.favorite, metadata.favorite, metadata.isFavorite),
    archived: booleanValue(raw.archived, metadata.archived),
    deletedAt,
    createdAt: stringValue(raw.createdAt || metadata.createdAt),
    updatedAt: stringValue(raw.updatedAt || metadata.updatedAt),
    tags: stringArray(raw.tags || metadata.tags || metadata.tagIds),
    model: stringValue(raw.model || raw.modelName || metadata.model || metadata.modelName),
    prompt: stringValue(raw.prompt || metadata.prompt),
    negativePrompt: stringValue(raw.negativePrompt || metadata.negativePrompt),
    fileSize: numberValue(raw.fileSize, raw.fileSizeBytes, metadata.fileSize, metadata.fileSizeBytes, metadata.sizeBytes),
    width: numberValue(raw.width, metadata.width) || undefined,
    height: numberValue(raw.height, metadata.height) || undefined,
    duration: numberValue(raw.duration, metadata.duration) || undefined,
    pageCount: numberValue(raw.pageCount, metadata.pageCount, metadata.slides) || undefined,
    documentCount: numberValue(raw.documentCount, metadata.documentCount) || undefined,
    aspectRatio: stringValue(raw.aspectRatio || metadata.aspectRatio || metadata.ratio),
    seed: (raw.seed || metadata.seed) as string | number | undefined,
    tokenCost: numberValue(raw.tokenCost, metadata.tokenCost) || undefined,
    pointCost: numberValue(raw.pointCost, metadata.pointCost) || undefined,
    generationDurationMs: numberValue(raw.generationDurationMs, metadata.generationDurationMs) || undefined,
    usageCount: numberValue(raw.usageCount, metadata.usageCount) || undefined,
    metadata,
  };
}

function normalizeOverview(value: unknown): AssetOverview {
  const raw = record(value);
  const storageBytes = numberValue(raw.storageBytes);
  const quota = numberValue(raw.storageQuotaBytes, raw.storageQuota);
  return {
    total: numberValue(raw.total),
    monthTotal: numberValue(raw.monthTotal),
    favoriteTotal: numberValue(raw.favoriteTotal),
    storageBytes,
    storageQuotaBytes: quota,
    storageUsagePercent: quota > 0 ? Math.min(100, Math.round(storageBytes / quota * 100)) : 0,
  };
}

function append(params: string[], key: string, value: unknown) {
  if (value === undefined || value === null || value === "") return;
  if (Array.isArray(value) && !value.length) return;
  params.push(`${encodeURIComponent(key)}=${encodeURIComponent(Array.isArray(value) ? value.join(",") : String(value))}`);
}

function assetQuery(page: number, pageSize: number, filters: AssetFilter, sort: AssetSort) {
  const params = ["paged=true"];
  append(params, "page", page);
  append(params, "pageSize", pageSize);
  append(params, "limit", pageSize);
  append(params, "offset", Math.max(0, (page - 1) * pageSize));
  append(params, "type", filters.type === "all" ? "" : filters.type);
  append(params, "status", filters.status === "recent" ? "" : filters.status);
  append(params, "keyword", filters.keyword.trim());
  append(params, "projectId", filters.projectId);
  append(params, "tagIds", filters.tagIds);
  append(params, "model", filters.model);
  append(params, "createdFrom", filters.createdFrom);
  append(params, "createdTo", filters.createdTo);
  append(params, "favorite", filters.favorite);
  append(params, "sort", sort);
  append(params, "lightweight", pageSize <= 4 ? true : undefined);
  append(params, "includeSummary", pageSize <= 4 ? false : undefined);
  return params.join("&");
}

export async function fetchAssetOverview(): Promise<AssetOverview> {
  const payload = record(await api<unknown>("/api/v1/assets/overview"));
  return normalizeOverview(payload.overview || payload.summary || payload);
}

export function fetchRecentWorksTask(limit = 20): ApiRequestTaskHandle<AssetItem[]> {
  const resolvedLimit = Math.max(1, Math.min(20, Math.round(Number(limit) || 20)));
  const requestUrl = `/api/v1/works/recent?limit=${resolvedLimit}`;
  const request = apiRequestTask<unknown>(requestUrl, { timeout: 3000 });
  return {
    abort: request.abort,
    promise: request.promise.then((value) => {
      const transform = beginWorksPerformanceStep("data_transform", {
        serialWait: true,
        source: "fetchRecentWorksTask",
        requestUrl,
      });
      const payload = record(value);
      const transformed = Array.isArray(payload.items)
        ? payload.items.map(normalizeAsset).filter(item => item.id)
        : [];
      transform.end({ itemCount: transformed.length });
      const mediaURLTiming = beginWorksPerformanceStep("image_url_processing", {
        serialWait: true,
        source: "fetchRecentWorksTask",
        requestUrl,
      });
      const items = transformed.map(item => ({
        ...item,
        remoteUrl: item.remoteUrl.trim(),
        thumbnailUrl: item.thumbnailUrl.trim(),
        fallbackUrl: item.fallbackUrl.trim(),
      }));
      mediaURLTiming.end({ itemCount: items.length });
      return items;
    }),
  };
}

export async function fetchAssetPage(page: number, pageSize: number, filters: AssetFilter, sort: AssetSort): Promise<AssetPageResponse> {
  const payload = record(await api<unknown>(`/api/v1/assets?${assetQuery(page, pageSize, filters, sort)}`));
  const items = Array.isArray(payload.items) ? payload.items.map(normalizeAsset).filter(item => item.id) : [];
  const total = numberValue(payload.total, items.length);
  const resolvedPageSize = numberValue(payload.pageSize, payload.limit, pageSize) || pageSize;
  const resolvedPage = numberValue(payload.page, page) || page;
  return {
    items,
    total,
    page: resolvedPage,
    pageSize: resolvedPageSize,
    hasMore: typeof payload.hasMore === "boolean" ? payload.hasMore : resolvedPage * resolvedPageSize < total,
    overview: payload.summary ? normalizeOverview(payload.summary) : undefined,
  };
}

export async function fetchAssetDetail(id: string): Promise<AssetDetail> {
  const payload = record(await api<unknown>(`/api/v1/assets/${encodeURIComponent(id)}`));
  const item = normalizeAsset(payload.item || payload);
  return {
    ...item,
    downloadUrl: `${getApiBaseURL()}/api/v1/assets/${encodeURIComponent(id)}/download`,
    shareUrl: item.remoteUrl,
    variables: record(item.metadata.variables),
  };
}

function normalizeTask(value: unknown): GenerationTask {
  const raw = record(value);
  const params = record(raw.params);
  const status = stringValue(raw.status || "PENDING").toUpperCase();
  const normalizedStatus: GenerationTask["status"] = ["PENDING", "QUEUED"].includes(status)
    ? "queued"
    : ["RUNNING", "PROCESSING", "RETRYING", "GENERATING"].includes(status)
      ? "generating"
      : ["SUCCEEDED", "SUCCESS", "COMPLETED"].includes(status)
        ? "completed"
        : ["CANCELLED", "CANCELED"].includes(status) ? "cancelled" : "failed";
  return {
    id: stringValue(raw.id),
    name: stringValue(params.title || params.name) || stringValue(raw.prompt).slice(0, 36) || "AI 创作任务",
    type: normalizeAssetType(raw.type || params.type),
    status: normalizedStatus,
    progress: Math.min(100, Math.max(0, numberValue(raw.progress, params.progress, params.percentage))),
    createdAt: stringValue(raw.createdAt),
    updatedAt: stringValue(raw.updatedAt),
    failureReason: stringValue(raw.failureReason || record(raw.error).message),
    resultIds: stringArray(raw.resultIds),
    prompt: stringValue(raw.prompt),
    model: stringValue(raw.model),
    params,
  };
}

export async function fetchTaskPage(page = 1, pageSize = 5): Promise<TaskPageResponse> {
  const params = `paged=true&priority=active&page=${page}&pageSize=${pageSize}&limit=${pageSize}&offset=${Math.max(0, (page - 1) * pageSize)}`;
  const payload = record(await api<unknown>(`/api/v1/generation-tasks?${params}`));
  const items = Array.isArray(payload.items) ? payload.items.map(normalizeTask).filter(item => item.id) : [];
  const total = numberValue(payload.total, items.length);
  return { items, total, page, pageSize, hasMore: typeof payload.hasMore === "boolean" ? payload.hasMore : page * pageSize < total };
}

export async function setAssetFavorite(id: string, favorite: boolean): Promise<AssetItem> {
  const payload = record(await api<unknown>(`/api/v1/assets/${encodeURIComponent(id)}/favorite`, { method: favorite ? "POST" : "DELETE" }));
  return normalizeAsset(payload.item || payload);
}

export async function renameAsset(id: string, name: string): Promise<AssetItem> {
  const payload = record(await api<unknown>(`/api/v1/assets/${encodeURIComponent(id)}`, { method: "PATCH", body: JSON.stringify({ name }) }));
  return normalizeAsset(payload.item || payload);
}

export async function archiveAsset(id: string): Promise<AssetItem> {
  const payload = record(await api<unknown>(`/api/v1/assets/${encodeURIComponent(id)}/archive`, { method: "POST" }));
  return normalizeAsset(payload.item || payload);
}

export async function deleteAsset(id: string): Promise<void> {
  await api(`/api/v1/assets/${encodeURIComponent(id)}`, { method: "DELETE" });
}

export async function restoreAsset(id: string): Promise<AssetItem> {
  const payload = record(await api<unknown>(`/api/v1/assets/${encodeURIComponent(id)}/restore`, { method: "POST" }));
  return normalizeAsset(payload.item || payload);
}

export async function permanentlyDeleteAsset(id: string): Promise<void> {
  await api(`/api/v1/assets/${encodeURIComponent(id)}/permanent`, { method: "DELETE" });
}

export async function moveAssetToProject(id: string, projectId: string, projectName = ""): Promise<AssetItem> {
  const payload = record(await api<unknown>(`/api/v1/assets/${encodeURIComponent(id)}/move-project`, {
    method: "POST",
    body: JSON.stringify({ projectId, projectName }),
  }));
  return normalizeAsset(payload.item || payload);
}

export async function batchAssets(input: AssetBatchPayload): Promise<AssetItem[]> {
  const payload = record(await api<unknown>("/api/v1/assets/batch", { method: "POST", body: JSON.stringify(input) }));
  return Array.isArray(payload.items) ? payload.items.map(normalizeAsset) : [];
}

export async function fetchProjects(): Promise<ProjectOption[]> {
  const payload = record(await api<unknown>("/api/v1/assets/projects"));
  if (!Array.isArray(payload.items)) return [];
  return payload.items.map(value => {
    const item = record(value);
    return { id: stringValue(item.id || item.projectId), name: stringValue(item.name || item.projectName), assetCount: numberValue(item.assetCount) };
  }).filter(item => item.id && item.name);
}

export async function cancelGenerationTask(id: string): Promise<GenerationTask> {
  return normalizeTask(await api<unknown>(`/api/v1/generation-tasks/${encodeURIComponent(id)}/cancel`, { method: "POST" }));
}

export async function retryGenerationTask(id: string): Promise<GenerationTask> {
  return normalizeTask(await api<unknown>(`/api/v1/generation-tasks/${encodeURIComponent(id)}/retry`, { method: "POST" }));
}

export async function deleteGenerationTask(id: string): Promise<{ ok: boolean; id: string }> {
  return api<{ ok: boolean; id: string }>(`/api/v1/generation-tasks/${encodeURIComponent(id)}`, { method: "DELETE" });
}
