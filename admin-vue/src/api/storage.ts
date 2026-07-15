import { apiClient } from "./client";

export interface StorageConfig {
  id: string;
  tenantId: string;
  name: string;
  provider: string;
  endpoint: string;
  signingEndpoint?: string;
  region?: string;
  bucket: string;
  publicDomain?: string;
  cdnDomain?: string;
  useSSL: boolean;
  forcePathStyle: boolean;
  isDefault: boolean;
  isSystem: boolean;
  status: string;
  lastTestStatus?: string;
  lastTestMessage?: string;
  lastTestAt?: string;
  hasAccessKey: boolean;
  hasSecretKey: boolean;
  updatedAt?: string;
}

export interface FileObject {
  fileId: string;
  tenantId: string;
  userId: string;
  storageConfigId: string;
  provider: string;
  bucket: string;
  objectKey: string;
  originalName: string;
  extension?: string;
  mimeType?: string;
  fileSize: number;
  businessType: string;
  businessId?: string;
  visibility: string;
  status: string;
  isTemporary: boolean;
  recycleExpiresAt?: string;
  createdAt: string;
}

export interface StorageQuota {
  tenantId: string;
  quotaBytes: number;
  usedBytes: number;
  reservedBytes: number;
  fileCount: number;
  warningPercent: number;
  criticalPercent: number;
}

export interface StorageOverview {
  totalFiles: number;
  totalBytes: number;
  pendingFiles: number;
  recycleFiles: number;
  abnormalFiles: number;
  temporaryBytes: number;
  providerBytes: Record<string, number>;
  quota: StorageQuota;
}

export interface StorageConfigMutation {
  tenantId?: string;
  name: string;
  provider: string;
  endpoint: string;
  signingEndpoint?: string;
  region?: string;
  bucket: string;
  accessKey?: string;
  secretKey?: string;
  sessionToken?: string;
  publicDomain?: string;
  cdnDomain?: string;
  useSSL: boolean;
  forcePathStyle: boolean;
  isDefault: boolean;
  status: string;
}

export async function getStorageOverview(params: Record<string, unknown> = {}) {
  return apiClient.get<unknown, StorageOverview>("/admin/storage/statistics/overview", { params });
}

export async function listStorageConfigs(params: Record<string, unknown> = {}) {
  return apiClient.get<unknown, { items: StorageConfig[] }>("/admin/storage/configs", { params });
}

export async function createStorageConfig(payload: StorageConfigMutation) {
  return apiClient.post<unknown, { item: StorageConfig }>("/admin/storage/configs", payload);
}

export async function updateStorageConfig(id: string, payload: StorageConfigMutation) {
  return apiClient.put<unknown, { item: StorageConfig }>(`/admin/storage/configs/${encodeURIComponent(id)}`, payload);
}

export async function testStorageConfig(id: string) {
  return apiClient.post<unknown, { status: string; message: string }>(`/admin/storage/configs/${encodeURIComponent(id)}/test`);
}

export async function deleteStorageConfig(id: string) {
  await apiClient.delete(`/admin/storage/configs/${encodeURIComponent(id)}`);
}

export async function listStorageFiles(params: Record<string, unknown> = {}) {
  return apiClient.get<unknown, { items: FileObject[]; total: number; page: number; pageSize: number }>("/admin/storage/files", { params });
}

export async function getStorageFileDownloadURL(fileId: string) {
  return apiClient.get<unknown, { url: string; expiresIn: number }>(`/admin/storage/files/${encodeURIComponent(fileId)}/download-url`);
}

export async function deleteStorageFile(fileId: string) {
  return apiClient.delete<unknown, { file: FileObject }>(`/admin/storage/files/${encodeURIComponent(fileId)}`);
}

export async function restoreStorageFile(fileId: string) {
  return apiClient.post<unknown, { file: FileObject }>(`/admin/storage/files/${encodeURIComponent(fileId)}/restore`);
}

export async function permanentlyDeleteStorageFile(fileId: string) {
  await apiClient.delete(`/admin/storage/files/${encodeURIComponent(fileId)}/permanent`);
}

export async function listStorageQuotas(params: Record<string, unknown> = {}) {
  return apiClient.get<unknown, { items: StorageQuota[] }>("/admin/storage/quotas", { params });
}

export async function updateStorageQuota(tenantId: string, payload: Pick<StorageQuota, "quotaBytes" | "warningPercent" | "criticalPercent">) {
  return apiClient.put<unknown, { item: StorageQuota }>(`/admin/storage/quotas/${encodeURIComponent(tenantId)}`, payload);
}
