<template>
  <view :id="id" :class="['auth-login-card', `mode-${mode}`, { compact }]">
    <text class="auth-card-title">{{ title }}</text>
    <text v-if="subtitle" class="auth-card-subtitle">{{ subtitle }}</text>
    <view class="auth-card-content"><slot /></view>
  </view>
</template>

<script setup lang="ts">
withDefaults(defineProps<{ id?: string; title: string; subtitle?: string; mode?: string; compact?: boolean }>(), {
  id: "auth-login-card",
  subtitle: "",
  mode: "default",
  compact: false,
});
</script>

<style scoped>
.auth-login-card {
  position: relative;
  z-index: 1;
  width: 100%;
  max-width: 335px;
  margin: 48px auto 0;
  padding: 24px 20px 28px;
  box-sizing: border-box;
  border: 1px solid #e8ebf5;
  border-radius: 24px;
  background: #fff;
  box-shadow: 0 10px 28px rgba(26, 36, 64, 0.08);
}
.auth-card-title { display: block; color: #181c28; font-size: 20px; line-height: 30px; font-weight: 700; text-align: center; }
.auth-card-subtitle { display: block; margin-top: 6px; color: #697085; font-size: 12px; line-height: 20px; text-align: center; }
.auth-card-content { margin-top: 19px; }
.auth-login-card.mode-sms, .auth-login-card.mode-password { margin-top: 32px; }
.auth-login-card.compact { padding-bottom: 22px; }
@media (max-width: 340px) {
  .auth-login-card { max-width: 288px; margin-top: 34px; padding: 21px 15px 24px; border-radius: 22px; }
  .auth-login-card.mode-sms, .auth-login-card.mode-password { margin-top: 24px; }
  .auth-card-title { font-size: 18px; line-height: 28px; }
  .auth-card-subtitle { font-size: 11px; }
  .auth-card-content { margin-top: 16px; }
}
</style>
