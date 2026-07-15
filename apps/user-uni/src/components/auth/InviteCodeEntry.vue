<template>
  <button :class="['auth-invite-entry', status]" hover-class="auth-invite-hover" @click="$emit('click')">
    <template v-if="status === 'carried' || status === 'filled' || status === 'valid'">
      <text class="auth-invite-check">✓</text>
      <text class="auth-invite-main">{{ status === "carried" ? "已携带邀请码" : "邀请码已填写" }}</text>
      <text class="auth-invite-action">修改 ›</text>
    </template>
    <template v-else-if="status === 'resolving'">
      <view class="auth-invite-spinner" />
      <text class="auth-invite-main">正在校验邀请码</text>
    </template>
    <template v-else>
      <text class="auth-invite-main">有邀请码？</text>
      <text class="auth-invite-action">去填写 ›</text>
    </template>
  </button>
</template>

<script setup lang="ts">
import type { InviteStatus } from "../../features/auth/types";
withDefaults(defineProps<{ status?: InviteStatus }>(), { status: "empty" });
defineEmits<{ click: [] }>();
</script>

<style scoped>
.auth-invite-entry { width: 100%; min-height: 42px; margin: 0; padding: 0 12px; display: flex; align-items: center; border: 0; border-radius: 12px; color: #697085; background: #f2f7ff; font-size: 12px; line-height: 22px; text-align: left; }
.auth-invite-entry::after { display: none; }
.auth-invite-entry.empty, .auth-invite-entry.invalid, .auth-invite-entry.expired, .auth-invite-entry.disabled, .auth-invite-entry.agent_frozen { justify-content: space-between; }
.auth-invite-entry.carried, .auth-invite-entry.filled, .auth-invite-entry.valid { border: 1px solid #dbe3f5; color: #4a6bff; background: #f2f7ff; }
.auth-invite-check { margin-right: 7px; color: #18a06a; font-size: 16px; font-weight: 700; }
.auth-invite-main { flex: 1; }
.auth-invite-action { margin-left: auto; color: #4a6bff; font-weight: 500; }
.auth-invite-spinner { width: 14px; height: 14px; margin-right: 8px; box-sizing: border-box; border: 2px solid #cfd8ff; border-top-color: #4a6bff; border-radius: 50%; animation: spin .8s linear infinite; }
.auth-invite-hover { opacity: .72; }
@keyframes spin { to { transform: rotate(360deg); } }
</style>
