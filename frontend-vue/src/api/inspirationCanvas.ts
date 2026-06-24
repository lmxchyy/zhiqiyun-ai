import { api } from "./client";
import type { Asset, GenerationTask } from "../types";

export interface InspirationReferenceImage {
  assetId: string;
  name: string;
  url: string;
}

export interface CreateInspirationImageInput {
  prompt: string;
  model: string;
  provider?: string;
  imageRatio: string;
  imageQuality: string;
  count: number;
  references: Asset[];
}

export function toReferenceImages(assets: Asset[]): InspirationReferenceImage[] {
  return assets.map(asset => ({
    assetId: asset.id,
    name: asset.name,
    url: asset.url
  }));
}

export function createInspirationImage(input: CreateInspirationImageInput): Promise<GenerationTask> {
  const referenceImages = toReferenceImages(input.references);
  return api<GenerationTask>("/api/v1/generation-tasks", {
    method: "POST",
    body: JSON.stringify({
      type: referenceImages.length ? "IMAGE_TO_IMAGE" : "TEXT_TO_IMAGE",
      prompt: input.prompt,
      model: input.model,
      params: {
        sourceModule: "inspiration-canvas",
        provider: input.provider,
        imageRatio: input.imageRatio,
        imageQuality: input.imageQuality,
        count: input.count,
        referenceImages
      }
    })
  });
}

export function deleteInspirationAsset(assetId: string): Promise<{ ok: boolean }> {
  return api<{ ok: boolean }>(`/api/v1/assets/${encodeURIComponent(assetId)}`, {
    method: "DELETE"
  });
}
