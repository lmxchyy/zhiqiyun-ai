import {
  createMultipartUploadController,
  createSmartVideoSdk,
  type MultipartUploadControllerOptions,
  type MultipartUploadHandle,
} from "@xianzhi/business-sdk";
import type {
  CreateSmartVideoAssetInput,
  CreateSmartVideoExportInput,
  CreateSmartVideoPlanTaskInput,
  CreateSmartVideoProjectInput,
  ReviseSmartVideoPlanInput,
  SmartVideoAssetType,
  UpdateSmartVideoProjectInput,
} from "@xianzhi/shared-types";
import { apiClient } from "./client";

export type {
  CreateSmartVideoAssetInput,
  CreateSmartVideoExportInput,
  CreateSmartVideoPlanTaskInput,
  CreateSmartVideoProjectInput,
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
  UpdateSmartVideoProjectInput,
} from "@xianzhi/shared-types";

export const smartVideoApi = createSmartVideoSdk(apiClient);
export const multipartUpload = createMultipartUploadController(apiClient);

export type LocalMediaPick = {
  path: string;
  name: string;
  mimeType: string;
  size?: number;
  assetType: SmartVideoAssetType;
};

function guessMimeType(name: string, assetType: SmartVideoAssetType) {
  const lower = name.toLowerCase();
  if (assetType === "IMAGE") {
    if (lower.endsWith(".png")) return "image/png";
    if (lower.endsWith(".webp")) return "image/webp";
    if (lower.endsWith(".gif")) return "image/gif";
    return "image/jpeg";
  }
  if (lower.endsWith(".mov")) return "video/quicktime";
  if (lower.endsWith(".m4v")) return "video/x-m4v";
  if (lower.endsWith(".webm")) return "video/webm";
  return "video/mp4";
}

function readLocalBlob(filePath: string, mimeType: string): Promise<Blob> {
  return new Promise((resolve, reject) => {
    const fs = uni.getFileSystemManager();
    fs.readFile({
      filePath,
      success: (result) => {
        try {
          const data = result.data as ArrayBuffer | string;
          const buffer = typeof data === "string" ? new TextEncoder().encode(data) : data;
          resolve(new Blob([buffer], { type: mimeType || "application/octet-stream" }));
        } catch (error) {
          reject(error);
        }
      },
      fail: (error) => reject(new Error(error.errMsg || "读取本地文件失败")),
    });
  });
}

function getLocalFileSize(filePath: string): Promise<number> {
  return new Promise((resolve, reject) => {
    uni.getFileInfo({
      filePath,
      success: (info) => resolve(Number(info.size) || 0),
      fail: (error) => reject(new Error(error.errMsg || "获取文件信息失败")),
    });
  });
}

/** Upload a local mini-program media file via multipart, then return file center id. */
export function uploadSmartVideoLocalFile(
  media: LocalMediaPick,
  options: Omit<MultipartUploadControllerOptions, "fetcher"> & { businessId?: string } = {},
): MultipartUploadHandle {
  const mimeType = media.mimeType || guessMimeType(media.name, media.assetType);
  let paused = false;
  let aborted = false;
  let inner: MultipartUploadHandle | null = null;

  const promise = (async () => {
    const size = media.size && media.size > 0 ? media.size : await getLocalFileSize(media.path);
    if (!size) throw new Error("素材文件为空，请重新选择");
    const blob = await readLocalBlob(media.path, mimeType);
    if (aborted) throw new Error("multipart upload aborted");
    inner = multipartUpload.uploadBlob(
      { name: media.name || `media_${Date.now()}`, type: mimeType, blob },
      {
        businessType: "smart_video",
        businessId: options.businessId,
        visibility: "PRIVATE",
      },
      {
        partSize: options.partSize,
        concurrency: options.concurrency,
        maxRetries: options.maxRetries,
        onProgress: (progress) => {
          if (paused && progress.status === "uploading") return;
          options.onProgress?.(progress);
        },
        signal: options.signal,
      },
    );
    if (paused) inner.pause();
    if (aborted) {
      await inner.abort();
      throw new Error("multipart upload aborted");
    }
    return inner.promise;
  })();

  return {
    promise,
    pause() {
      paused = true;
      inner?.pause();
    },
    resume() {
      paused = false;
      inner?.resume();
    },
    async abort() {
      aborted = true;
      if (inner) await inner.abort();
    },
  };
}

export function assetTypeFromMime(mimeType: string, fallback: SmartVideoAssetType = "VIDEO"): SmartVideoAssetType {
  if (String(mimeType || "").toLowerCase().startsWith("image/")) return "IMAGE";
  if (String(mimeType || "").toLowerCase().startsWith("video/")) return "VIDEO";
  return fallback;
}

export async function createSmartVideoProject(input: CreateSmartVideoProjectInput) {
  return smartVideoApi.createProject(input);
}

export async function getSmartVideoProject(projectId: string) {
  return smartVideoApi.getProject(projectId);
}

export async function updateSmartVideoProject(projectId: string, input: UpdateSmartVideoProjectInput) {
  return smartVideoApi.updateProject(projectId, input);
}

export async function listSmartVideoAssets(projectId: string) {
  return smartVideoApi.listAssets(projectId);
}

export async function addSmartVideoAsset(projectId: string, input: CreateSmartVideoAssetInput) {
  return smartVideoApi.addAsset(projectId, input);
}

export async function analyzeSmartVideoProject(projectId: string, idempotencyKey?: string) {
  return smartVideoApi.analyze(projectId, idempotencyKey);
}

export async function getSmartVideoAnalysis(projectId: string) {
  return smartVideoApi.getAnalysis(projectId);
}

export async function createSmartVideoPlanTask(projectId: string, input: CreateSmartVideoPlanTaskInput = {}) {
  return smartVideoApi.createPlanTask(projectId, input);
}

export async function getSmartVideoPlanTask(projectId: string, taskId: string) {
  return smartVideoApi.getPlanTask(projectId, taskId);
}

export async function listSmartVideoVersions(projectId: string) {
  return smartVideoApi.listVersions(projectId);
}

export async function getSmartVideoVersion(projectId: string, versionId: string) {
  return smartVideoApi.getVersion(projectId, versionId);
}

export async function reviseSmartVideoVersion(
  projectId: string,
  versionId: string,
  input: ReviseSmartVideoPlanInput,
) {
  return smartVideoApi.reviseVersion(projectId, versionId, input);
}

export async function confirmSmartVideoVersion(projectId: string, versionId: string) {
  return smartVideoApi.confirmVersion(projectId, versionId);
}

export async function estimateSmartVideoRender(projectId: string, versionId: string) {
  return smartVideoApi.estimateRender(projectId, versionId);
}

export async function createSmartVideoExport(projectId: string, input: CreateSmartVideoExportInput) {
  return smartVideoApi.createExport(projectId, input);
}

export async function getSmartVideoRenderTask(projectId: string, taskId: string) {
  return smartVideoApi.getRenderTask(projectId, taskId);
}

export async function cancelSmartVideoRenderTask(projectId: string, taskId: string) {
  return smartVideoApi.cancelRenderTask(projectId, taskId);
}

export async function retrySmartVideoRenderTask(projectId: string, taskId: string) {
  return smartVideoApi.retryRenderTask(projectId, taskId);
}
