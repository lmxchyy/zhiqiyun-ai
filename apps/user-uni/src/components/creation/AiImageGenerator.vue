<template>
  <view class="ai-image-generator">
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
              >×</button>
            </view>
            <button
              class="ai-image-generator__reference-add"
              type="button"
              :disabled="referenceImages.length >= referenceLimit || selectingReference"
              :aria-label="selectingReference ? '正在选择参考图' : '添加参考图'"
              hover-class="ai-image-generator__reference-add--pressed"
              @click='emit("choose-reference")'
            >
              <text class="ai-image-generator__reference-plus">＋</text>
              <text>添加参考</text>
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

          <text class="ai-image-generator__label">画幅比例</text>
          <view class="ai-image-generator__aspect-list">
            <button
              v-for="option in imageAspectOptions"
              :key="option.value"
              :class="['ai-image-generator__aspect', { 'is-selected': aspectRatio === option.value }]"
              type="button"
              :aria-pressed="aspectRatio === option.value"
              hover-class="ai-image-generator__aspect--pressed"
              @click='emit("update:aspectRatio", option.value)'
            >
              <text class="ai-image-generator__aspect-shape" :class="`is-${option.value.replace(':', '-')}`" />
              <text>{{ option.value === 'auto' ? '自动比例' : option.label }}</text>
              <text v-if="aspectRatio === option.value" class="ai-image-generator__check" aria-hidden="true">✓</text>
            </button>
          </view>

          <text class="ai-image-generator__label">图片清晰度</text>
          <view class="ai-image-generator__quality" role="group" aria-label="图片清晰度">
            <button
              v-for="option in imageQualityOptions"
              :key="option"
              :class="['ai-image-generator__quality-option', { 'is-selected': quality === option }]"
              type="button"
              :aria-pressed="quality === option"
              hover-class="ai-image-generator__quality-option--pressed"
              @click='emit("update:quality", option)'
            >
              <text>{{ option }}</text>
              <text v-if="quality === option" class="ai-image-generator__check" aria-hidden="true">✓</text>
            </button>
          </view>

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

            <view class="ai-image-generator__picker-field">
              <text class="ai-image-generator__picker-label">张数</text>
              <picker
                class="ai-image-generator__picker"
                :range="imageCountOptions"
                :value="selectedCountIndex"
                :disabled="busy"
                @change="onCountChange"
              >
                <view class="ai-image-generator__picker-value">
                  <text>{{ count }}张</text>
                  <text aria-hidden="true">⌄</text>
                </view>
              </picker>
            </view>
          </view>
        </view>

        <view class="ai-image-generator__live-region" aria-live="polite" aria-atomic="true">
          <text v-if="error" class="ai-image-generator__error">{{ error }}</text>
          <text v-else-if="statusMessage" class="ai-image-generator__status">{{ statusMessage }}</text>
          <text v-else-if="disabledReason" class="ai-image-generator__status">{{ disabledReason }}</text>
        </view>
        <view class="ai-image-generator__scroll-spacer" />
      </view>
    </scroll-view>

    <view class="ai-image-generator__footer">
      <view class="ai-image-generator__footer-content">
        <text class="ai-image-generator__estimate">{{ estimateLabel }}</text>
        <button
          class="ai-image-generator__generate"
          type="button"
          :disabled="!canGenerate"
          :aria-label="disabledReason || (busy ? '图片生成中…' : '生成图片')"
          hover-class="ai-image-generator__generate--pressed"
          @click='emit("generate")'
        >
          <text>{{ busy ? "图片生成中…" : "生成图片" }}</text>
        </button>
      </view>
    </view>
  </view>
</template>

<script setup lang="ts">
import { computed } from "vue";
import {
  imageAspectOptions,
  imageCountOptions,
  imageQualityOptions,
  type ImageAspectRatio,
  type ImageGeneratorModelOption,
  type ImageQuality,
} from "../../features/generation/imageCreation";

const props = withDefaults(defineProps<{
  prompt: string;
  aspectRatio: ImageAspectRatio;
  quality: ImageQuality;
  model: string;
  models: ImageGeneratorModelOption[];
  count: number;
  referenceImages: string[];
  referenceLimit?: number;
  busy?: boolean;
  selectingReference?: boolean;
  modelsLoading?: boolean;
  disabledReason?: string;
  error?: string;
  statusMessage?: string;
  estimateLabel: string;
}>(), {
  referenceLimit: 3,
  busy: false,
  selectingReference: false,
  modelsLoading: false,
  disabledReason: "",
  error: "",
  statusMessage: "",
});

const emit = defineEmits<{
  back: [];
  help: [];
  "choose-reference": [];
  "remove-reference": [index: number];
  "preview-reference": [index: number];
  optimize: [];
  generate: [];
  "update:prompt": [value: string];
  "update:aspectRatio": [value: ImageAspectRatio];
  "update:quality": [value: ImageQuality];
  "update:model": [value: string];
  "update:count": [value: number];
}>();

const canGenerate = computed(() => Boolean(props.prompt.trim()) && !props.busy && !props.disabledReason);
const selectedModelIndex = computed(() => Math.max(0, props.models.findIndex(item => item.code === props.model)));
const selectedModelName = computed(() => props.models[selectedModelIndex.value]?.name || "请选择模型");
const selectedCountIndex = computed(() => Math.max(0, imageCountOptions.indexOf(props.count as typeof imageCountOptions[number])));

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
  const selected = imageCountOptions[Number(event.detail.value)];
  if (selected) emit("update:count", selected);
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
.ai-image-generator__aspect,
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
  min-height: 64px;
  padding: max(8px, env(safe-area-inset-top, 0px)) 16px 8px;
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
.ai-image-generator__aspect--pressed,
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
  color: #4d5bf9;
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
  border-radius: 14px;
  color: var(--image-muted);
  background: #fafbff;
  font-size: 12px;
  line-height: 1.25;
}

.ai-image-generator__reference {
  position: relative;
  border-style: solid;
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
  top: 2px;
  right: 2px;
  display: flex;
  width: 20px;
  height: 20px;
  align-items: center;
  justify-content: center;
  min-height: 20px !important;
  margin: 0;
  padding: 0;
  border: 0;
  border-radius: 50%;
  color: #fff;
  background: rgba(17, 24, 39, 0.7);
  font-size: 16px;
  line-height: 1;
}

.ai-image-generator__reference-plus {
  font-size: 30px;
  line-height: 0.8;
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
  border-radius: 12px;
  color: #4d5bf9;
  background: #fff;
  font-size: 14px;
}

.ai-image-generator__settings {
  margin-top: 28px;
}

.ai-image-generator__settings-title {
  font-size: 20px;
}

.ai-image-generator__label {
  display: block;
  margin: 22px 0 10px;
  font-size: 16px;
  font-weight: 700;
}

.ai-image-generator__aspect-list {
  display: grid;
  grid-template-columns: repeat(5, minmax(0, 1fr));
  gap: 8px;
}

.ai-image-generator__aspect {
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
  border-radius: 14px;
  color: var(--image-muted);
  background: #fff;
  font-size: 13px;
}

.ai-image-generator__aspect.is-selected,
.ai-image-generator__quality-option.is-selected {
  border-color: #4d5bf9;
  color: #4d5bf9;
  background: #f4f3ff;
}

.ai-image-generator__aspect-shape {
  display: block;
  width: 34px;
  height: 30px;
  border: 2px solid currentColor;
  border-radius: 5px;
}

.ai-image-generator__aspect-shape.is-16-9 { width: 42px; height: 24px; }
.ai-image-generator__aspect-shape.is-9-16 { width: 24px; height: 42px; }
.ai-image-generator__aspect-shape.is-4-3 { width: 40px; height: 30px; }

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
  background: #4d5bf9;
  font-size: 12px;
}

.ai-image-generator__quality {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
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

.ai-image-generator__error,
.ai-image-generator__status {
  font-size: 13px;
}

.ai-image-generator__error { color: #d92d20; }
.ai-image-generator__status { color: var(--image-muted); }
.ai-image-generator__scroll-spacer { height: 92px; }

.ai-image-generator__footer {
  position: sticky;
  bottom: 0;
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

.ai-image-generator__estimate {
  flex: 1;
  color: var(--image-ink);
  font-size: 14px;
}

.ai-image-generator__generate {
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
}

.ai-image-generator__generate--pressed { background: var(--image-action-pressed); }
.ai-image-generator__generate[disabled] { opacity: 0.48; }

.ai-image-generator button:focus-visible,
.ai-image-generator textarea:focus-visible {
  outline: 3px solid rgba(66, 52, 153, 0.32);
  outline-offset: 2px;
}

@media (min-width: 768px) {
  .ai-image-generator__content { padding: 28px 36px; }
  .ai-image-generator__picker-row { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .ai-image-generator__footer-content { padding-right: 36px; padding-left: 36px; }
}

@media (min-width: 1200px) {
  .ai-image-generator__content,
  .ai-image-generator__footer-content { max-width: 960px; margin-right: auto; margin-left: auto; }
  .ai-image-generator__footer-content { padding-right: 0; padding-left: 0; }
}

@media (prefers-reduced-motion: reduce) {
  .ai-image-generator * { transition-duration: 0.01ms !important; animation-duration: 0.01ms !important; }
}
</style>
