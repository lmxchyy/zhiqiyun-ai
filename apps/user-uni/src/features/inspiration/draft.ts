import type { InspirationCreationDraft } from "./types.ts";

export const inspirationDraftKey = "xianzhi:inspiration-template-draft";

function validDraft(value: unknown): value is InspirationCreationDraft {
  if (!value || typeof value !== "object") return false;
  const draft = value as Partial<InspirationCreationDraft>;
  return draft.contractVersion === 1
    && Boolean(draft.templateRef?.id && draft.templateRef?.slug && draft.templateRef?.version)
    && Boolean(draft.contentType && draft.handoff?.targetType)
    && typeof draft.basePrompt === "string"
    && Boolean(draft.capabilityKey && draft.integrityToken && draft.createdAt && draft.expiresAt)
    && Boolean(draft.values && typeof draft.values === "object" && !Array.isArray(draft.values))
    && Array.isArray(draft.materials)
    && draft.materials.every(item => Boolean(item?.inputKey && item?.assetId))
    && Boolean(draft.parameters && typeof draft.parameters === "object");
}

export function saveInspirationDraft(draft: InspirationCreationDraft, now = Date.now()) {
  if (!validDraft(draft)) throw new Error("Creation Draft 契约无效");
  const createdAt = Date.parse(draft.createdAt);
  const expiresAt = Date.parse(draft.expiresAt);
  if (!Number.isFinite(createdAt) || !Number.isFinite(expiresAt) || expiresAt <= createdAt) {
    throw new Error("Creation Draft 有效期无效");
  }
  if (expiresAt <= now) throw new Error("Creation Draft 已过期，请重新使用模板");
  uni.setStorageSync(inspirationDraftKey, draft);
  return draft;
}

export function readInspirationDraft(templateId = "", now = Date.now()) {
  const draft = uni.getStorageSync(inspirationDraftKey) as InspirationCreationDraft | undefined;
  if (!validDraft(draft) || (templateId && draft.templateRef.id !== templateId) || Date.parse(draft.expiresAt) <= now) {
    if (draft) uni.removeStorageSync(inspirationDraftKey);
    return null;
  }
  return draft;
}

export function inspirationReferenceValidationMessage(draft: InspirationCreationDraft, referenceCount: number) {
  const requiredCount = draft.materials.length;
  if (requiredCount > 0 && referenceCount < requiredCount) return `请保留模板要求的 ${requiredCount} 个参考素材`;
  return "";
}

export function inspirationReferenceLimit(draft: InspirationCreationDraft | null | undefined, fallback = 3) {
  return Math.max(1, fallback, draft?.materials.length || 0);
}

export async function resolveInspirationDraftMaterialURLs(
  draft: InspirationCreationDraft,
  loadAsset: (assetId: string) => Promise<{ remoteUrl?: string; thumbnailUrl?: string }>,
  normalizeURL: (value: string) => string = value => value,
) {
  const uniqueIDs = [...new Set(draft.materials.map(item => item.assetId).filter(Boolean))];
  const assets = await Promise.all(uniqueIDs.map(assetId => loadAsset(assetId)));
  return assets.map(item => String(item.remoteUrl || item.thumbnailUrl || "").trim()).filter(Boolean).map(normalizeURL);
}
