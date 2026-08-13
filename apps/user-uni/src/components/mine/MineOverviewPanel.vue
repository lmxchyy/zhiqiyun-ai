<template>
  <view class="mine-overview-panel">
    <view class="mine-v5-profile-card">
      <AppImage v-if="headerBackground" class="mine-v5-header-background" :src="headerBackground" alt="个人中心背景" width="100%" height="100%" radius="12px" :lazy-load="false" />
      <view class="mine-v5-avatar">
        <AppImage v-if="avatarUrl || avatarFallback" :src="avatarUrl" :fallback="avatarFallback" local-fallback="/static/fallbacks/default-avatar.jpg" :alt="displayName" width="58px" height="58px" radius="50%" :lazy-load="false" />
        <text v-else>{{ avatarText }}</text>
      </view>
      <view class="mine-v5-profile-copy">
        <text class="mine-v5-profile-name">{{ displayName }}</text>
        <text class="mine-v5-profile-type">微信用户</text>
      </view>
      <button class="mine-v5-edit" @click="emit('edit-profile')">编辑资料 ›</button>
    </view>

    <button class="mine-v5-agent-entry" @click="hasAgentRole ? emit('switch-agent') : emit('navigate', 'agent-upgrade')">
      <view class="mine-v5-agent-copy">
        <text>{{ hasAgentRole ? '代理商角色已开通' : '代理商角色未开通' }}</text>
        <text>{{ hasAgentRole ? '查看推广、客户与订单分润' : '推广客户并获得订单分润' }}</text>
      </view>
      <text class="mine-v5-agent-action">{{ hasAgentRole ? '进入工作台 ›' : '升级 ›' }}</text>
    </button>

    <view class="mine-v5-balance-card" @click="emit('open-wallet')">
      <AppImage v-if="memberBackground" class="mine-v5-member-background" :src="memberBackground" alt="会员背景" width="100%" height="100%" radius="12px" />
      <text class="mine-v5-balance-title">我的剩余点数</text>
      <view class="mine-v5-balance-value"><text>{{ formatNumber(pointBalance) }}</text><text>点</text></view>
      <button class="mine-v5-recharge" @click.stop="emit('recharge')">去充值</button>
    </view>

    <view class="mine-v5-menu-card">
      <button class="mine-v5-menu-row" @click="emit('navigate', 'recharge-history')">
        <text class="mine-v5-menu-icon purple">充</text><text class="mine-v5-menu-label">充值中心</text><text class="mine-v5-chevron">›</text>
      </button>
      <button class="mine-v5-menu-row" @click="emit('open-task-records')">
        <text class="mine-v5-menu-icon blue">任</text><text class="mine-v5-menu-label">任务记录</text><text class="mine-v5-chevron">›</text>
      </button>
      <button class="mine-v5-menu-row" @click="emit('navigate', 'usage-details')">
        <text class="mine-v5-menu-icon violet">点</text><text class="mine-v5-menu-label">点数明细</text><text class="mine-v5-chevron">›</text>
      </button>
      <button class="mine-v5-menu-row" @click="emit('open-orders')">
        <text class="mine-v5-menu-icon orange">单</text><text class="mine-v5-menu-label">我的订单</text><text class="mine-v5-chevron">›</text>
      </button>
      <button class="mine-v5-menu-row" @click="emit('navigate', 'invite-promotion')">
        <text class="mine-v5-menu-icon pink">邀</text><text class="mine-v5-menu-label">邀请与推广</text><text class="mine-v5-chevron">›</text>
      </button>
      <button class="mine-v5-menu-row" @click="emit('navigate', 'role-permissions')">
        <text class="mine-v5-menu-icon blue">权</text><text class="mine-v5-menu-label">角色与权限</text><text class="mine-v5-chevron">›</text>
      </button>
      <button class="mine-v5-menu-row" @click="emit('open-settings')">
        <text class="mine-v5-menu-icon slate">设</text><text class="mine-v5-menu-label">账户设置</text><text class="mine-v5-chevron">›</text>
      </button>
    </view>
  </view>
</template>

<script setup lang="ts">
import AppImage from "../AppImage.vue";
import type { AppRole } from "../../types";
import type { MineView } from "../../types";

interface Props {
  displayName: string;
  avatarUrl?: string;
  avatarFallback?: string;
  headerBackground?: string;
  memberBackground?: string;
  pointBalance: number;
  roles: AppRole[];
  currentRole: AppRole;
}

const props = defineProps<Props>();

const emit = defineEmits<{
  (e: 'navigate', view: MineView): void;
  (e: 'edit-profile'): void;
  (e: 'switch-agent'): void;
  (e: 'open-wallet'): void;
  (e: 'recharge'): void;
  (e: 'open-task-records'): void;
  (e: 'open-orders'): void;
  (e: 'open-settings'): void;
}>();

const avatarText = computed(() => props.displayName.trim().slice(0, 1) || '知');
const hasAgentRole = computed(() => props.roles.includes('AGENT'));

function formatNumber(value: number) {
  return String(Math.max(0, Math.round(value || 0))).replace(/\B(?=(\d{3})+(?!\d))/g, ',');
}
</script>

<style scoped>
.mine-overview-panel {
  display: flex;
  flex-direction: column;
}
</style>
