import { api } from "../../api/client";
import type { InspirationCategory, InspirationDetailResponse, InspirationTemplate } from "./types";

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
  categories: () => cached("categories:miniprogram", 10 * 60 * 1000, () => api<{ items: InspirationCategory[] }>("/api/v1/inspirations/categories?platform=miniprogram")),
  featured: (category = "", seed = 0, limit = 8) => cached(`featured:${category}:${seed}:${limit}`, 2 * 60 * 1000, () => api<{ items: InspirationTemplate[]; total: number; seed: number }>(`/api/v1/inspirations/featured${queryString({ category, seed, limit, platform: "miniprogram" })}`).then((result) => ({
    ...result,
    items: (result.items || []).filter((item) => item.contentType !== "video"),
  }))),
  list: (input: { page: number; pageSize?: number; category?: string; contentType?: string; q?: string }) => api<{ items: InspirationTemplate[]; total: number; page: number; pageSize: number; hasMore: boolean }>(`/api/v1/inspirations${queryString({ ...input, platform: "miniprogram" })}`).then((result) => ({
    ...result,
    items: (result.items || []).filter((item) => item.contentType !== "video"),
  })),
  detail: (id: string) => api<InspirationDetailResponse>(`/api/v1/inspirations/${encodeURIComponent(id)}?platform=miniprogram`).then((result) => {
    if (result?.item?.contentType === "video") {
      throw new Error("该灵感模板暂不可用");
    }
    return result;
  }),
  favorite: (id: string, favorite: boolean) => api<{ favorite: boolean }>(`/api/v1/inspirations/${encodeURIComponent(id)}/favorite`, { method: favorite ? "PUT" : "DELETE" }),
  event: (id: string, eventType: "copy_prompt" | "use_template" | "generate_success", generationTaskId = "") => api(`/api/v1/inspirations/${encodeURIComponent(id)}/events`, { method: "POST", body: JSON.stringify({ eventType, generationTaskId, platform: "miniprogram" }) }),
};
