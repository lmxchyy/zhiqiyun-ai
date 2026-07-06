<template>
  <section class="ppt-generation-progress" :class="`is-${status}`" role="status" aria-live="polite">
    <div class="ppt-progress-head">
      <div>
        <strong>{{ statusText }}</strong>
        <span v-if="status === 'failed'">{{ errorMessage || "生成失败，请重试" }}</span>
        <span v-else>当前进度 {{ progress }}%，正在处理第 {{ currentPage || 0 }} / {{ slideCount }} 页</span>
      </div>
      <button v-if="status === 'failed'" type="button" title="重试生成PPT" aria-label="重试生成PPT" @click="$emit('retry')">重试</button>
      <i v-else class="ppt-progress-spinner" aria-hidden="true"></i>
    </div>
    <el-progress :percentage="progress" :stroke-width="8" />
  </section>
</template>

<script setup lang="ts">
import type { PptWorkflowStatus } from "../../types/ppt";

defineProps<{
  status: PptWorkflowStatus;
  statusText: string;
  progress: number;
  currentPage: number;
  slideCount: number;
  errorMessage?: string;
}>();

defineEmits<{
  retry: [];
}>();
</script>

<style scoped>
.ppt-generation-progress {
  display: grid;
  gap: 12px;
  padding: 16px;
  border: 1px solid #2b2b2b;
  border-radius: 10px;
  background: #0d0d0d;
}

.ppt-progress-head {
  display: flex;
  justify-content: space-between;
  gap: 12px;
}

.ppt-progress-head div {
  display: grid;
  gap: 5px;
}

.ppt-progress-head strong {
  color: #f4f4f5;
}

.ppt-progress-head span {
  color: #a1a1aa;
  font-size: 13px;
}

.ppt-progress-spinner {
  width: 22px;
  height: 22px;
  border: 2px solid #333;
  border-top-color: #f4f4f5;
  border-radius: 999px;
  animation: ppt-progress-spin 0.8s linear infinite;
}

.ppt-progress-head button {
  min-height: 34px;
  padding: 0 12px;
  border: 1px solid rgba(248, 113, 113, 0.35);
  border-radius: 8px;
  color: #fecaca;
  background: rgba(127, 29, 29, 0.28);
}

.ppt-generation-progress :deep(.el-progress-bar__outer) {
  background: #202020;
}

.ppt-generation-progress :deep(.el-progress-bar__inner) {
  background: #f4f4f5;
}

@keyframes ppt-progress-spin {
  to {
    transform: rotate(360deg);
  }
}
</style>
