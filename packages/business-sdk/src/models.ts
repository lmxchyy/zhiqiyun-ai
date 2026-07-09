import type { ApiClient } from "@xianzhi/api-client";
import type { ModelInfo } from "@xianzhi/shared-types";
import type { BusinessSdk } from "./types";

export function createModelsSdk(api: ApiClient): BusinessSdk["models"] {
  return {
    list: () => api.request<ModelInfo[]>("/api/v1/models")
  };
}
