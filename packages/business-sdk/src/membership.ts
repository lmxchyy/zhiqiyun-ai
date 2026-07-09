import type { ApiClient } from "@xianzhi/api-client";
import type { CommercePlan } from "@xianzhi/shared-types";
import { planFromCommercePlan } from "./mappers";
import type { BusinessSdk } from "./types";

export async function listPlans(api: ApiClient, planType?: string) {
  const query = planType ? `?planType=${encodeURIComponent(planType)}` : "";
  const payload = await api.request<CommercePlan[] | { items?: CommercePlan[] }>(`/api/v1/plans${query}`);
  const items = Array.isArray(payload) ? payload : payload.items || [];
  return items.map(planFromCommercePlan);
}

export function createMembershipSdk(api: ApiClient): BusinessSdk["membership"] {
  return {
    plans: () => listPlans(api)
  };
}
