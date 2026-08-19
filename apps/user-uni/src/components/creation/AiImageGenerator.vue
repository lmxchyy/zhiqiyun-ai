<template>
  <view class="ai-image-generator" :style="navigationStyle">
    <view class="ai-image-generator__header">
      <button
        class="ai-image-generator__icon-button"
        type="button"
        aria-label="返回"
        hover-class="ai-image-generator__icon-button--pressed"
        @click='emit("back")'
      >
        <text>‹</text>
      </button>
      <text class="ai-image-generator__title">AI生图</text>
      <button
        class="ai-image-generator__icon-button"
        type="button"
        aria-label="使用帮助"
        hover-class="ai-image-generator__icon-button--pressed"
        @click='emit("help")'
      >
        <text>?</text>
      </button>
    </view>

    <scroll-view class="ai-image-generator__scroll" scroll-y :show-scrollbar="false">
      <view class="ai-image-generator__content">
        <view class="ai-image-generator__helper">
          <text class="ai-image-generator__helper-icon" aria-hidden="true">✦</text>
          <text>输入想法，AI 将为你生成高质量图片</text>
        </view>

        <text class="ai-image-generator__heading">今天想生成什么？</text>
        <view class="ai-image-generator__prompt-card">
          <view class="ai-image-generator__references" aria-label="参考图片">
            <view
              v-for="(referenceImage, index) in referenceImages"
              :key="`${referenceImage}-${index}`"
              class="ai-image-generator__reference"
            >
              <button
                class="ai-image-generator__reference-preview"
                type="button"
                :aria-label="`预览第 ${index + 1} 张参考图`"
                hover-class="ai-image-generator__reference--pressed"
                @click='emit("preview-reference", index)'
              >
                <image :src="referenceImage" mode="aspectFill" />
              </button>
              <button
                class="ai-image-generator__reference-remove"
                type="button"
                :aria-label="`移除第 ${index + 1} 张参考图`"
                @click.stop='emit("remove-reference", index)'
              ><text class="ai-image-generator__reference-remove-glyph" aria-hidden="true">×</text></button>
            </view>
            <button
              class="ai-image-generator__reference-add"
              type="button"
              :disabled="referenceImages.length >= referenceLimit || selectingReference"
              :aria-label="selectingReference ? '正在选择参考图' : '添加参考图'"
              hover-class="ai-image-generator__reference-add--pressed"
              @click='emit("choose-reference")'
            >
              <template v-if="selectingReference">
                <text class="ai-image-generator__reference-loading" aria-hidden="true" />
                <text>选择中…</text>
              </template>
              <template v-else>
                <text class="ai-image-generator__reference-plus">＋</text>
                <text>添加参考</text>
              </template>
              <text>{{ referenceImages.length }}/{{ referenceLimit }}</text>
            </button>
          </view>

          <textarea
            class="ai-image-generator__textarea"
            :value="prompt"
            maxlength="500"
            placeholder="例如：生成一张水果店开业促销海报，橙色系，高级感"
            aria-label="图片描述"
            @input="onPromptInput"
          />
          <view class="ai-image-generator__prompt-footer">
            <text class="ai-image-generator__counter">{{ prompt.length }}/500</text>
            <button
              class="ai-image-generator__optimize"
              type="button"
              :disabled="!prompt.trim() || busy"
              hover-class="ai-image-generator__optimize--pressed"
              @click='emit("optimize")'
            >
              <text>✦ 帮我优化</text>
            </button>
          </view>
        </view>

        <view class="ai-image-generator__settings">
          <text class="ai-image-generator__settings-title">生成设置</text>

          <view
            :class="['ai-image-generator__schema-status', `is-${schemaStatus}`]"
            role="status"
            aria-live="polite"
          >
            <text v-if="schemaStatus === 'loading'" class="ai-image-generator__schema-spinner" aria-hidden="true" />
            <text>{{ schemaMessage || schemaStatusLabel }}</text>
          </view>

          <text class="ai-image-generator__label">画面比例</text>
          <view class="ai-image-generator__ratio-list">
            <button
              v-for="ratio in ratioOptions"
              :key="ratio.value"
              :class="['ai-image-generator__ratio-chip', { 'is-selected': selectedRatio === ratio.value }]"
              type="button"
              :aria-pressed="selectedRatio === ratio.value"
              hover-class="ai-image-generator__ratio-chip--pressed"
              @click='emit("update:selectedRatio", ratio.value)'
            >
              <text v-if="ratio.value !== 'auto'" class="ai-image-generator__ratio-shape" :style="ratioShapeStyle(ratio.value)" />
              <text>{{ ratio.label }}</text>
              <text v-if="selectedRatio === ratio.value" class="ai-image-generator__check" aria-hidden="true">✓</text>
            </button>
          </view>

          <template v-if="selectedRatio !== 'auto' && tierOptions.length > 0">
            <text class="ai-image-generator__label">图片清晰度</text>
            <view class="ai-image-generator__tier-list" role="group" aria-label="图片清晰度">
              <button
                v-for="tier in tierOptions"
                :key="tier.value"
                :class="['ai-image-generator__tier-chip', { 'is-selected': selectedTier === tier.value }]"
                type="button"
                :aria-pressed="selectedTier === tier.value"
                hover-class="ai-image-generator__tier-chip--pressed"
                @click='emit("update:selectedTier", tier.value)'
              >
                <text>{{ tier.label }}</text>
                <text v-if="selectedTier === tier.value" class="ai-image-generator__check" aria-hidden="true">✓</text>
              </button>
            </view>
          </template>

          <text v-if="size && size !== 'auto'" class="ai-image-generator__output-hint">当前输出 {{ size }}</text>

          <template v-if="qualityOptions.length">
            <text class="ai-image-generator__label">生成质量</text>
            <view class="ai-image-generator__quality" role="group" aria-label="生成质量">
              <button
                v-for="option in qualityOptions"
                :key="option.value"
                :class="['ai-image-generator__quality-option', { 'is-selected': quality === option.value }]"
                type="button"
                :aria-pressed="quality === option.value"
                hover-class="ai-image-generator__quality-option--pressed"
                @click='emit("update:quality", option.value)'
              >
                <text>{{ qualityLabel(option.value) }}</text>
                <text v-if="quality === option.value" class="ai-image-generator__check" aria-hidden="true">✓</text>
              </button>
            </view>
          </template>

          <view class="ai-image-generator__picker-row">
            <view class="ai-image-generator__picker-field">
              <text class="ai-image-generator__picker-label">模型</text>
              <picker
                class="ai-image-generator__picker"
                :range="models"
                range-key="name"
                :value="selectedModelIndex"
                :disabled="modelsLoading || !models.length || busy"
                @change="onModelChange"
              >
                <view class="ai-image-generator__picker-value">
                  <text>{{ modelsLoading ? '模型加载中…' : selectedModelName }}</text>
                  <text aria-hidden="true">⌄</text>
                </view>
              </picker>
            </view>

            <view v-if="countOptions.length" class="ai-image-generator__picker-field">
              <text class="ai-image-generator__picker-label">张数</text>
              <picker
                class="ai-image-generator__picker"
                :range="countOptions"
                range-key="label"
                :value="selectedCountIndex"
                :disabled="busy"
                @change="onCountChange"
              >
                <view class="ai-image-generator__picker-value">
                  <text>{{ count || countOptions[0]?.value }}张</text>
                  <text aria-hidden="true">⌄</text>
                </view>
              </picker>
            </view>
          </view>
        </view>

        <view
          :class="['ai-image-generator__live-region', `is-${viewState.tone}`]"
          :aria-live="viewState.tone === 'error' ? 'assertive' : 'polite'"
          aria-atomic="true"
        >
          <view v-if="viewState.liveMessage" class="ai-image-generator__live-message">
            <text :role="viewState.tone === 'error' ? 'alert' : 'status'">{{ viewState.liveMessage }}</text>
            <button
              v-if="viewState.tone === 'success' && !busy"
              class="ai-image-generator__view-result"
              type="button"
              @click='emit("view-result")'
            >查看结果</button>
          </view>
        </view>
        <view class="ai-image-generator__scroll-spacer" />
      </view>
    </scroll-view>

    <view class="ai-image-generator__footer">
      <view class="ai-image-generator__footer-content">
        <view class="ai-image-generator__footer-copy">
          <text class="ai-image-generator__estimate">{{ estimateLabel }}</text>
          <text
            v-if="viewState.disabledReason"
            id="image-generator-disabled-reason"
            class="ai-image-generator__disabled-reason"
          >{{ viewState.disabledReason }}</text>
        </view>
        <button
          class="ai-image-generator__generate"
          type="button"
          :disabled="!viewState.canSubmit"
          :aria-disabled="!viewState.canSubmit"
          :aria-describedby="viewState.disabledReason ? 'image-generator-disabled-reason' : undefined"
          :aria-label="viewState.disabledReason || viewState.primaryLabel"
          hover-class="ai-image-generator__generate--pressed"
          @click="onPrimaryAction"
        >
          <text v-if="viewState.showSpinner" class="ai-image-generator__generate-spinner" aria-hidden="true" />
          <text>{{ viewState.primaryLabel }}</text>
        </button>
      </view>
    </view>
  </view>
</template>

<script setup lang="ts">
import { computed } from "vue";
import { useMiniProgramNavigation } from "../../composables/useMiniProgramNavigation";
import {
  imageGeneratorViewState,
  type CanonicalImageQuality,
  type ImageControlOption,
  type ImageGeneratorModelOption,
  type ImageGeneratorStatusTone,
  type ImageSchemaLoadStatus,
  type ResolutionTier,
} from "../../features/generation/imageCreation";

const { navigationStyle } = useMiniProgramNavigation();

const props = withDefaults(defineProps<{
  prompt: string;
  size: string;
  sizeOptions: Array<ImageControlOption<string>>;
  selectedRatio: string;
  selectedTier: ResolutionTier;
  availableRatios: string[];
  availableTiers: ResolutionTier[];
  quality?: CanonicalImageQuality;
  qualityOptions: Array<ImageControlOption<CanonicalImageQuality>>;
  model: string;
  models: ImageGeneratorModelOption[];
  count?: number;
  countOptions: Array<ImageControlOption<number>>;
  referenceImages: string[];
  referenceLimit?: number;
  busy?: boolean;
  selectingReference?: boolean;
  modelsLoading?: boolean;
  schemaStatus?: ImageSchemaLoadStatus;
  schemaMessage?: string;
  disabledReason?: string;
  error?: string;
  statusMessage?: string;
  statusTone?: ImageGeneratorStatusTone;
  retryAvailable?: boolean;
  estimateLabel: string;
}>(), {
  referenceLimit: 3,
  busy: false,
  selectingReference: false,
  modelsLoading: false,
  schemaStatus: "idle",
  schemaMessage: "",
  disabledReason: "",
  error: "",
  statusMessage: "",
  statusTone: "idle",
  retryAvailable: false,
});

const emit = defineEmits<{
  back: [];
  help: [];
  "choose-reference": [];
  "remove-reference": [index: number];
  "preview-reference": [index: number];
  optimize: [];
  generate: [];
  retry: [];
  "view-result": [];
  "update:prompt": [value: string];
  "update:size": [value: string];
  "update:selectedRatio": [value: string];
  "update:selectedTier": [value: ResolutionTier];
  "update:quality": [value: CanonicalImageQuality];
  "update:model": [value: string];
  "update:count": [value: number];
}>();

const viewState = computed(() => imageGeneratorViewState({
  prompt: props.prompt,
  busy: props.busy,
  disabledReason: props.disabledReason,
  statusTone: props.statusTone,
  statusMessage: props.statusMessage,
  error: props.error,
  retryAvailable: props.retryAvailable,
}));
const selectedModelIndex = computed(() => Math.max(0, props.models.findIndex(item => item.code === props.model)));
const selectedModelName = computed(() => props.models[selectedModelIndex.value]?.name || "请选择模型");
const selectedCountIndex = computed(() => Math.max(0, props.countOptions.findIndex(option => option.value === props.count)));
const schemaStatusLabel = computed(() => {
  if (props.schemaStatus === "loading") return "正在读取当前模型参数";
  if (props.schemaStatus === "ready") return "图片参数已就绪";
  if (props.schemaStatus === "error") return "当前模型参数不可用";
  return "请选择图片模型";
});

const ratioOptions = computed(() => {
  const labels: Record<string, string> = {
    "auto": "自动",
    "1:1": "1:1",
    "16:9": "16:9",
    "9:16": "9:16",
    "4:3": "4:3",
    "3:4": "3:4",
    "3:2": "3:2",
    "2:3": "2:3",
  };
  return props.availableRatios.map(ratio => ({
    value: ratio,
    label: labels[ratio] || ratio,
  }));
});

const tierOptions = computed(() => {
  const labels: Record<string, string> = {
    "720p": "720P",
    "1K": "1K",
    "2K": "2K",
    "4K": "4K",
  };
  return props.availableTiers.map(tier => ({
    value: tier,
    label: labels[tier] || tier,
  }));
});

function qualityLabel(value: CanonicalImageQuality): string {
  const labels: Record<CanonicalImageQuality, string> = {
    auto: "自动",
    low: "低",
    medium: "中",
    high: "高",
  };
  return labels[value] || value;
}

function ratioShapeStyle(ratio: string) {
  if (ratio === "auto") return undefined;
  const parts = ratio.split(":").map(Number);
  if (parts.length !== 2 || !parts.every(n => Number.isFinite(n) && n > 0)) return undefined;
  const [w, h] = parts;
  const scale = Math.min(42 / w, 42 / h);
  return {
    width: `${Math.max(18, Math.round(w * scale))}px`,
    height: `${Math.max(18, Math.round(h * scale))}px`,
  };
}

function onPromptInput(event: Event | { detail: { value: string } }) {
  const detail = "detail" in event ? event.detail : undefined;
  if (typeof detail === "object" && detail !== null && "value" in detail) {
    emit("update:prompt", String(detail.value));
    return;
  }

  const target = "target" in event ? event.target as HTMLTextAreaElement | null : null;
  emit("update:prompt", target?.value || "");
}

function onModelChange(event: { detail: { value: string | number } }) {
  const selected = props.models[Number(event.detail.value)];
  if (selected) emit("update:model", selected.code);
}

function onCountChange(event: { detail: { value: string | number } }) {
  const selected = props.countOptions[Number(event.detail.value)];
  if (selected) emit("update:count", selected.value);
}

function onPrimaryAction() {
  if (!viewState.value.canSubmit) return;
  if (viewState.value.primaryAction === "retry") emit("retry");
  else emit("generate");
}
</script>

<style scoped>
.ai-image-generator {
  --image-brand: #423499;
  --image-brand-pressed: #30236f;
  --image-action: #ff771b;
  --image-action-pressed: #ed650a;
  --image-ink: #111827;
  --image-muted: #667085;
  --image-success: #067647;
  --image-line: #e1e6f1;
  --image-page: #f7f8fc;
  --image-radius: 16px;
  display: flex;
  flex-direction: column;
  min-height: 100vh;
  color: var(--image-ink);
  background: var(--image-page);
  font-family: "Be Vietnam Pro", system-ui, -apple-system, BlinkMacSystemFont, "PingFang SC", "Microsoft YaHei", sans-serif;
}

.ai-image-generator__title,
.ai-image-generator__heading,
.ai-image-generator__generate {
  font-family: "Plus Jakarta Sans", "PingFang SC", "Microsoft YaHei", sans-serif;
}

.ai-image-generator button,
.ai-image-generator__ratio-chip,
.ai-image-generator__reference-add {
  min-height: 44px;
}

.ai-image-generator button::after {
  border: none;
}

.ai-image-generator__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  box-sizing: border-box;
  min-height: var(--header-height, 64px);
  padding-top: var(--header-padding-top, 0px);
  padding-right: max(16px, var(--capsule-right-space, 0px));
  padding-bottom: 8px;
  padding-left: 16px;
  background: #fff;
}

.ai-image-generator__icon-button {
  width: 44px;
  margin: 0;
  padding: 0;
  border: 0;
  border-radius: 50%;
  color: var(--image-ink);
  background: transparent;
  font-size: 32px;
  line-height: 1;
}

.ai-image-generator__icon-button--pressed,
.ai-image-generator__ratio-chip--pressed,
.ai-image-generator__tier-chip--pressed,
.ai-image-generator__quality-option--pressed,
.ai-image-generator__reference--pressed,
.ai-image-generator__reference-add--pressed,
.ai-image-generator__optimize--pressed {
  opacity: 0.72;
}

.ai-image-generator__title {
  font-size: 22px;
  font-weight: 800;
}

.ai-image-generator__scroll {
  flex: 1;
  min-height: 0;
}

.ai-image-generator__content {
  width: 100%;
  box-sizing: border-box;
  padding: 16px;
  padding-bottom: calc(112px + env(safe-area-inset-bottom));
}

.ai-image-generator__helper {
  display: flex;
  justify-content: center;
  gap: 8px;
  margin: 8px 0 28px;
  color: var(--image-muted);
  font-size: 16px;
}

.ai-image-generator__helper-icon {
  color: var(--image-brand);
  font-size: 24px;
  line-height: 1;
}

.ai-image-generator__heading,
.ai-image-generator__settings-title {
  display: block;
  margin-bottom: 16px;
  font-size: 26px;
  font-weight: 800;
}

.ai-image-generator__prompt-card {
  display: flex;
  min-height: 300px;
  flex-direction: column;
  padding: 16px;
  border: 1px solid #b7bdfd;
  border-radius: var(--image-radius);
  background: #fff;
}

.ai-image-generator__references {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
}

.ai-image-generator__reference,
.ai-image-generator__reference-add {
  display: flex;
  width: 88px;
  height: 88px;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 3px;
  box-sizing: border-box;
  margin: 0;
  padding: 6px;
  overflow: hidden;
  border: 1px dashed #b7bdfd;
  border-radius: var(--image-radius);
  color: var(--image-muted);
  background: #fafbff;
  font-size: 12px;
  line-height: 1.25;
}

.ai-image-generator__reference {
  position: relative;
  border-style: solid;
  overflow: visible;
}

.ai-image-generator__reference-preview {
  width: 100%;
  height: 100%;
  min-height: 0 !important;
  margin: 0;
  padding: 0;
  border: 0;
  background: transparent;
}

.ai-image-generator__reference-preview image {
  width: 100%;
  height: 100%;
  border-radius: 8px;
}

.ai-image-generator__reference-remove {
  position: absolute;
  top: 0;
  right: 0;
  display: flex;
  width: 44px;
  height: 44px;
  align-items: center;
  justify-content: center;
  min-height: 44px !important;
  margin: 0;
  padding: 0;
  border: 0;
  background: transparent;
  line-height: 1;
}

.ai-image-generator__reference-remove-glyph {
  display: flex;
  width: 20px;
  height: 20px;
  align-items: center;
  justify-content: center;
  border-radius: 50%;
  color: #fff;
  background: rgba(17, 24, 39, 0.7);
  font-size: 16px;
}

.ai-image-generator__reference-plus {
  font-size: 30px;
  line-height: 0.8;
}

.ai-image-generator__reference-loading {
  width: 16px;
  height: 16px;
  border: 2px solid rgba(66, 52, 153, 0.24);
  border-top-color: var(--image-brand);
  border-radius: 50%;
  animation: ai-image-generator-spin 0.7s linear infinite;
}

@keyframes ai-image-generator-spin {
  to { transform: rotate(360deg); }
}

.ai-image-generator__textarea {
  width: 100%;
  min-height: 148px;
  flex: 1;
  box-sizing: border-box;
  padding: 16px 0;
  color: var(--image-ink);
  background: transparent;
  font-size: 16px;
  line-height: 1.65;
}

.ai-image-generator__prompt-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding-top: 10px;
  border-top: 1px solid var(--image-line);
}

.ai-image-generator__counter {
  color: var(--image-muted);
  font-size: 13px;
}

.ai-image-generator__optimize {
  margin: 0;
  padding: 0 14px;
  border: 1px solid #c7ceff;
  border-radius: var(--image-radius);
  color: var(--image-brand);
  background: #fff;
  font-size: 14px;
}

.ai-image-generator__settings {
  margin-top: 28px;
}

.ai-image-generator__settings-title {
  font-size: 20px;
}

.ai-image-generator__schema-status {
  display: flex;
  min-height: 44px;
  align-items: center;
  gap: 8px;
  padding: 0 12px;
  border: 1px solid var(--image-line);
  border-radius: 12px;
  color: var(--image-muted);
  background: #fff;
  font-size: 13px;
}

.ai-image-generator__schema-status.is-ready {
  min-height: 32px;
  padding: 0;
  border-color: transparent;
  color: var(--image-success);
  background: transparent;
}
.ai-image-generator__schema-status.is-error {
  border-color: #fecdca;
  color: #b42318;
  background: #fffbfa;
}

.ai-image-generator__schema-spinner,
.ai-image-generator__generate-spinner {
  width: 16px;
  height: 16px;
  flex: 0 0 auto;
  box-sizing: border-box;
  border: 2px solid rgba(66, 52, 153, 0.22);
  border-top-color: var(--image-brand);
  border-radius: 50%;
  animation: ai-image-generator-spin 0.7s linear infinite;
}

.ai-image-generator__label {
  display: block;
  margin: 22px 0 10px;
  font-size: 16px;
  font-weight: 700;
}

.ai-image-generator__ratio-list {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 8px;
}

.ai-image-generator__ratio-chip {
  position: relative;
  display: flex;
  min-width: 0;
  min-height: 112px;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 8px;
  margin: 0;
  padding: 8px 2px;
  border: 1px solid var(--image-line);
  border-radius: var(--image-radius);
  color: var(--image-muted);
  background: #fff;
  font-size: 13px;
}

.ai-image-generator__ratio-chip.is-selected,
.ai-image-generator__tier-chip.is-selected,
.ai-image-generator__quality-option.is-selected {
  border-color: var(--image-brand);
  color: var(--image-brand);
  background: #f4f3ff;
}

.ai-image-generator__ratio-shape {
  display: block;
  box-sizing: border-box;
  border: 2px solid currentColor;
  border-radius: 5px;
}

.ai-image-generator__tier-list {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.ai-image-generator__tier-chip {
  position: relative;
  display: flex;
  min-height: 52px;
  align-items: center;
  justify-content: center;
  margin: 0;
  padding: 0 20px;
  border: 1px solid var(--image-line);
  border-radius: var(--image-radius);
  color: var(--image-muted);
  background: #fff;
  font-size: 16px;
  font-weight: 600;
}

.ai-image-generator__output-hint {
  display: block;
  margin-top: 10px;
  color: var(--image-muted);
  font-size: 12px;
}

.ai-image-generator__check {
  position: absolute;
  top: 5px;
  right: 5px;
  display: flex;
  width: 20px;
  height: 20px;
  align-items: center;
  justify-content: center;
  border-radius: 50%;
  color: #fff;
  background: var(--image-brand);
  font-size: 12px;
}

.ai-image-generator__quality {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(100px, 1fr));
  overflow: hidden;
  border: 1px solid var(--image-line);
  border-radius: var(--image-radius);
}

.ai-image-generator__quality-option {
  position: relative;
  min-height: 52px;
  margin: 0;
  border: 0;
  border-radius: 0;
  color: var(--image-muted);
  background: #fff;
  font-size: 16px;
}

.ai-image-generator__quality-option + .ai-image-generator__quality-option {
  border-left: 1px solid var(--image-line);
}

.ai-image-generator__picker-row {
  display: grid;
  grid-template-columns: 1fr;
  gap: 12px;
  margin-top: 24px;
}

.ai-image-generator__picker-field {
  padding: 12px 14px;
  border: 1px solid var(--image-line);
  border-radius: var(--image-radius);
  background: #fff;
}

.ai-image-generator__picker-label {
  display: block;
  margin-bottom: 4px;
  color: var(--image-muted);
  font-size: 13px;
}

.ai-image-generator__picker-value {
  display: flex;
  min-height: 28px;
  align-items: center;
  justify-content: space-between;
  color: var(--image-ink);
  font-size: 16px;
  font-weight: 700;
}

.ai-image-generator__live-region {
  min-height: 22px;
  margin-top: 16px;
  text-align: center;
}

.ai-image-generator__live-message {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  font-size: 13px;
}

.ai-image-generator__live-message > text {
  min-width: 0;
  flex: 1;
  text-align: left;
}

.ai-image-generator__live-region.is-idle { color: var(--image-muted); }
.ai-image-generator__live-region.is-loading { color: var(--image-brand); }
.ai-image-generator__live-region.is-success { color: var(--image-success); }
.ai-image-generator__live-region.is-error { color: #b42318; }

.ai-image-generator__view-result {
  min-width: 88px;
  flex: 0 0 auto;
  margin: 0;
  padding: 0 12px;
  border: 1px solid currentColor;
  border-radius: 999px;
  color: var(--image-success);
  background: transparent;
  font-size: 13px;
  line-height: 42px;
  white-space: nowrap;
}
.ai-image-generator__scroll-spacer { display: none; }

.ai-image-generator__footer {
  position: fixed;
  right: 0;
  bottom: 0;
  left: 0;
  z-index: 2;
  border-top: 1px solid var(--image-line);
  background: rgba(255, 255, 255, 0.96);
  backdrop-filter: blur(12px);
}

.ai-image-generator__footer-content {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px 16px;
  padding-bottom: max(16px, env(safe-area-inset-bottom));
}

.ai-image-generator__footer-copy {
  display: flex;
  min-width: 0;
  flex: 1;
  flex-direction: column;
  gap: 2px;
}

.ai-image-generator__estimate {
  color: var(--image-ink);
  font-size: 14px;
}

.ai-image-generator__disabled-reason {
  color: #b42318;
  font-size: 12px;
  line-height: 1.35;
}

.ai-image-generator__generate {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  flex: 0 0 auto;
  min-width: 150px;
  min-height: 52px;
  margin: 0;
  padding: 0 20px;
  border: 0;
  border-radius: var(--image-radius);
  color: #231000;
  background: var(--image-action);
  font-size: 18px;
  font-weight: 800;
  white-space: nowrap;
}

.ai-image-generator__generate--pressed { background: var(--image-action-pressed); }
.ai-image-generator button:disabled,
.ai-image-generator picker[disabled] {
  cursor: not-allowed;
  opacity: 0.48;
}

.ai-image-generator__generate[aria-disabled="true"] {
  opacity: 0.48;
  cursor: not-allowed;
  box-shadow: none;
}

@media (hover: hover) {
  .ai-image-generator__icon-button:not([disabled]):hover,
  .ai-image-generator__ratio-chip:not([disabled]):hover,
  .ai-image-generator__tier-chip:not([disabled]):hover,
  .ai-image-generator__quality-option:not([disabled]):hover,
  .ai-image-generator__reference-preview:not([disabled]):hover,
  .ai-image-generator__reference-add:not([disabled]):hover,
  .ai-image-generator__optimize:not([disabled]):hover {
    opacity: 0.72;
  }

  .ai-image-generator__generate:not([disabled]):hover {
    background: var(--image-action-pressed);
  }
}

.ai-image-generator button:focus-visible,
.ai-image-generator textarea:focus-visible,
.ai-image-generator :deep(.uni-textarea-textarea:focus-visible) {
  outline: 3px solid rgba(66, 52, 153, 0.32);
  outline-offset: 2px;
}

@media (min-width: 768px) {
  .ai-image-generator__content {
    padding-top: 28px;
    padding-right: 36px;
    padding-bottom: calc(112px + env(safe-area-inset-bottom));
    padding-left: 36px;
  }
  .ai-image-generator__picker-row { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .ai-image-generator__footer-content { padding-right: 36px; padding-left: 36px; }
}

@media (min-width: 1200px) {
  .ai-image-generator__content,
  .ai-image-generator__footer-content { max-width: 960px; margin-right: auto; margin-left: auto; }
  .ai-image-generator__footer-content { padding-right: 0; padding-left: 0; }
}

@media (prefers-reduced-motion: reduce) {
  .ai-image-generator__reference-loading,
  .ai-image-generator__schema-spinner,
  .ai-image-generator__generate-spinner { animation: none !important; }
}
</style>
