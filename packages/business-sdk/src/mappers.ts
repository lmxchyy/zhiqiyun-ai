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

const draftOnlyParameterKeys = new Set([
  "mode",
  "prompt",
  "model",
  "modelName",
  "style",
  "stylePreset",
  "size",
  "aspectRatio",
  "aspect_ratio",
  "imageRatio",
  "quality",
  "imageQuality",
  "count",
  "imageCount",
  "generationCount",
  "referencePaths",
  "referenceImages",
  "files",
  "selectedFiles",
  "negativePrompt",
  "negative_prompt",
  "duration",
  "templateId",
  "contentType",
  "intent",
  "index",
  "providerRevisedPrompt",
  "provider_revised_prompt",
  "referenceCount",
  "type",
  "module_code",
  "billing_type",
  "sourceType",
  "contentType",
  "source",
  "provider",
  "providerTaskId",
  "provider_task_id",
  "thumbnailUrl",
  "thumbnail_url",
  "width",
  "height",
  "resolution",
  "fileId",
  "storageFileId",
  "storageTenantId",
  "storageProvider",
  "storageBucket",
  "storageObjectKey",
  "fileSize",
  "fileSizeBytes",
  "sourceUrl",
  "storageManaged",
  "inputImageIds",
  "inputImagesSnapshot",
  "terminal",
  "provider_name",
  "provider_company",
  "algorithm_name",
  "algorithm_filing_no",
  "algorithm_type",
  "model_version",
  "input_audit_status",
  "input_audit_service",
  "input_audit_request_id",
  "output_audit_status",
  "output_audit_service",
  "output_audit_request_id",
  "output_audit_reason",
  "ai_generated",
  "ai_label_status",
  "ai_label_text",
  "content_id",
  "generated_at",
  "download_derivative_required",
  "sourceAssetId",
  "sourceTaskId",
  "restoredParams",
  "slideCount",
  "dynamic",
]);

const videoParameterKeys = new Set([
  "duration",
  "resolution",
  "aspect_ratio",
  "fps",
  "motion_strength",
  "camera_movement",
  "generate_audio",
  "negative_prompt",
  "sourceReferenceAssetId",
  "sourceReferenceTaskId",
]);

const videoImageParameterKeys = [
  "reference_image",
  "first_frame",
  "last_frame",
  "image_url",
  "imageUrl",
  "image_urls",
  "imageUrls",
  "inputImageUrl",
  "input_image_url",
  "inputImageUrls",
  "referenceImages",
  "reference_images",
  "inputImages",
  "inputImagesSnapshot",
  "input_reference",
] as const;

const safeVideoCapabilities: VideoModelCapabilities = {
  supportsTextToVideo: true,
  supportsImageToVideo: false,
  supportsFirstFrame: false,
  supportsLastFrame: false,
  maxReferenceImages: 0,
  supportedDurations: [],
  supportedResolutions: [],
  supportedAspectRatios: [],
  supportedParameters: ["duration", "resolution", "aspect_ratio"],
};

export class VideoGenerationValidationError extends Error {
  readonly code: string;

  constructor(code: string, message: string) {
    super(message);
    this.name = "VideoGenerationValidationError";
    this.code = code;
  }
}

function recordValue(value: unknown): Record<string, unknown> {
  return value && typeof value === "object" && !Array.isArray(value)
    ? value as Record<string, unknown>
    : {};
}

function firstDefined(record: Record<string, unknown>, ...keys: string[]): unknown {
  for (const key of keys) {
    if (record[key] !== undefined && record[key] !== null) return record[key];
  }
  return undefined;
}

function strictBoolean(record: Record<string, unknown>, ...keys: string[]): boolean {
  return firstDefined(record, ...keys) === true;
}

function uniqueStrings(value: unknown): string[] {
  if (!Array.isArray(value)) return [];
  return [...new Set(value.map(item => String(item || "").trim()).filter(Boolean))];
}

function uniquePositiveNumbers(value: unknown): number[] {
  if (!Array.isArray(value)) return [];
  return [...new Set(value.map(Number).filter(item => Number.isFinite(item) && item > 0))];
}

export function normalizeVideoModelCapabilities(value: unknown): VideoModelCapabilities {
  const root = recordValue(value);
  const nested = recordValue(firstDefined(root, "videoCapabilities", "video_capabilities"));
  const source = Object.keys(nested).length ? nested : root;
  if (!Object.keys(source).length) return { ...safeVideoCapabilities };

  const supportsTextToVideo = strictBoolean(source, "supportsTextToVideo", "supports_text_to_video");
  const supportsImageToVideo = strictBoolean(source, "supportsImageToVideo", "supports_image_to_video");
  const supportsFirstFrame = supportsImageToVideo && strictBoolean(source, "supportsFirstFrame", "supports_first_frame");
  const configuredMax = Math.max(0, Math.floor(Number(firstDefined(source, "maxReferenceImages", "max_reference_images")) || 0));
  const supportsLastFrame = supportsFirstFrame
    && strictBoolean(source, "supportsLastFrame", "supports_last_frame")
    && configuredMax >= 2;
  const supportedParameters = uniqueStrings(firstDefined(source, "supportedParameters", "supported_parameters"));

  return {
    supportsTextToVideo,
    supportsImageToVideo: supportsImageToVideo && supportsFirstFrame && configuredMax >= 1,
    supportsFirstFrame,
    supportsLastFrame,
    maxReferenceImages: supportsImageToVideo && supportsFirstFrame ? configuredMax : 0,
    supportedDurations: uniquePositiveNumbers(firstDefined(source, "supportedDurations", "supported_durations")),
    supportedResolutions: uniqueStrings(firstDefined(source, "supportedResolutions", "supported_resolutions")),
    supportedAspectRatios: uniqueStrings(firstDefined(source, "supportedAspectRatios", "supported_aspect_ratios")),
    supportedParameters: supportedParameters.length
      ? supportedParameters
      : ["duration", "resolution", "aspect_ratio"],
  };
}

export function reconcileVideoGenerationState(
  state: { mode?: VideoGenerationMode; firstFrame?: string; lastFrame?: string },
  capabilitiesValue: unknown,
) {
  const capabilities = normalizeVideoModelCapabilities(capabilitiesValue);
  const requestedMode = state.mode === "IMAGE_TO_VIDEO" ? "IMAGE_TO_VIDEO" : "TEXT_TO_VIDEO";
  let mode: VideoGenerationMode = requestedMode;
  if (requestedMode === "TEXT_TO_VIDEO" && !capabilities.supportsTextToVideo) {
    mode = capabilities.supportsImageToVideo ? "IMAGE_TO_VIDEO" : "TEXT_TO_VIDEO";
  } else if (requestedMode === "IMAGE_TO_VIDEO" && !capabilities.supportsImageToVideo) {
    mode = capabilities.supportsTextToVideo ? "TEXT_TO_VIDEO" : "IMAGE_TO_VIDEO";
  }

  const originalFirstFrame = String(state.firstFrame || "").trim();
  const originalLastFrame = String(state.lastFrame || "").trim();
  const firstFrame = mode === "IMAGE_TO_VIDEO" ? originalFirstFrame : "";
  const lastFrame = mode === "IMAGE_TO_VIDEO" && capabilities.supportsLastFrame && capabilities.maxReferenceImages >= 2
    ? originalLastFrame
    : "";

  return {
    mode,
    firstFrame,
    lastFrame,
    modeChanged: mode !== requestedMode,
    clearedFirstFrame: Boolean(originalFirstFrame && !firstFrame),
    clearedLastFrame: Boolean(originalLastFrame && !lastFrame),
  };
}

function imageURLFromUnknown(value: unknown): string[] {
  if (typeof value === "string") return value.trim() ? [value.trim()] : [];
  if (Array.isArray(value)) return value.flatMap(imageURLFromUnknown);
  const record = recordValue(value);
  for (const key of ["url", "imageUrl", "image_url", "src"]) {
    const url = String(record[key] || "").trim();
    if (url) return [url];
  }
  return [];
}

function videoParameterImageURLs(parameters?: Record<string, unknown>): string[] {
  if (!parameters) return [];
  const merged = { ...recordValue(parameters.restoredParams), ...parameters };
  return videoImageParameterKeys.flatMap(key => imageURLFromUnknown(merged[key]));
}

function validateSupportedVideoValue<T extends string | number>(
  value: T,
  supported: T[],
  code: string,
  label: string,
) {
  if (supported.length && !supported.includes(value)) {
    throw new VideoGenerationValidationError(code, `当前模型不支持${label} ${value}`);
  }
}

function pickAllowedParameters(
  parameters: Record<string, unknown>,
  allowedKeys: Set<string>,
): Record<string, unknown> {
  const result: Record<string, unknown> = {};
  for (const key of allowedKeys) {
    if (parameters[key] !== undefined) result[key] = parameters[key];
  }
  return result;
}

export function generationParametersFromDraft(parameters?: Record<string, unknown>): Record<string, unknown> {
  if (!parameters) return {};
  const result: Record<string, unknown> = {
    ...recordValue(parameters.restoredParams),
    ...parameters,
  };
  const sourceAssetId = result.sourceAssetId;
  const sourceTaskId = result.sourceTaskId;
  for (const key of draftOnlyParameterKeys) delete result[key];
  if (typeof sourceAssetId === "string" && sourceAssetId.trim()) {
    result.sourceReferenceAssetId = sourceAssetId.trim();
  }
  if (typeof sourceTaskId === "string" && sourceTaskId.trim()) {
    result.sourceReferenceTaskId = sourceTaskId.trim();
  }
  for (const [key, value] of Object.entries(result)) {
    if (value === undefined) delete result[key];
  }
  return result;
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
  const extraParameters = generationParametersFromDraft(draft.parameters);
  const params = draft.mode === "video"
    ? (() => {
        const videoMode: VideoGenerationMode = type === "IMAGE_TO_VIDEO" ? "IMAGE_TO_VIDEO" : "TEXT_TO_VIDEO";
        const capabilities = normalizeVideoModelCapabilities(draft.videoCapabilities);
        const firstFrame = String(draft.firstFrame || "").trim();
        const lastFrame = String(draft.lastFrame || "").trim();
        const allImageURLs = [...new Set([
          ...referenceImages,
          ...videoParameterImageURLs(draft.parameters),
          firstFrame,
          lastFrame,
        ].filter(Boolean))];

        if (videoMode === "TEXT_TO_VIDEO") {
          if (!capabilities.supportsTextToVideo) {
            throw new VideoGenerationValidationError("VIDEO_MODE_NOT_SUPPORTED", "当前模型不支持文生视频模式");
          }
          if (allImageURLs.length) {
            throw new VideoGenerationValidationError("VIDEO_TEXT_MODE_IMAGE_FORBIDDEN", "文生视频模式不得携带首帧图、尾帧图或其他图片字段");
          }
        } else {
          if (!capabilities.supportsImageToVideo || !capabilities.supportsFirstFrame) {
            throw new VideoGenerationValidationError("VIDEO_MODE_NOT_SUPPORTED", "当前模型不支持图生视频模式");
          }
          if (!firstFrame) {
            throw new VideoGenerationValidationError("VIDEO_FIRST_FRAME_REQUIRED", "图生视频模式必须上传首帧图");
          }
          if (lastFrame && !capabilities.supportsLastFrame) {
            throw new VideoGenerationValidationError("VIDEO_LAST_FRAME_NOT_SUPPORTED", "当前模型不支持尾帧图");
          }
          if (allImageURLs.length > capabilities.maxReferenceImages) {
            throw new VideoGenerationValidationError("VIDEO_IMAGE_LIMIT_EXCEEDED", `当前模型最多允许 ${capabilities.maxReferenceImages} 张视频输入图片`);
          }
        }

        const supportedParameterKeys = capabilities.supportedParameters || [];
        const duration = draft.duration || Number(extraParameters.duration) || 5;
        const resolution = draft.quality || String(extraParameters.resolution || "") || "720p";
        const aspectRatio = draft.size || String(extraParameters.aspect_ratio || "") || "16:9";
        if (supportedParameterKeys.includes("duration")) {
          validateSupportedVideoValue(duration, capabilities.supportedDurations, "VIDEO_DURATION_NOT_SUPPORTED", "视频时长");
        }
        if (supportedParameterKeys.includes("resolution")) {
          validateSupportedVideoValue(resolution, capabilities.supportedResolutions, "VIDEO_RESOLUTION_NOT_SUPPORTED", "分辨率");
        }
        if (supportedParameterKeys.includes("aspect_ratio")) {
          validateSupportedVideoValue(aspectRatio, capabilities.supportedAspectRatios, "VIDEO_ASPECT_RATIO_NOT_SUPPORTED", "画面比例");
        }

        const providerParameters = pickAllowedParameters(extraParameters, videoParameterKeys);
        for (const key in providerParameters) {
          if (!supportedParameterKeys.includes(key)) delete providerParameters[key];
        }

        return {
          ...providerParameters,
          ...(supportedParameterKeys.includes("duration") ? { duration } : {}),
          ...(supportedParameterKeys.includes("resolution") ? { resolution } : {}),
          ...(supportedParameterKeys.includes("aspect_ratio") ? { aspect_ratio: aspectRatio } : {}),
          ...(draft.negativePrompt ? { negative_prompt: draft.negativePrompt } : {}),
          ...(videoMode === "IMAGE_TO_VIDEO" ? { first_frame: firstFrame } : {}),
          ...(videoMode === "IMAGE_TO_VIDEO" && lastFrame ? { last_frame: lastFrame } : {}),
        };
      })()
    : {
        ...extraParameters,
        size: draft.size || "1024x1024",
        quality: draft.quality || "standard",
        n: draft.count || 1,
        ...(draft.negativePrompt ? { negative_prompt: draft.negativePrompt } : {}),
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

export async function confirmResolvedVideoModel(
  requestedModel: string,
  resolvedModel: string,
  confirmSwitch: (message: string) => Promise<boolean>,
): Promise<string | null> {
  if (!resolvedModel || resolvedModel === requestedModel) return resolvedModel || requestedModel;
  return await confirmSwitch(`当前模型不可用，是否切换为 ${resolvedModel}？`) ? resolvedModel : null;
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
