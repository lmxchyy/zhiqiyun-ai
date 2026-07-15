<template>
  <view class="auth-field-block">
    <text class="auth-field-label">验证码</text>
    <view :class="['auth-code-shell', { focused, error: Boolean(error) }]">
      <input
        id="auth-code-input"
        class="auth-code-input"
        type="number"
        inputmode="numeric"
        :value="modelValue"
        :maxlength="6"
        placeholder="请输入验证码"
        confirm-type="done"
        @focus="focused = true"
        @blur="focused = false"
        @input="onInput"
        @confirm="$emit('confirm')"
      />
      <button class="auth-code-action" :disabled="disabled" hover-class="auth-code-hover" @click="$emit('send')">
        {{ actionLabel }}
      </button>
    </view>
    <text v-if="error" class="auth-field-error">{{ error }}</text>
  </view>
</template>

<script setup lang="ts">
import { ref } from "vue";

withDefaults(defineProps<{ modelValue: string; actionLabel: string; disabled?: boolean; error?: string }>(), {
  disabled: false,
  error: "",
});
const emit = defineEmits<{ "update:modelValue": [value: string]; send: []; confirm: [] }>();
const focused = ref(false);
function onInput(event: Event) {
  const detail = (event as unknown as { detail?: { value?: unknown } }).detail;
  emit("update:modelValue", String(detail?.value || "").replace(/\D/g, "").slice(0, 6));
}
</script>

<style scoped>
.auth-field-block { margin-bottom: 14px; }
.auth-field-label { display: block; margin-bottom: 6px; color: #181c28; font-size: 12px; line-height: 20px; font-weight: 500; }
.auth-code-shell { display: flex; align-items: center; height: 50px; box-sizing: border-box; overflow: hidden; border: 1px solid #e0e5f2; border-radius: 14px; background: #f2f7ff; }
.auth-code-shell.focused { border: 1.5px solid #4a6bff; background: #fff; }
.auth-code-shell.error { border-color: #eb404f; }
.auth-code-input { min-width: 0; height: 50px; flex: 1; padding-left: 15px; color: #181c28; font-size: 14px; line-height: 50px; }
.auth-code-action { width: 104px; height: 50px; margin: 0; padding: 0 12px; border: 0; color: #4a6bff; background: transparent; font-size: 12px; line-height: 50px; font-weight: 500; }
.auth-code-action::after { display: none; }
.auth-code-action[disabled] { color: #8c94a8; opacity: 1; }
.auth-code-hover { opacity: .7; }
.auth-field-error { display: block; margin-top: 5px; color: #eb404f; font-size: 10px; line-height: 16px; }
</style>
