import type { ApiClient } from "@xianzhi/api-client";
import type { AuthResponse } from "@xianzhi/shared-types";
import type { BusinessSdk } from "./types";

export function createAuthSdk(api: ApiClient): BusinessSdk["auth"] {
  return {
    me: () => api.request<AuthResponse>("/api/v1/auth/me")
  };
}
