import { adminRequest } from "./client";

export type InspirationContentType = "image" | "video" | "ppt";
export interface InspirationTemplate {
  id: string; tenantId: string; title: string; description: string; contentType: InspirationContentType;
  categoryId: string; categoryName?: string; coverUrl: string; thumbnailUrl?: string; resultUrl?: string;
  prompt: string; negativePrompt: string; modelId: string; scenarioCode?: string;
  displayConfig?: Record<string, unknown>; inputRequirements?: Record<string, unknown>; presetConfig?: Record<string, unknown>;
  parameters: Record<string, unknown>;
  referenceAssets: unknown[]; platforms: string[]; tags: string[]; applicableTenantIds: string[];
  featured: boolean; hot: boolean; pinned: boolean; sort: number; status: string; auditStatus: string;
  auditNote?: string; startTime?: string; endTime?: string; version: number; sourceAssetId?: string;
  sourceAuthorized: boolean; viewCount: number; copyCount: number; favoriteCount: number; useCount: number; generateCount: number;
}
export interface InspirationCategory { id: string; tenantId: string; code: string; name: string; sort: number; status: string }

export const inspirationAdminAPI = {
  list: (params: Record<string, unknown>) => adminRequest<{ items: InspirationTemplate[]; total: number; page: number; pageSize: number }>({ url: "/admin/inspirations", params, authMode: "required" }),
  categories: () => adminRequest<{ items: InspirationCategory[] }>({ url: "/admin/inspirations/categories", authMode: "required" }),
  statistics: () => adminRequest<Record<string, number>>({ url: "/admin/inspirations/statistics", authMode: "required" }),
  create: (data: Partial<InspirationTemplate>) => adminRequest<{ item: InspirationTemplate }>({ url: "/admin/inspirations", method: "POST", data, authMode: "required" }),
  update: (id: string, data: Partial<InspirationTemplate>) => adminRequest<{ item: InspirationTemplate }>({ url: `/admin/inspirations/${id}`, method: "PUT", data, authMode: "required" }),
  remove: (id: string) => adminRequest({ url: `/admin/inspirations/${id}`, method: "DELETE", authMode: "required" }),
  action: (id: string, action: "copy" | "publish" | "withdraw" | "audit/approve" | "audit/reject") => adminRequest<{ item: InspirationTemplate }>({ url: `/admin/inspirations/${id}/${action}`, method: "POST", authMode: "required" }),
  versions: (id: string) => adminRequest<{ items: Array<{ id: string; version: number; changeNote: string; createdBy: string; createdAt: string }> }>({ url: `/admin/inspirations/${id}/versions`, authMode: "required" }),
  rollback: (id: string, version: number) => adminRequest({ url: `/admin/inspirations/${id}/rollback`, method: "POST", data: { version }, authMode: "required" }),
  batch: (ids: string[], action: string) => adminRequest({ url: "/admin/inspirations/batch", method: "POST", data: { ids, action }, authMode: "required" }),
};
