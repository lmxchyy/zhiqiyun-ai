<template>
  <LoginCard :title="content.title" :subtitle="content.subtitle" mode="state" compact>
    <view class="auth-error-state">
      <view :class="['auth-state-icon', kind]">{{ content.icon }}</view>
      <view v-if="content.note" class="auth-state-note"><text>{{ content.note }}</text></view>
      <PrimaryLoginButton :label="content.primary" @activate="$emit('primary')" />
      <SecondaryLoginEntry :label="content.secondary" @activate="$emit('secondary')" />
    </view>
  </LoginCard>
</template>

<script setup lang="ts">
import { computed } from "vue";
import type { LoginErrorState } from "../../features/auth/types";
import LoginCard from "./LoginCard.vue";
import PrimaryLoginButton from "./PrimaryLoginButton.vue";
import SecondaryLoginEntry from "./SecondaryLoginEntry.vue";

const props = defineProps<{ kind: LoginErrorState }>();
defineEmits<{ primary: []; secondary: [] }>();
const content = computed(() => {
  const values: Record<LoginErrorState, { icon: string; title: string; subtitle: string; note: string; primary: string; secondary: string }> = {
    network: { icon: "⌁", title: "网络连接异常", subtitle: "请检查网络后重试，已填写的信息会保留", note: "", primary: "重新加载", secondary: "返回验证码登录" },
    frozen: { icon: "锁", title: "账号暂时无法使用", subtitle: "该账号已被冻结，请联系平台客服确认原因", note: "账号中的作品、余额和企业数据将继续保留", primary: "联系客服", secondary: "返回其他登录方式" },
    deactivated: { icon: "!", title: "账号已注销", subtitle: "当前账号已注销，如有疑问请联系平台客服", note: "", primary: "联系客服", secondary: "返回其他登录方式" },
    maintenance: { icon: "⚙", title: "系统维护中", subtitle: "服务暂时不可用，请稍后再试", note: "", primary: "重新加载", secondary: "返回验证码登录" },
    timeout: { icon: "!", title: "登录请求超时", subtitle: "网络响应较慢，已填写的信息会保留", note: "", primary: "重新登录", secondary: "返回验证码登录" },
    token: { icon: "!", title: "登录信息保存失败", subtitle: "账号已验证，但本地会话未能安全保存", note: "", primary: "重新登录", secondary: "返回其他登录方式" },
    profile: { icon: "!", title: "用户信息获取失败", subtitle: "请重新登录以同步账号信息", note: "", primary: "重新登录", secondary: "返回其他登录方式" },
  };
  return values[props.kind];
});
</script>

<style scoped>
.auth-error-state { text-align: center; }
.auth-state-icon { width: 96px; height: 96px; margin: 0 auto 25px; border-radius: 50%; color: #4a6bff; background: #edf2ff; font-size: 34px; line-height: 96px; font-weight: 700; }
.auth-state-icon.frozen, .auth-state-icon.deactivated { color: #8c5a00; background: #fff0e2; font-size: 25px; }
.auth-state-note { margin: 24px 0 20px; padding: 15px; border: 1px solid #e0e5f2; border-radius: 14px; color: #697085; background: #f7f9fd; font-size: 12px; line-height: 24px; }
.auth-error-state :deep(.auth-secondary-entry) { margin-top: 7px; }
</style>
