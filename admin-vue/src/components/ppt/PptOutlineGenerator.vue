<template>
  <section class="ppt-outline-generator">
    <div>
      <strong>大纲优先流程</strong>
      <span>先生成可编辑大纲，确认后再生成完整 PPT。</span>
    </div>
    <button
      type="button"
      :disabled="!canGenerate"
      :aria-busy="status === 'outlining'"
      :aria-label="status === 'outlining' ? '正在生成大纲' : '生成演示大纲'"
      :title="status === 'outlining' ? '正在生成大纲，请稍候' : '生成演示大纲'"
      @click="$emit('generate')"
    >
      <span v-if="status === 'outlining'" class="ppt-outline-generator-spinner" aria-hidden="true"></span>
      <svg v-else viewBox="0 0 24 24" aria-hidden="true">
        <path d="m21.64 3.64-1.28-1.28a1.21 1.21 0 0 0-1.72 0L2.36 18.64a1.21 1.21 0 0 0 0 1.72l1.28 1.28a1.2 1.2 0 0 0 1.72 0L21.64 5.36a1.2 1.2 0 0 0 0-1.72" />
        <path d="m14 7 3 3" />
        <path d="M5 6v4" />
        <path d="M19 14v4" />
        <path d="M10 2v2" />
        <path d="M7 8H3" />
        <path d="M21 16h-4" />
        <path d="M11 3H9" />
      </svg>
      <span>{{ status === "outlining" ? "正在生成大纲..." : "生成大纲" }}</span>
    </button>
  </section>
</template>

<script setup lang="ts">
import type { PptWorkflowStatus } from "../../types/ppt";

defineProps<{
  canGenerate: boolean;
  status: PptWorkflowStatus;
}>();

defineEmits<{
  generate: [];
}>();
</script>

<style scoped>
.ppt-outline-generator {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 14px;
  padding: 16px;
  border: 1px solid #262626;
  border-radius: 10px;
  background: #0d0d0d;
}

.ppt-outline-generator div {
  display: grid;
  gap: 5px;
}

.ppt-outline-generator strong {
  color: #f4f4f5;
}

.ppt-outline-generator span {
  color: #a1a1aa;
  font-size: 13px;
}

.ppt-outline-generator button {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  min-height: 40px;
  padding: 0 18px;
  border: 0;
  border-radius: 999px;
  color: #111;
  background: #f4f4f5;
  font-weight: 820;
  cursor: pointer;
}

.ppt-outline-generator button:focus-visible {
  outline: 2px solid rgba(34, 211, 238, 0.72);
  outline-offset: 2px;
}

.ppt-outline-generator button:disabled {
  opacity: 0.45;
  cursor: not-allowed;
}

.ppt-outline-generator svg {
  width: 16px;
  height: 16px;
  fill: none;
  stroke: currentColor;
  stroke-width: 2;
  stroke-linecap: round;
  stroke-linejoin: round;
}

.ppt-outline-generator-spinner {
  width: 15px;
  height: 15px;
  border: 2px solid rgba(17, 17, 17, 0.22);
  border-top-color: #111;
  border-radius: 999px;
  animation: ppt-outline-generator-spin 0.8s linear infinite;
}

@keyframes ppt-outline-generator-spin {
  to {
    transform: rotate(360deg);
  }
}
</style>
