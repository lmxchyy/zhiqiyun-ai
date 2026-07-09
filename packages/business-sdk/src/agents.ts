import type { ApiClient } from "@xianzhi/api-client";
import type { ChannelCenterResponse } from "@xianzhi/shared-types";
import type { BusinessSdk } from "./types";

export function createAgentsSdk(api: ApiClient): BusinessSdk["agents"] {
  return {
    async list() {
      const center = await api.request<ChannelCenterResponse>("/api/v1/channel/me");
      return [
        {
          id: center.agent.id,
          title: center.agent.name || center.agent.inviteCode || center.agent.id,
          description: `L${center.agent.level} / ${center.agent.status}`,
          tags: [center.agent.inviteCode, center.agent.status].filter(Boolean),
          tone: "var(--color-primary)"
        },
        ...center.children.map(item => ({
          id: item.id,
          title: item.name || item.inviteCode || item.id,
          description: `L${item.level} / ${item.status}`,
          tags: [item.inviteCode, item.status].filter(Boolean),
          tone: "var(--color-accent)"
        }))
      ];
    },
    center: () => api.request<ChannelCenterResponse>("/api/v1/channel/me")
  };
}
