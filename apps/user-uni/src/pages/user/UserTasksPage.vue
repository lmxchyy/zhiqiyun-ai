<template>
  <GenerationTaskListPage v-if="enterpriseTaskCenter" />
  <view v-else class="tasks-page">
    <view class="safe-top" />
    <view class="page-header">
      <button class="back-button" aria-label="返回作品页" @click="backOrHome('/pages/user/UserAssetsPage')">‹</button>
      <view class="header-copy">
        <text class="page-title">全部任务</text>
        <text class="page-subtitle">优先展示生成中、排队中和失败任务</text>
      </view>
      <text class="count-badge">{{ total }}</text>
    </view>

    <view v-if="error && !items.length" class="state-card error">
      <text>{{ error }}</text>
      <button class="state-button" @click="refresh">重新加载</button>
    </view>
    <view v-else-if="loading && !items.length" class="state-card"><text>正在加载任务...</text></view>
    <view v-else-if="items.length" class="task-list">
      <button v-for="task in items" :key="task.id" class="task-card" @click="openTask(task)">
        <view :class="['task-icon', taskTone(task)]"><text>{{ taskSymbol(task) }}</text></view>
        <view class="task-copy">
          <text class="task-title">{{ taskTitle(task) }}</text>
          <text class="task-meta">{{ taskDescription(task) }}</text>
          <text class="task-time">{{ formatTime(task.createdAt) }}</text>
        </view>
        <text class="task-arrow">›</text>
      </button>
    </view>
    <view v-else class="state-card"><text>暂无生成任务</text></view>

    <view class="load-state">
      <text v-if="loadingMore">正在加载更多...</text>
      <button v-else-if="hasMore" @click="loadMore">加载更多</button>
      <text v-else-if="items.length">已加载全部 {{ total }} 条任务</text>
    </view>
    <view class="bottom-safe" />
  </view>
</template>

<script setup lang="ts">
import { ref } from "vue";
import { onHide, onLoad, onPullDownRefresh, onReachBottom, onShow, onUnload } from "@dcloudio/uni-app";
import type { GenerationTask } from "@xianzhi/shared-types";
import { businessSdk } from "../../api/client";
import { miniProgramFeaturePages } from "../../config/miniProgramPages";
import { backOrHome } from "../../utils/miniProgramBusiness";
import GenerationTaskListPage from "../../components/assets/GenerationTaskListPage.vue";
import { useAssetStore } from "../../stores/assets";

const enterpriseTaskCenter = true;
const enterpriseStore = useAssetStore();

const pageSize = 20;
const items = ref<GenerationTask[]>([]);
const total = ref(0);
const hasMore = ref(false);
const loading = ref(false);
const loadingMore = ref(false);
const error = ref("");

async function load(reset = false) {
  if ((reset && loading.value) || (!reset && (loadingMore.value || !hasMore.value))) return;
  if (reset) loading.value = true;
  else loadingMore.value = true;
  error.value = "";
  try {
    const offset = reset ? 0 : items.value.length;
    const page = await businessSdk.generation.listTaskPage({ limit: pageSize, offset, prioritizeActive: true });
    items.value = reset ? page.items : mergeTasks(items.value, page.items);
    total.value = page.total;
    hasMore.value = page.hasMore;
  } catch (loadError) {
    error.value = loadError instanceof Error ? loadError.message : "任务加载失败";
  } finally {
    loading.value = false;
    loadingMore.value = false;
  }
}

function mergeTasks(current: GenerationTask[], incoming: GenerationTask[]) {
  const byId = new Map(current.map(item => [item.id, item]));
  incoming.forEach(item => byId.set(item.id, item));
  return [...byId.values()];
}

function refresh() { return load(true); }
function loadMore() { return load(false); }
function normalizedStatus(task: GenerationTask) { return String(task.status || "PENDING").toUpperCase(); }

function taskTitle(task: GenerationTask) {
  const configured = String(task.params?.title || task.params?.name || "").trim();
  return configured || task.prompt?.trim().slice(0, 28) || "AI 创作任务";
}

function taskDescription(task: GenerationTask) {
  const status = normalizedStatus(task);
  const progress = Number(task.params?.progress || task.params?.percentage || 0);
  if (["FAILED", "ERROR"].includes(status)) return "失败 · 点击查看原因或重新生成";
  if (["PENDING", "QUEUED"].includes(status)) return "排队中 · 等待开始生成";
  if (["RUNNING", "PROCESSING", "RETRYING"].includes(status)) return progress > 0 ? `生成中 · ${Math.min(100, Math.round(progress))}%` : "生成中";
  if (["SUCCEEDED", "SUCCESS", "COMPLETED"].includes(status)) return "已完成 · 可查看作品";
  return status;
}

function taskTone(task: GenerationTask) {
  const status = normalizedStatus(task);
  if (["FAILED", "ERROR"].includes(status)) return "danger";
  if (["SUCCEEDED", "SUCCESS", "COMPLETED"].includes(status)) return "success";
  return "pending";
}

function taskSymbol(task: GenerationTask) {
  const type = String(task.type || "").toUpperCase();
  if (type.includes("VIDEO")) return "视";
  if (type.includes("PPT")) return "P";
  return ["FAILED", "ERROR"].includes(normalizedStatus(task)) ? "重" : "生";
}

function openTask(task: GenerationTask) {
  const assetId = task.resultIds?.[0];
  if (assetId && ["SUCCEEDED", "SUCCESS", "COMPLETED"].includes(normalizedStatus(task))) {
    uni.navigateTo({ url: `${miniProgramFeaturePages.userAssetDetail}?id=${encodeURIComponent(assetId)}` });
    return;
  }
  uni.showModal({
    title: taskTitle(task),
    content: taskDescription(task),
    showCancel: false,
  });
}

function formatTime(value?: string) {
  const date = value ? new Date(value) : null;
  if (!date || Number.isNaN(date.getTime())) return "时间未知";
  const pad = (part: number) => String(part).padStart(2, "0");
  return `${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}`;
}

onLoad(() => {
  if (enterpriseTaskCenter) void enterpriseStore.fetchTasks(true);
  else void load(true);
});
onShow(() => {
  if (!enterpriseTaskCenter) return;
  enterpriseStore.startTaskPolling();
  if (enterpriseStore.tasks.length) void enterpriseStore.fetchTasks(true);
});
onHide(() => { if (enterpriseTaskCenter) enterpriseStore.stopTaskPolling(); });
onUnload(() => { if (enterpriseTaskCenter) enterpriseStore.stopTaskPolling(); });
onReachBottom(() => {
  if (enterpriseTaskCenter) void enterpriseStore.loadMoreTasks();
  else void loadMore();
});
onPullDownRefresh(() => {
  const request = enterpriseTaskCenter ? enterpriseStore.fetchTasks(true) : refresh();
  void request.finally(() => uni.stopPullDownRefresh());
});
</script>

<style scoped>
.tasks-page { min-height: 100vh; box-sizing: border-box; padding: 0 16px calc(24px + env(safe-area-inset-bottom)); color: #1a1f2e; background: #f7f8fc; }
.safe-top { height: calc(12px + env(safe-area-inset-top)); }
.page-header { display: flex; min-height: 58px; align-items: center; gap: 12px; }
.back-button, .task-card, .state-button, .load-state button { margin: 0; border: 0; }
.back-button::after, .task-card::after, .state-button::after, .load-state button::after { display: none; }
.back-button { width: 36px; height: 36px; padding: 0; border-radius: 12px; color: #594db2; background: #fff; font-size: 28px; line-height: 34px; }
.header-copy { min-width: 0; flex: 1; }
.page-title, .page-subtitle { display: block; }
.page-title { font-size: 22px; font-weight: 700; line-height: 28px; }
.page-subtitle { margin-top: 2px; color: #6e758c; font-size: 11px; }
.count-badge { min-width: 32px; padding: 5px 9px; border-radius: 15px; color: #594db2; background: #fff; font-size: 12px; text-align: center; }
.task-list { display: flex; padding-top: 12px; flex-direction: column; gap: 10px; }
.task-card { display: flex; width: 100%; min-height: 76px; box-sizing: border-box; padding: 12px; align-items: center; gap: 10px; border: 1px solid #e3e5f0; border-radius: 14px; background: #fff; text-align: left; }
.task-icon { display: flex; width: 40px; height: 40px; flex: 0 0 40px; align-items: center; justify-content: center; border-radius: 11px; color: #594db2; background: #f0f2ff; font-size: 14px; font-weight: 700; }
.task-icon.success { color: #198c62; background: #eaf8f1; }
.task-icon.danger { color: #ff781c; background: #fff2e5; }
.task-copy { display: flex; min-width: 0; flex: 1; flex-direction: column; gap: 3px; }
.task-title, .task-meta, .task-time { display: block; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.task-title { font-size: 14px; font-weight: 600; }
.task-meta { color: #6e758c; font-size: 11px; }
.task-time { color: #9aa0b2; font-size: 10px; }
.task-arrow { color: #6e758c; font-size: 22px; }
.state-card { display: flex; min-height: 360px; flex-direction: column; align-items: center; justify-content: center; color: #6e758c; font-size: 12px; text-align: center; }
.state-card.error { color: #d84a42; }
.state-button { width: auto; height: 36px; margin-top: 12px; padding: 0 16px; border-radius: 18px; color: #fff; background: #7d8cf5; font-size: 12px; }
.load-state { display: flex; min-height: 72px; align-items: center; justify-content: center; color: #8b91a5; font-size: 11px; }
.load-state button { width: auto; height: 34px; padding: 0 18px; border-radius: 17px; color: #594db2; background: #fff; font-size: 11px; }
.bottom-safe { height: 16px; }
</style>
