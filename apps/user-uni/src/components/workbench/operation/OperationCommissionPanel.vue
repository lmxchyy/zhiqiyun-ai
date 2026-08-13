<template>
  <view class="section-stack">
    <view class="section-card">
      <view class="section-header compact">
        <text class="section-title">中心分润</text>
        <text class="soft-tag">{{ commissions.length }} 条</text>
      </view>
      <view v-if="commissions.length" class="list-stack">
        <view v-for="commission in commissions" :key="rowKey(commission)" class="list-item" @click="emit('open-detail', commission)">
          <view>
            <text class="list-title">{{ rowString(commission, 'agentName') || '代理订单' }}</text>
            <text class="list-meta">{{ formatDate(rowDate(commission)) }}</text>
          </view>
          <text class="price-text">{{ formatCurrency(rowAmount(commission)) }}</text>
        </view>
      </view>
      <text v-else class="empty-text">暂无中心分润。</text>
    </view>
  </view>
</template>

<script setup lang="ts">
type CommissionRow = Record<string, unknown>;

interface Props {
  commissions: CommissionRow[];
}

defineProps<Props>();

const emit = defineEmits<{
  (e: 'open-detail', commission: CommissionRow): void;
}>();

function rowKey(row: unknown) {
  const value = row && typeof row === 'object' ? (row as Record<string, unknown>).id : '';
  return String(value || Math.random());
}

function rowString(row: unknown, key: string) {
  return row && typeof row === 'object' ? String((row as Record<string, unknown>)[key] || '') : '';
}

function rowAmount(row: unknown) {
  return row && typeof row === 'object' ? Number((row as Record<string, unknown>).amount || 0) : 0;
}

function rowDate(row: unknown) {
  return row && typeof row === 'object' ? String((row as Record<string, unknown>).createdAt || '') : '';
}

function formatDate(value: string) {
  return value ? value.slice(0, 10) : '--';
}

function formatCurrency(value: number) {
  return `¥${Number(value || 0).toLocaleString('zh-CN')}`;
}
</script>

<style scoped>
.section-stack {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
</style>
