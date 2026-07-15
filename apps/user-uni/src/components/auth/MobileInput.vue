<template>
  <view class="auth-field-block">
    <text class="auth-field-label">{{ label }}</text>
    <view :class="['auth-input-shell', { focused, error: Boolean(error) }]">
      <input
        id="auth-mobile-input"
        class="auth-input"
        type="number"
        inputmode="numeric"
        :value="modelValue"
        :maxlength="11"
        :placeholder="placeholder"
        confirm-type="next"
        @focus="focused = true"
        @blur="focused = false"
        @input="onInput"
      />
    </view>
    <text v-if="error" class="auth-field-error">{{ error }}</text>
  </view>
</template>

<script setup lang="ts">
import { ref } from "vue";

withDefaults(defineProps<{ modelValue: string; label?: string; placeholder?: string; error?: string }>(), {
  label: "手机号",
  placeholder: "请输入手机号",
  error: "",
});
const emit = defineEmits<{ "update:modelValue": [value: string] }>();
const focused = ref(false);
function onInput(event: Event) {
  const detail = (event as unknown as { detail?: { value?: unknown } }).detail;
  emit("update:modelValue", String(detail?.value || "").replace(/\D/g, "").slice(0, 11));
}
</script>

<style scoped>
.auth-field-block { margin-bottom: 12px; }
.auth-field-label { display: block; margin-bottom: 6px; color: #181c28; font-size: 12px; line-height: 20px; font-weight: 500; }
.auth-input-shell { height: 50px; box-sizing: border-box; overflow: hidden; border: 1px solid #e0e5f2; border-radius: 14px; background: #f2f7ff; }
.auth-input-shell.focused { border: 1.5px solid #4a6bff; background: #fff; }
.auth-input-shell.error { border-color: #eb404f; }
.auth-input { width: 100%; height: 50px; padding: 0 15px; box-sizing: border-box; color: #181c28; font-size: 14px; line-height: 50px; }
.auth-field-error { display: block; margin-top: 5px; color: #eb404f; font-size: 10px; line-height: 16px; }
</style>
