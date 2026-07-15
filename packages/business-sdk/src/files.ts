import type { ApiClient } from "@xianzhi/api-client";

export interface FileUploadInitInput {
  fileName: string;
  fileSize: number;
  mimeType?: string;
  businessType?: string;
  businessId?: string;
  visibility?: "PRIVATE" | "TENANT" | "SHARED" | "PUBLIC" | "SYSTEM";
  isTemporary?: boolean;
  expiresAt?: string;
  storageConfigId?: string;
}

export interface FileUploadTicket {
  fileId: string;
  objectKey: string;
  uploadMethod: "PUT";
  uploadUrl: string;
  expiresIn: number;
  headers: Record<string, string>;
}

export interface FileCenterObject {
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
  createdAt: string;
  updatedAt: string;
}

export interface DirectUploadSource {
  name: string;
  size: number;
  type?: string;
  body: BodyInit;
}

export function initFileUpload(api: ApiClient, input: FileUploadInitInput) {
  return api.request<FileUploadTicket, FileUploadInitInput>("/api/v1/files/upload/init", {
    method: "POST",
    body: input
  });
}

export function completeFileUpload(api: ApiClient, fileId: string) {
  return api.request<{ file: FileCenterObject }, { fileId: string }>("/api/v1/files/upload/complete", {
    method: "POST",
    body: { fileId }
  });
}

export async function uploadFileDirect(
  api: ApiClient,
  source: DirectUploadSource,
  input: Omit<FileUploadInitInput, "fileName" | "fileSize" | "mimeType"> = {},
  fetcher: typeof fetch = fetch
) {
  const ticket = await initFileUpload(api, {
    ...input,
    fileName: source.name,
    fileSize: source.size,
    mimeType: source.type || "application/octet-stream"
  });
  const response = await fetcher(ticket.uploadUrl, {
    method: ticket.uploadMethod,
    headers: { ...ticket.headers, "Content-Type": source.type || "application/octet-stream" },
    body: source.body
  });
  if (!response.ok) {
    throw new Error(`Direct file upload failed with HTTP ${response.status}`);
  }
  return completeFileUpload(api, ticket.fileId);
}

export function getFileAccessURL(api: ApiClient, fileId: string, download = false) {
  const action = download ? "download-url" : "access-url";
  return api.request<{ file: FileCenterObject; url: string; expiresIn: number }>(`/api/v1/files/${encodeURIComponent(fileId)}/${action}`);
}

export function moveFileToRecycleBin(api: ApiClient, fileId: string) {
  return api.request<{ file: FileCenterObject }>(`/api/v1/files/${encodeURIComponent(fileId)}`, { method: "DELETE" });
}

export function restoreFile(api: ApiClient, fileId: string) {
  return api.request<{ file: FileCenterObject }>(`/api/v1/files/${encodeURIComponent(fileId)}/restore`, { method: "POST" });
}

export function permanentlyDeleteFile(api: ApiClient, fileId: string) {
  return api.request<void>(`/api/v1/files/${encodeURIComponent(fileId)}/permanent`, { method: "DELETE" });
}
