<template>
  <!-- #ifdef H5 -->
  <EnterpriseFeishuConnectorPage v-if="isFeishuConnectorRoute" />
  <AiCreationPage v-else />
  <!-- #endif -->
</template>

<script setup lang="ts">
import { onLaunch, onShow } from "@dcloudio/uni-app";
// #ifdef H5
import AiCreationPage from "./pages/AiCreationPage.vue";
import EnterpriseFeishuConnectorPage from "./pages/enterprise/EnterpriseFeishuConnectorPage.vue";
// #endif
import { capturePromotionReferral, syncPendingPromotionReferral } from "./features/promotion/referral";
import { initializeAuth } from "./features/auth/gate";
import { initializeReviewMode } from "./features/reviewMode";
import { recordAgentInviteAppActivation } from "./features/invite/activation";
import { syncMiniProgramNavigation } from "./composables/useMiniProgramNavigation";

const isFeishuConnectorRoute = typeof window !== "undefined" && [
  "/mobile/enterprise/feishu",
  "/pages/enterprise/EnterpriseFeishuConnectorPage",
].includes(window.location.pathname);

interface PromotionLaunchOptions { query?: Record<string, unknown>; referrerInfo?: { extraData?: Record<string, unknown> } }

function captureLaunch(options?: PromotionLaunchOptions) {
  const query = { ...(options?.query || {}), ...((options?.referrerInfo?.extraData || {}) as Record<string, unknown>) };
  capturePromotionReferral(query);
}

onLaunch(options => {
  syncMiniProgramNavigation();
  initializeAuth();
  void initializeReviewMode();
  // #ifdef APP-PLUS
  void recordAgentInviteAppActivation();
  // #endif
  captureLaunch(options);
});
onShow(options => {
  syncMiniProgramNavigation();
  captureLaunch(options);
  void syncPendingPromotionReferral();
  // #ifdef APP-PLUS
  void recordAgentInviteAppActivation();
  // #endif
});
</script>

<style>
@import "./styles/typography.css";
</style>
