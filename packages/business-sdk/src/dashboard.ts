import type { ApiClient } from "@xianzhi/api-client";
import type { AuthResponse, UserDashboardResponse } from "@xianzhi/shared-types";
import { defaultFeatures, profileFromAuth, workFromAsset, workFromTask } from "./mappers";
import { listPlans } from "./membership";
import type { BusinessSdk } from "./types";

export function createDashboardSdk(api: ApiClient): BusinessSdk["dashboard"] {
  return {
    async getHomeOverview() {
      const [auth, dashboard, plans] = await Promise.all([
        api.request<AuthResponse>("/api/v1/auth/me"),
        api.request<UserDashboardResponse>("/api/v1/user/dashboard"),
        listPlans(api).catch(() => [])
      ]);
      const works = [...(dashboard.recentAssets || []).map(workFromAsset), ...(dashboard.recentTasks || []).map(workFromTask)];
      return {
        user: profileFromAuth(auth, Number(dashboard.summary?.availablePoints || 0)),
        features: defaultFeatures,
        works,
        plans
      };
    }
  };
}
