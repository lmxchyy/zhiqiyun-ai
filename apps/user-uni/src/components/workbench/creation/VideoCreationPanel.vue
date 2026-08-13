<template>
  <view class="video-creation-panel">
    <view class="video-safety-banner">
      <image class="video-safety-icon" src="/static/icons/check.svg" mode="aspectFit" />
      <text>请勿上传侵权、违法或含敏感信息的素材</text>
    </view>

    <view class="video-prompt-card">
      <view class="video-card-heading">
        <text class="video-card-title">描述想生成的视频</text>
      </view>
      <textarea
        :value="prompt"
        class="video-prompt-input"
        maxlength="1000"
        placeholder="例如：清晨的海边公路，一辆银色跑车高速驶过，电影感运镜，光影自然"
        @input="emit('update:prompt', inputValue($event))"
      />
      <text class="video-prompt-count">{{ promptLength }} / 1000</text>
    </view>

    <view
      class="video-reference-card"
      :class="{ 'is-uploaded': referencePaths.length, 'is-disabled': !supportsImageToVideo }"
      role="button"
      :aria-disabled="referenceSelecting || sourceLoading || !supportsImageToVideo"
      @click="handleReferenceClick"
    >
      <image
        v-if="referencePaths.length"
        class="video-reference-thumbnail"
        :src="referencePaths[0]"
        mode="aspectFill"
      />
      <image v-else class="video-reference-icon" src="/static/icons/assets.svg" mode="aspectFit" />
      <view class="video-reference-copy">
        <text class="video-reference-title">{{ referencePaths.length ? '参考图已上传' : '上传参考图（可选）' }}</text>
        <text class="video-reference-description">{{ referenceDescription }}</text>
      </view>
      <button
        v-if="referencePaths.length"
        class="video-reference-delete"
        type="button"
        aria-label="删除参考图"
        @click.stop="emit('remove-reference', 0)"
      >×</button>
      <image v-else class="video-row-chevron" src="/static/icons/chevron-right.svg" mode="aspectFit" />
    </view>

    <view v-if="videoModelOptions.length" class="video-model-row" @click="emit('open-model-sheet')">
      <text class="video-row-label">生成模型</text>
      <view class="video-row-value">
        <text>{{ modelDisplayName }}</text>
        <image class="video-row-chevron" src="/static/icons/chevron-right.svg" mode="aspectFit" />
      </view>
    </view>
    <view v-else class="video-model-row">
      <text class="video-row-label">生成模型</text>
      <text class="video-row-value">{{ modelSwitching ? '载入中...' : modelError || '暂无可用视频模型' }}</text>
    </view>

    <view class="video-basic-card">
      <view class="video-section-heading">
        <text class="video-section-title">基础参数</text>
        <text>选项随当前模型动态调整</text>
      </view>
      <view v-if="basicFields.length" class="video-basic-grid">
        <picker
          v-for="field in basicFields"
          :key="field.key"
          class="video-basic-picker"
          :range="field.options"
          :value="parameterOptionIndex(field)"
          @change="emit('select-parameter', { field, event: $event, section: 'basic' })"
        >
          <view class="video-basic-tile">
            <text class="video-basic-label">{{ fieldLabel(field) }}</text>
            <view class="video-basic-value">
              <text>{{ selectedLabel(field) }}</text>
              <image src="/static/icons/chevron-right.svg" mode="aspectFit" />
            </view>
          </view>
        </picker>
      </view>
      <text v-else class="video-parameter-empty">{{ modelError || '正在读取当前模型参数...' }}</text>
      <view v-if="audioField" class="video-audio-row">
        <view>
          <text class="video-audio-title">生成音频</text>
          <text class="video-audio-copy">为视频同步生成环境音</text>
        </view>
        <button
          type="button"
          :class="['video-audio-switch', { 'is-active': audioEnabled }]"
          :aria-label="`生成音频${audioLabel}`"
          @click="emit('toggle-audio')"
        ><text /></button>
      </view>
    </view>

    <view v-if="advancedFields.length" class="video-advanced-card">
      <button type="button" class="video-advanced-heading" @click="emit('toggle-advanced')">
        <text class="video-row-label">高级参数</text>
        <view class="video-row-value">
          <text>{{ advancedSummary }}</text>
          <image class="video-row-chevron" src="/static/icons/chevron-right.svg" mode="aspectFit" />
        </view>
      </button>
      <view v-if="advancedExpanded" class="video-advanced-options">
        <picker
          v-for="field in advancedFields"
          :key="field.key"
          class="video-advanced-picker"
          :range="field.options"
          :value="parameterOptionIndex(field)"
          @change="emit('select-parameter', { field, event: $event, section: 'advanced' })"
        >
          <view><text>{{ field.label }}</text><text>{{ selectedLabel(field) }}</text></view>
        </picker>
      </view>
    </view>

    <view class="video-generation-summary">
      <view>
        <text class="video-summary-main">{{ generationSummary }}</text>
        <text class="video-summary-copy">预计生成约 1–3 分钟 · 预计消耗以试算为准</text>
      </view>
      <text class="video-cost-pill">{{ costLabel }}</text>
    </view>

    <button
      type="button"
      :class="['video-primary-generate', { disabled: generationBusy }]"
      :disabled="generationBusy"
      @click.stop="emit('generate')"
    >
      <image src="/static/icons/create.svg" mode="aspectFit" />
      <text>{{ generateButtonLabel }}</text>
    </button>

    <text v-if="creationError" class="v31-generation-error video-generation-error">{{ creationError }}</text>

    <CreationTaskStatusCard
      :task="latestTask"
      :status-label="statusLabel"
      :button-label="buttonLabel"
      :feedback-text="feedbackText"
      :has-progress="hasProgress"
      :progress-style="progressStyle"
      :show-pending-badge="showPendingBadge"
      :show-result-image="true"
      @preview="emit('preview-result')"
      @open="emit('open-result')"
      @retry="emit('retry')"
    />
  </view>
</template>

<script setup lang="ts">
import CreationTaskStatusCard from "./CreationTaskStatusCard.vue";
import type { EditableVideoField, ModelInfo } from "@xianzhi/business-sdk";
import type { GenerationNotice } from "./types";

interface Props {
  prompt: string;
  promptLength: number;
  referencePaths: string[];
  referenceSelecting: boolean;
  sourceLoading: boolean;
  supportsImageToVideo: boolean;
  referenceDescription: string;
  videoModelOptions: ModelInfo[];
  modelDisplayName: string;
  modelSwitching: boolean;
  modelError: string;
  basicFields: EditableVideoField[];
  advancedFields: EditableVideoField[];
  parameterValues: Record<string, unknown>;
  advancedExpanded: boolean;
  audioField?: EditableVideoField | null;
  audioEnabled: boolean;
  audioLabel: string;
  advancedSummary: string;
  generationSummary: string;
  costLabel: string;
  generateButtonLabel: string;
  generationBusy: boolean;
  latestTask: GenerationNotice | null;
  statusLabel: string;
  buttonLabel: string;
  feedbackText?: string;
  hasProgress?: boolean;
  progressStyle?: string | Record<string, string>;
  showPendingBadge?: boolean;
  creationError: string;
}

const props = defineProps<Props>();

const emit = defineEmits<{
  (e: 'update:prompt', value: string): void;
  (e: 'choose-reference'): void;
  (e: 'remove-reference', index: number): void;
  (e: 'open-model-sheet'): void;
  (e: 'select-parameter', payload: { field: EditableVideoField; event: unknown; section: 'basic' | 'advanced' }): void;
  (e: 'toggle-audio'): void;
  (e: 'toggle-advanced'): void;
  (e: 'generate'): void;
  (e: 'preview-result'): void;
  (e: 'open-result'): void;
  (e: 'retry'): void;
}>();

function inputValue(event: unknown) {
  const target = event as { detail?: { value?: string } } | { target?: { value?: string } };
  return target && typeof target === "object"
    ? String((target as { detail?: { value?: string } }).detail?.value ?? (target as { target?: { value?: string } }).target?.value ?? "")
    : "";
}

function handleReferenceClick() {
  if (!props.referenceSelecting && !props.sourceLoading && props.supportsImageToVideo) {
    emit('choose-reference');
  }
}

function parameterOptionIndex(field: EditableVideoField) {
  const current = props.parameterValues[field.key];
  const index = field.options.findIndex(option => {
    if (typeof current === "number" || typeof option === "number") {
      return Number(current) === Number(option);
    }
    return current === option;
  });
  return current === undefined ? 0 : Math.max(0, index);
}

function fieldLabel(field: EditableVideoField) {
  if (field.key === "duration") return "时长";
  if (field.key === "aspect_ratio") return "画面比例";
  return field.label;
}

function selectedLabel(field: EditableVideoField) {
  const value = props.parameterValues[field.key];
  const translations: Record<string, Record<string, string>> = {
    motion_strength: { low: "低", medium: "中", high: "高" },
    camera_movement: { static: "固定", pan: "平移", push: "推进", pull: "拉远" },
  };
  const translated = translations[field.key]?.[String(value)];
  if (translated) return translated;
  if (field.key === "fps" && value !== undefined) return `${String(value)} FPS`;
  return value === undefined ? "-" : `${String(value)}${field.unit || ""}`;
}
</script>

<style scoped>
.video-creation-panel {
  display: flex;
  flex-direction: column;
}
</style>
