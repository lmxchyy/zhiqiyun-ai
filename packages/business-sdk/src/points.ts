import type { ApiClient } from "@xianzhi/api-client";
import type { PointAccountResponse } from "@xianzhi/shared-types";
import type { BusinessSdk } from "./types";

export function createPointsSdk(api: ApiClient): BusinessSdk["points"] {
  return {
    account: () => api.request<PointAccountResponse>("/api/v1/member/wallet")
  };
}
