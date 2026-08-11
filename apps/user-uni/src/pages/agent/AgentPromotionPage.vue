<template>
  <view class="agent-promotion-shell">
    <PromotionCenterScreen
      ref="screenRef"
      :show-back="false"
      header-fallback="/pages/agent/AgentOverviewPage"
    />
    <V531TabBar role="agent" active="promotion" @change="onAgentTab" />
  </view>
</template>

<script setup lang="ts">
import { ref } from "vue";
import { onShow } from "@dcloudio/uni-app";
import PromotionCenterScreen from "../../components/promotion/PromotionCenterScreen.vue";
import V531TabBar from "../../components/v531/V531TabBar.vue";
import { rolePage, type MiniProgramTabId } from "../../config/miniProgramPages";

const screenRef = ref<{ load?: (force: boolean) => Promise<void> | void } | null>(null);

onShow(() => {
  void screenRef.value?.load?.(false);
});

function onAgentTab(tab: MiniProgramTabId) {
  if (tab === "promotion") return;
  uni.reLaunch({
    url: rolePage("agent", tab),
    fail: () => uni.redirectTo({ url: rolePage("agent", tab) }),
  });
}
</script>

<style>
page { background: #f7f8fc; }
.agent-promotion-shell {
  min-height: 100vh;
  box-sizing: border-box;
  padding-bottom: calc(112px + env(safe-area-inset-bottom));
}
</style>
