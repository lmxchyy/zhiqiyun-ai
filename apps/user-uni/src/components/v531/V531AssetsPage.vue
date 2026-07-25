<template>
  <view class="works-page" :style="miniProgramNavigationStyle">
    <view class="works-header">
      <text class="works-title">作品</text>
      <view class="works-actions">
        <button class="header-action search-action" aria-label="搜索作品" @click="searchVisible = !searchVisible">
          <image class="header-action-icon" src="/static/icons/search.svg" mode="aspectFit" />
        </button>
        <button class="header-action" @click="showFilter">筛选</button>
        <button class="header-action" @click="$emit('batch')">管理</button>
      </view>
    </view>

    <view v-if="searchVisible" class="search-panel">
      <input v-model="query" class="asset-search" focus confirm-type="search" placeholder="搜索作品名称" />
      <button v-if="query" class="search-clear" aria-label="清空搜索" @click="query = ''">×</button>
    </view>

    <view class="overview-card">
      <view class="metric-item">
        <text class="metric-value">{{ total }}</text>
        <text class="metric-label">全部</text>
      </view>
      <view class="metric-item">
        <text class="metric-value">{{ monthTotal }}</text>
        <text class="metric-label">本月</text>
      </view>
      <view class="metric-item">
        <text class="metric-value">{{ favoriteTotal }}</text>
        <text class="metric-label">收藏</text>
      </view>
      <view class="metric-item">
        <text class="metric-value">{{ storageLabel }}</text>
        <text class="metric-label">空间</text>
      </view>
    </view>

    <view class="section-heading">
      <text class="section-title">作品类型</text>
      <button class="section-link" @click="category = 'all'">全部类型 <text class="link-arrow">›</text></button>
    </view>
    <view class="filter-card type-filter-card">
      <button
        v-for="item in categories"
        :key="item.id"
        :class="['filter-chip', { active: category === item.id }]"
        @click="category = item.id"
      >
        {{ item.label }}
      </button>
    </view>

    <view class="section-heading single">
      <text class="section-title">状态筛选</text>
    </view>
    <scroll-view scroll-x class="filter-card status-filter-card" :show-scrollbar="false">
      <view class="status-filter-row">
        <button
          v-for="item in statuses"
          :key="item.id"
          :class="['filter-chip', { active: status === item.id }]"
          @click="status = item.id"
        >
          {{ item.label }}
        </button>
      </view>
    </scroll-view>

    <view class="section-heading works-heading">
      <text class="section-title">最近作品</text>
      <button class="section-link" @click="$emit('all-assets')">查看全部 <text class="link-arrow">›</text></button>
    </view>
    <view class="asset-grid-wrap">
      <view v-if="loading" class="state-card"><text>正在加载作品...</text></view>
      <view v-else-if="error" class="state-card error">
        <text>{{ error }}</text>
        <button class="state-action" @click="$emit('retry')">重新加载</button>
      </view>
      <view v-else-if="visibleAssets.length" class="asset-grid">
        <button
          v-for="(asset, index) in visibleAssets"
          :key="asset.id"
          class="asset-card"
          @click="$emit('open', asset)"
        >
          <AppImage
            v-if="asset.thumbnailUrl"
            class="asset-cover"
            :src="asset.thumbnailUrl"
            :fallback="fallbackFor(asset, index)"
            :alt="asset.name"
            width="100%"
            height="34px"
            radius="12px"
          />
          <view v-else :class="['asset-cover', 'symbol-cover', typeTone(asset, index)]">
            <text>{{ symbolFor(asset, index) }}</text>
          </view>
          <text class="asset-name">{{ asset.name || '未命名作品' }}</text>
          <view class="asset-meta">
            <text>{{ labelFor(asset, index) }}</text>
            <text :class="['asset-status', statusTone(asset)]">{{ statusLabel(asset) }}</text>
          </view>
        </button>
      </view>
      <view v-else class="empty-state">
        <image class="empty-icon" src="/static/icons/assets-empty.svg" mode="aspectFit" />
        <text class="empty-title">暂无符合条件的作品</text>
        <text class="empty-description">完成一次 AI 创作后，成果会自动保存在这里。</text>
        <button class="state-action" @click="$emit('create')">开始创作</button>
      </view>
    </view>

    <view class="section-heading task-heading">
      <text class="section-title">最近任务</text>
      <button class="section-link" @click="$emit('tasks')">查看全部 <text class="link-arrow">›</text></button>
    </view>
    <view v-if="recentTasks.length" class="task-list">
      <button v-for="task in recentTasks" :key="task.id" class="task-card" @click="$emit('tasks')">
        <view :class="['task-icon', taskTone(task)]"><text>{{ taskSymbol(task) }}</text></view>
        <view class="task-copy">
          <text class="task-title">{{ taskTitle(task) }}</text>
          <text class="task-meta">{{ taskMeta(task) }}</text>
        </view>
        <text class="task-arrow">›</text>
      </button>
    </view>
    <view v-else class="task-empty">
      <text>暂无生成任务，新的任务会在这里显示。</text>
    </view>
    <view class="bottom-safe" />
  </view>
</template>

<script setup lang="ts">
import { computed, ref } from "vue";
import AppImage from "../AppImage.vue";
import { v531SlotsByPage } from "../../config/v531";

interface AssetLike {
  id: string;
  name: string;
  thumbnailUrl?: string;
  mediaType?: string;
  status?: string;
  createdAt?: string;
  metadata?: Record<string, unknown>;
}

interface TaskLike {
  id: string;
  type?: string;
  status?: string;
  prompt?: string;
  params?: Record<string, unknown>;
  createdAt?: string;
}

type AssetCategory = "all" | "image" | "video" | "ppt" | "document" | "agent" | "infographic" | "knowledge";
type AssetStatus = "recent" | "processing" | "completed" | "failed" | "favorite";

const props = withDefaults(
  defineProps<{
    assets?: AssetLike[];
    tasks?: TaskLike[];
    loading?: boolean;
    error?: string;
    total?: number;
    monthTotal?: number;
    favoriteTotal?: number;
    storageLabel?: string;
  }>(),
  { assets: () => [], tasks: () => [], loading: false, error: "", total: 0, monthTotal: 0, favoriteTotal: 0, storageLabel: "0%" },
);

defineEmits<{
  open: [asset: AssetLike];
  batch: [];
  retry: [];
  create: [];
  "all-assets": [];
  tasks: [];
}>();

const query = ref("");
const searchVisible = ref(false);
const category = ref<AssetCategory>("all");
const status = ref<AssetStatus>("recent");

const categories: Array<{ id: AssetCategory; label: string }> = [
  { id: "all", label: "全部" },
  { id: "image", label: "图片" },
  { id: "video", label: "视频" },
  { id: "ppt", label: "PPT" },
  { id: "document", label: "文档" },
  { id: "agent", label: "Agent" },
  { id: "infographic", label: "信息图" },
  { id: "knowledge", label: "知识库" },
];

const statuses: Array<{ id: AssetStatus; label: string }> = [
  { id: "recent", label: "最近" },
  { id: "processing", label: "生成中" },
  { id: "completed", label: "已完成" },
  { id: "failed", label: "失败" },
  { id: "favorite", label: "收藏" },
];

const safeAssets = computed(() => Array.isArray(props.assets) ? props.assets : []);
const safeTasks = computed(() => Array.isArray(props.tasks) ? props.tasks : []);
const favoriteTotal = computed(() => props.favoriteTotal || safeAssets.value.filter(isFavorite).length);
const latestAssets = computed(() => [...safeAssets.value]
  .sort((left, right) => compareCreatedAt(right.createdAt, left.createdAt))
  .slice(0, 4));
const recentTasks = computed(() => [...safeTasks.value]
  .sort((left, right) => {
    const priorityDifference = taskPriority(left) - taskPriority(right);
    return priorityDifference || compareCreatedAt(right.createdAt, left.createdAt);
  })
  .slice(0, 5));
const visibleAssets = computed(() => {
  const keyword = query.value.trim().toLowerCase();
  return latestAssets.value.filter((asset, index) => {
    const queryMatched = !keyword || asset.name.toLowerCase().includes(keyword);
    const categoryMatched = category.value === "all" || normalizedType(asset, index) === category.value;
    const assetStatus = normalizedStatus(asset);
    const statusMatched = status.value === "recent"
      || (status.value === "favorite" ? isFavorite(asset) : assetStatus === status.value);
    return queryMatched && categoryMatched && statusMatched;
  });
});

function normalizedType(asset: AssetLike, index = 0): AssetCategory {
  const value = String(asset.metadata?.type || asset.metadata?.mediaType || asset.mediaType || "").toLowerCase();
  if (value.includes("video")) return "video";
  if (value.includes("ppt") || value.includes("presentation")) return "ppt";
  if (value.includes("agent")) return "agent";
  if (value.includes("infographic") || value.includes("long_image") || value.includes("long-image")) return "infographic";
  if (value.includes("knowledge")) return "knowledge";
  if (value.includes("document") || value.includes("doc") || value.includes("pdf") || value.includes("text")) return "document";
  if (value.includes("image")) return "image";
  return index === 1 ? "video" : index === 2 ? "ppt" : "image";
}

function normalizedStatus(asset: AssetLike): Exclude<AssetStatus, "recent" | "favorite"> {
  const value = String(asset.status || asset.metadata?.status || asset.metadata?.taskStatus || "COMPLETED").toUpperCase();
  if (["FAILED", "ERROR", "CANCELLED"].some(item => value.includes(item))) return "failed";
  if (["PENDING", "QUEUED", "RUNNING", "PROCESSING", "RETRYING", "GENERATING"].some(item => value.includes(item))) return "processing";
  return "completed";
}

function isFavorite(asset: AssetLike) {
  const value = asset.metadata?.favorite ?? asset.metadata?.isFavorite ?? asset.metadata?.starred;
  return value === true || value === 1 || String(value).toLowerCase() === "true";
}

function showFilter() {
  uni.showActionSheet({
    itemList: statuses.map(item => item.label),
    success: result => { status.value = statuses[result.tapIndex]?.id || "recent"; },
  });
}

function fallbackFor(asset: AssetLike, index: number) {
  const type = normalizedType(asset, index);
  const slotType = type === "video" ? "video" : type === "ppt" ? "ppt" : type === "infographic" ? "long_image" : "image";
  return v531SlotsByPage.assets[`assets.cover.${slotType}`]?.fallbackUrl || "/static/fallbacks/default-cover.jpg";
}

function labelFor(asset: AssetLike, index: number) {
  return ({
    image: "图片",
    video: "视频",
    ppt: "PPT",
    document: "文档",
    agent: "Agent",
    infographic: "信息图",
    knowledge: "知识库",
    all: "作品",
  } as Record<AssetCategory, string>)[normalizedType(asset, index)];
}

function symbolFor(asset: AssetLike, index: number) {
  return ({ image: "图", video: "▶", ppt: "P", document: "文", agent: "A", infographic: "表", knowledge: "知", all: "作" } as Record<AssetCategory, string>)[normalizedType(asset, index)];
}

function typeTone(asset: AssetLike, index: number) {
  const type = normalizedType(asset, index);
  if (["video", "infographic"].includes(type)) return "orange";
  if (["agent", "knowledge"].includes(type)) return "green";
  return "purple";
}

function statusLabel(asset: AssetLike) {
  const value = normalizedStatus(asset);
  return value === "processing" ? "生成中" : value === "failed" ? "失败" : "已完成";
}

function statusTone(asset: AssetLike) {
  const value = normalizedStatus(asset);
  return value === "failed" ? "failed" : value === "processing" ? "processing" : "completed";
}

function taskStatus(task: TaskLike) {
  return String(task.status || "PENDING").toUpperCase();
}

function taskPriority(task: TaskLike) {
  const value = taskStatus(task);
  return ["PENDING", "QUEUED", "RUNNING", "PROCESSING", "RETRYING", "FAILED", "ERROR"].includes(value) ? 0 : 1;
}

function compareCreatedAt(left?: string, right?: string) {
  const leftTime = Date.parse(String(left || ""));
  const rightTime = Date.parse(String(right || ""));
  const normalizedLeft = Number.isFinite(leftTime) ? leftTime : 0;
  const normalizedRight = Number.isFinite(rightTime) ? rightTime : 0;
  return normalizedLeft - normalizedRight;
}

function taskTitle(task: TaskLike) {
  const configuredTitle = String(task.params?.title || task.params?.name || "").trim();
  if (configuredTitle) return configuredTitle;
  if (task.prompt?.trim()) return task.prompt.trim().slice(0, 18);
  const type = String(task.type || "").toUpperCase();
  if (type.includes("VIDEO")) return "视频生成任务";
  if (type.includes("PPT")) return "PPT 生成任务";
  return "AI 创作任务";
}

function taskSymbol(task: TaskLike) {
  const type = String(task.type || "").toUpperCase();
  if (type.includes("VIDEO")) return "视";
  if (type.includes("PPT")) return "P";
  return taskStatus(task).includes("FAILED") ? "重" : "生";
}

function taskTone(task: TaskLike) {
  const value = taskStatus(task);
  if (["FAILED", "ERROR", "CANCELLED"].some(item => value.includes(item))) return "danger";
  if (["SUCCEEDED", "SUCCESS", "COMPLETED"].some(item => value.includes(item))) return "success";
  return "pending";
}

function taskMeta(task: TaskLike) {
  const value = taskStatus(task);
  const progress = Number(task.params?.progress || task.params?.percentage || 0);
  if (["FAILED", "ERROR", "CANCELLED"].some(item => value.includes(item))) return "失败 · 再次生成";
  if (["SUCCEEDED", "SUCCESS", "COMPLETED"].some(item => value.includes(item))) return "已完成 · 可下载";
  return progress > 0 ? `生成中 · ${Math.min(100, Math.round(progress))}%` : "生成中";
}
</script>

<style scoped>
.works-page {
  min-height: 100vh;
  box-sizing: border-box;
  padding: calc(16px + var(--header-padding-top, 20px)) 16px calc(104px + env(safe-area-inset-bottom));
  color: #1a1f2e;
  background: #f7f8fc;
  font-family: -apple-system, BlinkMacSystemFont, "PingFang SC", "Microsoft YaHei", "Noto Sans SC", sans-serif;
}
.works-header {
  display: flex;
  height: 48px;
  padding-right: var(--capsule-right-space, 0px);
  box-sizing: border-box;
  align-items: flex-start;
  justify-content: space-between;
}
.works-title {
  font-size: 26px;
  font-weight: 700;
  line-height: 31px;
}
.works-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}
.header-action,
.section-link,
.filter-chip,
.asset-card,
.task-card,
.state-action,
.search-clear {
  margin: 0;
  border: 0;
}
.header-action::after,
.section-link::after,
.filter-chip::after,
.asset-card::after,
.task-card::after,
.state-action::after,
.search-clear::after {
  display: none;
}
.header-action {
  width: auto;
  height: 24px;
  padding: 0;
  color: #594db2;
  background: transparent;
  font-size: 13px;
  font-weight: 500;
  line-height: 24px;
}
.search-action {
  display: flex;
  width: 18px;
  align-items: center;
  justify-content: center;
}
.header-action-icon {
  width: 16px;
  height: 16px;
}
.search-panel {
  position: relative;
  margin-bottom: 12px;
}
.asset-search {
  width: 100%;
  height: 42px;
  box-sizing: border-box;
  padding: 0 42px 0 14px;
  border: 1px solid #e3e5f0;
  border-radius: 14px;
  background: #fff;
  font-size: 13px;
}
.search-clear {
  position: absolute;
  top: 5px;
  right: 5px;
  width: 32px;
  height: 32px;
  padding: 0;
  color: #6e758c;
  background: transparent;
  font-size: 20px;
  line-height: 32px;
}
.overview-card {
  display: grid;
  height: 96px;
  box-sizing: border-box;
  padding: 10px;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 8px;
  border: 1px solid #e3e5f0;
  border-radius: 16px;
  background: #fff;
}
.metric-item {
  display: flex;
  min-width: 0;
  padding: 4px;
  flex-direction: column;
  gap: 4px;
}
.metric-value,
.metric-label {
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.metric-value {
  font-size: 18px;
  font-weight: 700;
  line-height: 22px;
}
.metric-label {
  color: #6e758c;
  font-size: 11px;
  line-height: 13px;
}
.section-heading {
  display: flex;
  height: 24px;
  margin-top: 12px;
  align-items: flex-start;
  justify-content: space-between;
}
.section-heading.single {
  margin-top: 12px;
}
.section-title {
  font-size: 16px;
  font-weight: 700;
  line-height: 19px;
}
.section-link {
  width: auto;
  height: 24px;
  padding: 0;
  color: #7d8cf5;
  background: transparent;
  font-size: 12px;
  font-weight: 500;
  line-height: 18px;
}
.link-arrow {
  font-size: 16px;
  line-height: 18px;
}
.filter-card {
  box-sizing: border-box;
  border-radius: 14px;
  background: #fff;
}
.type-filter-card {
  display: flex;
  min-height: 82px;
  padding: 8px;
  align-content: flex-start;
  align-items: flex-start;
  flex-wrap: wrap;
  gap: 6px 8px;
}
.status-filter-card {
  height: 44px;
  padding: 6px;
  white-space: nowrap;
}
.status-filter-row {
  display: flex;
  width: max-content;
  gap: 6px;
}
.filter-chip {
  width: auto;
  height: 29px;
  box-sizing: border-box;
  padding: 0 12px;
  border-radius: 999px;
  color: #6e758c;
  background: #f7f8fc;
  font-size: 12px;
  font-weight: 500;
  line-height: 29px;
}
.filter-chip.active {
  color: #fff;
  background: #7d8cf5;
}
.works-heading {
  margin-top: 12px;
}
.asset-grid-wrap {
  min-height: 0;
}
.asset-grid {
  display: grid;
  padding-top: 12px;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px 14px;
}
.asset-card {
  display: flex;
  min-width: 0;
  height: 104px;
  box-sizing: border-box;
  padding: 10px;
  flex-direction: column;
  align-items: stretch;
  gap: 8px;
  overflow: hidden;
  border: 1px solid #e3e5f0;
  border-radius: 16px;
  background: #fff;
  text-align: left;
}
.asset-cover {
  display: block;
  width: 100%;
  height: 34px;
  overflow: hidden;
  border-radius: 12px;
  flex: 0 0 34px;
}
.symbol-cover {
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 28px;
  font-weight: 700;
  line-height: 34px;
}
.symbol-cover.purple {
  color: #594db2;
  background: #f0f2ff;
}
.symbol-cover.orange {
  color: #ff781c;
  background: #fff2e5;
}
.symbol-cover.green {
  color: #198c62;
  background: #eaf8f1;
}
.asset-name {
  display: block;
  overflow: hidden;
  font-size: 13px;
  font-weight: 500;
  line-height: 16px;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.asset-meta {
  display: flex;
  height: 18px;
  align-items: flex-start;
  justify-content: space-between;
  color: #6e758c;
  font-size: 11px;
  line-height: 13px;
}
.asset-status.completed,
.asset-status.processing {
  color: #7d8cf5;
}
.asset-status.failed {
  color: #ff781c;
}
.state-card,
.empty-state {
  display: flex;
  min-height: 220px;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  color: #6e758c;
  font-size: 13px;
  text-align: center;
}
.state-card.error {
  color: #d84a42;
}
.state-action {
  width: auto;
  height: 36px;
  margin-top: 14px;
  padding: 0 18px;
  border-radius: 18px;
  color: #fff;
  background: #7d8cf5;
  font-size: 12px;
  line-height: 36px;
}
.empty-icon {
  width: 54px;
  height: 54px;
}
.empty-title,
.empty-description {
  display: block;
}
.empty-title {
  margin-top: 10px;
  color: #1a1f2e;
  font-size: 15px;
  font-weight: 600;
}
.empty-description {
  max-width: 250px;
  margin-top: 6px;
  font-size: 11px;
  line-height: 18px;
}
.task-heading {
  margin-top: 12px;
}
.task-list {
  display: flex;
  padding-top: 12px;
  flex-direction: column;
  gap: 12px;
}
.task-card {
  display: flex;
  width: 100%;
  height: 64px;
  box-sizing: border-box;
  padding: 12px;
  align-items: center;
  gap: 10px;
  border: 1px solid #e3e5f0;
  border-radius: 14px;
  background: #fff;
  text-align: left;
}
.task-icon {
  display: flex;
  width: 36px;
  height: 36px;
  flex: 0 0 36px;
  align-items: center;
  justify-content: center;
  border-radius: 10px;
  color: #594db2;
  background: #f0f2ff;
  font-size: 14px;
  font-weight: 700;
}
.task-icon.success {
  color: #198c62;
  background: #eaf8f1;
}
.task-icon.danger {
  color: #ff781c;
  background: #fff2e5;
}
.task-copy {
  display: flex;
  min-width: 0;
  flex: 1;
  flex-direction: column;
  gap: 3px;
}
.task-title,
.task-meta {
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.task-title {
  font-size: 14px;
  font-weight: 500;
  line-height: 17px;
}
.task-meta {
  color: #6e758c;
  font-size: 11px;
  line-height: 13px;
}
.task-arrow {
  color: #6e758c;
  font-size: 22px;
  line-height: 27px;
}
.task-empty {
  display: flex;
  min-height: 64px;
  margin-top: 12px;
  padding: 0 16px;
  align-items: center;
  border: 1px dashed #dfe2ef;
  border-radius: 14px;
  color: #8b91a5;
  background: rgba(255, 255, 255, 0.64);
  font-size: 11px;
}
.bottom-safe {
  height: 16px;
}
</style>
