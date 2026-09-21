export type VideoPromptPreflightMode = "TEXT_TO_VIDEO" | "IMAGE_TO_VIDEO" | "VIDEO_TO_VIDEO" | string;

export type VideoPromptPreflightWarningCode =
  | "VIDEO_PROMPT_DURATION_MISMATCH"
  | "VIDEO_PROMPT_REFERENCE_MISSING"
  | "VIDEO_PROMPT_MODE_MISMATCH"
  | "VIDEO_PROMPT_COMPLEXITY_WARNING";

export interface VideoPromptPreflightInput {
  prompt: string;
  duration?: number | string | null;
  inputMode?: VideoPromptPreflightMode | null;
  referenceImageCount?: number | null;
}

export interface VideoPromptPreflightWarning {
  code: VideoPromptPreflightWarningCode;
  message: string;
  severity: "warning";
}

export interface VideoPromptPreflightResult {
  warnings: VideoPromptPreflightWarning[];
  requestedDurations: number[];
  hasReferenceImageInstruction: boolean;
  complexitySignals: string[];
}

const DURATION_PATTERN = /(?:时长|持续|duration|length|生成|视频)?\s*(\d{1,3})\s*(?:秒|s|seconds?)(?!\w)/giu;
const TIMELINE_RANGE_PATTERN = /(?:第\s*)?\d{1,3}\s*(?:秒|s)?\s*[-–—~～至到]\s*\d{1,3}\s*(?:秒|s)/giu;
const CLOCK_RANGE_PATTERN = /\b\d{1,2}:\d{2}(?::\d{2})?\s*[-–—~～至到]\s*\d{1,2}:\d{2}(?::\d{2})?\b/giu;
const REFERENCE_IMAGE_PATTERN = /(?:参考(?:图|图片|素材)|根据(?:我?上传|提供|这|该)?(?:的)?(?:图片|图像|照片)|(?:第\s*)?(?:一|二|三|1|2|3)\s*(?:张)?\s*(?:参考图|图片)|\b(?:reference\s+images?|reference\s+photos?|input\s+images?|uploaded\s+images?)\b)/iu;
const EXPLICIT_REFERENCE_COUNT_PATTERN = /(?:\d{1,2}|一|二|三|四|五|六|七)\s*(?:张|个)?\s*(?:参考图|参考图片|图片|图像|reference\s+images?)/iu;

const COMPLEXITY_SIGNAL_PATTERNS: Array<[string, RegExp]> = [
  ["multi_scene", /多场景|多个场景|分场景|multi[-\s]?scene|multiple\s+scenes/iu],
  ["multi_shot", /多镜头|多个镜头|镜头切换|分镜|(?:镜头|shot)\s*(?:\d+|[一二三四五六七八九十])|multi[-\s]?shot|multiple\s+shots|shot\s+list/iu],
  ["subtitles", /字幕|屏幕文字|标题字卡|subtitles?|on[-\s]?screen\s+text/iu],
  ["voiceover", /配音|旁白|口播|voice[-\s]?over|narration|voice\s+acting/iu],
  ["synchronized_audio", /同步音频|同步声音|音画同步|同步配乐|sync(?:hronized)?\s+(?:audio|sound)|lip[-\s]?sync/iu],
];

function positiveNumber(value: unknown): number | undefined {
  const parsed = Number(value);
  return Number.isFinite(parsed) && parsed > 0 ? parsed : undefined;
}

function durationCandidatesPrompt(prompt: string) {
  const withoutTimeline = prompt
    .replace(TIMELINE_RANGE_PATTERN, " ")
    .replace(CLOCK_RANGE_PATTERN, " ");
  return [...withoutTimeline.matchAll(DURATION_PATTERN)]
    .map(match => Number(match[1]))
    .filter(value => Number.isFinite(value) && value > 0 && value <= 180)
    .filter((value, index, values) => values.indexOf(value) === index);
}

function normalizedMode(value: unknown) {
  const mode = String(value || "").trim().toUpperCase();
  if (mode === "TEXT" || mode === "TEXT-TO-VIDEO" || mode === "TEXT_TO_VIDEO") return "TEXT_TO_VIDEO";
  if (mode === "IMAGE" || mode === "IMAGE-TO-VIDEO" || mode === "IMAGE_TO_VIDEO") return "IMAGE_TO_VIDEO";
  if (mode === "VIDEO" || mode === "VIDEO-TO-VIDEO" || mode === "VIDEO_TO_VIDEO") return "VIDEO_TO_VIDEO";
  return mode;
}

export function inspectVideoPromptPreflight(input: VideoPromptPreflightInput): VideoPromptPreflightResult {
  const prompt = String(input.prompt || "").trim();
  const requestedDurations = durationCandidatesPrompt(prompt);
  const duration = positiveNumber(input.duration);
  const referenceImageCount = Math.max(0, Math.floor(positiveNumber(input.referenceImageCount) || 0));
  const hasReferenceImageInstruction = REFERENCE_IMAGE_PATTERN.test(prompt);
  const explicitReferenceCount = EXPLICIT_REFERENCE_COUNT_PATTERN.test(prompt);
  const mode = normalizedMode(input.inputMode);
  const complexitySignals = COMPLEXITY_SIGNAL_PATTERNS
    .filter(([, pattern]) => pattern.test(prompt))
    .map(([signal]) => signal);
  const warnings: VideoPromptPreflightWarning[] = [];

  if (duration && requestedDurations.some(requested => requested !== duration)) {
    const requested = requestedDurations.find(value => value !== duration) as number;
    warnings.push({
      code: "VIDEO_PROMPT_DURATION_MISMATCH",
      message: `提示词要求 ${requested} 秒，但当前选择为 ${duration} 秒，建议统一后再生成。`,
      severity: "warning",
    });
  }

  if ((hasReferenceImageInstruction || explicitReferenceCount) && referenceImageCount === 0) {
    warnings.push({
      code: "VIDEO_PROMPT_REFERENCE_MISSING",
      message: "提示词要求使用参考图片，但当前未检测到参考图片。",
      severity: "warning",
    });
  }

  if (mode === "TEXT_TO_VIDEO" && (hasReferenceImageInstruction || explicitReferenceCount)) {
    warnings.push({
      code: "VIDEO_PROMPT_MODE_MISMATCH",
      message: "当前为文生视频，但提示词包含参考图片指令；如需使用参考图，请手动切换为图生视频。",
      severity: "warning",
    });
  }

  if (complexitySignals.length >= 3) {
    warnings.push({
      code: "VIDEO_PROMPT_COMPLEXITY_WARNING",
      message: "当前提示词包含多场景、分镜、字幕或配音等复合要求，部分视频模型的完成稳定性可能降低，建议拆分场景或适当简化；仍可继续生成。",
      severity: "warning",
    });
  }

  return { warnings, requestedDurations, hasReferenceImageInstruction, complexitySignals };
}

export function videoGenerationFailureMessage(value: unknown, fallback = "生成失败，请稍后重试") {
  const record = value && typeof value === "object" && !Array.isArray(value)
    ? value as Record<string, unknown>
    : {};
  const candidates = [
    record.errorCode,
    record.code,
    record.failureReason,
    record.errorMessage,
    record.error,
    value,
  ].map(item => {
    if (typeof item === "string") return item.trim();
    if (!item || typeof item !== "object") return "";
    try {
      return JSON.stringify(item);
    } catch {
      return "";
    }
  }).filter(Boolean);
  const text = candidates[0] || "";
  const billingStatus = String(record.billingStatus || record.billing_status || "").trim().toUpperCase();
  const releasedPoints = positiveNumber(record.releasedPoints ?? record.released_points ?? record.refundedPoints ?? record.refunded_points) || 0;
  const taskStatus = String(record.taskStatus || record.task_status || record.status || "").trim().toUpperCase();
  const providerFailed = candidates.some(candidate => {
    const lower = candidate.toLowerCase();
    return lower.includes("provider_async_generation_failed")
      || lower.includes("video generation failed")
      || lower.includes("generation_failed")
      || candidate.includes("上游未能完成本次视频生成");
  }) || (taskStatus === "FAILED" && Boolean(billingStatus));
  if (!providerFailed) return text || fallback;

  let billingCopy = "积分状态将按现有规则处理。";
  if (billingStatus === "RELEASED" || releasedPoints > 0) {
    billingCopy = `积分已释放${releasedPoints > 0 ? ` ${releasedPoints} 积分` : ""}。`;
  } else if (billingStatus === "RESERVED") {
    billingCopy = "积分仍处于预留状态，尚未确认释放。";
  } else if (billingStatus === "CAPTURED") {
    billingCopy = "积分状态为已扣除，请联系支持核对。";
  } else if (billingStatus === "BILLING_FAILED") {
    billingCopy = "计费未完成，未确认扣除。";
  }
  return `上游未能完成本次视频生成，复杂提示词、参考素材要求或上游临时异常都可能导致失败。${billingCopy}`;
}
