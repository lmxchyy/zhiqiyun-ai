import { adminFetchResponse, adminRequest } from "./client";

type ReferenceImagePayload = {
  item?: { url?: string };
  url?: string;
};

export async function fetchResourceBlob(url: string, options: { auth?: boolean } = {}) {
  const response = await adminFetchResponse(url, { method: "GET" }, options);
  return response.blob();
}

export function downloadAssetBlob(assetId: string) {
  return adminRequest<Blob>({
    method: "GET",
    url: `/assets/${encodeURIComponent(assetId)}/download`,
    responseType: "blob"
  });
}

export async function uploadReferenceImage(file: File) {
  const form = new FormData();
  form.append("file", file, file.name || "reference-image.png");
  const payload = await adminRequest<ReferenceImagePayload>({
    method: "POST",
    url: "/reference-images",
    data: form
  });
  const url = String(payload.item?.url || payload.url || "").trim();
  if (!url) throw new Error("参考图上传结果缺少 URL");
  return url;
}
