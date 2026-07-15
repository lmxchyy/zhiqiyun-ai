<template>
  <view v-if="visible" class="auth-loading-layer" @touchmove.stop.prevent>
    <view class="auth-loading-overlay" />
    <view class="auth-loading-card">
      <view class="auth-loading-spinner"><view /></view>
      <text class="auth-loading-title">{{ title }}</text>
      <text class="auth-loading-copy">请稍候，不要重复点击</text>
    </view>
  </view>
</template>

<script setup lang="ts">
import { computed } from "vue";
import type { LoadingStep } from "../../features/auth/types";
const props = defineProps<{ visible: boolean; step: LoadingStep }>();
const labels: Record<LoadingStep, string> = {
  authorizing: "正在获取授权…",
  validating: "正在验证账号…",
  logging_in: "正在登录…",
  registering: "正在创建账号…",
  entering: "正在进入知启云AI…",
};
const title = computed(() => labels[props.step]);
</script>

<style scoped>
.auth-loading-layer { position: fixed; z-index: 120; inset: 0; display: flex; align-items: center; justify-content: center; }
.auth-loading-overlay { position: absolute; inset: 0; background: rgba(10, 15, 28, .38); }
.auth-loading-card { position: relative; width: 245px; padding: 34px 22px 32px; box-sizing: border-box; border-radius: 22px; background: #fff; box-shadow: 0 16px 36px rgba(20,31,61,.18); text-align: center; }
.auth-loading-spinner { position: relative; width: 58px; height: 58px; margin: 0 auto 20px; box-sizing: border-box; border: 6px solid #dbe3ff; border-top-color: #4a6bff; border-radius: 50%; animation: auth-spin .9s linear infinite; }
.auth-loading-spinner view { position: absolute; top: -6px; right: 2px; width: 14px; height: 14px; border-radius: 50%; background: #4a6bff; }
.auth-loading-title, .auth-loading-copy { display: block; }
.auth-loading-title { color: #181c28; font-size: 19px; line-height: 30px; font-weight: 700; }
.auth-loading-copy { margin-top: 9px; color: #697085; font-size: 12px; line-height: 20px; }
@keyframes auth-spin { to { transform: rotate(360deg); } }
</style>
