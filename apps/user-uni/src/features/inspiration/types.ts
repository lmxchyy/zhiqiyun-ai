export type InspirationContentType = "image" | "video" | "ppt";

export interface InspirationCategory {
  id: string;
  code: string;
  name: string;
  sort: number;
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
  parameters?: Record<string, unknown>;
  referenceAssets?: unknown[];
  tags?: string[];
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
