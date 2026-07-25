<template>
  <view class="promotion-page-header" :style="navigationStyle">
    <view class="promotion-header-row">
      <button v-if="showBack" class="promotion-header-back" aria-label="返回" @click="goBack"><text>‹</text></button>
      <image class="promotion-header-logo" :src="loginLogo" mode="aspectFit" />
      <view class="promotion-header-copy">
        <text class="promotion-header-title">{{ title }}</text>
        <text v-if="subtitle" class="promotion-header-subtitle">{{ subtitle }}</text>
      </view>
      <slot name="action" />
    </view>
  </view>
</template>

<script setup lang="ts">
import loginLogo from "../../assets/zhiqiyun-logo-transparent.png";
import { useMiniProgramNavigation } from "../../composables/useMiniProgramNavigation";

withDefaults(defineProps<{ title: string; subtitle?: string; showBack?: boolean; fallback?: string }>(), { subtitle: "", showBack: true, fallback: "/pages/user/UserMinePage" });
const { navigationStyle } = useMiniProgramNavigation();

function goBack() {
  const pages = getCurrentPages();
  if (pages.length > 1) { uni.navigateBack(); return; }
  uni.switchTab({ url: "/pages/user/UserMinePage", fail: () => uni.reLaunch({ url: "/pages/user/UserHomePage" }) });
}
</script>

<style scoped>
.promotion-page-header {
  box-sizing: border-box;
  padding-top: var(--header-padding-top);
  padding-right: max(18px, var(--capsule-right-space));
  padding-left: 18px;
}

.promotion-header-row { min-height: var(--navigation-bar-height); }
</style>
