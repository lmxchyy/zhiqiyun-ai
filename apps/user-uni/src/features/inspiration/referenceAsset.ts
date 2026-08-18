function record(value: unknown): Record<string, unknown> {
  return value && typeof value === "object" && !Array.isArray(value) ? value as Record<string, unknown> : {};
}

function stringValue(value: unknown) {
  return typeof value === "string" ? value.trim() : "";
}

export interface ReferenceAssetUploadResult {
  assetId: string;
  name: string;
  previewUrl: string;
  mimeType: string;
}

export function referenceAssetFromPayload(value: unknown): ReferenceAssetUploadResult {
  const payload = record(value);
  const item = record(payload.item || payload);
  const assetId = stringValue(item.assetId);
  if (!assetId) throw new Error("上传接口未返回 assetId");
  return {
    assetId,
    name: stringValue(item.name),
    previewUrl: stringValue(item.url || item.path),
    mimeType: stringValue(item.contentType),
  };
}
