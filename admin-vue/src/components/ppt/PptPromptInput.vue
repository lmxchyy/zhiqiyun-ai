<template>
  <label class="ppt-prompt-input">
    <textarea
      :value="modelValue"
      maxlength="500"
      aria-describedby="ppt-prompt-input-counter"
      title="PPT主题提示词"
      aria-label="PPT主题提示词"
      placeholder="请输入你想生成的PPT主题，例如：AI赋能企业营销增长方案"
      spellcheck="true"
      @input="$emit('update:modelValue', ($event.target as HTMLTextAreaElement).value)"
      @keydown.ctrl.enter.prevent="$emit('submit')"
      @keydown.meta.enter.prevent="$emit('submit')"
    ></textarea>
    <footer aria-live="polite">
      <span id="ppt-prompt-input-counter">{{ modelValue.length }}/500</span>
      <button v-if="modelValue" type="button" title="清空提示词" aria-label="清空提示词" @click="$emit('clear')">清空</button>
    </footer>
  </label>
</template>

<script setup lang="ts">
defineProps<{
  modelValue: string;
}>();

defineEmits<{
  "update:modelValue": [value: string];
  clear: [];
  submit: [];
}>();
</script>

<style scoped>
.ppt-prompt-input {
  display: grid;
  gap: 8px;
}

.ppt-prompt-input textarea {
  min-height: 176px;
  width: 100%;
  box-sizing: border-box;
  padding: 20px;
  border: 1px solid #242424;
  border-radius: 12px;
  outline: 0;
  resize: vertical;
  color: #f8fafc;
  caret-color: #fff;
  background: #0a0a0a;
  font-size: 16px;
  line-height: 1.7;
  transition: border-color 0.16s ease, box-shadow 0.16s ease, background-color 0.16s ease;
}

.ppt-prompt-input textarea:focus,
.ppt-prompt-input textarea:focus-visible {
  border-color: #444;
  box-shadow: 0 0 0 3px rgba(125, 141, 246, 0.16);
}

.ppt-prompt-input textarea::placeholder {
  color: #8d8d93;
}

.ppt-prompt-input footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  color: #85858b;
  font-size: 12px;
}

.ppt-prompt-input button {
  min-height: 24px;
  padding: 0 6px;
  border: 0;
  border-radius: 6px;
  color: #d4d4d8;
  background: transparent;
  cursor: pointer;
  transition: color 0.16s ease, background-color 0.16s ease;
}

.ppt-prompt-input button:hover,
.ppt-prompt-input button:focus-visible {
  color: #ffffff;
  background: rgba(255, 255, 255, 0.08);
  outline: 0;
}
</style>
