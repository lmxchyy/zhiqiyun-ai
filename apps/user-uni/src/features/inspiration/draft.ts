import type { InspirationDetailResponse } from "./types";

export const inspirationDraftKey = "xianzhi:inspiration-template-draft";

export interface InspirationCreationDraft {
  templateId: string;
  contentType: "image" | "video" | "ppt";
  prompt: string;
  negativePrompt: string;
  modelId: string;
  parameters: Record<string, unknown>;
  referenceAssets: unknown[];
  modelFallbackApplied: boolean;
  createdAt: number;
}

export function saveInspirationDraft(payload: InspirationDetailResponse) {
  const modelId = payload.modelAvailable ? String(payload.item.modelId || "") : String(payload.compatibleModelId || "");
  const draft: InspirationCreationDraft = {
    templateId: payload.item.id,
    contentType: payload.item.contentType,
    prompt: String(payload.item.prompt || ""),
    negativePrompt: String(payload.item.negativePrompt || ""),
    modelId,
    parameters: payload.item.parameters || {},
    referenceAssets: payload.item.referenceAssets || [],
    modelFallbackApplied: !payload.modelAvailable,
    createdAt: Date.now(),
  };
  uni.setStorageSync(inspirationDraftKey, draft);
  return draft;
}

export function readInspirationDraft(templateId = "") {
  const draft = uni.getStorageSync(inspirationDraftKey) as InspirationCreationDraft | undefined;
  if (!draft || (templateId && draft.templateId !== templateId) || Date.now() - Number(draft.createdAt || 0) > 30 * 60 * 1000) return null;
  return draft;
}
