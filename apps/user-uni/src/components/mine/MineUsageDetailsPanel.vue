<template>
  <view class="mine-detail-stack">
    <view class="usage-summary">
      <text>本月消耗</text><text>{{ formatNumber(monthlyPointCost) }} 点</text><text>当前余额 {{ formatNumber(pointBalance) }} 点</text>
      <view v-for="item in usageBreakdown" :key="item.label" class="usage-bar-row"><text>{{ item.label }}</text><view><view :class="item.tone" :style="{ width: `${item.percent}%` }"></view></view></view>
    </view>
    <view class="filter-strip"><button class="active" @click="emit('cycle-type')">{{ usageFilterLabel }}</button><button :class="{ active: currentMonthOnly }" @click="emit('toggle-current-month')">{{ currentMonthOnly ? '本月' : '全部时间' }}</button><text @click="emit('export')">导出明细</text></view>
    <view class="record-list">
      <view v-if="filteredUsageRecords.length" v-for="record in filteredUsageRecords.slice(0, 8)" :key="rowKey(record)" class="record-row" @click="emit('open-detail', record)">
        <text class="record-icon purple">{{ usageIcon(record) }}</text>
        <view><text>{{ usageTitle(record) }}</text><text>{{ formatDate(rowDate(record)) }}</text></view>
        <text class="record-value purple">-{{ formatNumber(rowPointCost(record)) }} 点</text>
      </view>
      <view v-else class="mine-empty"><text>暂无消耗明细</text><text>生成图片、视频或 PPT 后，将按模型记录扣点。</text></view>
    </view>
    <view class="billing-note"><text>计费规则</text><text>基础价 × 数量 × 参数倍率，最终点数向上取整。</text></view>
  </view>
</template>

<script setup lang="ts">
interface UsageRecord extends Record<string, unknown> {}
interface UsageBreakdownItem { label: string; percent: number; tone: string }

interface Props {
  monthlyPointCost: number;
  pointBalance: number;
  usageBreakdown: UsageBreakdownItem[];
  usageFilterLabel: string;
  currentMonthOnly: boolean;
  filteredUsageRecords: UsageRecord[];
}

defineProps<Props>();

const emit = defineEmits<{
  (e: 'cycle-type'): void;
  (e: 'toggle-current-month'): void;
  (e: 'export'): void;
  (e: 'open-detail', record: UsageRecord): void;
}>();

function rowString(row: unknown, key: string) {
  return row && typeof row === 'object' ? String((row as Record<string, unknown>)[key] || '') : '';
}

function rowNumber(row: unknown, key: string) {
  return row && typeof row === 'object' ? Number((row as Record<string, unknown>)[key] || 0) : 0;
}

function rowDate(row: unknown) {
  return rowString(row, 'createdAt') || rowString(row, 'occurredAt') || rowString(row, 'updatedAt') || rowString(row, 'paidAt');
}

function rowKey(row: unknown) {
  return rowString(row, 'id') || rowString(row, 'eventId') || rowString(row, 'taskId') || rowString(row, 'orderId');
}

function rowPointCost(row: unknown) {
  return Math.abs(rowNumber(row, 'pointCost') || rowNumber(row, 'points') || rowNumber(row, 'amount') || rowNumber(row, 'delta'));
}

function usageTitle(row: unknown) {
  return rowString(row, 'modelName') || rowString(row, 'model') || rowString(row, 'module') || rowString(row, 'description') || rowString(row, 'changeType') || 'AI 服务';
}

function usageIcon(row: unknown) {
  const title = usageTitle(row).toLowerCase();
  if (title.includes('video') || title.includes('视频')) return '视';
  if (title.includes('ppt')) return 'P';
  if (title.includes('image') || title.includes('生图')) return '图';
  return 'AI';
}

function formatNumber(value: number) {
  return String(Math.max(0, Math.round(value || 0))).replace(/\B(?=(\d{3})+(?!\d))/g, ',');
}

function formatDate(value: string) {
  if (!value) return '时间待同步';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  const pad = (part: number) => String(part).padStart(2, '0');
  return `${pad(date.getMonth() + 1)}/${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}`;
}
</script>

<style scoped>
.mine-detail-stack {
  display: flex;
  margin-top: 14px;
  padding-bottom: 18px;
  flex-direction: column;
  gap: 14px;
}
</style>
