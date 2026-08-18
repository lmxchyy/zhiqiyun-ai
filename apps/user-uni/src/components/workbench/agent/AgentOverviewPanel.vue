<template>
  <view class="section-stack">
    <view class="agent-v4-hero">
      <view class="agent-v4-hero-top">
        <view>
          <text>本月预估分润</text>
          <text>{{ formatCurrency(totalCommission) }}</text>
        </view>
        <text>{{ agentLevelLabel }}</text>
      </view>
      <text class="agent-v4-copy">登录后优先查看推广增长、客户、订单与结算结果。</text>
      <view class="agent-v4-metrics">
        <button @click="emit('tab', 'customers')">
          <text>{{ formatNumber(directCustomers) }}</text>
          <text>客户</text>
        </button>
        <button @click="emit('open-team')">
          <text>{{ formatNumber(childAgents) }}</text>
          <text>团队</text>
        </button>
        <button @click="emit('open-withdrawals')">
          <text>{{ formatCurrency(availableToWithdraw) }}</text>
          <text>可提现</text>
        </button>
      </view>
    </view>

    <view class="agent-v4-entry-card">
      <view class="section-header compact"><text class="section-title">经营入口</text><text class="soft-tag">{{ agentName }}</text></view>
      <button @click="emit('tab', 'promotion')"><text class="agent-v4-icon green">推</text><view><text>推广中心</text><text>专属链接、小程序分享与邀请记录</text></view><text>{{ conversionRate }}% 转化</text></button>
      <button @click="emit('tab', 'customers')"><text class="agent-v4-icon purple">客</text><view><text>客户管理</text><text>绑定客户与客户订单</text></view><text>{{ formatNumber(directCustomers) }} 人</text></button>
      <button @click="emit('tab', 'commission')"><text class="agent-v4-icon green">润</text><view><text>分润中心</text><text>订单分润与提现记录</text></view><text>{{ formatCurrency(availableToWithdraw) }}</text></button>
      <button @click="emit('open-team')"><text class="agent-v4-icon orange">队</text><view><text>团队管理</text><text>直属代理与成员业绩</text></view><text>{{ formatNumber(childAgents) }} 人</text></button>
    </view>

    <button class="agent-v4-cta" @click="emit('tab', 'promotion')">查看推广数据</button>
  </view>
</template>

<script setup lang="ts">
interface Props {
  agentName: string;
  agentLevelLabel: string;
  totalCommission: number;
  directCustomers: number;
  childAgents: number;
  availableToWithdraw: number;
  conversionRate: number;
}

defineProps<Props>();

const emit = defineEmits<{
  (e: 'tab', tab: 'promotion' | 'customers' | 'commission'): void;
  (e: 'open-team'): void;
  (e: 'open-withdrawals'): void;
}>();

function formatNumber(value: number) {
  return Number(value || 0).toLocaleString('zh-CN');
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
