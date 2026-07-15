<template>
  <view class="auth-field-block">
    <text class="auth-field-label">密码</text>
    <view :class="['auth-password-shell', { focused, error: Boolean(error) }]">
      <input
        id="auth-password-input"
        class="auth-password-input"
        :password="!visible"
        :value="modelValue"
        maxlength="64"
        placeholder="请输入登录密码"
        confirm-type="done"
        @focus="focused = true"
        @blur="focused = false"
        @input="onInput"
        @confirm="$emit('confirm')"
      />
      <button class="auth-password-toggle" :aria-label="visible ? '隐藏密码' : '显示密码'" @click="visible = !visible">
        <text>{{ visible ? "◉" : "◎" }}</text>
      </button>
    </view>
    <text v-if="error" class="auth-field-error">{{ error }}</text>
  </view>
</template>

<script setup lang="ts">
import { ref } from "vue";

withDefaults(defineProps<{ modelValue: string; error?: string }>(), { error: "" });
const emit = defineEmits<{ "update:modelValue": [value: string]; confirm: [] }>();
const visible = ref(false);
const focused = ref(false);
function onInput(event: Event) {
  const detail = (event as unknown as { detail?: { value?: unknown } }).detail;
  emit("update:modelValue", String(detail?.value || ""));
}
</script>

<style scoped>
.auth-field-block { margin-bottom: 4px; }
.auth-field-label { display: block; margin-bottom: 6px; color: #181c28; font-size: 12px; line-height: 20px; font-weight: 500; }
.auth-password-shell { display: flex; align-items: center; height: 50px; box-sizing: border-box; overflow: hidden; border: 1px solid #e0e5f2; border-radius: 14px; background: #f2f7ff; }
.auth-password-shell.focused { border: 1.5px solid #4a6bff; background: #fff; }
.auth-password-shell.error { border-color: #eb404f; }
.auth-password-input { min-width: 0; height: 50px; flex: 1; padding-left: 15px; color: #181c28; font-size: 14px; line-height: 50px; }
.auth-password-toggle { width: 52px; height: 50px; margin: 0; padding: 0; border: 0; color: #697085; background: transparent; font-size: 18px; line-height: 50px; }
.auth-password-toggle::after { display: none; }
.auth-field-error { display: block; margin-top: 5px; color: #eb404f; font-size: 10px; line-height: 16px; }
</style>
