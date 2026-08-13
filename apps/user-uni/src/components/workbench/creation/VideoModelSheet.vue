<template>
  <BottomSheet :visible="visible" title="选择生成模型" @close="emit('close')">
    <scroll-view scroll-y class="video-model-sheet-list">
      <button
        v-for="item in options"
        :key="item.code"
        type="button"
        :class="['video-model-sheet-item', { active: item.code === selectedCode }]"
        :disabled="switching"
        @click="emit('select', item.code)"
      >
        <view class="video-model-sheet-copy">
          <text class="video-model-sheet-title">{{ videoModelTitle(item) }}</text>
          <text class="video-model-sheet-subtitle">{{ videoModelSubtitle(item) }}</text>
        </view>
        <text v-if="item.code === selectedCode" class="video-model-sheet-check">✓</text>
      </button>
    </scroll-view>
  </BottomSheet>
</template>

<script setup lang="ts">
import BottomSheet from "../../auth/BottomSheet.vue";
import type { ModelInfo } from "@xianzhi/business-sdk";

interface Props {
  visible: boolean;
  options: ModelInfo[];
  selectedCode: string;
  switching: boolean;
}

defineProps<Props>();

const emit = defineEmits<{
  (e: 'close'): void;
  (e: 'select', code: string): void;
}>();

function videoModelTitle(item?: ModelInfo | null) {
  return item?.displayName || item?.name || item?.code || '未知模型';
}

function videoModelSubtitle(item?: ModelInfo | null) {
  return [item?.provider, item?.description].filter(Boolean).join(' · ') || '暂无说明';
}
</script>

<style scoped>
.video-model-sheet-list { max-height: 58vh; }
.video-model-sheet-item { display: flex; width: 100%; min-height: 62px; margin: 0; padding: 12px 14px; align-items: center; justify-content: space-between; border: 1px solid #e5eaf6; border-radius: 12px; background: #fff; text-align: left; }
.video-model-sheet-item + .video-model-sheet-item { margin-top: 10px; }
.video-model-sheet-item.active { border-color: #c9d2ff; background: #f4f3ff; }
.video-model-sheet-copy { min-width: 0; flex: 1; }
.video-model-sheet-title { display: block; color: #111827; font-size: 13px; font-weight: 600; }
.video-model-sheet-subtitle { display: block; margin-top: 4px; color: #697386; font-size: 10px; }
.video-model-sheet-check { margin-left: 12px; color: #5b55d6; font-size: 16px; font-weight: 700; }
</style>
