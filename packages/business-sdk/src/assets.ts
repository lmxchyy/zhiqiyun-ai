import type { ApiClient } from "@xianzhi/api-client";
import type { Asset } from "@xianzhi/shared-types";
import { workFromAsset, workFromTask } from "./mappers";
import { listTasks } from "./generation";
import type { BusinessSdk, PageOptions, PagedItems } from "./types";

export function listAssets(api: ApiClient) {
  return api.request<Asset[]>("/api/v1/assets");
}

export function listAssetPage(api: ApiClient, options: PageOptions = {}) {
  const params = [
    "paged=true",
    `limit=${encodeURIComponent(String(options.limit || 20))}`,
    `offset=${encodeURIComponent(String(options.offset || 0))}`,
  ];
  return api.request<PagedItems<Asset>>(`/api/v1/assets?${params.join("&")}`);
}

export function createAssetsSdk(api: ApiClient): BusinessSdk["assets"] {
  return {
    list: () => listAssets(api),
    listPage: options => listAssetPage(api, options),
    async getWorks() {
      const [assets, tasks] = await Promise.all([listAssets(api).catch(() => []), listTasks(api).catch(() => [])]);
      return [...assets.map(workFromAsset), ...tasks.map(workFromTask)];
    }
  };
}
