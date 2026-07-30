import type {
  InspirationDetailResponse,
  InspirationDisplayConfig,
  InspirationInputRequirements,
  InspirationPresetConfig,
} from "./types";

export const inspirationDraftKey = "xianzhi:inspiration-template-draft";

export interface InspirationCreationDraft {
  templateId: string;
  contentType: "image" | "video" | "ppt";
  prompt: string;
  negativePrompt: string;
  modelId: string;
  scenarioCode: string;
  displayConfig: InspirationDisplayConfig;
  inputRequirements: InspirationInputRequirements;
  presetConfig: InspirationPresetConfig;
  parameters: Record<string, unknown>;
  referenceAssets: unknown[];
  modelFallbackApplied: boolean;
  createdAt: number;
}

export function saveInspirationDraft(payload: InspirationDetailResponse) {
  const modelId = payload.modelAvailable ? String(payload.item.modelId || "") : String(payload.compatibleModelId || "");
  const scenarioCode = String(payload.item.scenarioCode || "").trim();
  const inputRequirements = payload.item.inputRequirements || {};
  const requiresUserPhoto = scenarioCode === "photo_restoration" && inputRequirements.referenceImageRequired === true;
  const draft: InspirationCreationDraft = {
    templateId: payload.item.id,
    contentType: payload.item.contentType,
    prompt: String(payload.item.prompt || ""),
    negativePrompt: String(payload.item.negativePrompt || ""),
    modelId,
    scenarioCode,
    displayConfig: payload.item.displayConfig || {},
    inputRequirements,
    presetConfig: payload.item.presetConfig || {},
    parameters: payload.item.parameters || {},
    referenceAssets: requiresUserPhoto ? [] : payload.item.referenceAssets || [],
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

export function inspirationReferenceValidationMessage(
  draft: Pick<InspirationCreationDraft, "scenarioCode" | "inputRequirements">,
  referenceCount: number,
) {
  const requirements = draft.inputRequirements || {};
  const required = requirements.referenceImageRequired === true;
  const minimum = required ? Math.max(1, Number(requirements.referenceImageMin || 1)) : Math.max(0, Number(requirements.referenceImageMin || 0));
  const maximum = Math.max(minimum, Number(requirements.referenceImageMax ?? Math.max(minimum, 3)));
  if (referenceCount < minimum) {
    return draft.scenarioCode === "photo_restoration" ? "请先上传需要修复的照片" : `请至少上传 ${minimum} 张参考图片`;
  }
  if (referenceCount > maximum) return `最多上传 ${maximum} 张参考图片`;
  return "";
}

export function inspirationReferenceLimit(
  draft: Pick<InspirationCreationDraft, "inputRequirements"> | null | undefined,
  fallback = 3,
) {
  const configured = Number(draft?.inputRequirements?.referenceImageMax);
  if (!Number.isFinite(configured) || configured < 1) return fallback;
  return Math.max(1, Math.min(fallback, Math.trunc(configured)));
}
