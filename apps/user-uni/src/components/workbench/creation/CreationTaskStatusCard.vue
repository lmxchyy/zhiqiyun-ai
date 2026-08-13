<template>
  <view v-if="task" :class="['v31-generation-state', task.tone, wrapperClass]">
    <image
      v-if="showResultImage && task.resultUrl"
      class="v31-generation-result"
      :src="task.resultUrl"
      mode="aspectFill"
      @click="emit('preview')"
    />
    <view class="v31-generation-summary">
      <view class="v31-generation-title-row">
        <text class="v31-generation-title">{{ task.title }}</text>
        <text v-if="showPendingBadge" class="v31-live-badge">实时</text>
      </view>
      <text class="v31-generation-meta">任务 {{ task.id }} · {{ statusLabel }}</text>
      <view v-if="showPendingBadge" class="v31-generation-progress-track">
        <view :class="['v31-generation-progress-value', { indeterminate: !hasProgress } ]" :style="progressStyle" />
      </view>
      <text v-if="showPendingBadge" class="v31-generation-feedback">{{ feedbackText }}</text>
    </view>
    <button v-if="task.tone === 'success'" @click="emit('open')">{{ task.resultId ? '查看结果' : '查看作品' }}</button>
    <button v-else-if="task.tone === 'danger'" @click="emit('retry')">重新生成</button>
    <text v-else class="v31-generation-running">{{ buttonLabel }}</text>
  </view>
</template>

<script setup lang="ts">
import type { GenerationTask } from "../../../types";

interface Props {
  task: GenerationTask | null;
  statusLabel: string;
  buttonLabel: string;
  feedbackText?: string;
  hasProgress?: boolean;
  progressStyle?: string | Record<string, string>;
  showPendingBadge?: boolean;
  showResultImage?: boolean;
  wrapperClass?: string;
}

defineProps<Props>();

const emit = defineEmits<{
  (e: 'preview'): void;
  (e: 'open'): void;
  (e: 'retry'): void;
}>();
</script>

<style scoped>
.v31-generation-state { margin-top: 12px; }
</style>
