<template>
  <view class="mine-detail-stack">
    <view class="invite-hero">
      <text>我的推广码</text><text>{{ inviteCode }}</text><text class="agent-pill">{{ hasAgentRole ? agentLevelLabel : '待开通' }}</text>
      <view class="invite-link"><text>{{ inviteLink }}</text><button @click="emit('copy-invite')">复制</button></view>
      <text>{{ hasAgentRole ? '客户通过链接注册后将自动绑定' : '成为代理商后可启用客户绑定与分润' }}</text>
    </view>
    <view class="promotion-stats"><view><text>{{ stats.visits }}</text><text>访问</text></view><view><text>{{ stats.registrations }}</text><text>注册</text></view><view><text>{{ stats.orders }}</text><text>成交</text></view><view><text class="orange-text">{{ conversionRate }}%</text><text>转化率</text></view></view>
    <view class="mine-panel compact"><text class="mine-section-title">选择分享方式</text><view class="share-grid"><button :disabled="!hasAgentRole" open-type="share"><text class="green">微</text><text>微信好友</text></button><button :disabled="!hasAgentRole" open-type="share"><text class="purple">圈</text><text>朋友圈</text></button><button :disabled="!hasAgentRole" @click="emit('poster')"><text class="orange">图</text><text>生成海报</text></button></view></view>
    <view class="mine-panel compact"><text class="mine-section-title">推广流程</text><view class="step-row four"><view v-for="(step, index) in promotionSteps" :key="step"><text :class="['step-index', { active: index === 0 }]">{{ index + 1 }}</text><text>{{ step }}</text></view></view></view>
    <button v-if="hasAgentRole" class="primary-action" open-type="share">分享专属链接</button>
    <button v-else class="orange-action" @click="emit('upgrade')">成为代理商后开启推广</button>
  </view>
</template>

<script setup lang="ts">
interface Props {
  inviteCode: string;
  inviteLink: string;
  hasAgentRole: boolean;
  agentLevelLabel: string;
  conversionRate: number;
  stats: { visits: number; registrations: number; orders: number };
  promotionSteps: string[];
}

defineProps<Props>();

const emit = defineEmits<{
  (e: 'copy-invite'): void;
  (e: 'poster'): void;
  (e: 'upgrade'): void;
}>();
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
