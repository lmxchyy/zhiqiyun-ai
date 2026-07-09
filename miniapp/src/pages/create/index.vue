<template>
  <zq-page-shell>
    <view class="page-head">
      <text class="page-head__title">创作</text>
      <text class="page-head__copy">图片、视频、PPT 和 Agent 统一从这里发起。</text>
    </view>

    <view class="create-card zq-card">
      <wd-tabs v-model="draft.mode" custom-class="create-tabs">
        <wd-tab v-for="mode in createModes" :key="mode.value" :name="mode.value" :title="mode.label" />
      </wd-tabs>

      <view class="form-block">
        <text class="form-label">提示词</text>
        <wd-textarea
          v-model="draft.prompt"
          :maxlength="500"
          clearable
          show-word-limit
          placeholder="描述你的创作需求，例如：生成一张科技感企业服务海报，蓝紫主色，突出 AI 生产力"
        />
      </view>

      <view class="form-block">
        <text class="form-label">上传参考图</text>
        <view class="upload-box" @click="mockUpload">
          <text class="upload-box__plus">+</text>
          <text class="upload-box__text">上传图片</text>
          <text class="upload-box__hint">支持商品图、草图、品牌参考</text>
        </view>
      </view>

      <view class="form-block">
        <text class="form-label">模型选择</text>
        <view class="chip-row">
          <text
            v-for="model in modelOptions"
            :key="model"
            class="choice-chip"
            :class="{ active: draft.model === model }"
            @click="draft.model = model"
          >
            {{ model }}
          </text>
        </view>
      </view>

      <view class="form-block">
        <text class="form-label">风格选择</text>
        <view class="chip-row">
          <text
            v-for="style in styleOptions"
            :key="style"
            class="choice-chip"
            :class="{ active: draft.style === style }"
            @click="draft.style = style"
          >
            {{ style }}
          </text>
        </view>
      </view>

      <view class="form-grid">
        <view class="form-block">
          <text class="form-label">尺寸</text>
          <view class="chip-row">
            <text
              v-for="size in sizeOptions"
              :key="size"
              class="choice-chip compact"
              :class="{ active: draft.size === size }"
              @click="draft.size = size"
            >
              {{ size }}
            </text>
          </view>
        </view>
        <view class="form-block">
          <text class="form-label">质量</text>
          <view class="chip-row">
            <text
              v-for="quality in qualityOptions"
              :key="quality"
              class="choice-chip compact"
              :class="{ active: draft.quality === quality }"
              @click="draft.quality = quality"
            >
              {{ quality }}
            </text>
          </view>
        </view>
      </view>

      <view class="count-row">
        <view>
          <text class="form-label">生成数量</text>
          <text class="count-row__hint">用于控制本次任务消耗积分</text>
        </view>
        <wd-input-number v-model="draft.count" :min="1" :max="4" />
      </view>

      <wd-button block type="primary" custom-class="generate-button" :loading="submitting" @click="submit">
        生成
      </wd-button>
    </view>
  </zq-page-shell>
</template>

<script setup lang="ts">
import { reactive, ref } from 'vue'
import ZqPageShell from '@/components/zq-page-shell.vue'
import { createModes, modelOptions, qualityOptions, sizeOptions, styleOptions } from '@/mock/create'
import { createGenerationTask } from '@/services/miniapp'
import type { CreateDraft } from '@/types/domain'
import { showMockToast } from '@/utils/navigation'

const submitting = ref(false)
const draft = reactive<CreateDraft>({
  mode: 'image',
  prompt: '',
  model: modelOptions[0],
  style: styleOptions[0],
  size: sizeOptions[0],
  quality: qualityOptions[1],
  count: 1,
  referenceImages: [],
})

function mockUpload() {
  draft.referenceImages.push(`mock-reference-${draft.referenceImages.length + 1}`)
  showMockToast('参考图已加入 mock 队列')
}

async function submit() {
  if (!draft.prompt.trim()) {
    showMockToast('请先输入提示词')
    return
  }
  submitting.value = true
  try {
    await createGenerationTask({ ...draft })
    showMockToast('生成任务已创建，结果将进入作品中心')
  }
  finally {
    submitting.value = false
  }
}
</script>

<style scoped lang="scss">
.page-head {
  padding: 42rpx 4rpx 22rpx;
}

.page-head__title,
.page-head__copy {
  display: block;
}

.page-head__title {
  color: var(--color-text-primary);
  font-size: 46rpx;
  font-weight: 900;
}

.page-head__copy {
  margin-top: 10rpx;
  color: var(--color-text-secondary);
  font-size: 25rpx;
}

.create-card {
  padding: 22rpx;
}

.form-block {
  margin-top: 26rpx;
}

.form-label {
  display: block;
  margin-bottom: 14rpx;
  color: var(--color-text-primary);
  font-size: 26rpx;
  font-weight: 800;
}

.upload-box {
  display: grid;
  place-items: center;
  min-height: 180rpx;
  border: 2rpx dashed rgba(125, 141, 246, 0.42);
  border-radius: var(--radius-lg);
  background: rgba(125, 141, 246, 0.06);
}

.upload-box__plus,
.upload-box__text,
.upload-box__hint {
  display: block;
}

.upload-box__plus {
  color: var(--color-primary);
  font-size: 44rpx;
  font-weight: 300;
}

.upload-box__text {
  color: var(--color-primary-dark);
  font-size: 26rpx;
  font-weight: 800;
}

.upload-box__hint {
  margin-top: 8rpx;
  color: var(--color-text-muted);
  font-size: 22rpx;
}

.chip-row {
  display: flex;
  flex-wrap: wrap;
  gap: 14rpx;
}

.choice-chip {
  display: inline-flex;
  align-items: center;
  min-height: 56rpx;
  padding: 0 20rpx;
  border: 1rpx solid var(--color-border);
  border-radius: 999rpx;
  background: #fff;
  color: var(--color-text-secondary);
  font-size: 23rpx;
  font-weight: 700;
}

.choice-chip.active {
  border-color: rgba(125, 141, 246, 0.42);
  background: rgba(125, 141, 246, 0.12);
  color: var(--color-primary-dark);
}

.choice-chip.compact {
  min-height: 52rpx;
}

.form-grid {
  display: grid;
  grid-template-columns: 1fr;
  gap: 2rpx;
}

.count-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 20rpx;
  margin-top: 30rpx;
  padding: 22rpx;
  border-radius: var(--radius-lg);
  background: #f9fafc;
}

.count-row__hint {
  display: block;
  color: var(--color-text-secondary);
  font-size: 22rpx;
}

:deep(.generate-button) {
  height: 92rpx !important;
  margin-top: 30rpx;
  background: var(--color-accent) !important;
  border-color: var(--color-accent) !important;
  border-radius: var(--radius-md) !important;
  font-weight: 900 !important;
}
</style>
