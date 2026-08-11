import type {
  CreateSmartVideoAssetInput,
  CreateSmartVideoExportInput,
  CreateSmartVideoPlanTaskInput,
  CreateSmartVideoProjectInput,
  ReorderSmartVideoAssetsInput,
  ReviseSmartVideoPlanInput,
  SmartVideoAnalysisSummary,
  SmartVideoPlanTask,
  SmartVideoProject,
  SmartVideoProjectAsset,
  SmartVideoProjectVersion,
  SmartVideoRenderQuote,
  SmartVideoRenderTask,
  UpdateSmartVideoProjectInput
} from "@xianzhi/shared-types";
import { adminFetchResponse, adminRequest, apiClient } from "./client";

export type {
  CreateSmartVideoAssetInput,
  CreateSmartVideoExportInput,
  CreateSmartVideoPlanTaskInput,
  CreateSmartVideoProjectInput,
  ReorderSmartVideoAssetsInput,
  ReviseSmartVideoPlanInput,
  SmartVideoAnalysisSummary,
  SmartVideoEditPlanV1,
  SmartVideoPlanTask,
  SmartVideoProject,
  SmartVideoProjectAsset,
  SmartVideoProjectVersion,
  SmartVideoRenderQuote,
  SmartVideoRenderTask,
  SmartVideoSceneV1,
  UpdateSmartVideoProjectInput
} from "@xianzhi/shared-types";

function newIdempotencyKey(prefix = "sv") {
  return `${prefix}_${Date.now().toString(36)}_${Math.random().toString(36).slice(2, 10)}`;
}

function withIdempotency(headers: Record<string, string> = {}, key?: string) {
  return {
    ...headers,
    "Idempotency-Key": (key || "").trim() || newIdempotencyKey()
  };
}

const projectPath = (projectId: string) => `/video-projects/${encodeURIComponent(projectId)}`;

export async function listSmartVideoProjects() {
  const res = await adminRequest<{ items: SmartVideoProject[] }>({
    method: "GET",
    url: "/video-projects"
  });
  return res.items || [];
}

export async function createSmartVideoProject(input: CreateSmartVideoProjectInput) {
  return adminRequest<SmartVideoProject>({
    method: "POST",
    url: "/video-projects",
    data: input
  });
}

export async function getSmartVideoProject(projectId: string) {
  return adminRequest<SmartVideoProject>({
    method: "GET",
    url: projectPath(projectId)
  });
}

export async function updateSmartVideoProject(projectId: string, input: UpdateSmartVideoProjectInput) {
  return adminRequest<SmartVideoProject>({
    method: "PATCH",
    url: projectPath(projectId),
    data: input
  });
}

export async function deleteSmartVideoProject(projectId: string) {
  await adminRequest<void>({
    method: "DELETE",
    url: projectPath(projectId)
  });
}

export async function listSmartVideoAssets(projectId: string) {
  const res = await adminRequest<{ items: SmartVideoProjectAsset[] }>({
    method: "GET",
    url: `${projectPath(projectId)}/assets`
  });
  return res.items || [];
}

export async function addSmartVideoAsset(projectId: string, input: CreateSmartVideoAssetInput) {
  return adminRequest<SmartVideoProjectAsset>({
    method: "POST",
    url: `${projectPath(projectId)}/assets`,
    data: input
  });
}

export async function reorderSmartVideoAssets(projectId: string, input: ReorderSmartVideoAssetsInput) {
  const res = await adminRequest<{ items: SmartVideoProjectAsset[] }>({
    method: "PUT",
    url: `${projectPath(projectId)}/assets/order`,
    data: input
  });
  return res.items || [];
}

export async function deleteSmartVideoAsset(projectId: string, assetId: string) {
  await adminRequest<void>({
    method: "DELETE",
    url: `${projectPath(projectId)}/assets/${encodeURIComponent(assetId)}`
  });
}

export async function analyzeSmartVideoProject(projectId: string, idempotencyKey?: string) {
  return adminRequest<SmartVideoAnalysisSummary>({
    method: "POST",
    url: `${projectPath(projectId)}/analyze`,
    headers: withIdempotency({}, idempotencyKey)
  });
}

export async function getSmartVideoAnalysis(projectId: string) {
  return adminRequest<SmartVideoAnalysisSummary>({
    method: "GET",
    url: `${projectPath(projectId)}/analysis`
  });
}

export async function retrySmartVideoAssetAnalysis(projectId: string, assetId: string) {
  return adminRequest<SmartVideoAnalysisSummary>({
    method: "POST",
    url: `${projectPath(projectId)}/assets/${encodeURIComponent(assetId)}/retry-analysis`
  });
}

export async function createSmartVideoPlanTask(projectId: string, input: CreateSmartVideoPlanTaskInput = {}) {
  const key = input.idempotencyKey;
  const body = { ...input };
  delete (body as { idempotencyKey?: string }).idempotencyKey;
  return adminRequest<SmartVideoPlanTask>({
    method: "POST",
    url: `${projectPath(projectId)}/plan-tasks`,
    data: { ...body, idempotencyKey: key },
    headers: withIdempotency({}, key)
  });
}

export async function getSmartVideoPlanTask(projectId: string, taskId: string) {
  return adminRequest<SmartVideoPlanTask>({
    method: "GET",
    url: `${projectPath(projectId)}/plan-tasks/${encodeURIComponent(taskId)}`
  });
}

export async function listSmartVideoVersions(projectId: string) {
  const res = await adminRequest<{ items: SmartVideoProjectVersion[] }>({
    method: "GET",
    url: `${projectPath(projectId)}/versions`
  });
  return res.items || [];
}

export async function getSmartVideoVersion(projectId: string, versionId: string) {
  return adminRequest<SmartVideoProjectVersion>({
    method: "GET",
    url: `${projectPath(projectId)}/versions/${encodeURIComponent(versionId)}`
  });
}

export async function reviseSmartVideoVersion(projectId: string, versionId: string, input: ReviseSmartVideoPlanInput) {
  return adminRequest<SmartVideoProjectVersion>({
    method: "POST",
    url: `${projectPath(projectId)}/versions/${encodeURIComponent(versionId)}/revisions`,
    data: input
  });
}

export async function confirmSmartVideoVersion(projectId: string, versionId: string) {
  return adminRequest<{ project: SmartVideoProject; confirmedVersion: SmartVideoProjectVersion }>({
    method: "POST",
    url: `${projectPath(projectId)}/versions/${encodeURIComponent(versionId)}/confirm`
  });
}

export async function estimateSmartVideoRender(projectId: string, versionId: string) {
  return adminRequest<SmartVideoRenderQuote>({
    method: "GET",
    url: `${projectPath(projectId)}/versions/${encodeURIComponent(versionId)}/render-estimate`
  });
}

export async function createSmartVideoExport(projectId: string, input: CreateSmartVideoExportInput) {
  const key = input.idempotencyKey;
  return adminRequest<SmartVideoRenderTask>({
    method: "POST",
    url: `${projectPath(projectId)}/render-tasks`,
    data: { versionId: input.versionId, idempotencyKey: key },
    headers: withIdempotency({}, key)
  });
}

export async function getSmartVideoRenderTask(projectId: string, taskId: string) {
  return adminRequest<SmartVideoRenderTask>({
    method: "GET",
    url: `${projectPath(projectId)}/render-tasks/${encodeURIComponent(taskId)}`
  });
}

export async function cancelSmartVideoRenderTask(projectId: string, taskId: string) {
  return adminRequest<SmartVideoRenderTask>({
    method: "POST",
    url: `${projectPath(projectId)}/render-tasks/${encodeURIComponent(taskId)}/cancel`
  });
}

export async function retrySmartVideoRenderTask(projectId: string, taskId: string) {
  return adminRequest<SmartVideoRenderTask>({
    method: "POST",
    url: `${projectPath(projectId)}/render-tasks/${encodeURIComponent(taskId)}/retry`,
    headers: withIdempotency()
  });
}

export interface MultipartUploadProgress {
  uploadedBytes: number;
  totalBytes: number;
  completedParts: number;
  totalParts: number;
  status: "uploading" | "paused" | "completing" | "completed" | "aborted" | "failed";
}

export interface MultipartUploadHandle {
  promise: Promise<{ fileId: string; objectKey: string }>;
  pause: () => void;
  resume: () => void;
  abort: () => Promise<void>;
}

interface MultipartSession {
  uploadId: string;
  fileId: string;
  objectKey: string;
  partSize: number;
  totalParts: number;
}

interface MultipartPartTicket {
  partNumber: number;
  uploadUrl: string;
  headers?: Record<string, string>;
}

async function initMultipartUpload(input: {
  fileName: string;
  fileSize: number;
  mimeType?: string;
  businessType?: string;
  businessId?: string;
  partSize?: number;
}) {
  return adminRequest<MultipartSession>({
    method: "POST",
    url: "/files/upload/multipart/init",
    data: input,
    headers: withIdempotency({}, `mpu_${input.fileName}_${input.fileSize}`)
  });
}

async function presignMultipartPart(uploadId: string, partNumber: number) {
  return adminRequest<MultipartPartTicket>({
    method: "POST",
    url: `/files/upload/multipart/${encodeURIComponent(uploadId)}/parts/${partNumber}`
  });
}

async function completeMultipartUpload(uploadId: string, parts: Array<{ partNumber: number; etag: string }>) {
  return adminRequest<{ file: { fileId: string; objectKey: string } }>({
    method: "POST",
    url: `/files/upload/multipart/${encodeURIComponent(uploadId)}/complete`,
    data: { parts }
  });
}

async function abortMultipartUpload(uploadId: string) {
  await adminRequest<void>({
    method: "POST",
    url: `/files/upload/multipart/${encodeURIComponent(uploadId)}/abort`
  });
}

function sleep(ms: number) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

function extractEtag(response: Response) {
  const raw = response.headers.get("etag") || response.headers.get("ETag") || "";
  return raw.replace(/^W\//, "").trim() || `"part-${Date.now()}"`;
}

/**
 * Resumable multipart upload for workbench media.
 * Uses File Center multipart APIs; PUT goes directly to the signed URL.
 */
export function uploadSmartVideoBlob(
  source: File | { name: string; type?: string; blob: Blob },
  options: {
    businessId?: string;
    partSize?: number;
    concurrency?: number;
    maxRetries?: number;
    onProgress?: (progress: MultipartUploadProgress) => void;
  } = {}
): MultipartUploadHandle {
  const name = "name" in source && typeof source.name === "string" ? source.name : "upload.bin";
  const blob = source instanceof Blob ? source : source.blob;
  const type = ("type" in source && source.type) || "application/octet-stream";
  const partSize = Math.max(options.partSize || 8 * 1024 * 1024, 5 * 1024 * 1024);
  const concurrency = Math.max(1, Math.min(options.concurrency || 3, 6));
  const maxRetries = options.maxRetries ?? 3;
  let paused = false;
  let aborted = false;
  let uploadId = "";

  const pause = () => {
    paused = true;
  };
  const resume = () => {
    paused = false;
  };
  const abort = async () => {
    aborted = true;
    if (uploadId) {
      try {
        await abortMultipartUpload(uploadId);
      } catch {
        // best-effort
      }
    }
  };

  const waitIfPaused = async () => {
    while (paused && !aborted) await sleep(120);
    if (aborted) throw new Error("上传已取消");
  };

  const promise = (async () => {
    const session = await initMultipartUpload({
      fileName: name,
      fileSize: blob.size,
      mimeType: type,
      businessType: "smart_video",
      businessId: options.businessId,
      partSize
    });
    uploadId = session.uploadId;
    const totalParts = session.totalParts || Math.ceil(blob.size / (session.partSize || partSize));
    const effectivePartSize = session.partSize || partSize;
    const completed: Array<{ partNumber: number; etag: string }> = [];
    let nextPart = 1;
    let uploadedBytes = 0;

    const report = (status: MultipartUploadProgress["status"]) => {
      options.onProgress?.({
        uploadedBytes,
        totalBytes: blob.size,
        completedParts: completed.length,
        totalParts,
        status
      });
    };
    report("uploading");

    const uploadOne = async (partNumber: number) => {
      await waitIfPaused();
      const start = (partNumber - 1) * effectivePartSize;
      const end = Math.min(blob.size, start + effectivePartSize);
      const chunk = blob.slice(start, end);
      let lastError: unknown;
      for (let attempt = 0; attempt <= maxRetries; attempt += 1) {
        try {
          await waitIfPaused();
          const ticket = await presignMultipartPart(uploadId, partNumber);
          // Signed object storage PUT must go through the approved client transport.
          const response = await adminFetchResponse(
            ticket.uploadUrl,
            {
              method: "PUT",
              headers: { ...(ticket.headers || {}) },
              body: chunk
            },
            { auth: false }
          );
          completed.push({ partNumber, etag: extractEtag(response) });
          uploadedBytes += chunk.size;
          report(paused ? "paused" : "uploading");
          return;
        } catch (error) {
          lastError = error;
          if (aborted) throw error;
          await sleep(Math.min(2000, 200 * 2 ** attempt));
        }
      }
      throw lastError instanceof Error ? lastError : new Error(`分片 ${partNumber} 上传失败`);
    };

    const workers = Array.from({ length: Math.min(concurrency, totalParts) }, async () => {
      while (true) {
        if (aborted) throw new Error("上传已取消");
        const partNumber = nextPart;
        nextPart += 1;
        if (partNumber > totalParts) return;
        await uploadOne(partNumber);
      }
    });
    await Promise.all(workers);
    report("completing");
    completed.sort((a, b) => a.partNumber - b.partNumber);
    const result = await completeMultipartUpload(uploadId, completed);
    report("completed");
    return {
      fileId: result.file.fileId,
      objectKey: result.file.objectKey
    };
  })().catch(async (error) => {
    options.onProgress?.({
      uploadedBytes: 0,
      totalBytes: blob.size,
      completedParts: 0,
      totalParts: 0,
      status: aborted ? "aborted" : "failed"
    });
    if (uploadId && !aborted) {
      try {
        await abortMultipartUpload(uploadId);
      } catch {
        // ignore
      }
    }
    throw error;
  });

  return { promise, pause, resume, abort };
}

/** Expose axios client for tests that stub adapters. */
export function smartVideoApiClient() {
  return apiClient;
}
