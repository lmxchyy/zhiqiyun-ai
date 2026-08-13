<template>
  <view class="section-stack">
    <view class="section-card">
      <view class="section-header">
        <view>
          <text class="section-kicker">运营中心</text>
          <text class="section-title">{{ operationName }}</text>
        </view>
        <text class="soft-tag">{{ operationStatus }}</text>
      </view>
      <view class="quick-grid">
        <button class="quick-item" @click="emit('tab', 'agents')">
          <text class="quick-value">{{ operationAgents.length }}</text>
          <text class="quick-label">代理商</text>
        </button>
        <button class="quick-item" @click="emit('tab', 'orders')">
          <text class="quick-value">{{ operationOrders.length }}</text>
          <text class="quick-label">订单</text>
        </button>
        <button class="quick-item" @click="emit('tab', 'commission')">
          <text class="quick-value">{{ formatCurrency(operationCommissionTotal) }}</text>
          <text class="quick-label">中心分润</text>
        </button>
      </view>
    </view>
  </view>
</template>

<script setup lang="ts">
interface Props {
  operationName: string;
  operationStatus: string;
  operationCommissionTotal: number;
  operationAgents: unknown[];
  operationOrders: unknown[];
}

defineProps<Props>();

const emit = defineEmits<{
  (e: 'tab', tab: 'agents' | 'orders' | 'commission'): void;
}>();

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
