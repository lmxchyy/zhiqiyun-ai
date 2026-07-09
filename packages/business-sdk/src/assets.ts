import type { ApiClient } from "@xianzhi/api-client";
import type { Asset } from "@xianzhi/shared-types";
import { workFromAsset, workFromTask } from "./mappers";
import { listTasks } from "./generation";
import type { BusinessSdk } from "./types";

export function listAssets(api: ApiClient) {
  return api.request<Asset[]>("/api/v1/assets");
}

export function createAssetsSdk(api: ApiClient): BusinessSdk["assets"] {
  return {
    list: () => listAssets(api),
    async getWorks() {
      const [assets, tasks] = await Promise.all([listAssets(api).catch(() => []), listTasks(api).catch(() => [])]);
      return [...assets.map(workFromAsset), ...tasks.map(workFromTask)];
    }
  };
}
