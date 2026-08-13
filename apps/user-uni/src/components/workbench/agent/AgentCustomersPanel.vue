<template>
  <view class="section-stack">
    <view class="section-card">
      <view class="section-header compact">
        <text class="section-title">拓展客户</text>
        <text class="soft-tag">{{ customers.length }} 人</text>
      </view>
      <view v-if="customers.length" class="list-stack">
        <view v-for="customer in customers" :key="rowKey(customer)" class="list-item" @click="emit('open-detail', customer)">
          <view>
            <text class="list-title">{{ customerName(customer) }}</text>
            <text class="list-meta">{{ customerEmail(customer) }}</text>
          </view>
          <view class="list-side">
            <text class="price-text">{{ formatNumber(rowNumber(customer, 'pointsAvailable')) }} 点</text>
            <text class="status-tag success">{{ rowString(customer, 'plan') || '客户' }}</text>
          </view>
        </view>
      </view>
      <text v-else class="empty-text">暂无客户，先分享小程序码或推广链接。</text>
    </view>
  </view>
</template>

<script setup lang="ts">
import type { ChannelAgent } from "../../../types";

interface Props {
  customers: ChannelAgent[];
}

defineProps<Props>();

const emit = defineEmits<{
  (e: 'open-detail', customer: ChannelAgent): void;
}>();

function rowKey(row: unknown) {
  const value = row && typeof row === 'object' ? (row as Record<string, unknown>).id : '';
  return String(value || Math.random());
}

function customerName(row: unknown) {
  return row && typeof row === 'object' ? String((row as Record<string, unknown>).name || (row as Record<string, unknown>).nickname || '客户') : '客户';
}

function customerEmail(row: unknown) {
  return row && typeof row === 'object' ? String((row as Record<string, unknown>).email || (row as Record<string, unknown>).phone || '') : '';
}

function rowNumber(row: unknown, key: string) {
  return row && typeof row === 'object' ? Number((row as Record<string, unknown>)[key] || 0) : 0;
}

function rowString(row: unknown, key: string) {
  return row && typeof row === 'object' ? String((row as Record<string, unknown>)[key] || '') : '';
}

function formatNumber(value: number) {
  return Number(value || 0).toLocaleString('zh-CN');
}
</script>

<style scoped>
.section-stack {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
</style>
