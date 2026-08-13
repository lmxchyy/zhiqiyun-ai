<template>
  <view class="mine-detail-stack">
    <view class="record-summary dark">
      <text>累计充值</text>
      <text>{{ formatCurrency(totalRechargeCents) }}</text>
      <text>累计到账 {{ formatNumber(totalRechargePoints) }} 点</text>
      <button @click="emit('invoice')">电子凭证</button>
    </view>
    <view class="filter-strip">
      <button :class="{ active: activeFilter === 'all' }" @click="emit('update:filter', 'all')">全部</button>
      <button :class="{ active: activeFilter === 'success' }" @click="emit('update:filter', 'success')">成功</button>
      <button :class="{ active: activeFilter === 'refund' }" @click="emit('update:filter', 'refund')">退款</button>
      <text @click="emit('cycle-range')">{{ historyRangeLabel }}⌄</text>
    </view>
    <view class="record-list">
      <view v-if="filteredOrders.length" v-for="order in filteredOrders.slice(0, 8)" :key="rowKey(order)" class="record-row" @click="emit('open-detail', order)">
        <text :class="['record-icon', statusTone(order)]">{{ statusTone(order) === 'success' ? '✓' : '单' }}</text>
        <view><text>{{ orderTitle(order) }}</text><text>{{ orderMeta(order) }}</text></view>
        <text :class="['record-value', statusTone(order)]">{{ orderValue(order) }}</text>
      </view>
      <view v-else class="mine-empty"><text>暂无充值订单</text><text>完成充值后，到账与退款状态会显示在这里。</text></view>
    </view>
    <button class="secondary-action" @click="emit('invoice')">查看电子凭证</button>
  </view>
</template>

<script setup lang="ts">
interface Props {
  filteredOrders: unknown[];
  activeFilter: 'all' | 'success' | 'refund';
  historyRangeLabel: string;
  totalRechargeCents: number;
  totalRechargePoints: number;
}

defineProps<Props>();

const emit = defineEmits<{
  (e: 'invoice'): void;
  (e: 'update:filter', value: 'all' | 'success' | 'refund'): void;
  (e: 'cycle-range'): void;
  (e: 'open-detail', order: unknown): void;
}>();

function asRecord(value: unknown): Record<string, unknown> { return value && typeof value === 'object' ? value as Record<string, unknown> : {}; }
function rowString(row: unknown, key: string) { const value = asRecord(row)[key]; return typeof value === 'string' ? value : ''; }
function rowNumber(row: unknown, key: string) { const value = Number(asRecord(row)[key]); return Number.isFinite(value) ? value : 0; }
function rowDate(row: unknown) { return rowString(row, 'createdAt') || rowString(row, 'occurredAt') || rowString(row, 'updatedAt') || rowString(row, 'paidAt'); }
function rowKey(row: unknown) { return rowString(row, 'id') || rowString(row, 'orderId') || rowString(row, 'transactionId') || rowDate(row); }
function isPaidOrder(row: unknown) { return ['PAID', 'SUCCESS', 'SUCCEEDED', 'SETTLED', 'ACTIVE'].includes(rowString(row, 'status').toUpperCase()); }
function isRechargeOrder(row: unknown) { const type = `${rowString(row, 'orderType')} ${rowString(row, 'businessOrderType')} ${rowString(row, 'planId')}`.toUpperCase(); return type.includes('RECHARGE'); }
function orderPoints(row: unknown) { const snapshot = asRecord(asRecord(row).priceSnapshot); return rowNumber(snapshot, 'rechargePoints') || rowNumber(row, 'tokenGrantAmount') || rowNumber(row, 'tokenAmount'); }
function orderTitle(row: unknown) { const plan = rowString(row, 'planId'); if (plan.includes('agent')) return '代理套餐 996 元'; const cents = rowNumber(row, 'amountCents') || rowNumber(row, 'amount'); return isRechargeOrder(row) ? `充值 ${Math.round(cents / 100)} 元` : rowString(row, 'name') || '平台订单'; }
function orderMeta(row: unknown) { const created = rowDate(row); return `${created ? created.slice(0, 10) : '时间待同步'} · ${rowString(row, 'paymentMethod') || '微信支付'}`; }
function orderValue(row: unknown) { const points = orderPoints(row); if (isRechargeOrder(row) && points > 0) return `+${formatNumber(points)} 点`; return formatCurrency(rowNumber(row, 'amountCents') || rowNumber(row, 'amount')); }
function statusTone(row: unknown) { const status = rowString(row, 'status').toUpperCase(); if (['PAID', 'SUCCESS', 'SUCCEEDED', 'SETTLED', 'ACTIVE'].includes(status)) return 'success'; if (['FAILED', 'CANCELLED', 'REJECTED', 'REFUNDED'].includes(status)) return 'danger'; return 'warning'; }
function formatNumber(value: number) { return String(Math.max(0, Math.round(value || 0))).replace(/\B(?=(\d{3})+(?!\d))/g, ','); }
function formatCurrency(cents: number) { return `¥${(Math.max(0, cents || 0) / 100).toFixed(2)}`; }
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
