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
      <view class="canvas-board" :style="boardStyle">
        <view
          v-for="(item, index) in visibleItems"
          :key="item.asset.id"
          class="canvas-record"
          :style="recordStyle(index)"
        >
          <view class="record-visual result-visual">
            <view class="result-pills">
              <text>{{ taskCount(item.task) }}张</text>
              <text>{{ statusLabel(item.task.status) }}</text>
            </view>
            <button
              type="button"
              class="record-media"
              :style="mediaStyle(item.asset)"
              :aria-label="`打开第 ${index + 1} 轮图片`"
              @click="openImage(item.asset)"
            >
              <image :src="item.asset.url" :alt="item.asset.name" mode="aspectFit" />
            </button>
            <view class="result-info-row">
              <text class="record-resolution">结果 {{ resultIndex(item.asset, index) }}  {{ assetResolution(item.asset) }}</text>
              <view class="result-quick-actions" aria-label="图片快捷操作">
                <button type="button" aria-label="加入编辑" @click="$emit('edit', item.asset)">
                  <text aria-hidden="true">✧</text>
                  <text class="action-tooltip">加入编辑</text>
                </button>
                <button type="button" aria-label="下载" @click="downloadImage(item.asset)">
                  <text aria-hidden="true">⇩</text>
                  <text class="action-tooltip">下载</text>
                </button>
                <button type="button" aria-label="发布到画廊" @click="shareImage(item.asset)">
                  <text aria-hidden="true">⌯</text>
                  <text class="action-tooltip">发布到画廊</text>
                </button>
              </view>
            </view>
            <view class="result-bottom-actions">
              <button type="button" class="regenerate" :aria-label="`复用第 ${index + 1} 个结果重新生成`" @click="$emit('reuse', item.task)">
                <text aria-hidden="true">↻</text>
                <text>全部重新生成</text>
              </button>
              <button type="button" class="delete-icon" :aria-label="`删除第 ${index + 1} 个结果`" @click="$emit('delete', item.asset)">⌫</button>
              <button type="button" class="close-icon" :aria-label="`关闭第 ${index + 1} 个结果`" @click="closeAsset(item.asset)">×</button>
            </view>
          </view>
        </view>
        <view
          v-if="pendingCount > 0"
          class="canvas-record pending-record"
          :style="recordStyle(visibleItems.length)"
          aria-live="polite"
        >
          <view class="pending-visual">
            <view class="pending-pills">
              <text>{{ pendingCount }}张</text>
              <text>处理中</text>
            </view>
            <view class="pending-card">
              <text class="pending-title">正在创建图片</text>
              <view class="pending-dot-field" aria-hidden="true"></view>
            </view>
          </view>
        </view>
      </view>
    </scroll-view>
    <view v-if="!visibleItems.length && pendingCount <= 0" class="canvas-empty">
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
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from "vue";
import type { Asset, GenerationTask, TaskStatus } from "../types";

const props = defineProps<{
  items: Array<{ task: GenerationTask; asset: Asset }>;
  historyItems: Array<{ task: GenerationTask; asset: Asset }>;
  historyCount: number;
  showingHistory: boolean;
  quota: number;
  runningCount: number;
  pendingCount: number;
}>();

const emit = defineEmits<{
  reuse: [task: GenerationTask];
  delete: [asset: Asset];
  edit: [asset: Asset];
  "select-history": [item: { task: GenerationTask; asset: Asset }];
  "restore-history": [];
  "new-canvas": [];
  "clear-canvas": [];
}>();

const isNarrow = ref(false);
const isHistoryPanelOpen = ref(false);
const viewportWidth = ref(1200);
const hiddenAssetIds = ref(new Set<string>());
const maxHistoryItems = 20;
const visibleHistoryItems = computed(() => props.historyItems.slice(-maxHistoryItems).reverse());
const visibleItems = computed(() => props.items.filter(item => !hiddenAssetIds.value.has(item.asset.id)));
const hiddenHistoryCount = computed(() => Math.max(0, props.historyItems.length - maxHistoryItems));
const isMedium = computed(() => viewportWidth.value <= 1100);
const renderCount = computed(() => visibleItems.value.length + (props.pendingCount > 0 ? 1 : 0));
const rowGap = computed(() => {
  if (isNarrow.value) return 500;
  if (!renderCount.value) return 360;
  return Math.max(430, Math.round(Math.min(230, viewportWidth.value - 320)) + 200);
});
const boardWidth = computed(() => {
  if (typeof window === "undefined") return 1200;
  if (isNarrow.value) return viewportWidth.value;
  if (isMedium.value) return Math.max(720, viewportWidth.value);
  return Math.max(1180, viewportWidth.value - 72);
});
const recordX = computed(() => {
  if (isNarrow.value) return Math.round(viewportWidth.value * 0.04);
  if (renderCount.value) return 40;
  if (isMedium.value) return Math.max(32, Math.round((boardWidth.value - Math.min(820, viewportWidth.value - 96)) / 2));
  return Math.max(42, Math.round(boardWidth.value * 0.1));
});
const scrollTop = ref(0);
const boardHeight = computed(() => {
  if (!renderCount.value) return 1;
  return Math.max(isNarrow.value ? 900 : 1100, renderCount.value * rowGap.value + 260);
});
const boardStyle = computed(() => {
  if (isNarrow.value) return undefined;
  return { width: `${boardWidth.value}px`, height: `${boardHeight.value}px` };
});

function recordStyle(index: number) {
  if (isNarrow.value) return undefined;
  return { transform: `translate(${recordX.value}px, ${120 + index * rowGap.value}px)` };
}

function updateViewport() {
  if (typeof window === "undefined") {
    viewportWidth.value = 1200;
  } else {
    const documentWidth = document.documentElement?.clientWidth || window.innerWidth;
    viewportWidth.value = Math.min(window.innerWidth, documentWidth);
  }
  isNarrow.value = viewportWidth.value <= 760;
}

function selectHistory(item: { task: GenerationTask; asset: Asset }) {
  isHistoryPanelOpen.value = false;
  emit("select-history", item);
}

function openImage(asset: Asset) {
  if (!asset.url) return;
  uni.previewImage({
    urls: visibleItems.value.map(item => item.asset.url).filter(Boolean),
    current: asset.url
  });
}

function assetResolution(asset: Asset) {
  const metadata = asset.metadata || {};
  const resolution = metadata.resolution;
  if (typeof resolution === "string" && resolution.trim()) return resolution;
  const width = Number(metadata.width);
  const height = Number(metadata.height);
  if (Number.isFinite(width) && Number.isFinite(height) && width > 0 && height > 0) {
    return `${width}x${height}`;
  }
  return "未知";
}

function assetDimensions(asset: Asset) {
  const width = Number(asset.metadata?.width);
  const height = Number(asset.metadata?.height);
  if (Number.isFinite(width) && Number.isFinite(height) && width > 0 && height > 0) {
    return { width, height };
  }
  const resolution = typeof asset.metadata?.resolution === "string" ? asset.metadata.resolution : "";
  const match = resolution.match(/(\d+)\s*x\s*(\d+)/i);
  if (match) {
    return { width: Number(match[1]), height: Number(match[2]) };
  }
  return { width: 1, height: 1 };
}

function mediaStyle(asset: Asset) {
  const { width, height } = assetDimensions(asset);
  return { aspectRatio: `${width} / ${height}` };
}

function taskCount(task: GenerationTask) {
  const value = Number(task.params?.count);
  return Number.isFinite(value) && value > 0 ? value : task.resultIds?.length || 1;
}

function resultIndex(asset: Asset, fallbackIndex: number) {
  const value = Number(asset.metadata?.index);
  return Number.isFinite(value) && value > 0 ? value : fallbackIndex + 1;
}

function downloadImage(asset: Asset) {
  if (!asset.url || typeof document === "undefined") return;
  const link = document.createElement("a");
  link.href = asset.url;
  link.download = `${asset.name || asset.id || "image"}.png`;
  link.target = "_blank";
  link.rel = "noopener noreferrer";
  document.body.appendChild(link);
  link.click();
  link.remove();
}

async function shareImage(asset: Asset) {
  if (!asset.url) return;
  try {
    if (typeof navigator !== "undefined" && navigator.clipboard) {
      await navigator.clipboard.writeText(asset.url);
      uni.showToast({ title: "链接已复制", icon: "success" });
      return;
    }
  } catch {
    // Fall through to opening the image if clipboard is unavailable.
  }
  openImage(asset);
}

function closeAsset(asset: Asset) {
  hiddenAssetIds.value = new Set([...hiddenAssetIds.value, asset.id]);
}

onMounted(() => {
  updateViewport();
  window.addEventListener("resize", updateViewport);
});

onBeforeUnmount(() => {
  window.removeEventListener("resize", updateViewport);
});

watch(
  () => [visibleItems.value.length, props.pendingCount, boardHeight.value],
  () => {
    void nextTick(() => {
      scrollTop.value = boardHeight.value;
    });
  },
  { immediate: true }
);

watch(
  () => props.items.map(item => item.asset.id),
  ids => {
    hiddenAssetIds.value = new Set([...hiddenAssetIds.value].filter(id => ids.includes(id)));
  }
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
