import type {
  Asset,
  AuthResponse,
  CommercePlan,
  CreateDraft,
  CreateGenerationTaskRequest,
  FeatureEntry,
  GenerationTask,
  MembershipPlan,
  UserProfile,
  WorkItem,
  WorkStatus,
  WorkType
} from "@xianzhi/shared-types";

export const defaultFeatures: FeatureEntry[] = [
  { id: "image", title: "AI Image", subtitle: "Text to image and reference image generation", icon: "IMAGE", tone: "primary", path: "/pages/create/index" },
  { id: "video", title: "AI Video", subtitle: "Image/video generation workflows", icon: "VIDEO", tone: "accent", path: "/pages/create/index" },
  { id: "ppt", title: "PPT", subtitle: "Generate slide decks from topics", icon: "PPT", tone: "dark", path: "/pages/create/index" },
  { id: "agent", title: "Agent", subtitle: "Reusable business agents", icon: "AI", tone: "green", path: "/pages/agents/index" }
];

export function normalizeStatus(status: string): WorkStatus {
  const value = status.toUpperCase();
  if (value === "SUCCEEDED" || value === "COMPLETED") return "succeeded";
  if (value === "FAILED" || value === "ERROR") return "failed";
  if (value === "PROCESSING" || value === "RUNNING" || value === "RETRYING") return "processing";
  return "queued";
}

export function normalizeWorkType(asset: Asset | GenerationTask): WorkType {
  const rawType = "mediaType" in asset ? asset.mediaType : asset.type;
  const value = String(rawType || "").toLowerCase();
  if (value.includes("video")) return "video";
  if (value.includes("ppt") || value.includes("document")) return "ppt";
  return "image";
}

export function taskRequestFromDraft(draft: CreateDraft): CreateGenerationTaskRequest {
  const referenceImages = draft.referenceImages.filter(Boolean);
  const referenceImage = referenceImages[0];
  const type = draft.mode === "video"
    ? referenceImage ? "IMAGE_TO_VIDEO" : "TEXT_TO_VIDEO"
    : draft.mode === "ppt"
      ? "PPT_GENERATION"
      : referenceImage ? "IMAGE_TO_IMAGE" : "TEXT_TO_IMAGE";
  const referencePayload = referenceImages.map((url, index) => ({
    url,
    name: `reference-${index + 1}`,
  }));
  const params = draft.mode === "video"
    ? {
        duration: 5,
        resolution: draft.quality || "720p",
        aspect_ratio: draft.size || "16:9",
        generate_audio: true,
        ...(referenceImage ? { reference_image: referenceImage } : {}),
      }
    : {
        size: draft.size || "1024x1024",
        quality: draft.quality || "standard",
        n: draft.count || 1,
        ...(referenceImage ? { reference_image: referenceImage } : {}),
        ...(referencePayload.length ? { referenceImages: referencePayload } : {}),
      };
  return {
    type,
    moduleCode: draft.mode === "video" ? "video_generation" : "image_generation",
    prompt: draft.prompt,
    model: draft.model,
    params,
  };
}

export function profileFromAuth(auth: AuthResponse, points = 0): UserProfile {
  const name = auth.user.name || auth.user.email || "User";
  return {
    id: auth.user.id,
    name,
    avatarText: name.slice(0, 1).toUpperCase(),
    memberLevel: auth.user.memberLevel || auth.user.planId || "free",
    points,
    agentEnabled: Boolean(auth.agent)
  };
}

export function workFromAsset(asset: Asset): WorkItem {
  return {
    id: asset.id,
    title: asset.name || asset.metadata?.title?.toString() || asset.id,
    type: normalizeWorkType(asset),
    status: "succeeded",
    model: String(asset.metadata?.model || asset.metadata?.provider || "-"),
    prompt: String(asset.metadata?.prompt || asset.metadata?.description || ""),
    createdAt: asset.createdAt || asset.updatedAt || "",
    thumbnailUrl: asset.thumbnailUrl,
    url: asset.url
  };
}

export function workFromTask(task: GenerationTask): WorkItem {
  return {
    id: task.id,
    title: task.prompt || task.id,
    type: normalizeWorkType(task),
    status: normalizeStatus(task.status),
    model: task.model || "-",
    prompt: task.prompt || "",
    createdAt: task.createdAt || task.updatedAt || ""
  };
}

export function planFromCommercePlan(plan: CommercePlan): MembershipPlan {
  const points = Number(plan.rechargePoints || plan.pointAmount || plan.tokenAmount || 0);
  const priceCents = Number(plan.priceCents || plan.amountCents || 0);
  return {
    id: plan.id,
    name: plan.name,
    price: priceCents > 0 ? `\u00a5${(priceCents / 100).toFixed(0)}` : "Free",
    points,
    benefits: Array.isArray(plan.benefits) ? plan.benefits.map(String) : [],
    recommended: Boolean(plan.recommended || plan.metadata?.recommended)
  };
}
