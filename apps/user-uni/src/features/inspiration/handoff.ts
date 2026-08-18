import type { InspirationCreationDraft } from "./types";

export function inspirationDraftRoute(draft: Pick<InspirationCreationDraft, "contentType" | "handoff" | "templateRef">) {
  const templateId = encodeURIComponent(draft.templateRef.id);
  if (draft.handoff.targetType === "IMAGE_CREATION" && draft.contentType === "image") {
    return `/pages/user/UserImageCreationPage?templateId=${templateId}`;
  }
  if (draft.handoff.targetType === "VIDEO_CREATION" && draft.contentType === "video") {
    return `/pages/user/UserVideoCreationPage?templateId=${templateId}`;
  }
  if (draft.handoff.targetType === "PPT_CREATION" && draft.contentType === "ppt") {
    return `/packagePpt/pages/create?templateId=${templateId}`;
  }
  throw new Error("该模板目标创作能力暂不可用");
}

export async function completeInspirationHandoff(
  draft: InspirationCreationDraft,
  dependencies: {
    save: (draft: InspirationCreationDraft) => void;
    recordUse: () => Promise<unknown>;
    navigate: (url: string) => void;
  },
) {
  const url = inspirationDraftRoute(draft);
  dependencies.save(draft);
  try {
    void Promise.resolve(dependencies.recordUse()).catch(() => undefined);
  } catch {
    // Analytics must never block the handoff.
  }
  dependencies.navigate(url);
  return url;
}
