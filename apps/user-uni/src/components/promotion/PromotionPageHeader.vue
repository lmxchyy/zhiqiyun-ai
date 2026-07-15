<template>
  <view class="promotion-page-header" :style="{ paddingTop: `${statusBarHeight}px` }">
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
import { ref } from "vue";
import loginLogo from "../../assets/zhiqiyun-logo-transparent.png";

withDefaults(defineProps<{ title: string; subtitle?: string; showBack?: boolean; fallback?: string }>(), { subtitle: "", showBack: true, fallback: "/pages/user/UserMinePage" });
const statusBarHeight = ref(20);
try { statusBarHeight.value = uni.getSystemInfoSync().statusBarHeight || 20; } catch { /* use design fallback */ }

function goBack() {
  const pages = getCurrentPages();
  if (pages.length > 1) { uni.navigateBack(); return; }
  uni.switchTab({ url: "/pages/user/UserMinePage", fail: () => uni.reLaunch({ url: "/pages/user/UserHomePage" }) });
}
</script>
