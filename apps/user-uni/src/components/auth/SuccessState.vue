<template>
  <LoginCard title="欢迎加入知启云AI" subtitle="账号已创建成功" mode="state" compact>
    <view class="auth-success-state">
      <view class="auth-success-icon">✓</view>
      <view v-if="benefitText" class="auth-benefit"><text class="auth-benefit-star">✦</text><text>{{ benefitText }}</text></view>
      <PrimaryLoginButton label="开始使用" @activate="$emit('start')" />
      <text class="auth-success-note">将返回登录前正在使用的功能</text>
    </view>
  </LoginCard>
</template>

<script setup lang="ts">
import LoginCard from "./LoginCard.vue";
import PrimaryLoginButton from "./PrimaryLoginButton.vue";
withDefaults(defineProps<{ benefitText?: string }>(), { benefitText: "新人体验权益已到账" });
defineEmits<{ start: [] }>();
</script>

<style scoped>
.auth-success-state { text-align: center; }
.auth-success-icon { width: 96px; height: 96px; margin: 3px auto 24px; border-radius: 50%; color: #18a06a; background: #e2fbf1; font-size: 42px; line-height: 96px; font-weight: 500; }
.auth-benefit { min-height: 64px; margin: 0 0 28px; padding: 0 20px; display: flex; align-items: center; justify-content: center; gap: 18px; box-sizing: border-box; border: 1px solid #cedcff; border-radius: 14px; color: #181c28; background: #f2f7ff; font-size: 14px; line-height: 22px; }
.auth-benefit-star { color: #4a6bff; font-size: 24px; }
.auth-success-note { display: block; margin-top: 17px; color: #8c94a8; font-size: 11px; line-height: 18px; }
</style>
