<template>
  <!-- #ifdef H5 -->
  <AiCreationPage />
  <!-- #endif -->
</template>

<script setup lang="ts">
import { onLaunch, onShow } from "@dcloudio/uni-app";
import AiCreationPage from "./pages/AiCreationPage.vue";
import { capturePromotionReferral, syncPendingPromotionReferral } from "./features/promotion/referral";

interface PromotionLaunchOptions { query?: Record<string, unknown>; referrerInfo?: { extraData?: Record<string, unknown> } }

function captureLaunch(options?: PromotionLaunchOptions) {
  const query = { ...(options?.query || {}), ...((options?.referrerInfo?.extraData || {}) as Record<string, unknown>) };
  capturePromotionReferral(query);
}

onLaunch(options => captureLaunch(options));
onShow(options => {
  captureLaunch(options);
  void syncPendingPromotionReferral();
});
</script>

<style>
@import "./styles/typography.css";
</style>
