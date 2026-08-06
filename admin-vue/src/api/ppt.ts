import { adminRequest, apiClient } from "./client";
import {
  mockCreatePptTask,
  mockDeletePptTask,
  mockExportPdf,
  mockGeneratePptImage,
  mockGeneratePptOutline,
  mockGetPptDraftHistory,
  mockGetPptHistory,
  mockGetPptTaskStatus,
  mockImageModels,
  mockRegeneratePptSlide,
  mockSavePptOutline,
  mockSearchPptImages,
  mockTextModels,
  mockUpsertPptDraft
} from "./ppt.mock";
import type {
  PptCreateSessionRequest,
  PptGenerateOutlineRequest,
  PptGenerateImageRequest,
  PptGenerateRequest,
  PptGenerateResponse,
  PptHistoryItem,
  PptImageOption,
  PptModelOption,
  PptOutline,
  PptRegenerateVisualRequest,
  PptRegenerateVisualResponse,
  PptSkill,
  PptSlide,
  PptTaskResponse
} from "../types/ppt";

export type {
  PptCreateMode,
  PptCreateSessionRequest,
  PptAgentMessage,
  PptAgentStage,
  PptGenerateOutlineRequest,
  PptGenerateImageRequest,
  PptGenerateRequest,
  PptGenerateResponse,
  PptHistoryItem,
  PptImageOption,
  PptImageSource,
  PptLanguage,
  PptModelOption,
  PptOutline,
  PptOutlineSlide,
  PptRegenerateVisualRequest,
  PptRegenerateVisualResponse,
  PptSlide,
  PptTaskResponse,
  PptTaskStatus,
  PptTheme,
  PptTextContent,
  PptAudience,
  PptScenario,
  PptSkill,
  PptSlideBlock,
  PptTone,
  PptWorkflowStatus
} from "../types/ppt";

export async function getPptSkills(): Promise<PptSkill[]> {
  return adminRequest<PptSkill[]>({
    method: "GET",
    url: "/ppt/skills"
  });
}

export async function createPptSession(request: PptCreateSessionRequest): Promise<PptTaskResponse> {
  return adminRequest<PptTaskResponse>({
    method: "POST",
    url: "/ppt/sessions",
    data: request
  });
}

export async function postPptSessionMessage(taskId: string, message: string, idempotencyKey: string): Promise<PptTaskResponse> {
  return adminRequest<PptTaskResponse>({
    method: "POST",
    url: `/ppt/sessions/${encodeURIComponent(taskId)}/messages`,
    headers: { "Idempotency-Key": idempotencyKey },
    data: { message }
  });
}

export async function confirmPptSessionOutline(taskId: string, idempotencyKey: string): Promise<PptTaskResponse> {
  return adminRequest<PptTaskResponse>({
    method: "POST",
    url: `/ppt/sessions/${encodeURIComponent(taskId)}/confirm-outline`,
    headers: { "Idempotency-Key": idempotencyKey },
    data: {}
  });
}

export async function getPptAgentTask(taskId: string): Promise<PptTaskResponse> {
  return adminRequest<PptTaskResponse>({
    method: "GET",
    url: `/ppt/tasks/${encodeURIComponent(taskId)}`
  });
}

export async function revisePptSessionSlide(taskId: string, slideId: string, instruction: string, idempotencyKey: string): Promise<PptTaskResponse> {
  return adminRequest<PptTaskResponse>({
    method: "POST",
    url: `/ppt/sessions/${encodeURIComponent(taskId)}/revise-slide`,
    headers: { "Idempotency-Key": idempotencyKey },
    data: { slideId, instruction }
  });
}

export async function generatePptOutline(request: PptGenerateOutlineRequest): Promise<PptOutline> {
  try {
    return await adminRequest<PptOutline>({
      method: "POST",
      url: "/ppt/outline/generate",
      data: request
    });
  } catch (error) {
    console.warn("[ppt] outline API fallback to mock", error);
    return mockGeneratePptOutline(request);
  }
}

export async function updatePptOutline(outline: PptOutline): Promise<PptOutline> {
  try {
    return await adminRequest<PptOutline>({
      method: "POST",
      url: "/ppt/outline/save",
      data: outline
    });
  } catch (error) {
    console.warn("[ppt] save outline API fallback to mock", error);
    return mockSavePptOutline(outline);
  }
}

export async function createPptTask(request: PptGenerateRequest): Promise<PptGenerateResponse> {
  try {
    return await adminRequest<PptGenerateResponse>({
      method: "POST",
      url: "/ppt/generate",
      data: request
    });
  } catch (error) {
    console.warn("[ppt] create task API fallback to mock", error);
    return mockCreatePptTask(request);
  }
}

export async function getPptTaskStatus(taskId: string): Promise<PptTaskResponse> {
  try {
    return await adminRequest<PptTaskResponse>({
      method: "GET",
      url: `/ppt/tasks/${encodeURIComponent(taskId)}`
    });
  } catch (error) {
    console.warn("[ppt] task status API fallback to mock", error);
    return mockGetPptTaskStatus(taskId);
  }
}

export async function getPptHistory(): Promise<PptHistoryItem[]> {
  try {
    const history = await adminRequest<PptHistoryItem[]>({
      method: "GET",
      url: "/ppt/history"
    });
    if (Array.isArray(history) && history.length) {
      return mergeLocalDrafts(history);
    }
    return mockGetPptHistory();
  } catch (error) {
    console.warn("[ppt] history API fallback to mock", error);
    return mockGetPptHistory();
  }
}

export async function upsertPptDraft(item: PptHistoryItem): Promise<PptHistoryItem> {
  return mockUpsertPptDraft(item);
}

export async function deletePptTask(taskId: string): Promise<void> {
  try {
    await adminRequest<{ ok: boolean }>({
      method: "DELETE",
      url: `/ppt/tasks/${encodeURIComponent(taskId)}`
    });
  } catch (error) {
    console.warn("[ppt] delete task API fallback to mock", error);
    await mockDeletePptTask(taskId);
  }
}

export async function regeneratePptSlide(slide: PptSlide): Promise<PptSlide> {
  try {
    return await adminRequest<PptSlide>({
      method: "POST",
      url: `/ppt/slides/${encodeURIComponent(slide.id)}/regenerate`,
      data: slide
    });
  } catch (error) {
    console.warn("[ppt] regenerate slide API fallback to mock", error);
    return mockRegeneratePptSlide(slide);
  }
}

export async function regeneratePptSlideVisual(presentationId: string, slideId: string, request: PptRegenerateVisualRequest): Promise<PptRegenerateVisualResponse> {
  return adminRequest<PptRegenerateVisualResponse>({
    method: "POST",
    url: `/presentations/${encodeURIComponent(presentationId)}/slides/${encodeURIComponent(slideId)}/regenerate-visual`,
    data: request
  });
}

export async function deletePptSlideVisual(presentationId: string, slideId: string): Promise<PptRegenerateVisualResponse> {
  return adminRequest<PptRegenerateVisualResponse>({
    method: "DELETE",
    url: `/presentations/${encodeURIComponent(presentationId)}/slides/${encodeURIComponent(slideId)}/visual`
  });
}

export async function restorePptSlideVisual(presentationId: string, slideId: string, createdAt: string, url: string, storageRef?: string): Promise<PptRegenerateVisualResponse> {
  return adminRequest<PptRegenerateVisualResponse>({
    method: "POST",
    url: `/presentations/${encodeURIComponent(presentationId)}/slides/${encodeURIComponent(slideId)}/visual/restore`,
    data: { createdAt, url, storageRef }
  });
}

export async function exportPpt(taskId: string): Promise<{ url: string; filename: string }> {
  const blob = (await apiClient.request({
    method: "POST",
    url: "/ppt/export/pptx",
    data: { taskId },
    responseType: "blob",
    headers: {
      Accept: "application/vnd.openxmlformats-officedocument.presentationml.presentation"
    }
  })) as Blob;
  if (!blob || blob.size === 0) {
    throw new Error("PPTX 导出接口没有返回文件内容");
  }
  return {
    url: URL.createObjectURL(blob),
    filename: `xianzhi-ppt-${taskId}.pptx`
  };
}

export async function exportPdf(taskId: string): Promise<{ url: string }> {
  try {
    return await adminRequest<{ url: string }>({
      method: "POST",
      url: "/ppt/export/pdf",
      data: { taskId }
    });
  } catch (error) {
    console.warn("[ppt] export pdf API fallback to mock", error);
    mockExportPdf();
  }
}

export async function generatePptImage(request: PptGenerateImageRequest): Promise<PptImageOption> {
  try {
    const image = await adminRequest<PptImageOption>({
      method: "POST",
      url: "/ppt/images/generate",
      data: request
    });
    if (image?.url) return image;
    if (!shouldRequireRealPptImage(request)) {
      console.warn("[ppt] generate image API returned empty url, fallback to mock");
      return mockGeneratePptImage(request.slide);
    }
    throw new Error("PPT 配图接口没有返回图片地址");
  } catch (error) {
    if (shouldRequireRealPptImage(request)) {
      throw error;
    }
    console.warn("[ppt] generate image API fallback to mock", error);
    return mockGeneratePptImage(request.slide);
  }
}

function shouldRequireRealPptImage(request: PptGenerateImageRequest) {
  const model = request.imageModel.trim();
  return Boolean(model && model !== "default-image");
}

export async function searchPptImages(keyword: string): Promise<PptImageOption[]> {
  try {
    const images = await adminRequest<PptImageOption[]>({
      method: "GET",
      url: "/ppt/images/search",
      params: { keyword }
    });
    const availableImages = Array.isArray(images) ? images.filter(image => image.url) : [];
    if (availableImages.length) return availableImages;
    console.warn("[ppt] search images API returned empty results, fallback to mock");
    return mockSearchPptImages(keyword);
  } catch (error) {
    console.warn("[ppt] search images API fallback to mock", error);
    return mockSearchPptImages(keyword);
  }
}

export async function getPptTextModels(): Promise<PptModelOption[]> {
  try {
    const models = await adminRequest<PptModelOption[]>({
      method: "GET",
      url: "/ppt/models/text"
    });
    return Array.isArray(models) && models.length ? models : mockTextModels;
  } catch (error) {
    console.warn("[ppt] text models API fallback to mock", error);
    return mockTextModels;
  }
}

export async function getPptImageModels(): Promise<PptModelOption[]> {
  try {
    const models = await adminRequest<PptModelOption[]>({
      method: "GET",
      url: "/ppt/models/image"
    });
    return Array.isArray(models) && models.length ? models : mockImageModels;
  } catch (error) {
    console.warn("[ppt] image models API fallback to mock", error);
    return mockImageModels;
  }
}

function mergeLocalDrafts(history: PptHistoryItem[]) {
  const drafts = mockGetPptDraftHistory();
  if (!drafts.length) return history;
  const remoteIds = new Set(history.map(item => item.taskId));
  return [...drafts.filter(item => !remoteIds.has(item.taskId)), ...history].sort((a, b) => {
    return new Date(b.updatedAt || b.createdAt || 0).getTime() - new Date(a.updatedAt || a.createdAt || 0).getTime();
  });
}

export const createPptGenerationTask = createPptTask;
export const getPptGenerationTask = getPptTaskStatus;
export const listPptHistory = getPptHistory;

export async function requestPptDownload(taskId: string): Promise<{ url: string }> {
  const task = await getPptTaskStatus(taskId);
  if (!task.pptUrl) {
    throw new Error("PPT 下载接口已预留，当前任务暂无下载地址");
  }
  return { url: task.pptUrl };
}
