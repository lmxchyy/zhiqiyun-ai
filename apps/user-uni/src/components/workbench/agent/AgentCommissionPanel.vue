<template>
  <view class="section-stack">
    <view class="wallet-card agent">
      <text class="wallet-label">可提现收益</text>
      <text class="wallet-value">{{ formatCurrency(availableToWithdraw) }}</text>
      <text class="wallet-copy">累计 {{ formatCurrency(totalCommission) }} · 已提现 {{ formatCurrency(withdrawn) }}</text>
      <button type="button" class="wallet-button" @click="emit('withdraw')">申请提现</button>
    </view>
    <button type="button" class="outline-button" @click="emit('open-withdrawals')">查看提现记录</button>

    <view class="section-card">
      <view class="section-header compact">
        <text class="section-title">分润明细</text>
        <text class="soft-tag">{{ commissions.length }} 条</text>
      </view>
      <view v-if="commissions.length" class="list-stack">
        <view v-for="commission in commissions.slice(0, 8)" :key="rowKey(commission)" class="list-item" @click="emit('open-detail', commission)">
          <view>
            <text class="list-title">订单 {{ rowString(commission, 'orderId') || rowString(commission, 'id') }}</text>
            <text class="list-meta">{{ formatDate(rowDate(commission)) }}</text>
          </view>
          <view class="list-side">
            <text class="price-text">{{ formatCurrency(rowAmount(commission)) }}</text>
            <text :class="['status-tag', statusTone(rowStatus(commission))]">{{ rowStatus(commission) }}</text>
          </view>
        </view>
      </view>
      <text v-else class="empty-text">暂无分润记录。</text>
    </view>
  </view>
</template>

<script setup lang="ts">
interface Props {
  availableToWithdraw: number;
  totalCommission: number;
  withdrawn: number;
  commissions: unknown[];
}

defineProps<Props>();

const emit = defineEmits<{
  (e: 'withdraw'): void;
  (e: 'open-withdrawals'): void;
  (e: 'open-detail', commission: unknown): void;
}>();

function rowKey(row: unknown) {
  const value = row && typeof row === 'object' ? (row as Record<string, unknown>).id : '';
  return String(value || Math.random());
}

function rowString(row: unknown, key: string) {
  return row && typeof row === 'object' ? String((row as Record<string, unknown>)[key] || '') : '';
}

function rowStatus(row: unknown) {
  return rowString(row, 'status') || '记录';
}

function rowAmount(row: unknown) {
  return row && typeof row === 'object' ? Number((row as Record<string, unknown>).amount || 0) : 0;
}

function rowDate(row: unknown) {
  return row && typeof row === 'object' ? String((row as Record<string, unknown>).createdAt || '') : '';
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
