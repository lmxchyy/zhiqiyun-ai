<template>
  <view class="ppt-creation-panel">
    <text class="v31-ppt-title">您今天想制作什么样的演示文稿？</text>
    <textarea
      :value="prompt"
      class="v31-ppt-input"
      maxlength="500"
      placeholder="请输入主题，例如：AI赋能企业营销增长方案"
      @input="emit('update:prompt', inputValue($event))"
    />
    <view class="v31-ppt-options">
      <button @click="emit('cycle-slides')">{{ slideCount }}张幻灯片</button>
      <button :class="{ active: dynamic }" @click="emit('toggle-dynamic')">{{ dynamic ? '动态的' : '静态的' }}</button>
      <button class="active" @click="emit('toggle-language')">{{ language === 'zh' ? '中文' : '英文' }}</button>
      <button @click="emit('cycle-model')">{{ model }}</button>
      <view
        :class="['v31-ppt-submit', { disabled: generationBusy }]"
        role="button"
        hover-class="v31-action-pressed"
        @click.stop="emit('generate')"
      >{{ generationBusy ? '…' : '→' }}</view>
    </view>
    <text v-if="creationError" class="v31-generation-error">{{ creationError }}</text>

    <text class="v31-section-title">示例主题</text>
    <view class="v31-example-grid">
      <button v-for="topic in topics" :key="topic" @click="emit('select-topic', topic)">{{ topic }}</button>
    </view>

    <CreationTaskStatusCard
      :task="latestTask"
      :status-label="statusLabel"
      :button-label="buttonLabel"
      :feedback-text="feedbackText"
      :has-progress="hasProgress"
      :progress-style="progressStyle"
      :show-pending-badge="showPendingBadge"
      :show-result-image="false"
      @preview="emit('preview-result')"
      @open="emit('open-result')"
      @retry="emit('retry')"
    />

    <view class="v31-draft-card">
      <text class="v31-draft-title">未完成项目会保留在最近浏览</text>
      <text class="v31-draft-copy">选择文本内容、自定义主题后，即使返回首页，也能继续从草稿进入。</text>
      <view class="v31-workflow-tags"><text>自动保存草稿</text><text>生成大纲</text><text>主题预览</text></view>
      <button class="v31-ppt-editor-entry" type="button" @click="emit('open-editor')">管理已有 PPT 与单页视觉</button>
    </view>
  </view>
</template>

<script setup lang="ts">
import CreationTaskStatusCard from "./CreationTaskStatusCard.vue";
import type { GenerationTask } from "../../../types";

interface Props {
  prompt: string;
  creationError: string;
  slideCount: number;
  dynamic: boolean;
  language: string;
  model: string;
  topics: string[];
  generationBusy: boolean;
  latestTask: GenerationTask | null;
  statusLabel: string;
  buttonLabel: string;
  feedbackText?: string;
  hasProgress?: boolean;
  progressStyle?: string | Record<string, string>;
  showPendingBadge?: boolean;
}

defineProps<Props>();

const emit = defineEmits<{
  (e: 'update:prompt', value: string): void;
  (e: 'cycle-slides'): void;
  (e: 'toggle-dynamic'): void;
  (e: 'toggle-language'): void;
  (e: 'cycle-model'): void;
  (e: 'generate'): void;
  (e: 'select-topic', topic: string): void;
  (e: 'open-editor'): void;
  (e: 'preview-result'): void;
  (e: 'open-result'): void;
  (e: 'retry'): void;
}>();

function inputValue(event: unknown) {
  const target = event as { detail?: { value?: string } } | { target?: { value?: string } };
  return target && typeof target === 'object'
    ? String((target as { detail?: { value?: string } }).detail?.value ?? (target as { target?: { value?: string } }).target?.value ?? "")
    : "";
}
</script>

<style scoped>
.ppt-creation-panel {
  display: flex;
  flex-direction: column;
}
</style>
