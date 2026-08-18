<template>
  <view class="mine-detail-stack">
    <view class="role-hero">
      <text>当前角色</text>
      <text>{{ currentRoleLabel }}</text>
      <text>页面与操作按当前角色权限动态展示</text>
      <text class="role-status">使用中</text>
      <button v-if="!hasAgentRole" @click="emit('upgrade')">代理商角色未开通　去升级 ›</button>
      <button v-else @click="emit('switch-agent')">切换到代理商 ›</button>
    </view>
    <view class="mine-panel compact">
      <text class="mine-section-title">已授权角色</text>
      <view v-for="role in roleRows" :key="role.id" class="role-row">
        <text class="mine-menu-icon green">✓</text>
        <view><text>{{ role.label }}</text><text>{{ role.description }}</text></view>
        <text :class="role.id === currentRole ? 'success-text' : 'muted-text'">{{ role.id === currentRole ? '当前' : '已启用' }}</text>
      </view>
    </view>
    <view class="mine-panel compact">
      <text class="mine-section-title">当前角色权限</text>
      <view v-for="permission in grantedPermissionRows" :key="permission" class="permission-row"><text>{{ permission }}</text><text class="success-text">可用</text></view>
    </view>
    <button v-if="!hasAgentRole" class="orange-action" @click="emit('upgrade')">成为代理商</button>
    <button v-else class="primary-action" @click="emit('switch-agent')">进入代理工作台</button>
  </view>
</template>

<script setup lang="ts">
import type { AppRole } from "../../types";

interface Props {
  currentRole: AppRole;
  currentRoleLabel: string;
  hasAgentRole: boolean;
  roleRows: Array<{ id: AppRole; label: string; description: string }>;
  grantedPermissionRows: string[];
}

defineProps<Props>();

const emit = defineEmits<{
  (e: 'upgrade'): void;
  (e: 'switch-agent'): void;
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
