<template>
  <view class="section-stack">
    <view class="profile-card">
      <image class="profile-logo" :src="logo" mode="aspectFit" />
      <view>
        <text class="profile-name">{{ agentName }}</text>
        <text class="profile-meta">{{ agentLevelLabel }} · {{ agentStatus }}</text>
      </view>
    </view>
    <view class="section-card">
      <view class="section-header compact">
        <text class="section-title">代理权益</text>
        <text class="soft-tag">已开通</text>
      </view>
      <view class="config-row">
        <text>邀请码</text>
        <text>{{ inviteCode }}</text>
      </view>
      <view class="config-row">
        <text>开通条件</text>
        <text>{{ openCondition }}</text>
      </view>
      <view class="config-row">
        <text>保级条件</text>
        <text>{{ keepCondition }}</text>
      </view>
      <button type="button" class="outline-button" @click="emit('expand-child-agents')">拓展下级代理</button>
      <view class="v31-batch-actions"><button class="active" @click="emit('open-team')">团队成员</button><button @click="emit('open-orders')">客户订单</button></view>
    </view>
    <view class="section-card">
      <view class="section-header compact"><text class="section-title">{{ roleLabel }}功能</text><text class="soft-tag">按权限展示</text></view>
      <view class="v31-batch-actions"><button v-for="item in menuItems" :key="item.id" :class="{ active: item.primary }" @click="emit('menu', item.id)">{{ item.label }}</button></view>
    </view>
  </view>
</template>

<script setup lang="ts">
interface Props {
  logo: string;
  agentName: string;
  agentLevelLabel: string;
  agentStatus: string;
  inviteCode: string;
  openCondition: string;
  keepCondition: string;
  roleLabel: string;
  menuItems: Array<{ id: string; label: string; primary?: boolean }>;
}

defineProps<Props>();

const emit = defineEmits<{
  (e: 'expand-child-agents'): void;
  (e: 'open-team'): void;
  (e: 'open-orders'): void;
  (e: 'menu', id: string): void;
}>();
</script>

<style scoped>
.section-stack {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
</style>
