import type {
  InspirationComposeRequest,
  InspirationContentType,
  InspirationDetailResponse,
  InspirationTemplate,
  PublicTemplateDefinition,
  PublicTemplateInput,
  PublicTemplateInputOption,
  PublicTemplateInputValidation,
  PublicTemplateVisibilityCondition,
  TemplateAssetValues,
  TemplateFormSection,
  TemplateInputControl,
  TemplateInputType,
  TemplateUploadedAsset,
} from "./types.ts";

const controls = new Set<TemplateInputControl>([
  "TEXT", "TEXTAREA", "SELECT", "MULTI_SELECT", "SEGMENTED", "BOOLEAN", "NUMBER", "SLIDER", "ASSET_UPLOAD",
]);
const inputTypes = new Set<TemplateInputType>([
  "TEXT", "TEXTAREA", "NUMBER", "SELECT", "MULTI_SELECT", "BOOLEAN", "IMAGE", "VIDEO", "FILE",
]);
const sections: TemplateFormSection[] = ["materials", "requirements", "preferences", "advanced"];
const forbiddenPublicKeys = /(prompt|composer|binding|modelhint|provider|executor|workflow|failurepolicy|definition|apikey|secret|token|capability)/i;

function record(value: unknown): Record<string, unknown> {
  return value && typeof value === "object" && !Array.isArray(value) ? value as Record<string, unknown> : {};
}

function stringValue(value: unknown) {
  return typeof value === "string" ? value.trim() : "";
}

function numberValue(value: unknown, fallback = 0) {
  const number = Number(value);
  return Number.isFinite(number) ? number : fallback;
}

function booleanValue(value: unknown) {
  return value === true;
}

function stringList(value: unknown) {
  return Array.isArray(value) ? value.map(stringValue).filter(Boolean) : [];
}

function sanitizePublicValue(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(sanitizePublicValue);
  if (!value || typeof value !== "object") return value;
  const result: Record<string, unknown> = {};
  for (const [key, item] of Object.entries(value as Record<string, unknown>)) {
    if (!forbiddenPublicKeys.test(key)) result[key] = sanitizePublicValue(item);
  }
  return result;
}

function normalizeOption(value: unknown): PublicTemplateInputOption | null {
  const item = record(value);
  const label = stringValue(item.label);
  if (!label || !("value" in item)) return null;
  return { label, value: item.value };
}

function optionalNumber(source: Record<string, unknown>, key: string) {
  if (!(key in source) || source[key] === null || source[key] === "") return undefined;
  const value = Number(source[key]);
  return Number.isFinite(value) ? value : undefined;
}

function normalizeValidation(value: unknown): PublicTemplateInputValidation {
  const item = record(value);
  return {
    minLength: optionalNumber(item, "minLength"),
    maxLength: optionalNumber(item, "maxLength"),
    min: optionalNumber(item, "min"),
    max: optionalNumber(item, "max"),
    minItems: optionalNumber(item, "minItems"),
    maxItems: optionalNumber(item, "maxItems"),
    pattern: stringValue(item.pattern) || undefined,
    accept: stringList(item.accept),
  };
}

function normalizeVisibility(value: unknown): PublicTemplateVisibilityCondition | undefined {
  const item = record(value);
  const inputKey = stringValue(item.inputKey);
  const operator = stringValue(item.operator);
  if (!inputKey || !operator) return undefined;
  return { inputKey, operator, ...(Object.prototype.hasOwnProperty.call(item, "value") ? { value: item.value } : {}) };
}

function normalizeInput(value: unknown, index: number): PublicTemplateInput | null {
  const item = record(value);
  const key = stringValue(item.key);
  const type = stringValue(item.type).toUpperCase() as TemplateInputType;
  const label = stringValue(item.label);
  if (!key || !label || !inputTypes.has(type)) return null;
  const controlText = stringValue(item.control).toUpperCase() as TemplateInputControl;
  const sectionText = stringValue(item.section) as TemplateFormSection;
  const options = Array.isArray(item.options) ? item.options.map(normalizeOption).filter((option): option is PublicTemplateInputOption => Boolean(option)) : [];
  return {
    key,
    type,
    control: controls.has(controlText) ? controlText : undefined,
    label,
    required: booleanValue(item.required),
    helpText: stringValue(item.helpText) || undefined,
    placeholder: stringValue(item.placeholder) || undefined,
    ...(Object.prototype.hasOwnProperty.call(item, "default") ? { default: item.default } : {}),
    options,
    validation: normalizeValidation(item.validation),
    visibleWhen: normalizeVisibility(item.visibleWhen),
    section: sections.includes(sectionText) ? sectionText : undefined,
    order: optionalNumber(item, "order") ?? index,
    advanced: booleanValue(item.advanced),
  };
}

function normalizeSchema(value: unknown): PublicTemplateDefinition {
  const item = record(value);
  const inputs = Array.isArray(item.inputs)
    ? item.inputs.map(normalizeInput).filter((input): input is PublicTemplateInput => Boolean(input))
    : [];
  const presets = record(item.presets);
  return {
    inputs,
    presentation: sanitizePublicValue(record(item.presentation)) as Record<string, unknown>,
    presets: { inputDefaults: sanitizePublicValue(record(presets.inputDefaults)) as Record<string, unknown> },
    handoff: { targetType: stringValue(record(item.handoff).targetType) },
  };
}

function normalizeContentType(value: unknown): InspirationContentType {
  const type = stringValue(value).toLowerCase();
  return (["image", "video", "ppt", "text", "agent", "workflow"] as InspirationContentType[]).includes(type as InspirationContentType)
    ? type as InspirationContentType
    : "image";
}

function normalizeSummary(value: unknown): InspirationTemplate {
  const item = record(value);
  return {
    id: stringValue(item.id),
    slug: stringValue(item.slug),
    title: stringValue(item.title),
    description: stringValue(item.description),
    contentType: normalizeContentType(item.contentType),
    categoryId: stringValue(item.categoryId),
    categoryCode: stringValue(item.categoryCode) || undefined,
    categoryName: stringValue(item.categoryName) || undefined,
    coverUrl: stringValue(item.coverUrl),
    thumbnailUrl: stringValue(item.thumbnailUrl) || undefined,
    resultUrl: stringValue(item.resultUrl) || undefined,
    platforms: stringList(item.platforms),
    tags: stringList(item.tags),
    featured: booleanValue(item.featured),
    hot: booleanValue(item.hot),
    pinned: booleanValue(item.pinned),
    sort: numberValue(item.sort),
    templateVersion: Math.max(1, Math.trunc(numberValue(item.templateVersion, 1))),
    favorite: booleanValue(item.favorite),
    viewCount: numberValue(item.viewCount),
    copyCount: numberValue(item.copyCount),
    favoriteCount: numberValue(item.favoriteCount),
    useCount: numberValue(item.useCount),
    generateCount: numberValue(item.generateCount),
  };
}

export function normalizePublicTemplateDetailResponse(value: unknown): InspirationDetailResponse {
  const payload = record(value);
  const item = record(payload.item);
  return {
    item: { ...normalizeSummary(item), schema: normalizeSchema(item.schema) },
    aiGenerated: payload.aiGenerated !== false,
  };
}

export function templateInputControl(input: Pick<PublicTemplateInput, "type" | "control">): TemplateInputControl {
  if (input.control && controls.has(input.control)) return input.control;
  if (["IMAGE", "VIDEO", "FILE"].includes(input.type)) return "ASSET_UPLOAD";
  return input.type as TemplateInputControl;
}

export function templateAssetMaxItems(input: Pick<PublicTemplateInput, "validation">) {
  return Math.max(1, Number(input.validation?.minItems || 0), Number(input.validation?.maxItems || 0));
}

function defaultSection(input: PublicTemplateInput): TemplateFormSection {
  if (input.advanced) return "advanced";
  if (templateInputControl(input) === "ASSET_UPLOAD") return "materials";
  if (["TEXT", "TEXTAREA"].includes(templateInputControl(input))) return "requirements";
  return "preferences";
}

function valuesEqual(left: unknown, right: unknown) {
  return JSON.stringify(left) === JSON.stringify(right);
}

export function templateInputVisible(input: PublicTemplateInput, values: Record<string, unknown>) {
  const condition = input.visibleWhen;
  if (!condition) return true;
  const actual = values[condition.inputKey];
  switch (condition.operator.toLowerCase()) {
  case "eq":
  case "equals": return valuesEqual(actual, condition.value);
  case "neq":
  case "not_equals": return !valuesEqual(actual, condition.value);
  case "in": return Array.isArray(condition.value) && condition.value.some(value => valuesEqual(actual, value));
  case "not_in": return Array.isArray(condition.value) && !condition.value.some(value => valuesEqual(actual, value));
  case "truthy": return Boolean(actual);
  case "falsy": return !actual;
  default: return false;
  }
}

export function templateInitialValues(inputs: PublicTemplateInput[], presetDefaults: Record<string, unknown> = {}) {
  const values: Record<string, unknown> = {};
  for (const input of inputs) {
    if (templateInputControl(input) === "ASSET_UPLOAD") continue;
    if (Object.prototype.hasOwnProperty.call(presetDefaults, input.key)) values[input.key] = presetDefaults[input.key];
    else if (Object.prototype.hasOwnProperty.call(input, "default")) values[input.key] = input.default;
  }
  return values;
}

export interface TemplateInputGroup {
  key: TemplateFormSection;
  label: string;
  inputs: PublicTemplateInput[];
}

const sectionLabels: Record<TemplateFormSection, string> = {
  materials: "需要你提供",
  requirements: "你想要什么",
  preferences: "效果偏好",
  advanced: "更多设置",
};

export function groupTemplateInputs(inputs: PublicTemplateInput[], values: Record<string, unknown>): TemplateInputGroup[] {
  return sections.map(key => ({
    key,
    label: sectionLabels[key],
    inputs: inputs
      .filter(input => (input.section || defaultSection(input)) === key && templateInputVisible(input, values))
      .map((input, index) => ({ input, index }))
      .sort((left, right) => Number(left.input.order ?? left.index) - Number(right.input.order ?? right.index))
      .map(item => item.input),
  })).filter(group => group.inputs.length > 0);
}

function uploadedAssets(assets: TemplateUploadedAsset[] | undefined) {
  return (assets || []).filter(item => item.status === "uploaded" && Boolean(item.assetId));
}

function acceptedMime(mimeType: string | undefined, accepts: string[]) {
  if (!accepts.length || !mimeType) return true;
  const mime = mimeType.toLowerCase();
  return accepts.some(value => {
    const accept = value.toLowerCase();
    return accept === mime || (accept.endsWith("/*") && mime.startsWith(accept.slice(0, -1)));
  });
}

export function validateTemplateInputValues(
  inputs: PublicTemplateInput[],
  values: Record<string, unknown>,
  assets: TemplateAssetValues,
) {
  const errors: Record<string, string> = {};
  for (const input of inputs) {
    if (!templateInputVisible(input, values)) continue;
    const validation = input.validation || {};
    if (templateInputControl(input) === "ASSET_UPLOAD") {
      const all = assets[input.key] || [];
      if (all.some(item => item.status === "uploading")) {
        errors[input.key] = `${input.label}正在上传`;
        continue;
      }
      if (all.some(item => item.status === "failed")) {
        errors[input.key] = `${input.label}上传失败，请重试`;
        continue;
      }
      const uploaded = uploadedAssets(all);
      const minimum = Math.max(input.required ? 1 : 0, Number(validation.minItems || 0));
      if (uploaded.length < minimum) {
        errors[input.key] = input.required && minimum === 1 ? `请上传${input.label}` : `${input.label}至少需要 ${minimum} 个素材`;
        continue;
      }
      if (validation.maxItems !== undefined && uploaded.length > validation.maxItems) {
        errors[input.key] = `${input.label}最多上传 ${validation.maxItems} 个素材`;
        continue;
      }
      if (uploaded.some(item => !acceptedMime(item.mimeType, validation.accept || []))) {
        errors[input.key] = `${input.label}格式不受支持`;
      }
      continue;
    }

    const value = values[input.key];
    const empty = value === undefined || value === null || value === "" || (Array.isArray(value) && value.length === 0);
    if (input.required && empty) {
      errors[input.key] = `请填写${input.label}`;
      continue;
    }
    if (empty) continue;
    if (typeof value === "string") {
      if (validation.minLength !== undefined && value.length < validation.minLength) errors[input.key] = `${input.label}至少输入 ${validation.minLength} 个字符`;
      else if (validation.maxLength !== undefined && value.length > validation.maxLength) errors[input.key] = `${input.label}最多输入 ${validation.maxLength} 个字符`;
      else if (validation.pattern) {
        try {
          if (!new RegExp(validation.pattern).test(value)) errors[input.key] = `${input.label}格式不正确`;
        } catch {
          errors[input.key] = `${input.label}校验规则不可用`;
        }
      }
    }
    if (typeof value === "number") {
      if (validation.min !== undefined && value < validation.min) errors[input.key] = `${input.label}不能小于 ${validation.min}`;
      else if (validation.max !== undefined && value > validation.max) errors[input.key] = `${input.label}不能大于 ${validation.max}`;
    }
    if (Array.isArray(value)) {
      if (validation.minItems !== undefined && value.length < validation.minItems) errors[input.key] = `${input.label}至少选择 ${validation.minItems} 项`;
      else if (validation.maxItems !== undefined && value.length > validation.maxItems) errors[input.key] = `${input.label}最多选择 ${validation.maxItems} 项`;
    }
  }
  return errors;
}

export function buildInspirationComposeRequest(
  templateVersion: number,
  values: Record<string, unknown>,
  assets: TemplateAssetValues,
): InspirationComposeRequest {
  const materials = Object.entries(assets).flatMap(([inputKey, items]) => uploadedAssets(items).map(item => ({ inputKey, assetId: item.assetId })));
  return { templateVersion, values: { ...values }, materials };
}

function composeErrorCode(error: unknown) {
  const item = record(error);
  const payload = record(item.payload);
  return stringValue(item.apiCode || payload.code).toUpperCase();
}

export type InspirationComposeErrorAction = "auth" | "reload" | "input" | "material" | "schema" | "network" | "unknown";

export function inspirationComposeErrorAction(error: unknown): InspirationComposeErrorAction {
  const item = record(error);
  const statusCode = numberValue(item.statusCode);
  const code = composeErrorCode(error);
  if (statusCode === 401 || code === "AUTH_REQUIRED" || code === "UNAUTHORIZED") return "auth";
  if (statusCode === 409 || code.includes("VERSION_CONFLICT")) return "reload";
  if (code.includes("MATERIAL")) return "material";
  if (statusCode === 422 || code.includes("INPUT_REQUIRED") || code.includes("INPUT_INVALID")) return "input";
  if (code.includes("DEFINITION") || code.includes("SCHEMA")) return "schema";
  if (statusCode === 0) return "network";
  return "unknown";
}
