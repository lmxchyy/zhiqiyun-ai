import type { VideoModelCapabilities } from "@xianzhi/shared-types";

export const VIDEO_PARAMETER_KEYS = [
  "duration",
  "resolution",
  "aspect_ratio",
  "fps",
  "generate_audio",
  "motion_strength",
  "camera_movement",
] as const;

export type VideoParameterKey = typeof VIDEO_PARAMETER_KEYS[number];

export interface EditableVideoField {
  key: VideoParameterKey;
  label: string;
  type: string;
  defaultValue?: unknown;
  options: unknown[];
  min?: number;
  max?: number;
  unit?: string;
}

type UnknownRecord = Record<string, unknown>;

const videoParameterKeySet = new Set<string>(VIDEO_PARAMETER_KEYS);

function record(value: unknown): UnknownRecord {
  return value && typeof value === "object" && !Array.isArray(value)
    ? value as UnknownRecord
    : {};
}

function schemaFields(schema: unknown): UnknownRecord[] {
  const direct = record(schema);
  const nested = record(direct.schema);
  const values = Array.isArray(direct.fields)
    ? direct.fields
    : Array.isArray(nested.fields)
      ? nested.fields
      : [];
  return values.map(record);
}

function canonicalVideoParameterKey(value: unknown): VideoParameterKey | "" {
  const key = String(value || "").trim();
  const canonical = key === "ratio" ? "aspect_ratio" : key;
  return videoParameterKeySet.has(canonical) ? canonical as VideoParameterKey : "";
}

function supportedParameterSet(capabilities: VideoModelCapabilities | UnknownRecord): Set<string> {
  const raw = capabilities as VideoModelCapabilities & { supported_parameters?: unknown };
  const values = Array.isArray(raw.supportedParameters)
    ? raw.supportedParameters
    : Array.isArray(raw.supported_parameters)
      ? raw.supported_parameters
      : [];
  return new Set(values.map(value => canonicalVideoParameterKey(value)).filter(Boolean));
}

function sameOption(left: unknown, right: unknown): boolean {
  if (typeof left === "number" || typeof right === "number") {
    const leftNumber = Number(left);
    const rightNumber = Number(right);
    return Number.isFinite(leftNumber) && Number.isFinite(rightNumber) && leftNumber === rightNumber;
  }
  return left === right;
}

function capabilityOptions(
  key: VideoParameterKey,
  capabilities: VideoModelCapabilities | UnknownRecord,
): unknown[] {
  const value = capabilities as VideoModelCapabilities;
  if (key === "duration") return Array.isArray(value.supportedDurations) ? value.supportedDurations : [];
  if (key === "resolution") return Array.isArray(value.supportedResolutions) ? value.supportedResolutions : [];
  if (key === "aspect_ratio") return Array.isArray(value.supportedAspectRatios) ? value.supportedAspectRatios : [];
  return [];
}

function intersectOptions(schemaOptions: unknown[], capabilityValues: unknown[]): unknown[] {
  if (!capabilityValues.length) return [...schemaOptions];
  if (!schemaOptions.length) return [...capabilityValues];
  return schemaOptions.filter(option => capabilityValues.some(value => sameOption(option, value)));
}

function optionalNumber(value: unknown): number | undefined {
  if (value === null || value === undefined || value === "") return undefined;
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : undefined;
}

export function deriveEditableVideoFields(
  schema: unknown,
  capabilities: VideoModelCapabilities | UnknownRecord,
): EditableVideoField[] {
  const supported = supportedParameterSet(capabilities);
  const result: EditableVideoField[] = [];
  const seen = new Set<VideoParameterKey>();

  for (const raw of schemaFields(schema)) {
    const key = canonicalVideoParameterKey(raw.key);
    const userEditable = raw.userEditable ?? raw.user_editable;
    if (!key || seen.has(key) || raw.visible !== true || userEditable !== true || !supported.has(key)) {
      continue;
    }
    seen.add(key);
    const rawOptions = Array.isArray(raw.options) ? raw.options : [];
    const field: EditableVideoField = {
      key,
      label: String(raw.label || key),
      type: String(raw.type || (key === "generate_audio" ? "boolean" : "select")).toLowerCase(),
      defaultValue: raw.default ?? raw.defaultValue,
      options: intersectOptions(rawOptions, capabilityOptions(key, capabilities)),
    };
    const min = optionalNumber(raw.min);
    const max = optionalNumber(raw.max);
    if (min !== undefined) field.min = min;
    if (max !== undefined) field.max = max;
    if (raw.unit !== undefined && raw.unit !== null) field.unit = String(raw.unit);
    result.push(field);
  }
  return result;
}

function normalizePreviousValues(values: Record<string, unknown>): Record<string, unknown> {
  const normalized = { ...values };
  if (normalized.aspect_ratio === undefined && normalized.ratio !== undefined) {
    normalized.aspect_ratio = normalized.ratio;
  }
  delete normalized.ratio;
  return normalized;
}

function legalFieldValue(field: EditableVideoField, value: unknown): boolean {
  if (value === undefined || value === null || value === "") return false;
  if (field.options.length) return field.options.some(option => sameOption(option, value));
  if (field.type === "boolean" || field.type === "switch") return typeof value === "boolean";
  if (field.type === "number") {
    const parsed = Number(value);
    if (!Number.isFinite(parsed)) return false;
    if (field.min !== undefined && parsed < field.min) return false;
    if (field.max !== undefined && parsed > field.max) return false;
  }
  return true;
}

function normalizedFieldValue(field: EditableVideoField, value: unknown): unknown {
  if (field.type === "number") return Number(value);
  return value;
}

function fallbackFieldValue(field: EditableVideoField): unknown {
  if (legalFieldValue(field, field.defaultValue)) {
    return normalizedFieldValue(field, field.defaultValue);
  }
  if (field.options.length) return field.options[0];
  if ((field.type === "boolean" || field.type === "switch") && typeof field.defaultValue === "boolean") {
    return field.defaultValue;
  }
  return undefined;
}

export function transitionVideoParameterValues(
  previous: Record<string, unknown>,
  fields: EditableVideoField[],
): Record<string, unknown> {
  const normalized = normalizePreviousValues(previous);
  const result: Record<string, unknown> = {};
  for (const field of fields) {
    const previousValue = normalized[field.key];
    const value = legalFieldValue(field, previousValue)
      ? normalizedFieldValue(field, previousValue)
      : fallbackFieldValue(field);
    if (value !== undefined) result[field.key] = value;
  }
  return result;
}

export function buildVideoSubmissionParameters(
  values: Record<string, unknown>,
  fields: EditableVideoField[],
): Record<string, unknown> {
  const transitioned = transitionVideoParameterValues(values, fields);
  const result: Record<string, unknown> = {};
  for (const field of fields) {
    if (transitioned[field.key] !== undefined) result[field.key] = transitioned[field.key];
  }
  return result;
}
