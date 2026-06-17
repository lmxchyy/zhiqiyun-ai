<template>
    <view class="creation-canvas">
    <view class="canvas-grid"></view>
    <view class="canvas-toolbar" role="toolbar" aria-label="画布操作">
      <button type="button" class="history-button" aria-label="打开历史对话" @click="isHistoryPanelOpen = !isHistoryPanelOpen">
        <text class="button-icon" aria-hidden="true">↺</text>
        <text>历史对话</text>
        <text class="button-count">{{ historyCount }}</text>
      </button>
      <button type="button" class="new-button" aria-label="新建空白画布" @click="$emit('new-canvas')">
        <text class="button-icon" aria-hidden="true">＋</text>
        <text>新建</text>
      </button>
      <button type="button" class="icon-button clear-history-button" aria-label="清空历史" @click="$emit('clear-canvas')">
        <text class="trash-icon" aria-hidden="true">
          <text class="trash-lid"></text>
          <text class="trash-can"></text>
        </text>
      </button>
    </view>
    <view class="canvas-stats" aria-label="画布状态">
      <text class="stat-pill">额度 <text>{{ quota }}</text></text>
      <text class="stat-pill">运行 <text>{{ runningCount }}</text></text>
    </view>
    <view v-if="isHistoryPanelOpen && historyItems.length" class="canvas-history-panel">
      <view class="history-panel-head">
        <text>历史对话</text>
        <text>{{ historyItems.length }}</text>
      </view>
      <button
        v-for="(item, index) in visibleHistoryItems"
        :key="item.asset.id"
        type="button"
        class="history-card"
        :aria-label="`显示第 ${index + 1} 条历史对话`"
        @click="selectHistory(item)"
      >
        <text class="history-title">{{ item.task.prompt }}</text>
        <text class="history-meta">{{ item.task.model || "mock-standard" }} · {{ item.task.pointCost || 0 }} 积分</text>
        <text :class="['history-status', { failed: item.task.status === 'FAILED' }]">
          {{ statusLabel(item.task.status) }}
        </text>
      </button>
      <text v-if="hiddenHistoryCount > 0" class="history-more">还有 {{ hiddenHistoryCount }} 条历史，可在我的作品查看</text>
    </view>
    <scroll-view class="canvas-viewport" scroll-y :scroll-top="scrollTop">
      <view class="canvas-board" :style="{ width: boardWidth + 'px', height: boardHeight + 'px' }">
        <view
          v-for="(item, index) in items"
          :key="item.asset.id"
          class="canvas-record"
          :style="{ transform: `translate(${recordX}px, ${120 + index * rowGap}px)` }"
        >
          <view class="record-media">
            <image :src="item.asset.url" :alt="item.asset.name" mode="aspectFit" />
          </view>
          <view class="record-meta">
            <view class="record-head">
              <text>第 {{ index + 1 }} 轮</text>
              <text>{{ typeLabel(item.task.type) }}</text>
              <text :class="['status', { failed: item.task.status === 'FAILED' }]">
                {{ statusLabel(item.task.status) }}
              </text>
              <text>{{ formatTime(item.task.workerFinishedAt || item.task.updatedAt || item.task.createdAt) }}</text>
            </view>
            <text class="prompt-text">{{ item.task.prompt }}</text>
            <view class="record-actions">
              <button type="button" :aria-label="`复用第 ${index + 1} 轮配置`" @click="$emit('reuse', item.task)">复用配置</button>
              <button type="button" class="delete" :aria-label="`删除第 ${index + 1} 轮图片`" @click="$emit('delete', item.asset)">删除</button>
            </view>
          </view>
        </view>
      </view>
    </scroll-view>
    <view v-if="!items.length" class="canvas-empty">
      <view class="empty-rule-row">
        <text class="empty-rule"></text>
        <text class="empty-kicker">GENERATIVE · ATELIER</text>
        <text class="empty-rule"></text>
      </view>
      <text class="empty-title">Turn ideas into images</text>
      <text class="empty-copy">
        {{ showingHistory ? "在同一窗口里保留本地历史与任务状态，并从已有结果图继续发起新的无状态编辑。" : "在同一窗口里保留本地历史与任务状态，并从已有结果图继续发起新的无状态编辑。" }}
      </text>
      <view class="empty-flow-row">
        <text>01</text>
        <text class="empty-rule"></text>
        <text class="empty-flow">SKETCH → RENDER</text>
        <text class="empty-rule"></text>
        <text>02</text>
      </view>
    </view>
  </view>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import type { Asset, GenerationTask, TaskStatus } from "../types";

const props = defineProps<{
  items: Array<{ task: GenerationTask; asset: Asset }>;
  historyItems: Array<{ task: GenerationTask; asset: Asset }>;
  historyCount: number;
  showingHistory: boolean;
  quota: number;
  runningCount: number;
}>();

const emit = defineEmits<{
  reuse: [task: GenerationTask];
  delete: [asset: Asset];
  "select-history": [item: { task: GenerationTask; asset: Asset }];
  "restore-history": [];
  "new-canvas": [];
  "clear-canvas": [];
}>();

const isNarrow = ref(false);
const isHistoryPanelOpen = ref(false);
const viewportWidth = ref(1200);
const maxHistoryItems = 20;
const visibleHistoryItems = computed(() => props.historyItems.slice(-maxHistoryItems).reverse());
const hiddenHistoryCount = computed(() => Math.max(0, props.historyItems.length - maxHistoryItems));
const isMedium = computed(() => viewportWidth.value <= 1100);
const rowGap = computed(() => (isNarrow.value ? 500 : props.items.length ? 520 : 360));
const boardWidth = computed(() => {
  if (typeof window === "undefined") return 1200;
  if (isNarrow.value) return Math.max(360, viewportWidth.value);
  if (isMedium.value) return Math.max(720, viewportWidth.value);
  return Math.max(1180, viewportWidth.value - 72);
});
const recordX = computed(() => {
  if (isNarrow.value) return Math.round(viewportWidth.value * 0.04);
  if (props.items.length) return 40;
  if (isMedium.value) return Math.max(32, Math.round((boardWidth.value - Math.min(820, viewportWidth.value - 96)) / 2));
  return Math.max(42, Math.round(boardWidth.value * 0.1));
});
const scrollTop = ref(0);
const boardHeight = computed(() => {
  if (!props.items.length) return 1;
  return Math.max(isNarrow.value ? 900 : 1100, props.items.length * rowGap.value + 260);
});

function updateViewport() {
  viewportWidth.value = typeof window !== "undefined" ? window.innerWidth : 1200;
  isNarrow.value = viewportWidth.value <= 760;
}

function selectHistory(item: { task: GenerationTask; asset: Asset }) {
  isHistoryPanelOpen.value = false;
  emit("select-history", item);
}

onMounted(() => {
  updateViewport();
  window.addEventListener("resize", updateViewport);
});

onBeforeUnmount(() => {
  window.removeEventListener("resize", updateViewport);
});

watch(
  () => props.items.length,
  () => {
    scrollTop.value = 0;
  },
  { immediate: true }
);

function typeLabel(type: GenerationTask["type"]) {
  return (
    {
      TEXT_TO_IMAGE: "画图",
      IMAGE_TO_IMAGE: "编辑图",
      TEXT_TO_VIDEO: "视频",
      IMAGE_TO_VIDEO: "图生视频"
    } as const
  )[type];
}

function statusLabel(status: TaskStatus) {
  return (
    {
      SUCCEEDED: "已完成",
      FAILED: "失败",
      CANCELLED: "已取消",
      QUEUED: "排队中",
      PROCESSING: "生成中",
      RETRYING: "重试中"
    } as const
  )[status] || status;
}

function formatTime(value?: string) {
  const d = new Date(value || Date.now());
  return `${String(d.getMonth() + 1).padStart(2, "0")}/${String(d.getDate()).padStart(2, "0")} ${String(
    d.getHours()
  ).padStart(2, "0")}:${String(d.getMinutes()).padStart(2, "0")}`;
}
</script>
