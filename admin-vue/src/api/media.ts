import { apiClient } from "./client";

export interface MediaAsset {
  id: string;
  tenantId: string;
  name: string;
  categoryId?: string;
  categoryName?: string;
  assetType: string;
  mimeType: string;
  fileExt: string;
  originalName: string;
  originalUrl?: string;
  cdnUrl?: string;
  thumbnailUrl?: string;
  width: number;
  height: number;
  aspectRatio: number;
  fileSize: number;
  fileHash: string;
  status: string;
  auditStatus: string;
  isDefault: boolean;
  usageCount: number;
  sourceType: string;
  createdBy?: string;
  createdAt: string;
  updatedAt: string;
}

export interface MediaCategory { id: string; tenantId: string; parentId?: string; name: string; code: string; sortOrder: number; status: string }
export interface PageAssetSlot {
  id: string; tenantId: string; pageCode: string; moduleCode: string; slotKey: string; slotName: string;
  assetId?: string; fallbackAssetId?: string; materialUrl?: string; fallbackUrl?: string; altText?: string;
  sortOrder: number; isEnabled: boolean; effectiveStartTime?: string; effectiveEndTime?: string; extraConfig?: Record<string, unknown>;
}
export interface PageConfig { id: string; tenantId: string; pageCode: string; version: number; config: Record<string, unknown>; status: string; publishedAt?: string; updatedAt: string }
export interface PageConfigVersion { id: string; version: number; changeNote?: string; createdBy?: string; createdAt: string; config: Record<string, unknown> }
export interface AssetUsage { id: string; pageCode: string; moduleCode: string; slotKey: string; businessType: string; createdAt: string }

export async function listMediaAssets(params: Record<string, unknown> = {}) {
  return apiClient.get<unknown, { items: MediaAsset[]; total: number; page: number; pageSize: number }>("/admin/media/assets", { params });
}
export async function uploadMediaAssets(files: File[], fields: Record<string, string> = {}, onProgress?: (percent: number) => void) {
  const form = new FormData();
  files.forEach((file) => form.append(files.length > 1 ? "files" : "file", file));
  Object.entries(fields).forEach(([key, value]) => value && form.append(key, value));
  return apiClient.post<unknown, { item?: MediaAsset; items?: MediaAsset[] }>(files.length > 1 ? "/admin/media/assets/batch-upload" : "/admin/media/assets/upload", form, {
    headers: { "Content-Type": "multipart/form-data" },
    onUploadProgress: (event) => onProgress?.(event.total ? Math.round(event.loaded * 100 / event.total) : 0),
  });
}
export async function updateMediaAsset(id: string, data: Record<string, unknown>) { return apiClient.put<unknown, { item: MediaAsset }>(`/admin/media/assets/${encodeURIComponent(id)}`, data); }
export async function setMediaAssetEnabled(id: string, enabled: boolean) { return apiClient.post<unknown, { item: MediaAsset }>(`/admin/media/assets/${encodeURIComponent(id)}/${enabled ? "enable" : "disable"}`); }
export async function deleteMediaAsset(id: string) { await apiClient.delete(`/admin/media/assets/${encodeURIComponent(id)}`); }
export async function listMediaAssetUsages(id: string) { return apiClient.get<unknown, { items: AssetUsage[] }>(`/admin/media/assets/${encodeURIComponent(id)}/usages`); }
export async function listMediaCategories() { return apiClient.get<unknown, { items: MediaCategory[] }>("/admin/media/categories"); }
export async function createMediaCategory(data: Partial<MediaCategory>) { return apiClient.post<unknown, { item: MediaCategory }>("/admin/media/categories", data); }
export async function updateMediaCategory(id: string, data: Partial<MediaCategory>) { return apiClient.put<unknown, { item: MediaCategory }>(`/admin/media/categories/${encodeURIComponent(id)}`, data); }
export async function deleteMediaCategory(id: string) { await apiClient.delete(`/admin/media/categories/${encodeURIComponent(id)}`); }
export async function getPageConfig(pageCode: string) { return apiClient.get<unknown, { item: PageConfig; slots: PageAssetSlot[] }>(`/admin/page-configs/${pageCode}`); }
export async function savePageConfig(pageCode: string, config: Record<string, unknown>) { return apiClient.put<unknown, { item: PageConfig }>(`/admin/page-configs/${pageCode}`, { config }); }
export async function publishPageConfig(pageCode: string, changeNote: string) { return apiClient.post<unknown, { item: PageConfig }>(`/admin/page-configs/${pageCode}/publish`, { changeNote }); }
export async function listPageConfigVersions(pageCode: string) { return apiClient.get<unknown, { items: PageConfigVersion[] }>(`/admin/page-configs/${pageCode}/versions`); }
export async function rollbackPageConfig(pageCode: string, version: number) { return apiClient.post<unknown, { item: PageConfig }>(`/admin/page-configs/${pageCode}/rollback/${version}`); }
export async function listPageSlots(pageCode: string) { return apiClient.get<unknown, { items: PageAssetSlot[] }>(`/admin/page-slots/${pageCode}`); }
export async function updatePageSlot(pageCode: string, slotKey: string, data: Partial<PageAssetSlot>) { return apiClient.put<unknown, { item: PageAssetSlot }>(`/admin/page-slots/${pageCode}/${encodeURIComponent(slotKey)}`, data); }
