import type { ReviewModeConfig } from "@xianzhi/shared-auth";
import { reactive, readonly } from "vue";
import { apiClient } from "../api/client";

const defaults: ReviewModeConfig = {
  enabled: false,
  hideRecharge: false,
  hideWallet: false,
  hideInvite: false,
  hideCommission: false,
  hideAgentCenter: false,
  hideOperatorCenter: false,
  hideSensitiveMarketing: false,
};

const config = reactive<ReviewModeConfig>({ ...defaults });
let loading: Promise<ReviewModeConfig> | null = null;

export const reviewMode = readonly(config);

export function initializeReviewMode(): Promise<ReviewModeConfig> {
  if (loading) return loading;
  const request = apiClient.request<ReviewModeConfig>("/api/v1/app/review-mode", { auth: "none" })
    .then((remote: ReviewModeConfig) => {
      Object.assign(config, defaults, remote);
      return { ...config };
    })
    .catch(() => ({ ...config }))
    .finally(() => {
      loading = null;
    });
  loading = request;
  return request;
}

export function reviewModeHides(flag: keyof Omit<ReviewModeConfig, "enabled">): boolean {
  return config.enabled && config[flag];
}
