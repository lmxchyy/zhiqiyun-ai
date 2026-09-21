export const CANONICAL_VIDEO_REQUEST_VERSION = 1 as const;

export type CanonicalVideoInputMode = "TEXT_TO_VIDEO" | "IMAGE_TO_VIDEO";
export type CanonicalVideoInputModeHint = CanonicalVideoInputMode | "VIDEO_TO_VIDEO";
export type CanonicalConsistencyStatus = "ok" | "warning";
export type CanonicalConsistencyAction = "use_structured" | "apply_hint" | "edit_prompt";

export type CanonicalVideoWarningCode =
  | "VIDEO_PROMPT_DURATION_MISMATCH"
  | "VIDEO_PROMPT_ASPECT_RATIO_MISMATCH"
  | "VIDEO_PROMPT_RESOLUTION_MISMATCH"
  | "VIDEO_PROMPT_REFERENCE_MISSING"
  | "VIDEO_PROMPT_MODE_MISMATCH"
  | "VIDEO_PROMPT_COMPLEXITY_WARNING";

export interface CanonicalVideoCapabilities {
  supported_durations?: number[];
  supported_resolutions?: string[];
  supported_aspect_ratios?: string[];
}

export interface CanonicalVideoStructuredInput {
  model?: unknown;
  input_mode?: unknown;
  inputMode?: unknown;
  duration?: unknown;
  aspect_ratio?: unknown;
  ratio?: unknown;
  resolution?: unknown;
  quality?: unknown;
  reference_images?: unknown;
  referenceImages?: unknown;
  first_frame?: unknown;
  firstFrame?: unknown;
  last_frame?: unknown;
  lastFrame?: unknown;
  parameters?: Record<string, unknown>;
}

export interface PromptIntentEvidence {
  field: "duration" | "aspect_ratio" | "resolution" | "input_mode" | "reference_images";
  normalized_value: unknown;
  source_kind: "explicit_text";
  confidence: "high";
}

export interface PromptIntentHints {
  parser_version: 1;
  requested_duration_seconds?: number;
  requested_aspect_ratio?: string;
  requested_resolution?: string;
  requested_input_mode?: CanonicalVideoInputModeHint;
  reference_image_requested: boolean;
  requested_reference_count?: number;
  complexity_signals: string[];
  evidence: PromptIntentEvidence[];
}

export interface CanonicalConsistencyWarning {
  code: CanonicalVideoWarningCode;
  field: "duration" | "aspect_ratio" | "resolution" | "input_mode" | "reference_images" | "prompt";
  structured_value?: unknown;
  hinted_value?: unknown;
  severity: "warning";
  actions: CanonicalConsistencyAction[];
}

export interface CanonicalConsistencyResult {
  status: CanonicalConsistencyStatus;
  warning_codes: CanonicalVideoWarningCode[];
  warnings: CanonicalConsistencyWarning[];
}

export interface CanonicalVideoOptionalParameters {
  fps?: number;
  generate_audio?: boolean;
  motion_strength?: number;
  camera_movement?: string;
}

export interface CanonicalVideoExecution {
  model: string;
  input_mode: CanonicalVideoInputMode;
  duration_seconds: number;
  aspect_ratio: string;
  resolution: string;
  reference_images: string[];
  first_frame?: string;
  last_frame?: string;
  optional_parameters: CanonicalVideoOptionalParameters;
}

export interface CanonicalVideoRequest {
  schema_version: typeof CANONICAL_VIDEO_REQUEST_VERSION;
  prompt: string;
  execution: CanonicalVideoExecution;
  prompt_intent_hints: PromptIntentHints;
  consistency_result: CanonicalConsistencyResult;
}

export interface BuildCanonicalVideoRequestInput {
  prompt: string;
  structured: CanonicalVideoStructuredInput;
  capabilities?: CanonicalVideoCapabilities;
}

export class CanonicalVideoRequestError extends Error {
  readonly code: string;

  constructor(code: string, message: string) {
    super(message);
    this.name = "CanonicalVideoRequestError";
    this.code = code;
  }
}

const DURATION_PATTERN = /(?:时长|持续|duration|length|生成|视频|总时长|视频长度)?\s*(\d{1,3})\s*(?:秒|s|seconds?)(?!\w)/giu;
const TIMELINE_RANGE_PATTERN = /(?:第\s*)?\d{1,3}\s*(?:秒|s)?\s*[-–—~～至到]\s*\d{1,3}\s*(?:秒|s)/giu;
const CLOCK_RANGE_PATTERN = /\b\d{1,2}:\d{2}(?::\d{2})?\s*[-–—~～至到]\s*\d{1,2}:\d{2}(?::\d{2})?\b/giu;
const CONTEXTUAL_DURATION_PATTERN = /(?:前|最后|起初|开头|结尾|第)\s*\d{1,3}\s*(?:秒|s)|\d{1,3}\s*(?:秒|s)\s*(?:后|内|时)/giu;
const TIMELINE_SIGNAL_PATTERN = /(?:第\s*)?\d{1,3}\s*(?:秒|s)?\s*[-–—~～至到]\s*\d{1,3}\s*(?:秒|s)/iu;
const ASPECT_PATTERN = /(?:比例|画幅|aspect\s*ratio|ratio)?\s*(\d{1,2})\s*:\s*(\d{1,2})/giu;
const ASPECT_ALIAS_PATTERNS: Array<[string, RegExp]> = [
  ["9:16", /竖屏|portrait/iu],
  ["16:9", /横屏|landscape/iu],
  ["1:1", /方形|square/iu],
];
const RESOLUTION_PATTERN = /\b(\d{3,4}\s*p|[1248]\s*k)\b/giu;
const REFERENCE_PATTERN = /(?:参考(?:图|图片|素材)|根据(?:我?上传|提供|这|该)?(?:的)?(?:图片|图像|照片)|(?:第\s*)?(?:一|二|三|1|2|3)\s*(?:张)?\s*(?:参考图|图片|图|照片)|\b(?:第一|第二|第三)\s*张?(?:图|图片|照片)|\breference\s+images?|\breference\s+photos?|\binput\s+images?|\buploaded\s+images?)/iu;
const REFERENCE_COUNT_PATTERN = /(?:\d{1,2}|一|二|三|四|五|六|七)\s*(?:张|个)?\s*(?:参考图|参考图片|图片|图像|reference\s+images?)/iu;
const INPUT_MODE_PATTERNS: Array<[CanonicalVideoInputModeHint, RegExp]> = [
  ["VIDEO_TO_VIDEO", /视频转视频|video[-\s]?to[-\s]?video/iu],
  ["IMAGE_TO_VIDEO", /图生视频|image[-\s]?to[-\s]?video/iu],
  ["TEXT_TO_VIDEO", /文生视频|text[-\s]?to[-\s]?video/iu],
];
const COMPLEXITY_PATTERNS: Array<[string, RegExp]> = [
  ["timeline", TIMELINE_SIGNAL_PATTERN],
  ["multi_scene", /多场景|多个场景|分场景|multi[-\s]?scene|multiple\s+scenes/iu],
  ["multi_shot", /多镜头|多个镜头|镜头切换|分镜|(?:镜头|shot)\s*(?:\d+|[一二三四五六七八九十])|multi[-\s]?shot|multiple\s+shots|shot\s+list/iu],
  ["subtitles", /字幕|屏幕文字|标题字卡|subtitles?|on[-\s]?screen\s+text/iu],
  ["voiceover", /配音|旁白|口播|voice[-\s]?over|narration|voice\s+acting/iu],
  ["synchronized_audio", /同步音频|同步声音|音画同步|同步配乐|sync(?:hronized)?\s+(?:audio|sound)|lip[-\s]?sync/iu],
];

function text(value: unknown): string {
  return String(value ?? "").trim();
}

function positiveNumber(value: unknown): number | undefined {
  const parsed = Number(value);
  return Number.isFinite(parsed) && parsed > 0 ? parsed : undefined;
}

function normalizeDuration(value: unknown): number | undefined {
  const raw = text(value).toLowerCase().replace(/s$/, "");
  if (!raw) return undefined;
  const parsed = Number(raw);
  return Number.isInteger(parsed) && parsed > 0 ? parsed : undefined;
}

function normalizeAspectRatio(value: unknown): string | undefined {
  const match = text(value).match(/^(\d{1,2})\s*:\s*(\d{1,2})$/);
  if (!match || Number(match[1]) <= 0 || Number(match[2]) <= 0) return undefined;
  return `${Number(match[1])}:${Number(match[2])}`;
}

function normalizeResolution(value: unknown): string | undefined {
  const normalized = text(value).toLowerCase().replace(/\s+/g, "");
  return /^\d{3,4}p$|^[1248]k$/.test(normalized) ? normalized : undefined;
}

function normalizeMode(value: unknown): CanonicalVideoInputMode | undefined {
  const mode = text(value).toUpperCase().replace(/-/g, "_");
  if (mode === "TEXT" || mode === "TEXT_TO_VIDEO") return "TEXT_TO_VIDEO";
  if (mode === "IMAGE" || mode === "IMAGE_TO_VIDEO") return "IMAGE_TO_VIDEO";
  return undefined;
}

function uniqueStrings(values: string[]): string[] {
  return [...new Set(values.map(text).filter(Boolean))];
}

function stringList(value: unknown): string[] {
  if (Array.isArray(value)) return uniqueStrings(value.map(item => text(item)));
  const single = text(value);
  return single ? [single] : [];
}

function explicitDurationCandidates(prompt: string): number[] {
  const withoutTimeline = prompt
    .replace(TIMELINE_RANGE_PATTERN, " ")
    .replace(CLOCK_RANGE_PATTERN, " ")
    .replace(CONTEXTUAL_DURATION_PATTERN, " ");
  return [...withoutTimeline.matchAll(DURATION_PATTERN)]
    .map(match => Number(match[1]))
    .filter(value => Number.isInteger(value) && value > 0 && value <= 180)
    .filter((value, index, values) => values.indexOf(value) === index);
}

function chineseCount(value: string): number | undefined {
  const counts: Record<string, number> = { 一: 1, 二: 2, 三: 3, 四: 4, 五: 5, 六: 6, 七: 7 };
  const token = value.match(/\d+|[一二三四五六七]/)?.[0];
  if (!token) return undefined;
  return counts[token] ?? Number(token);
}

function promptInputMode(prompt: string, referenceImageRequested: boolean): CanonicalVideoInputModeHint | undefined {
  for (const [mode, pattern] of INPUT_MODE_PATTERNS) {
    if (pattern.test(prompt)) return mode;
  }
  return referenceImageRequested ? "IMAGE_TO_VIDEO" : undefined;
}

function complexitySignals(prompt: string): string[] {
  return COMPLEXITY_PATTERNS
    .filter(([, pattern]) => pattern.test(prompt))
    .map(([signal]) => signal);
}

export function parseVideoPromptIntentHints(promptInput: string): PromptIntentHints {
  const prompt = text(promptInput);
  const durations = explicitDurationCandidates(prompt);
  const aspectRatios = [...prompt.matchAll(ASPECT_PATTERN)]
    .map(match => normalizeAspectRatio(`${match[1]}:${match[2]}`))
    .filter((value): value is string => Boolean(value));
  for (const [ratio, pattern] of ASPECT_ALIAS_PATTERNS) {
    if (pattern.test(prompt)) aspectRatios.push(ratio);
  }
  const uniqueAspectRatios = aspectRatios.filter((value, index, values) => values.indexOf(value) === index);
  const resolutions = [...prompt.matchAll(RESOLUTION_PATTERN)]
    .map(match => normalizeResolution(match[1]))
    .filter((value): value is string => Boolean(value))
    .filter((value, index, values) => values.indexOf(value) === index);
  const referenceImageRequested = REFERENCE_PATTERN.test(prompt) || REFERENCE_COUNT_PATTERN.test(prompt);
  const countMatch = prompt.match(REFERENCE_COUNT_PATTERN);
  const requestedReferenceCount = countMatch ? chineseCount(countMatch[0]) : undefined;
  const requestedInputMode = promptInputMode(prompt, referenceImageRequested);
  const signals = complexitySignals(prompt);
  const evidence: PromptIntentEvidence[] = [];
  if (durations[0] !== undefined) evidence.push({ field: "duration", normalized_value: durations[0], source_kind: "explicit_text", confidence: "high" });
  if (uniqueAspectRatios[0]) evidence.push({ field: "aspect_ratio", normalized_value: uniqueAspectRatios[0], source_kind: "explicit_text", confidence: "high" });
  if (resolutions[0]) evidence.push({ field: "resolution", normalized_value: resolutions[0], source_kind: "explicit_text", confidence: "high" });
  if (requestedInputMode) evidence.push({ field: "input_mode", normalized_value: requestedInputMode, source_kind: "explicit_text", confidence: "high" });
  if (referenceImageRequested) evidence.push({ field: "reference_images", normalized_value: requestedReferenceCount ?? true, source_kind: "explicit_text", confidence: "high" });
  return {
    parser_version: 1,
    ...(durations[0] !== undefined ? { requested_duration_seconds: durations[0] } : {}),
    ...(uniqueAspectRatios[0] ? { requested_aspect_ratio: uniqueAspectRatios[0] } : {}),
    ...(resolutions[0] ? { requested_resolution: resolutions[0] } : {}),
    ...(requestedInputMode ? { requested_input_mode: requestedInputMode } : {}),
    reference_image_requested: referenceImageRequested,
    ...(requestedReferenceCount ? { requested_reference_count: requestedReferenceCount } : {}),
    complexity_signals: signals,
    evidence,
  };
}

function warning(
  code: CanonicalVideoWarningCode,
  field: CanonicalConsistencyWarning["field"],
  structuredValue: unknown,
  hintedValue: unknown,
): CanonicalConsistencyWarning {
  return {
    code,
    field,
    ...(structuredValue !== undefined ? { structured_value: structuredValue } : {}),
    ...(hintedValue !== undefined ? { hinted_value: hintedValue } : {}),
    severity: "warning",
    actions: ["use_structured", "apply_hint", "edit_prompt"],
  };
}

function checkConsistency(
  execution: CanonicalVideoExecution,
  hints: PromptIntentHints,
): CanonicalConsistencyResult {
  const warnings: CanonicalConsistencyWarning[] = [];
  if (hints.requested_duration_seconds !== undefined && hints.requested_duration_seconds !== execution.duration_seconds) {
    warnings.push(warning("VIDEO_PROMPT_DURATION_MISMATCH", "duration", execution.duration_seconds, hints.requested_duration_seconds));
  }
  if (hints.requested_aspect_ratio && hints.requested_aspect_ratio !== execution.aspect_ratio) {
    warnings.push(warning("VIDEO_PROMPT_ASPECT_RATIO_MISMATCH", "aspect_ratio", execution.aspect_ratio, hints.requested_aspect_ratio));
  }
  if (hints.requested_resolution && hints.requested_resolution !== execution.resolution) {
    warnings.push(warning("VIDEO_PROMPT_RESOLUTION_MISMATCH", "resolution", execution.resolution, hints.requested_resolution));
  }
  if (hints.requested_input_mode && hints.requested_input_mode !== execution.input_mode) {
    warnings.push(warning("VIDEO_PROMPT_MODE_MISMATCH", "input_mode", execution.input_mode, hints.requested_input_mode));
  } else if (hints.reference_image_requested && execution.input_mode === "TEXT_TO_VIDEO") {
    warnings.push(warning("VIDEO_PROMPT_MODE_MISMATCH", "input_mode", execution.input_mode, "IMAGE_TO_VIDEO"));
  }
  if (hints.reference_image_requested && execution.reference_images.length === 0 && !execution.first_frame) {
    warnings.push(warning("VIDEO_PROMPT_REFERENCE_MISSING", "reference_images", [], hints.requested_reference_count ?? true));
  }
  if (hints.complexity_signals.length >= 3) {
    warnings.push(warning("VIDEO_PROMPT_COMPLEXITY_WARNING", "prompt", undefined, hints.complexity_signals));
  }
  return {
    status: warnings.length ? "warning" : "ok",
    warning_codes: warnings.map(item => item.code),
    warnings,
  };
}

function required(value: string, field: string): string {
  if (!value) throw new CanonicalVideoRequestError("VIDEO_CANONICAL_REQUIRED", `${field} is required`);
  return value;
}

function assertCapability<T>(value: T, supported: T[] | undefined, field: string): T {
  if (supported?.length && !supported.some(candidate => candidate === value)) {
    throw new CanonicalVideoRequestError("VIDEO_CANONICAL_UNSUPPORTED_PARAMETER", `${field} is not supported`);
  }
  return value;
}

function normalizedOptionalParameters(input: CanonicalVideoStructuredInput): CanonicalVideoOptionalParameters {
  const parameters = input.parameters || {};
  const fpsValue = parameters.fps ?? input.parameters?.fps;
  const fps = fpsValue === undefined || fpsValue === "" ? undefined : Number(fpsValue);
  const audioValue = parameters.generate_audio ?? parameters.generateAudio;
  const motionValue = parameters.motion_strength;
  const cameraValue = parameters.camera_movement;
  if (fps !== undefined && (!Number.isInteger(fps) || fps <= 0)) throw new CanonicalVideoRequestError("VIDEO_CANONICAL_INVALID_PARAMETER", "fps is invalid");
  const motion = motionValue === undefined || motionValue === "" ? undefined : Number(motionValue);
  if (motion !== undefined && !Number.isFinite(motion)) throw new CanonicalVideoRequestError("VIDEO_CANONICAL_INVALID_PARAMETER", "motion_strength is invalid");
  if (audioValue !== undefined && typeof audioValue !== "boolean") throw new CanonicalVideoRequestError("VIDEO_CANONICAL_INVALID_PARAMETER", "generate_audio is invalid");
  return {
    ...(fps !== undefined ? { fps } : {}),
    ...(audioValue !== undefined ? { generate_audio: audioValue } : {}),
    ...(motion !== undefined ? { motion_strength: motion } : {}),
    ...(text(cameraValue) ? { camera_movement: text(cameraValue) } : {}),
  };
}

export function buildCanonicalVideoRequest(input: BuildCanonicalVideoRequestInput): CanonicalVideoRequest {
  const structured = input.structured || {};
  const prompt = required(text(input.prompt), "prompt");
  const model = required(text(structured.model), "model");
  const inputMode = normalizeMode(structured.input_mode ?? structured.inputMode);
  if (!inputMode) throw new CanonicalVideoRequestError("VIDEO_CANONICAL_INVALID_MODE", "input_mode is invalid");
  const duration = normalizeDuration(structured.duration);
  if (duration === undefined) throw new CanonicalVideoRequestError("VIDEO_CANONICAL_INVALID_DURATION", "duration is invalid");
  const aspectRatio = normalizeAspectRatio(structured.aspect_ratio ?? structured.ratio);
  if (!aspectRatio) throw new CanonicalVideoRequestError("VIDEO_CANONICAL_INVALID_ASPECT_RATIO", "aspect_ratio is invalid");
  const resolutionValue = structured.resolution ?? structured.quality;
  const resolution = normalizeResolution(resolutionValue);
  if (!resolution) throw new CanonicalVideoRequestError("VIDEO_CANONICAL_INVALID_RESOLUTION", "resolution is invalid");
  const firstFrame = text(structured.first_frame ?? structured.firstFrame);
  const lastFrame = text(structured.last_frame ?? structured.lastFrame);
  const referenceImages = stringList(structured.reference_images ?? structured.referenceImages);
  const capabilities = input.capabilities || {};
  assertCapability(duration, capabilities.supported_durations, "duration");
  assertCapability(resolution, capabilities.supported_resolutions?.map(normalizeResolution).filter((value): value is string => Boolean(value)), "resolution");
  assertCapability(aspectRatio, capabilities.supported_aspect_ratios?.map(normalizeAspectRatio).filter((value): value is string => Boolean(value)), "aspect_ratio");
  if (inputMode === "TEXT_TO_VIDEO" && (firstFrame || lastFrame || referenceImages.length)) {
    throw new CanonicalVideoRequestError("VIDEO_CANONICAL_TEXT_MODE_REFERENCES", "text-to-video cannot contain references");
  }
  const execution: CanonicalVideoExecution = {
    model,
    input_mode: inputMode,
    duration_seconds: duration,
    aspect_ratio: aspectRatio,
    resolution,
    reference_images: referenceImages,
    ...(firstFrame ? { first_frame: firstFrame } : {}),
    ...(lastFrame ? { last_frame: lastFrame } : {}),
    optional_parameters: normalizedOptionalParameters(structured),
  };
  const promptIntentHints = parseVideoPromptIntentHints(prompt);
  return {
    schema_version: CANONICAL_VIDEO_REQUEST_VERSION,
    prompt,
    execution,
    prompt_intent_hints: promptIntentHints,
    consistency_result: checkConsistency(execution, promptIntentHints),
  };
}

export function canonicalVideoRequestRepresentation(request: CanonicalVideoRequest): string {
  const stable = (value: unknown): string => {
    if (value === null || typeof value !== "object") return JSON.stringify(value);
    if (Array.isArray(value)) return `[${value.map(stable).join(",")}]`;
    const record = value as Record<string, unknown>;
    return `{${Object.keys(record).sort().map(key => `${JSON.stringify(key)}:${stable(record[key])}`).join(",")}}`;
  };
  return stable(request);
}
