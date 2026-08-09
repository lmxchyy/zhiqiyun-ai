<template>
  <view class="free-image-edit">
    <view class="free-image-edit__header">
      <button class="free-image-edit__back" aria-label="返回上一页" @click="emit('back')">‹</button>
      <text class="free-image-edit__title">自由P图</text>
    </view>

    <scroll-view class="free-image-edit__scroll" scroll-y :show-scrollbar="false">
      <!-- 单图上传区 -->
      <view class="free-image-edit__section">
        <view
          :class="['free-image-edit__upload', { 'has-image': imagePath }]"
          @click="!imagePath && emit('choose-image')"
        >
          <template v-if="imagePath">
            <image
              class="free-image-edit__upload-image"
              :src="imagePath"
              mode="aspectFill"
              @click="emit('preview-image')"
            />
            <view class="free-image-edit__upload-actions">
              <button class="free-image-edit__upload-replace" type="button" @click.stop="emit('choose-image')">替换图片</button>
              <button class="free-image-edit__upload-remove" type="button" @click.stop="emit('remove-image')">×</button>
            </view>
          </template>
          <template v-else>
            <view class="free-image-edit__upload-empty">
              <image class="free-image-edit__upload-icon" src="/static/icons/edit-square.svg" mode="aspectFit" />
              <text class="free-image-edit__upload-hint">请添加图片</text>
            </view>
          </template>
        </view>
      </view>

      <!-- 效果区域 -->
      <view class="free-image-edit__section">
        <text class="free-image-edit__section-title">选择图片效果</text>

        <view class="free-image-edit__prompt">
          <textarea
            v-model="localPrompt"
            class="free-image-edit__textarea"
            maxlength="3000"
            placeholder="描述你想修改的效果，例如：更换背景为海边日落"
            @input="onPromptInput"
          />
          <view class="free-image-edit__prompt-footer">
            <text class="free-image-edit__counter">{{ localPrompt.length }} / 3000</text>
            <button
              v-if="localPrompt"
              class="free-image-edit__clear"
              type="button"
              @click="clearPrompt"
            >清空</button>
          </view>
        </view>
      </view>

      <!-- 六个效果预设卡片 -->
      <view class="free-image-edit__section">
        <view class="free-image-edit__preset-grid">
          <button
            v-for="preset in presets"
            :key="preset.id"
            :class="['free-image-edit__preset', { 'is-selected': selectedPresetId === preset.id }]"
            type="button"
            @click="selectPreset(preset.prompt)"
          >
            <view class="free-image-edit__preset-body">
              <text class="free-image-edit__preset-title">{{ preset.title }}</text>
              <text class="free-image-edit__preset-desc">{{ preset.prompt }}</text>
            </view>
            <image
              v-if="selectedPresetId === preset.id"
              class="free-image-edit__preset-check"
              src="/static/icons/check.svg"
              mode="aspectFit"
            />
          </button>
        </view>
      </view>

      <!-- 底部留白，避免被固定按钮遮挡 -->
      <view class="free-image-edit__scroll-spacer" />
    </scroll-view>

    <!-- 底部固定操作区 -->
    <view class="free-image-edit__footer">
      <button
        :class="['free-image-edit__generate', { 'is-busy': busy }]"
        :disabled="busy"
        type="button"
        @click="emit('generate')"
      >
        <text v-if="busy" class="free-image-edit__generate-spinner" />
        <text>{{ busy ? '生成中...' : '开始生成' }}</text>
      </button>
    </view>

    <text v-if="error" class="free-image-edit__error">{{ error }}</text>
  </view>
</template>

<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { freeImageEditPresets, selectedFreeImageEditPresetId } from "../../features/generation/freeImageEdit";

const props = defineProps<{
  imagePath: string;
  prompt: string;
  busy: boolean;
  selectingImage: boolean;
  error: string;
}>();

const emit = defineEmits<{
  back: [];
  "choose-image": [];
  "remove-image": [];
  "preview-image": [];
  "update:prompt": [value: string];
  generate: [];
}>();

const presets = freeImageEditPresets;

const localPrompt = ref(props.prompt);

watch(() => props.prompt, (value) => {
  localPrompt.value = value;
});

const selectedPresetId = computed(() => selectedFreeImageEditPresetId(localPrompt.value));

function onPromptInput() {
  emit("update:prompt", localPrompt.value);
}

function selectPreset(prompt: string) {
  localPrompt.value = prompt;
  emit("update:prompt", prompt);
}

function clearPrompt() {
  localPrompt.value = "";
  emit("update:prompt", "");
}
</script>

<style scoped>
.free-image-edit {
  display: flex;
  flex-direction: column;
  min-height: var(--window-height, 100vh);
  background: #f8fafc;
  position: relative;
}

/* 顶部栏 */
.free-image-edit__header {
  display: flex;
  align-items: center;
  min-height: var(--header-height, 57px);
  padding-top: var(--header-padding-top, 0px);
  padding-right: var(--capsule-right-space, 0px);
  padding-left: 0;
  background: #fff;
  border-bottom: 1px solid #ddd;
  position: sticky;
  top: 0;
  z-index: 10;
}

.free-image-edit__back {
  width: 44px;
  height: 44px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 28px;
  color: #202537;
  background: none;
  border: none;
  padding: 0;
  margin: 0;
  line-height: 1;
  flex-shrink: 0;
}

.free-image-edit__back::after {
  border: none;
}

.free-image-edit__title {
  flex: 1;
  text-align: center;
  font-size: 17px;
  font-weight: 600;
  color: #202537;
  padding-right: 44px;
}

/* 滚动区 */
.free-image-edit__scroll {
  flex: 1;
  padding: 16px;
}

.free-image-edit__scroll-spacer {
  height: 100px;
}

/* 区域 */
.free-image-edit__section {
  margin-bottom: 16px;
}

.free-image-edit__section-title {
  font-size: 15px;
  font-weight: 600;
  color: #202537;
  margin-bottom: 10px;
  display: block;
}

/* 上传区 */
.free-image-edit__upload {
  min-height: 220px;
  border: 1px solid #c6c6cc;
  border-radius: 12px;
  background: #fff;
  display: flex;
  align-items: center;
  justify-content: center;
  overflow: hidden;
  position: relative;
}

.free-image-edit__upload.has-image {
  display: block;
}

.free-image-edit__upload-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
}

.free-image-edit__upload-icon {
  width: 40px;
  height: 40px;
  opacity: 0.4;
}

.free-image-edit__upload-hint {
  font-size: 14px;
  color: #999;
}

.free-image-edit__upload-image {
  width: 100%;
  height: 220px;
}

.free-image-edit__upload-actions {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 12px;
  padding: 10px 16px;
}

.free-image-edit__upload-replace {
  font-size: 13px;
  color: #4a72ff;
  background: none;
  border: 1px solid #4a72ff;
  border-radius: 6px;
  padding: 4px 14px;
  line-height: 1.4;
}

.free-image-edit__upload-replace::after {
  border: none;
}

.free-image-edit__upload-remove {
  width: 28px;
  height: 28px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 18px;
  color: #999;
  background: #f0f0f0;
  border: none;
  border-radius: 50%;
  padding: 0;
  line-height: 1;
}

.free-image-edit__upload-remove::after {
  border: none;
}

/* 输入区 */
.free-image-edit__prompt {
  min-height: 160px;
  border: 1px solid #ddd;
  border-radius: 12px;
  background: #fff;
  padding: 12px;
  display: flex;
  flex-direction: column;
}

.free-image-edit__textarea {
  width: 100%;
  flex: 1;
  min-height: 100px;
  font-size: 14px;
  color: #202537;
  line-height: 1.6;
}

.free-image-edit__prompt-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding-top: 8px;
}

.free-image-edit__counter {
  font-size: 12px;
  color: #bbb;
}

.free-image-edit__clear {
  font-size: 12px;
  color: #4a72ff;
  background: none;
  border: none;
  padding: 0;
}

.free-image-edit__clear::after {
  border: none;
}

/* 预设卡片 */
.free-image-edit__preset-grid {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.free-image-edit__preset {
  display: flex;
  align-items: center;
  justify-content: space-between;
  background: #fff;
  border: 1.5px solid #e8e8ed;
  border-radius: 12px;
  padding: 14px;
  text-align: left;
  width: 100%;
}

.free-image-edit__preset::after {
  border: none;
}

.free-image-edit__preset.is-selected {
  border: 2px solid #4a72ff;
}

.free-image-edit__preset-body {
  flex: 1;
  min-width: 0;
}

.free-image-edit__preset-title {
  font-size: 15px;
  font-weight: 600;
  color: #202537;
  display: block;
  margin-bottom: 4px;
}

.free-image-edit__preset.is-selected .free-image-edit__preset-title {
  color: #4a72ff;
}

.free-image-edit__preset-desc {
  font-size: 12px;
  color: #999;
  line-height: 1.5;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.free-image-edit__preset-check {
  width: 22px;
  height: 22px;
  flex-shrink: 0;
  margin-left: 10px;
  background: #4a72ff;
  border-radius: 50%;
}

/* 底部固定操作区 */
.free-image-edit__footer {
  padding: 12px 16px;
  padding-bottom: max(16px, env(safe-area-inset-bottom, 0px));
  background: rgba(255, 255, 255, 0.92);
  backdrop-filter: blur(10px);
  border-top: 1px solid #eee;
  position: sticky;
  bottom: 0;
  z-index: 10;
}

.free-image-edit__generate {
  width: 100%;
  min-height: 46px;
  border-radius: 999px;
  color: #fff;
  background: #ff6b00;
  font-size: 16px;
  font-weight: 600;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  border: none;
}

.free-image-edit__generate::after {
  border: none;
}

.free-image-edit__generate.is-busy {
  opacity: 0.7;
}

.free-image-edit__generate-spinner {
  width: 16px;
  height: 16px;
  border: 2px solid rgba(255, 255, 255, 0.3);
  border-top-color: #fff;
  border-radius: 50%;
  animation: free-image-edit-spin 0.6s linear infinite;
}

@keyframes free-image-edit-spin {
  to {
    transform: rotate(360deg);
  }
}

/* 错误提示 */
.free-image-edit__error {
  position: fixed;
  bottom: 100px;
  left: 50%;
  transform: translateX(-50%);
  background: rgba(220, 53, 69, 0.92);
  color: #fff;
  font-size: 13px;
  padding: 8px 20px;
  border-radius: 8px;
  z-index: 20;
  max-width: 90%;
  text-align: center;
}
</style>
