<template>
  <button
    :class="['auth-primary-button', state]"
    :disabled="disabled || loading"
    :open-type="openType || undefined"
    hover-class="auth-primary-button-pressed"
    @click="$emit('activate', $event)"
    @getphonenumber="$emit('getphonenumber', $event)"
  >
    <view v-if="loading" class="auth-button-spinner" />
    <text>{{ loading ? loadingText : label }}</text>
  </button>
</template>

<script setup lang="ts">
withDefaults(defineProps<{
  label: string;
  loading?: boolean;
  loadingText?: string;
  disabled?: boolean;
  state?: "default" | "success" | "error";
  openType?: string;
}>(), {
  loading: false,
  loadingText: "正在处理…",
  disabled: false,
  state: "default",
  openType: "",
});

defineEmits<{ activate: [event: unknown]; getphonenumber: [event: unknown] }>();
</script>

<style scoped>
.auth-primary-button {
  width: 100%; height: 50px; margin: 0; padding: 0 16px; box-sizing: border-box;
  display: flex; align-items: center; justify-content: center; gap: 10px;
  border: 0; border-radius: 15px; color: #fff; background: #4a6bff;
  box-shadow: 0 8px 18px rgba(46, 71, 204, 0.22); font-size: 15px; line-height: 24px; font-weight: 500;
}
.auth-primary-button::after { display: none; }
.auth-primary-button[disabled] { color: #9ca4b5; background: #d9deec; box-shadow: none; opacity: 1; }
.auth-primary-button.success { background: #18a06a; }
.auth-primary-button.error { background: #eb404f; }
.auth-primary-button-pressed { background: #3f5be0; opacity: 0.96; }
.auth-button-spinner { width: 16px; height: 16px; box-sizing: border-box; border: 2px solid rgba(255,255,255,.45); border-top-color: #fff; border-radius: 50%; animation: spin .8s linear infinite; }
@keyframes spin { to { transform: rotate(360deg); } }
@media (max-width: 340px) { .auth-primary-button { height: 46px; border-radius: 14px; font-size: 14px; } }
</style>
