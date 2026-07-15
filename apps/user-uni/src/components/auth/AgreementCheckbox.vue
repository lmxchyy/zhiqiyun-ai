<template>
  <view :class="['auth-agreement', { highlight }]">
    <button :class="['auth-checkbox', { checked: modelValue }]" aria-label="同意用户协议和隐私政策" @click="$emit('update:modelValue', !modelValue)">
      <text v-if="modelValue">✓</text>
    </button>
    <view class="auth-agreement-copy">
      <text @click="$emit('update:modelValue', !modelValue)">我已阅读并同意</text>
      <text class="auth-agreement-link" @click.stop="$emit('open', 'user')">《用户协议》</text>
      <text>和</text>
      <text class="auth-agreement-link" @click.stop="$emit('open', 'privacy')">《隐私政策》</text>
    </view>
  </view>
</template>

<script setup lang="ts">
withDefaults(defineProps<{ modelValue: boolean; highlight?: boolean }>(), { highlight: false });
defineEmits<{ "update:modelValue": [value: boolean]; open: [type: "user" | "privacy"] }>();
</script>

<style scoped>
.auth-agreement { display: flex; align-items: flex-start; gap: 8px; padding: 4px 0; border-radius: 8px; transition: background 160ms ease; }
.auth-agreement.highlight { padding: 5px 6px; background: #fff1f2; animation: auth-pulse .7s ease 2; }
.auth-checkbox { width: 16px; height: 16px; min-width: 16px; margin: 2px 0 0; padding: 0; border: 1px solid #adb8d1; border-radius: 4px; color: #fff; background: #fff; font-size: 11px; line-height: 14px; }
.auth-checkbox::after { display: none; }
.auth-checkbox.checked { border-color: #4a6bff; background: #4a6bff; }
.auth-agreement-copy { display: flex; flex-wrap: wrap; color: #697085; font-size: 11px; line-height: 20px; }
.auth-agreement-link { color: #4a6bff; font-weight: 500; }
@keyframes auth-pulse { 50% { background: #ffe0e3; } }
@media (max-width: 340px) { .auth-agreement-copy { font-size: 10px; line-height: 14px; } }
</style>
