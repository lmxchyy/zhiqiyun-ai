import { api } from "../../api/client";
import { normalizePublicTemplateDetailResponse } from "./contracts";
import { recordInspirationEvent } from "./events";
import type {
  InspirationCategory,
  InspirationComposeRequest,
  InspirationComposeResponse,
  InspirationDetailResponse,
  InspirationTemplate,
} from "./types";

function queryString(params: Record<string, string | number | boolean | undefined>) {
  const query = Object.entries(params).filter(([, value]) => value !== undefined && value !== "").map(([key, value]) => `${encodeURIComponent(key)}=${encodeURIComponent(String(value))}`).join("&");
  return query ? `?${query}` : "";
}

const responseCache = new Map<string, { expiresAt: number; value: unknown }>();
async function cached<T>(key: string, ttl: number, loader: () => Promise<T>) {
  const hit = responseCache.get(key);
  if (hit && hit.expiresAt > Date.now()) return hit.value as T;
  const value = await loader();
  responseCache.set(key, { expiresAt: Date.now() + ttl, value });
  return value;
}

export const inspirationAPI = {
  categories: () => cached("categories", 10 * 60 * 1000, () => api<{ items: InspirationCategory[] }>("/api/v1/inspirations/categories")),
  featured: (category = "", seed = 0, limit = 8) => cached(`featured:${category}:${seed}:${limit}`, 2 * 60 * 1000, () => api<{ items: InspirationTemplate[]; total: number; seed: number }>(`/api/v1/inspirations/featured${queryString({ category, seed, limit, platform: "miniprogram" })}`)),
  list: (input: { page: number; pageSize?: number; category?: string; contentType?: string; q?: string }) => api<{ items: InspirationTemplate[]; total: number; page: number; pageSize: number; hasMore: boolean }>(`/api/v1/inspirations${queryString({ ...input, platform: "miniprogram" })}`),
  detail: async (slug: string): Promise<InspirationDetailResponse> => normalizePublicTemplateDetailResponse(
    await api<unknown>(`/api/v1/inspirations/${encodeURIComponent(slug)}?platform=miniprogram`),
  ),
  compose: (slug: string, body: InspirationComposeRequest) => api<InspirationComposeResponse>(`/api/v1/inspirations/${encodeURIComponent(slug)}/compose`, {
    method: "POST",
    body: JSON.stringify(body),
  }),
  favorite: (slug: string, favorite: boolean) => api<{ favorite: boolean }>(`/api/v1/inspirations/${encodeURIComponent(slug)}/favorite`, { method: favorite ? "PUT" : "DELETE" }),
  event: recordInspirationEvent,
};
