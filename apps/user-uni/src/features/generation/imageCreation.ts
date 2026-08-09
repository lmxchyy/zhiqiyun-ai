import type { ModelInfo } from "@xianzhi/shared-types";

export type ImageAspectRatio = "auto" | "1:1" | "16:9" | "9:16" | "4:3";
export type ImageQuality = "1K" | "2K";

export interface ImageGeneratorModelOption {
  code: string;
  name: string;
  pointCost?: number;
}

export const imageAspectOptions: Array<{ value: ImageAspectRatio; label: string }> = [
  { value: "auto", label: "auto" },
  { value: "1:1", label: "1:1" },
  { value: "16:9", label: "16:9" },
  { value: "9:16", label: "9:16" },
  { value: "4:3", label: "4:3" },
];

export const imageQualityOptions: ImageQuality[] = ["1K", "2K"];
export const imageCountOptions = [1, 2, 4] as const;

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
  return models.some(model => model.code === requested) ? requested : models[0]?.code || "";
}

export function imagePointEstimateLabel(model: ImageGeneratorModelOption | undefined, count: number): string {
  return typeof model?.pointCost === "number" && model.pointCost >= 0
    ? `预计 ${Math.round(model.pointCost * Math.max(1, count))} 积分`
    : "以生成时结算为准";
}
