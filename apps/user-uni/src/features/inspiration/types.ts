export type InspirationContentType = "image" | "video" | "ppt";

export interface InspirationCategory {
  id: string;
  code: string;
  name: string;
  sort: number;
}

export interface InspirationDisplayConfig {
  comparisonMode?: "single" | "side_by_side" | "slider";
  beforeUrl?: string;
  afterUrl?: string;
}

export function inspirationComparisonSources(config?: InspirationDisplayConfig) {
  const beforeUrl = String(config?.beforeUrl || "").trim();
  const afterUrl = String(config?.afterUrl || "").trim();
  if (!beforeUrl || !afterUrl) return null;
  return {
    mode: config?.comparisonMode === "slider" ? "slider" as const : "side_by_side" as const,
    beforeUrl,
    afterUrl,
  };
}

export interface InspirationInputRequirements {
  referenceImageRequired?: boolean;
  referenceImageMin?: number;
  referenceImageMax?: number;
}

export interface InspirationPresetConfig {
  colorMode?: "natural" | "black_white";
  identityProtection?: boolean;
  restoreStrength?: "light" | "standard" | "strong";
  [key: string]: unknown;
}

export interface InspirationTemplate {
  id: string;
  title: string;
  description: string;
  contentType: InspirationContentType;
  categoryId: string;
  categoryCode?: string;
  categoryName?: string;
  coverUrl: string;
  thumbnailUrl?: string;
  resultUrl?: string;
  prompt?: string;
  negativePrompt?: string;
  modelId?: string;
  scenarioCode?: string;
  displayConfig?: InspirationDisplayConfig;
  inputRequirements?: InspirationInputRequirements;
  presetConfig?: InspirationPresetConfig;
  parameters?: Record<string, unknown>;
  referenceAssets?: unknown[];
  tags?: string[];
  featured?: boolean;
  hot?: boolean;
  pinned?: boolean;
  favorite: boolean;
  viewCount: number;
  favoriteCount: number;
  useCount: number;
  generateCount: number;
}

export interface InspirationDetailResponse {
  item: InspirationTemplate;
  modelAvailable: boolean;
  compatibleModelId: string;
  aiGenerated: boolean;
}
