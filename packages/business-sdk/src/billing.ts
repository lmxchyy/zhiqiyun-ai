import type { ApiClient } from "@xianzhi/api-client";
import { listPlans } from "./membership";
import type { BusinessSdk } from "./types";

export function createBillingSdk(api: ApiClient): BusinessSdk["billing"] {
  return {
    wallet: () => api.request("/api/v1/member/wallet"),
    pointsAccount: () => api.request("/api/v1/points/account"),
    plans: (planType) => listPlans(api, planType),
    createOrder: (input) => api.request("/api/v1/orders/create", { method: "POST", body: input }),
    createRechargeOrder: (input) => api.request("/api/v1/points/recharge-orders", { method: "POST", body: input }),
    createSubscriptionOrder: (input) => api.request("/api/v1/points/subscription-orders", { method: "POST", body: input }),
    createAgentJoinOrder: (input) => api.request("/api/v1/agent/join-order", { method: "POST", body: input }),
    createOperationCenterJoinOrder: (input) => api.request("/api/v1/operation-center/join-order", { method: "POST", body: input })
  };
}
