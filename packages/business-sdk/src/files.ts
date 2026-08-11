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

export interface MultipartInitInput extends FileUploadInitInput {
  partSize?: number;
}

export interface MultipartSession {
  uploadId: string;
  fileId: string;
  objectKey: string;
  partSize: number;
  totalParts: number;
  expiresIn?: number;
}

export interface MultipartPartTicket {
  partNumber: number;
  uploadUrl: string;
  headers: Record<string, string>;
  expiresIn?: number;
}

export interface MultipartCompletedPart {
  partNumber: number;
  etag: string;
}

export interface MultipartUploadControllerOptions {
  partSize?: number;
  concurrency?: number;
  maxRetries?: number;
  fetcher?: typeof fetch;
  onProgress?: (progress: MultipartUploadProgress) => void;
  signal?: AbortSignal;
}

export interface MultipartUploadProgress {
  uploadedBytes: number;
  totalBytes: number;
  completedParts: number;
  totalParts: number;
  status: "uploading" | "paused" | "completing" | "completed" | "aborted" | "failed";
}

export interface MultipartUploadHandle {
  promise: Promise<{ file: FileCenterObject }>;
  pause(): void;
  resume(): void;
  abort(): Promise<void>;
}

function sleep(ms: number) {
  return new Promise(resolve => setTimeout(resolve, ms));
}

async function putPartWithRetry(
  fetcher: typeof fetch,
  ticket: MultipartPartTicket,
  body: Blob,
  maxRetries: number,
  signal?: AbortSignal
): Promise<string> {
  let lastError: unknown;
  for (let attempt = 0; attempt <= maxRetries; attempt++) {
    if (signal?.aborted) throw new Error("multipart upload aborted");
    try {
      const response = await fetcher(ticket.uploadUrl, {
        method: "PUT",
        headers: ticket.headers,
        body,
        signal
      });
      if (!response.ok) {
        throw new Error(`part ${ticket.partNumber} failed with HTTP ${response.status}`);
      }
      const etag = response.headers.get("etag") || response.headers.get("ETag") || "";
      if (!etag) throw new Error(`part ${ticket.partNumber} missing etag`);
      return etag.replace(/"/g, "");
    } catch (error) {
      lastError = error;
      if (attempt >= maxRetries || signal?.aborted) break;
      await sleep(Math.min(1000 * 2 ** attempt, 8000));
    }
  }
  throw lastError instanceof Error ? lastError : new Error("multipart part upload failed");
}

/**
 * Resumable multipart upload controller.
 * Backend routes (Task 3): init / part-url / complete / abort.
 * Concurrency default 3; failed parts retry with exponential backoff up to maxRetries.
 */
export function createMultipartUploadController(api: ApiClient) {
  return {
    init(input: MultipartInitInput) {
      return api.request<MultipartSession, MultipartInitInput>("/api/v1/files/upload/multipart/init", {
        method: "POST",
        body: input,
        auth: "required"
      });
    },
    presignPart(uploadId: string, partNumber: number) {
      return api.request<MultipartPartTicket>(
        `/api/v1/files/upload/multipart/${encodeURIComponent(uploadId)}/parts/${partNumber}`,
        { method: "POST", auth: "required" }
      );
    },
    complete(uploadId: string, parts: MultipartCompletedPart[]) {
      return api.request<{ file: FileCenterObject }, { parts: MultipartCompletedPart[] }>(
        `/api/v1/files/upload/multipart/${encodeURIComponent(uploadId)}/complete`,
        { method: "POST", body: { parts }, auth: "required" }
      );
    },
    abort(uploadId: string) {
      return api.request<void>(`/api/v1/files/upload/multipart/${encodeURIComponent(uploadId)}/abort`, {
        method: "POST",
        auth: "required"
      });
    },
    uploadBlob(
      source: { name: string; type?: string; blob: Blob },
      input: Omit<FileUploadInitInput, "fileName" | "fileSize" | "mimeType"> = {},
      options: MultipartUploadControllerOptions = {}
    ): MultipartUploadHandle {
      const partSize = Math.max(options.partSize || 8 * 1024 * 1024, 5 * 1024 * 1024);
      const concurrency = Math.max(1, Math.min(options.concurrency || 3, 6));
      const maxRetries = options.maxRetries ?? 3;
      const fetcher = options.fetcher || fetch;
      let paused = false;
      let aborted = false;
      let uploadId = "";
      const abortController = new AbortController();
      if (options.signal) {
        options.signal.addEventListener("abort", () => abortController.abort(), { once: true });
      }

      const pause = () => {
        paused = true;
      };
      const resume = () => {
        paused = false;
      };
      const abort = async () => {
        aborted = true;
        abortController.abort();
        if (uploadId) {
          try {
            await this.abort(uploadId);
          } catch {
            // best-effort abort
          }
        }
      };

      const waitIfPaused = async () => {
        while (paused && !aborted) {
          await sleep(120);
        }
        if (aborted) throw new Error("multipart upload aborted");
      };

      const promise = (async () => {
        const session = await this.init({
          ...input,
          fileName: source.name,
          fileSize: source.blob.size,
          mimeType: source.type || "application/octet-stream",
          partSize
        });
        uploadId = session.uploadId;
        const totalParts = session.totalParts || Math.ceil(source.blob.size / (session.partSize || partSize));
        const effectivePartSize = session.partSize || partSize;
        const completed: MultipartCompletedPart[] = [];
        let nextPart = 1;
        let uploadedBytes = 0;

        const report = (status: MultipartUploadProgress["status"]) => {
          options.onProgress?.({
            uploadedBytes,
            totalBytes: source.blob.size,
            completedParts: completed.length,
            totalParts,
            status
          });
        };
        report("uploading");

        const workers = Array.from({ length: Math.min(concurrency, totalParts) }, async () => {
          while (true) {
            await waitIfPaused();
            const partNumber = nextPart++;
            if (partNumber > totalParts) return;
            const start = (partNumber - 1) * effectivePartSize;
            const end = Math.min(start + effectivePartSize, source.blob.size);
            const chunk = source.blob.slice(start, end);
            const ticket = await this.presignPart(uploadId, partNumber);
            const etag = await putPartWithRetry(fetcher, ticket, chunk, maxRetries, abortController.signal);
            completed.push({ partNumber, etag });
            uploadedBytes += chunk.size;
            report(paused ? "paused" : "uploading");
          }
        });
        await Promise.all(workers);
        completed.sort((a, b) => a.partNumber - b.partNumber);
        report("completing");
        const result = await this.complete(uploadId, completed);
        report("completed");
        return result;
      })().catch(async error => {
        options.onProgress?.({
          uploadedBytes: 0,
          totalBytes: source.blob.size,
          completedParts: 0,
          totalParts: 0,
          status: aborted ? "aborted" : "failed"
        });
        if (uploadId && !aborted) {
          try {
            await this.abort(uploadId);
          } catch {
            // ignore
          }
        }
        throw error;
      });

      return { promise, pause, resume, abort };
    }
  };
}
