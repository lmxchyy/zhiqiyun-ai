<template>
  <view class="overview-card">
    <view class="metrics-row">
      <view v-for="item in metrics" :key="item.label" class="metric">
        <view :class="['metric-icon', item.tone]"><image :src="item.icon" mode="aspectFit" /></view>
        <view class="metric-copy">
          <view v-if="loading" class="metric-skeleton" />
          <template v-else>
            <text class="metric-label">{{ item.label }}</text>
            <text class="metric-value">{{ item.value }}</text>
          </template>
        </view>
      </view>
    </view>

    <view class="storage-row">
      <view class="metric-icon coral"><image src="/static/icons/overview-storage.svg" mode="aspectFit" /></view>
      <view class="storage-copy">
        <text class="metric-label">存储空间</text>
        <text class="storage-value">{{ storagePercent }}</text>
      </view>
      <view class="storage-progress-wrap">
        <view class="storage-progress-head"><text>{{ storageUsageLabel }}</text><text class="storage-arrow">››</text></view>
        <view class="storage-track"><view class="storage-progress" :style="{ width: `${progressPercent}%` }" /></view>
      </view>
    </view>
  </view>
</template>

<script setup lang="ts">
import { computed } from "vue";
import type { AssetOverview } from "../../features/assets/types";

const props = withDefaults(defineProps<{ overview: AssetOverview; loading?: boolean }>(), { loading: false });

const metrics = computed(() => [
  { label: "全部作品", value: formatCount(props.overview.total), icon: "/static/icons/overview-all.svg", tone: "blue" },
  { label: "本月新增", value: formatCount(props.overview.monthTotal), icon: "/static/icons/overview-month.svg", tone: "cyan" },
  { label: "收藏数量", value: formatCount(props.overview.favoriteTotal), icon: "/static/icons/overview-favorite.svg", tone: "amber" },
]);
const progressPercent = computed(() => props.overview.storageQuotaBytes > 0
  ? Math.min(100, Math.max(0, Number(props.overview.storageUsagePercent) || (props.overview.storageBytes / props.overview.storageQuotaBytes) * 100))
  : 0);
const storagePercent = computed(() => props.overview.storageQuotaBytes > 0 ? `${Math.round(progressPercent.value)}%` : "--");
const storageUsageLabel = computed(() => props.overview.storageQuotaBytes > 0
  ? `${formatBytes(props.overview.storageBytes)} / ${formatBytes(props.overview.storageQuotaBytes)}`
  : `${formatBytes(props.overview.storageBytes)} / --`);

function formatCount(value: number) {
  return String(Math.max(0, Math.round(Number(value) || 0))).replace(/\B(?=(\d{3})+(?!\d))/g, ",");
}

function formatBytes(value: number) {
  const bytes = Math.max(0, Number(value) || 0);
  if (bytes >= 1024 ** 3) return `${(bytes / 1024 ** 3).toFixed(bytes >= 10 * 1024 ** 3 ? 1 : 2)}GB`;
  if (bytes >= 1024 ** 2) return `${(bytes / 1024 ** 2).toFixed(1)}MB`;
  if (bytes >= 1024) return `${(bytes / 1024).toFixed(1)}KB`;
  return bytes > 0 ? `${Math.round(bytes)}B` : "0GB";
}
</script>

<style scoped>
.overview-card {
  min-height: 136px;
  box-sizing: border-box;
  margin-top: 12px;
  padding: 16px;
  border: 1px solid #e2e6f0;
  border-radius: 16px;
  background: #fff;
  box-shadow: 0 3px 12px rgba(41, 51, 82, .025);
}

.metrics-row {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 10px;
}

.metric {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 8px;
}

.metric-icon {
  display: flex;
  width: 30px;
  height: 30px;
  flex: 0 0 30px;
  align-items: center;
  justify-content: center;
  border-radius: 9px;
}

.metric-icon image { width: 18px; height: 18px; }
.metric-icon.blue { background: #5b6ee1; }
.metric-icon.cyan { background: #20aeb8; }
.metric-icon.amber { background: #f5a524; }
.metric-icon.coral { background: #ee6470; }

.metric-copy,
.storage-copy {
  display: flex;
  min-width: 0;
  flex-direction: column;
}

.metric-label {
  display: block;
  overflow: hidden;
  color: #697386;
  font-size: var(--font-meta-size, 10px);
  font-weight: var(--font-weight-regular, 400);
  line-height: var(--font-meta-line, 15px);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.metric-value,
.storage-value {
  display: block;
  margin-top: 1px;
  color: #182033;
  font-family: var(--font-family-number, Arial, sans-serif);
  font-size: 18px;
  font-weight: var(--font-weight-bold, 700);
  line-height: 24px;
}

.metric-skeleton {
  width: 48px;
  height: 32px;
  border-radius: 8px;
  background: #eef1f6;
  animation: pulse 1.2s ease-in-out infinite;
}

.storage-row {
  display: grid;
  margin-top: 14px;
  padding-top: 13px;
  grid-template-columns: 30px 82px minmax(0, 1fr);
  align-items: center;
  gap: 8px;
  border-top: 1px solid #eef0f5;
}

.storage-progress-wrap { min-width: 0; }
.storage-progress-head {
  display: flex;
  margin-bottom: 7px;
  align-items: center;
  justify-content: space-between;
  color: #697386;
  font-family: var(--font-family-number, Arial, sans-serif);
  font-size: var(--font-meta-size, 10px);
  font-weight: var(--font-weight-medium, 500);
  line-height: var(--font-meta-line, 15px);
}

.storage-arrow {
  color: #98a2b3;
  font-size: 13px;
  letter-spacing: 0;
}

.storage-track {
  height: 4px;
  overflow: hidden;
  border-radius: 4px;
  background: #eceff6;
}

.storage-progress {
  height: 100%;
  min-width: 0;
  border-radius: 4px;
  background: #5b6ee1;
}

@media (max-width: 350px) {
  .overview-card { padding-right: 13px; padding-left: 13px; }
  .metrics-row { gap: 6px; }
  .metric { gap: 6px; }
  .metric-icon { width: 28px; height: 28px; flex-basis: 28px; }
  .storage-row { grid-template-columns: 28px 76px minmax(0, 1fr); gap: 6px; }
}

@keyframes pulse { 50% { opacity: .55; } }
</style>
