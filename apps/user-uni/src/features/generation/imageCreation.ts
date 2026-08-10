import { taskRequestFromDraft } from "@xianzhi/business-sdk";
import type {
  CreateDraft,
  CreateGenerationTaskRequest,
  ModelInfo,
} from "@xianzhi/shared-types";

export type CanonicalImageQuality = "standard" | "high";

export interface ImageGeneratorModelOption {
  code: string;
  name: string;
  pointCost?: number;
}

export interface ImageControlOption<T> {
  value: T;
  label: string;
}

export interface CanonicalImageSelection {
  size?: string;
  quality?: CanonicalImageQuality;
  count?: number;
}

export interface ImageCreationSelection {
  size?: unknown;
  quality?: unknown;
  count?: unknown;
}

export interface AvailableImageCreationContract {
  available: true;
  modelName: string;
  sizeOptions: Array<ImageControlOption<string>>;
  qualityOptions: Array<ImageControlOption<CanonicalImageQuality>>;
  countOptions: Array<ImageControlOption<number>>;
  defaultSelection: CanonicalImageSelection;
  declared: {
    size: boolean;
    quality: boolean;
    count: boolean;
  };
  required: {
    size: boolean;
    quality: boolean;
    count: boolean;
  };
}

export interface UnavailableImageCreationContract {
  available: false;
  reason: string;
}

export type ImageCreationContract = AvailableImageCreationContract | UnavailableImageCreationContract;

export interface CompatibleImageInspiration {
  compatible: true;
  selection: CanonicalImageSelection;
  canonical: CanonicalImageSelection;
}

export interface IncompatibleImageInspiration {
  compatible: false;
  reason: string;
}

export type ImageInspirationRestoreResult = CompatibleImageInspiration | IncompatibleImageInspiration;

export type CanonicalImageRequestSnapshot = object;

export interface ImageClientRequestKeyState {
  fingerprint: string;
  clientRequestId: string;
}

export type ImageRequestPreviousOutcome = "network-uncertain" | "terminal-failure";

export interface NextImageClientRequestKeyInput {
  fingerprint: string;
  existing?: ImageClientRequestKeyState;
  previousOutcome?: ImageRequestPreviousOutcome;
}

export interface ImageReferenceUploadCache {
  sourceSnapshot: string[];
  uploadedURLs: string[];
}

export interface ResolveImageReferenceUploadsInput {
  sourceReferences: string[];
  cache?: ImageReferenceUploadCache;
  previousOutcome?: ImageRequestPreviousOutcome;
}

export interface ResolvedImageReferenceUploads {
  referenceImages: string[];
  cache: ImageReferenceUploadCache;
  reused: boolean;
}

export interface ImageTaskSubmissionState {
  uploadCache?: ImageReferenceUploadCache;
  requestKey?: ImageClientRequestKeyState;
  previousOutcome?: ImageRequestPreviousOutcome;
}

export type ImageSchemaLoadStatus = "idle" | "loading" | "ready" | "error";

export type ImageSchemaFetchResult =
  | { applied: false }
  | {
      applied: true;
      status: "ready";
      message: string;
      contract: AvailableImageCreationContract;
    }
  | {
      applied: true;
      status: "error";
      message: string;
      contract: UnavailableImageCreationContract;
    };

export interface ImageSchemaFetchInput {
  requestedModel: string;
  currentModel: string;
  requestSequence: number;
  latestSequence: number;
  response?: unknown;
  error?: unknown;
}

export type ImageGeneratorStatusTone = "idle" | "loading" | "success" | "error";

export interface ImageGeneratorViewStateInput {
  prompt: string;
  busy: boolean;
  disabledReason: string;
  statusTone: ImageGeneratorStatusTone;
  statusMessage: string;
  error: string;
  retryAvailable: boolean;
}

export interface ImageGeneratorViewState {
  canSubmit: boolean;
  disabledReason: string;
  primaryAction: "generate" | "retry";
  primaryLabel: string;
  showSpinner: boolean;
  showRetry: boolean;
  tone: ImageGeneratorStatusTone;
  liveMessage: string;
}

export interface CanonicalImageDraftInput {
  contract: AvailableImageCreationContract;
  selection: ImageCreationSelection;
  prompt: string;
  model: string;
  style: string;
  referenceImages?: string[];
  negativePrompt?: string;
  parameters?: Record<string, unknown>;
  clientRequestId?: string;
}

export type SubmitCanonicalImageTaskInput = Omit<
  CanonicalImageDraftInput,
  "referenceImages" | "clientRequestId"
> & ImageTaskSubmissionState & {
  sourceReferences: string[];
};

export interface SubmitCanonicalImageTaskDependencies<Task> {
  uploadReferences: (sourceReferences: string[]) => Promise<string[]>;
  createTask: (draft: CreateDraft) => Promise<Task>;
  clientIdFactory: () => string;
}

interface ImageTaskSubmissionRequestEvidence {
  draft: CreateDraft;
  requestSnapshot: CreateGenerationTaskRequest;
  finalRequest: CreateGenerationTaskRequest;
  fingerprint: string;
}

export type ImageTaskSubmissionResult<Task> =
  | ({
      ok: true;
      task: Task;
      state: ImageTaskSubmissionState;
    } & ImageTaskSubmissionRequestEvidence)
  | ({
      ok: false;
      error: unknown;
      state: ImageTaskSubmissionState;
    } & Partial<ImageTaskSubmissionRequestEvidence>);

type UnknownRecord = Record<string, unknown>;

export function imageModelOptions(models: ModelInfo[]): ImageGeneratorModelOption[] {
  return models
    .filter(model => model.online === true)
    .filter(model => (model.capabilities || []).some(capability => {
      const value = String(capability).toUpperCase();
      return value === "TEXT_TO_IMAGE" || value === "IMAGE_TO_IMAGE" || value === "IMAGE_GENERATION";
    }))
    .map(model => ({ code: model.code, name: model.name || model.code, pointCost: model.pointCost }));
}

export function resolveImageModelCode(models: ImageGeneratorModelOption[], requested: string): string {
  return models.some(model => model.code === requested) ? requested : "";
}

function recordValue(value: unknown): UnknownRecord | undefined {
  return value !== null && typeof value === "object" && !Array.isArray(value)
    ? value as UnknownRecord
    : undefined;
}

function schemaFields(response: UnknownRecord): UnknownRecord[] | undefined {
  const schema = recordValue(response.schema);
  if (!schema || !Array.isArray(schema.fields)) return undefined;
  return schema.fields.map(recordValue).filter((field): field is UnknownRecord => Boolean(field));
}

function fieldByKey(fields: UnknownRecord[], key: string): UnknownRecord | undefined {
  return fields.find(field => field.key === key);
}

function uniqueValues<T>(values: T[]): T[] {
  return values.filter((value, index) => values.indexOf(value) === index);
}

function greatestCommonDivisor(left: number, right: number): number {
  let a = left;
  let b = right;
  while (b !== 0) {
    const remainder = a % b;
    a = b;
    b = remainder;
  }
  return a;
}

function canonicalSizeParts(value: unknown): [number, number] | undefined {
  if (typeof value !== "string" || !/^[1-9]\d*x[1-9]\d*$/.test(value)) return undefined;
  const [width, height] = value.split("x").map(Number);
  if (!Number.isSafeInteger(width) || !Number.isSafeInteger(height) || width <= 0 || height <= 0) {
    return undefined;
  }
  return [width, height];
}

function reducedSizeLabel(value: string): string {
  const parts = canonicalSizeParts(value);
  if (!parts) throw new Error(`invalid canonical image size ${value}`);
  const [width, height] = parts;
  const divisor = greatestCommonDivisor(width, height);
  return `${width / divisor}:${height / divisor}`;
}

function validCount(value: unknown, field: UnknownRecord): value is number {
  if (typeof value !== "number" || !Number.isInteger(value) || value <= 0) return false;
  const min = typeof field.min === "number" && Number.isFinite(field.min) ? field.min : undefined;
  const max = typeof field.max === "number" && Number.isFinite(field.max) ? field.max : undefined;
  return (min === undefined || value >= min) && (max === undefined || value <= max);
}

export function deriveImageCreationContract(
  requestedModel: string,
  rawResponse: unknown,
): ImageCreationContract {
  const response = recordValue(rawResponse);
  if (!response) {
    return { available: false, reason: "当前模型暂无可用的图片参数配置" };
  }
  if (response.module_code !== "image_generation") {
    return { available: false, reason: "当前返回的参数配置不是图片生成配置" };
  }

  const responseModel = typeof response.model_name === "string" ? response.model_name : "";
  if (!requestedModel || responseModel !== requestedModel) {
    return {
      available: false,
      reason: `图片参数配置与所选模型不一致：期望 ${requestedModel || "未选择"}，实际 ${responseModel || "未提供"}`,
    };
  }

  const fields = schemaFields(response);
  if (!fields) {
    return { available: false, reason: "当前模型暂无可用的图片参数配置" };
  }

  const sizeField = fieldByKey(fields, "size");
  const sizeValues = uniqueValues(
    (Array.isArray(sizeField?.options) ? sizeField.options : [])
      .filter((value): value is string => typeof value === "string" && Boolean(canonicalSizeParts(value))),
  );
  if (!sizeField || sizeValues.length === 0) {
    return { available: false, reason: "当前模型没有可用的图片尺寸选项" };
  }

  const qualityField = fieldByKey(fields, "quality");
  const qualityValues = uniqueValues(
    (Array.isArray(qualityField?.options) ? qualityField.options : [])
      .filter((value): value is CanonicalImageQuality => value === "standard" || value === "high"),
  );
  if (qualityField && qualityValues.length === 0) {
    return { available: false, reason: "当前模型没有可用的图片质量选项" };
  }

  const countField = fieldByKey(fields, "n");
  let countValues: number[] = [];
  if (countField) {
    const rawOptions = Array.isArray(countField.options) ? countField.options : [];
    countValues = rawOptions.length > 0
      ? uniqueValues(rawOptions.filter((value): value is number => validCount(value, countField)))
      : validCount(countField.default, countField) ? [countField.default] : [];
    if (countValues.length === 0) {
      return { available: false, reason: "当前模型没有可用的生成数量选项" };
    }
  }

  const defaultSelection: CanonicalImageSelection = {};
  if (typeof sizeField.default === "string" && sizeValues.includes(sizeField.default)) {
    defaultSelection.size = sizeField.default;
  }
  if (
    qualityField
    && (qualityField.default === "standard" || qualityField.default === "high")
    && qualityValues.includes(qualityField.default)
  ) {
    defaultSelection.quality = qualityField.default;
  }
  if (countField && typeof countField.default === "number" && countValues.includes(countField.default)) {
    defaultSelection.count = countField.default;
  }

  return {
    available: true,
    modelName: responseModel,
    sizeOptions: sizeValues.map(value => ({ value, label: reducedSizeLabel(value) })),
    qualityOptions: qualityValues.map(value => ({ value, label: value })),
    countOptions: countValues.map(value => ({ value, label: String(value) })),
    defaultSelection,
    declared: {
      size: true,
      quality: Boolean(qualityField),
      count: Boolean(countField),
    },
    required: {
      size: sizeField.required === true,
      quality: qualityField?.required === true,
      count: countField?.required === true,
    },
  };
}

export function initialImageSelection(
  contract: AvailableImageCreationContract,
): CanonicalImageSelection {
  const selection: CanonicalImageSelection = {
    size: contract.defaultSelection.size || contract.sizeOptions[0]?.value,
  };
  if (contract.declared.quality) {
    selection.quality = contract.defaultSelection.quality || contract.qualityOptions[0]?.value;
  }
  if (contract.declared.count) {
    selection.count = contract.defaultSelection.count || contract.countOptions[0]?.value;
  }
  return toCanonicalImageSelection(contract, selection);
}

export function resolveImageSchemaFetchResult(
  input: ImageSchemaFetchInput,
): ImageSchemaFetchResult {
  if (
    input.requestSequence !== input.latestSequence
    || input.currentModel !== input.requestedModel
  ) {
    return { applied: false };
  }

  if (input.error !== undefined) {
    const contract: UnavailableImageCreationContract = {
      available: false,
      reason: "图片参数读取失败，请稍后重试",
    };
    return { applied: true, status: "error", message: contract.reason, contract };
  }

  const contract = deriveImageCreationContract(input.requestedModel, input.response);
  if (!contract.available) {
    return { applied: true, status: "error", message: contract.reason, contract };
  }
  return { applied: true, status: "ready", message: "图片参数已就绪", contract };
}

export function toCanonicalImageSelection(
  contract: AvailableImageCreationContract,
  rawSelection: ImageCreationSelection,
): CanonicalImageSelection {
  if (!contract || contract.available !== true) {
    throw new Error("当前图片参数配置不可用");
  }
  const selection = recordValue(rawSelection) || {};
  const supportedKeys = new Set(["size", "quality", "count"]);
  const unknownKey = Object.keys(selection).find(key => !supportedKeys.has(key));
  if (unknownKey) throw new Error(`图片选择包含不支持字段 ${unknownKey}`);

  const canonical: CanonicalImageSelection = {};
  if (selection.size !== undefined) {
    if (!contract.declared.size) throw new Error("当前模型未声明图片尺寸");
    if (typeof selection.size !== "string" || !contract.sizeOptions.some(option => option.value === selection.size)) {
      throw new Error(`当前模型不支持图片尺寸 ${String(selection.size)}`);
    }
    canonical.size = selection.size;
  } else if (contract.required.size) {
    throw new Error("必须选择图片尺寸");
  }

  if (selection.quality !== undefined) {
    if (!contract.declared.quality) throw new Error("当前模型未声明图片质量");
    if (
      (selection.quality !== "standard" && selection.quality !== "high")
      || !contract.qualityOptions.some(option => option.value === selection.quality)
    ) {
      throw new Error(`当前模型不支持图片质量 ${String(selection.quality)}`);
    }
    canonical.quality = selection.quality;
  } else if (contract.required.quality) {
    throw new Error("必须选择图片质量");
  }

  if (selection.count !== undefined) {
    if (!contract.declared.count) throw new Error("当前模型未声明生成数量");
    if (
      typeof selection.count !== "number"
      || !Number.isInteger(selection.count)
      || selection.count <= 0
      || !contract.countOptions.some(option => option.value === selection.count)
    ) {
      throw new Error(`当前模型不支持生成数量 ${String(selection.count)}`);
    }
    canonical.count = selection.count;
  } else if (contract.required.count) {
    throw new Error("必须选择生成数量");
  }

  return canonical;
}

export function restoreImageInspirationSelection(
  contract: ImageCreationContract,
  rawParameters: unknown,
): ImageInspirationRestoreResult {
  if (!contract.available) {
    return { compatible: false, reason: contract.reason };
  }
  const parameters = recordValue(rawParameters) || {};
  const ratio = typeof parameters.ratio === "string" ? parameters.ratio : "";
  if (!ratio) {
    return { compatible: false, reason: "灵感缺少有效的 ratio" };
  }

  const matchingSizes = contract.sizeOptions.filter(option => option.label === ratio);
  if (matchingSizes.length === 0) {
    return { compatible: false, reason: `当前模型不支持灵感比例 ${ratio}` };
  }
  const defaultMatch = matchingSizes.find(option => option.value === contract.defaultSelection.size);
  const sizeOption = defaultMatch || (matchingSizes.length === 1 ? matchingSizes[0] : undefined);
  if (!sizeOption) {
    return { compatible: false, reason: `灵感比例 ${ratio} 对应多个尺寸，无法确定生成尺寸` };
  }

  const quality = parameters.quality;
  if (quality !== "standard" && quality !== "high") {
    return { compatible: false, reason: "灵感 quality 必须是 standard 或 high" };
  }
  if (!contract.declared.quality || !contract.qualityOptions.some(option => option.value === quality)) {
    return { compatible: false, reason: `当前模型不支持灵感图片质量 ${quality}` };
  }

  const count = parameters.count;
  if (typeof count !== "number" || !Number.isInteger(count) || count <= 0) {
    return { compatible: false, reason: "灵感 count 必须是正整数" };
  }
  if (!contract.declared.count || !contract.countOptions.some(option => option.value === count)) {
    return { compatible: false, reason: `当前模型不支持灵感生成数量 ${String(count)}` };
  }

  const selection: CanonicalImageSelection = { size: sizeOption.value, quality, count };
  return {
    compatible: true,
    selection,
    canonical: toCanonicalImageSelection(contract, selection),
  };
}

const imageDraftSelectionKeys = new Set([
  "ratio",
  "aspectRatio",
  "aspect_ratio",
  "imageRatio",
  "imageQuality",
  "size",
  "quality",
  "count",
  "n",
  "generationCount",
  "imageCount",
  "prompt",
  "model",
  "modelName",
  "mode",
  "contentType",
  "referenceImages",
  "referencePaths",
  "negativePrompt",
  "negative_prompt",
  "style",
  "stylePreset",
]);

export function canonicalImageParameters(rawParameters: unknown): Record<string, unknown> {
  const parameters: Record<string, unknown> = {};
  const raw = recordValue(rawParameters) || {};
  for (const [key, value] of Object.entries(raw)) {
    if (!imageDraftSelectionKeys.has(key)) parameters[key] = value;
  }
  return parameters;
}

export function buildCanonicalImageDraft(input: CanonicalImageDraftInput) {
  if (input.contract.modelName !== input.model) {
    throw new Error(`图片参数配置与所选模型不一致：期望 ${input.model}，实际 ${input.contract.modelName}`);
  }
  const canonical = toCanonicalImageSelection(input.contract, input.selection);
  const parameters = canonicalImageParameters(input.parameters);

  return {
    mode: "image" as const,
    prompt: input.prompt,
    model: input.model,
    style: input.style,
    referenceImages: [...(input.referenceImages || [])],
    negativePrompt: input.negativePrompt,
    parameters,
    clientRequestId: input.clientRequestId,
    ...canonical,
  };
}

export function imageGeneratorViewState(
  input: ImageGeneratorViewStateInput,
): ImageGeneratorViewState {
  const emptyPromptReason = input.prompt.trim() ? "" : "请先描述想生成的图片";
  const disabledReason = emptyPromptReason || input.disabledReason;
  const tone = input.busy ? "loading" : input.statusTone;
  const showRetry = !input.busy
    && !disabledReason
    && tone === "error"
    && input.retryAvailable;
  return {
    canSubmit: !input.busy && !disabledReason,
    disabledReason,
    primaryAction: showRetry ? "retry" : "generate",
    primaryLabel: input.busy ? "图片生成中…" : showRetry ? "重新生成" : "生成图片",
    showSpinner: input.busy,
    showRetry,
    tone,
    liveMessage: input.error || input.statusMessage || disabledReason,
  };
}

export function imageRequestOutcomeForError(error: unknown): ImageRequestPreviousOutcome {
  const record = recordValue(error);
  const statusCode = Number(record?.statusCode ?? record?.status);
  if (statusCode === 0) return "network-uncertain";
  if (Number.isFinite(statusCode)) return "terminal-failure";

  const name = error instanceof Error ? error.name : String(record?.name || "");
  const message = error instanceof Error ? error.message : String(record?.message || "");
  return name === "TypeError" && /network|fetch|网络|timeout|timed out|ECONN|ERR_NETWORK/i.test(message)
    ? "network-uncertain"
    : "terminal-failure";
}

function sameReferenceSnapshot(left: string[], right: string[]) {
  return left.length === right.length && left.every((value, index) => value === right[index]);
}

export async function resolveImageReferenceUploads(
  input: ResolveImageReferenceUploadsInput,
  upload: (sourceReferences: string[]) => Promise<string[]>,
): Promise<ResolvedImageReferenceUploads> {
  const sourceSnapshot = [...input.sourceReferences];
  const cached = input.cache;
  if (
    input.previousOutcome === "network-uncertain"
    && cached
    && sameReferenceSnapshot(cached.sourceSnapshot, sourceSnapshot)
    && cached.uploadedURLs.length === sourceSnapshot.length
  ) {
    return {
      referenceImages: [...cached.uploadedURLs],
      cache: {
        sourceSnapshot: [...cached.sourceSnapshot],
        uploadedURLs: [...cached.uploadedURLs],
      },
      reused: true,
    };
  }

  const uploadedURLs = await upload([...sourceSnapshot]);
  const cache: ImageReferenceUploadCache = {
    sourceSnapshot,
    uploadedURLs: [...uploadedURLs],
  };
  return { referenceImages: [...uploadedURLs], cache, reused: false };
}

export async function submitCanonicalImageTask<Task>(
  input: SubmitCanonicalImageTaskInput,
  dependencies: SubmitCanonicalImageTaskDependencies<Task>,
): Promise<ImageTaskSubmissionResult<Task>> {
  const state: ImageTaskSubmissionState = {
    uploadCache: input.uploadCache
      ? {
          sourceSnapshot: [...input.uploadCache.sourceSnapshot],
          uploadedURLs: [...input.uploadCache.uploadedURLs],
        }
      : undefined,
    requestKey: input.requestKey ? { ...input.requestKey } : undefined,
    previousOutcome: input.previousOutcome,
  };
  let draft: CreateDraft | undefined;
  let requestSnapshot: CreateGenerationTaskRequest | undefined;
  let finalRequest: CreateGenerationTaskRequest | undefined;
  let fingerprint: string | undefined;
  let createTaskStarted = false;

  try {
    const references = await resolveImageReferenceUploads({
      sourceReferences: input.sourceReferences,
      cache: input.uploadCache,
      previousOutcome: input.previousOutcome,
    }, dependencies.uploadReferences);
    state.uploadCache = references.cache;

    const canonicalDraft = buildCanonicalImageDraft({
      contract: input.contract,
      selection: input.selection,
      prompt: input.prompt,
      model: input.model,
      style: input.style,
      referenceImages: references.referenceImages,
      negativePrompt: input.negativePrompt,
      parameters: input.parameters,
    });
    requestSnapshot = taskRequestFromDraft(canonicalDraft);
    fingerprint = imageRequestFingerprint(requestSnapshot);
    const requestKey = nextImageClientRequestKey({
      fingerprint,
      existing: input.requestKey,
      previousOutcome: input.previousOutcome,
    }, dependencies.clientIdFactory);
    state.requestKey = requestKey;
    draft = { ...canonicalDraft, clientRequestId: requestKey.clientRequestId };
    finalRequest = taskRequestFromDraft(draft);

    createTaskStarted = true;
    const task = await dependencies.createTask(draft);
    const status = String(recordValue(task)?.status || "").toUpperCase();
    state.previousOutcome = ["FAILED", "ERROR"].includes(status)
      ? "terminal-failure"
      : undefined;
    return {
      ok: true,
      task,
      state,
      draft,
      requestSnapshot,
      finalRequest,
      fingerprint,
    };
  } catch (error) {
    if (createTaskStarted) state.previousOutcome = imageRequestOutcomeForError(error);
    return {
      ok: false,
      error,
      state,
      draft,
      requestSnapshot,
      finalRequest,
      fingerprint,
    };
  }
}

function stableRequestSnapshot(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(stableRequestSnapshot);
  const record = recordValue(value);
  if (!record) return value;

  const sorted: UnknownRecord = {};
  for (const key of Object.keys(record).sort()) {
    sorted[key] = stableRequestSnapshot(record[key]);
  }
  return sorted;
}

export function imageRequestFingerprint(requestSnapshot: CanonicalImageRequestSnapshot): string {
  const request = recordValue(requestSnapshot);
  if (!request) throw new Error("图片 canonical request 必须是对象");

  const semanticRequest: UnknownRecord = {};
  for (const [key, value] of Object.entries(request)) {
    if (key === "clientRequestId") continue;
    semanticRequest[key] = value;
  }
  return JSON.stringify(stableRequestSnapshot(semanticRequest));
}

export function nextImageClientRequestKey(
  input: NextImageClientRequestKeyInput,
  uuidFactory: () => string,
): ImageClientRequestKeyState {
  if (!input.fingerprint) throw new Error("图片请求 fingerprint 不能为空");
  if (
    input.previousOutcome === "network-uncertain"
    && input.existing?.fingerprint === input.fingerprint
    && input.existing.clientRequestId
  ) {
    return { ...input.existing };
  }

  const uuid = uuidFactory();
  if (typeof uuid !== "string" || !uuid.trim()) throw new Error("图片请求 UUID 不能为空");
  return {
    fingerprint: input.fingerprint,
    clientRequestId: `image_${uuid.trim()}`,
  };
}

export function imagePointEstimateLabel(
  _model: ImageGeneratorModelOption | undefined,
  _count: number,
): string {
  return "以生成时结算为准";
}
