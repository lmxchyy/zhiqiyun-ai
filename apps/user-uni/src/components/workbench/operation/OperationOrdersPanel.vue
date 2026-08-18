<template>
  <view class="section-stack">
    <view class="section-card">
      <view class="section-header compact">
        <text class="section-title">区域订单</text>
        <text class="soft-tag">{{ orders.length }} 笔</text>
      </view>
      <view v-if="orders.length" class="list-stack">
        <view v-for="order in orders" :key="rowKey(order)" class="list-item" @click="emit('open-detail', order)">
          <view>
            <text class="list-title">{{ orderTitle(order) }}</text>
            <text class="list-meta">{{ formatDate(rowDate(order)) }}</text>
          </view>
          <view class="list-side">
            <text class="price-text">{{ formatCurrency(rowAmount(order)) }}</text>
            <text :class="['status-tag', statusTone(rowStatus(order))]">{{ rowStatus(order) }}</text>
          </view>
        </view>
      </view>
      <text v-else class="empty-text">暂无区域订单。</text>
    </view>
  </view>
</template>

<script setup lang="ts">
type OrderRow = Record<string, unknown>;

interface Props {
  orders: OrderRow[];
}

defineProps<Props>();

const emit = defineEmits<{
  (e: 'open-detail', order: OrderRow): void;
}>();

function rowKey(row: unknown) {
  const value = row && typeof row === 'object' ? (row as Record<string, unknown>).id : '';
  return String(value || Math.random());
}

function orderTitle(row: unknown) {
  return row && typeof row === 'object' ? String((row as Record<string, unknown>).title || (row as Record<string, unknown>).name || (row as Record<string, unknown>).orderNo || '订单') : '订单';
}

function rowDate(row: unknown) {
  return row && typeof row === 'object' ? String((row as Record<string, unknown>).createdAt || '') : '';
}

function rowAmount(row: unknown) {
  return row && typeof row === 'object' ? Number((row as Record<string, unknown>).amount || 0) : 0;
}

function rowStatus(row: unknown) {
  return row && typeof row === 'object' ? String((row as Record<string, unknown>).status || '订单') : '订单';
}

function statusTone(status: string) {
  return /success|done|paid/i.test(status) ? 'success' : 'pending';
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
