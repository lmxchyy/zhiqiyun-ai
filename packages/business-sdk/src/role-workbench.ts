import type { ApiClient } from "@xianzhi/api-client";
import type { Asset, ChannelCenterResponse } from "@xianzhi/shared-types";
import type { BusinessSdk, ItemsResponse, MemberProfileResponse, OperationProfileResponse, RoleWalletResponse } from "./types";

function normalizeAssets(payload: Asset[] | { items?: Asset[] }) {
  return Array.isArray(payload) ? payload : payload.items || [];
}

export function createRoleWorkbenchSdk(api: ApiClient): BusinessSdk["roleWorkbench"] {
  return {
    memberProfile: () => api.request<MemberProfileResponse>("/api/v1/member/profile"),
    wallet: () => api.request<RoleWalletResponse>("/api/v1/member/wallet"),
    pointsAccount: () => api.request<RoleWalletResponse>("/api/v1/points/account"),
    async recentAssets(limit = 8) {
      const payload = await api.request<Asset[] | { items?: Asset[] }>("/api/v1/assets");
      return normalizeAssets(payload).slice(0, limit);
    },
    channelCenter: () => api.request<ChannelCenterResponse>("/api/v1/channel/me"),
    operationProfile: () => api.request<OperationProfileResponse>("/api/v1/operation-center/profile"),
    operationAgents: () => api.request<ItemsResponse>("/api/v1/operation-center/agents"),
    operationOrders: () => api.request<ItemsResponse>("/api/v1/operation-center/orders"),
    operationCommissions: () => api.request<ItemsResponse>("/api/v1/operation-center/commissions")
  };
}
