<template>
  <button class="inspiration-card" @click="$emit('open', item)">
    <view class="cover-wrap" :class="`type-${item.contentType}`">
      <view v-if="comparison" class="comparison-cover">
        <view class="comparison-pane">
          <AppImage :src="comparison.beforeUrl" :alt="`${item.title}修复前`" width="100%" height="100%" radius="7px 0 0 7px" />
          <text class="comparison-label">修复前</text>
        </view>
        <view class="comparison-pane">
          <AppImage :src="comparison.afterUrl" :alt="`${item.title}修复后`" width="100%" height="100%" radius="0 7px 7px 0" />
          <text class="comparison-label">修复后</text>
        </view>
      </view>
      <AppImage v-else :src="item.thumbnailUrl || item.coverUrl" :fallback="item.coverUrl" :alt="item.title" width="100%" height="100%" radius="7px" />
      <text v-if="item.hot" class="hot-mark">热门</text>
      <text v-else class="ai-mark">AI生成示例</text>
      <text class="type-mark">{{ typeLabel }}</text>
      <view v-if="item.contentType === 'video'" class="play-mark">▶</view>
    </view>
    <text class="card-title">{{ item.title }}</text>
    <text class="prompt-summary">{{ item.description }}</text>
    <view class="card-foot">
      <text>♡ {{ item.favoriteCount }} · 使用 {{ item.useCount }}</text>
      <text class="use-action">{{ item.scenarioCode === 'photo_restoration' ? '立即使用' : '生成同款' }}</text>
    </view>
  </button>
</template>

<script setup lang="ts">
import { computed } from "vue";
import AppImage from "../AppImage.vue";
import { inspirationComparisonSources, type InspirationTemplate } from "../../features/inspiration/types";

const props = defineProps<{ item: InspirationTemplate }>();
defineEmits<{ open: [item: InspirationTemplate] }>();
const typeLabel = computed(() => ({ image: "图片", video: "视频", ppt: "PPT" }[props.item.contentType]));
const comparison = computed(() => inspirationComparisonSources(props.item.displayConfig));
</script>

<style scoped>
.inspiration-card{width:100%;margin:0;padding:8px;border:1px solid #e8eaf1;border-radius:8px;background:#fff;text-align:left;box-shadow:0 6px 20px rgba(31,39,67,.05)}
.inspiration-card::after{display:none}
.cover-wrap{position:relative;height:190px;overflow:hidden;border-radius:7px;background:#f1f3f8}
.cover-wrap.type-video{height:150px}
.cover-wrap.type-ppt{height:170px}
.comparison-cover{display:grid;width:100%;height:100%;grid-template-columns:1fr 1fr;gap:1px;background:#fff}
.comparison-pane{position:relative;min-width:0;height:100%;overflow:hidden}
.comparison-label{position:absolute;z-index:2;right:6px;bottom:6px;padding:3px 6px;border-radius:4px;color:#fff;background:rgba(18,24,43,.72);font-size:8px}
.ai-mark,.hot-mark,.type-mark{position:absolute;z-index:3;top:8px;padding:4px 7px;border-radius:4px;color:#fff;background:rgba(19,25,47,.72);font-size:8px}
.ai-mark,.hot-mark{left:8px}
.hot-mark{background:#f05a32}
.type-mark{right:8px}
.play-mark{position:absolute;z-index:3;top:50%;left:50%;display:grid;width:38px;height:38px;place-items:center;transform:translate(-50%,-50%);border-radius:50%;color:#fff;background:rgba(28,34,58,.68);font-size:14px}
.card-title,.prompt-summary{display:block}
.card-title{margin:10px 3px 0;color:#171c2d;font-size:14px;font-weight:700;line-height:20px}
.prompt-summary{height:34px;margin:4px 3px 0;overflow:hidden;color:#7c8497;font-size:10px;line-height:17px}
.card-foot{display:flex;margin:10px 3px 2px;align-items:center;justify-content:space-between;color:#8a92a5;font-size:9px}
.use-action{color:#4a6cff;font-weight:650}
</style>
