<template>
  <view class="workbench-header">
    <image class="workbench-logo" :src="logoSrc" mode="aspectFit" />
    <view class="workbench-copy">
      <text class="workbench-title">{{ title }}</text>
      <text class="workbench-subtitle">{{ subtitle }}</text>
    </view>
    <view v-if="showRoleBadge && roleLabel" class="workbench-role-badge">{{ roleLabel }}</view>
  </view>
</template>

<script setup lang="ts">
interface WorkbenchHeaderProps {
  logoSrc: string;
  title: string;
  subtitle: string;
  roleLabel?: string;
  showRoleBadge?: boolean;
}

withDefaults(defineProps<WorkbenchHeaderProps>(), {
  roleLabel: "",
  showRoleBadge: true,
});
</script>

<style scoped>
.workbench-header {
  display: flex;
  min-height: var(--navigation-bar-height, 44px);
  margin-top: 5px;
  padding-right: var(--capsule-right-space, 0px);
  box-sizing: border-box;
  align-items: center;
  gap: 10px;
}

.workbench-logo {
  width: 38px;
  height: 38px;
  flex: 0 0 38px;
  border: 1px solid #dde5ff;
  border-radius: 8px;
  background: #ffffff;
}

.workbench-copy {
  min-width: 0;
  flex: 1;
  display: flex;
  flex-direction: column;
}

.workbench-title {
  color: #0f172a;
  font-size: 17px;
  font-weight: 600;
  line-height: 24px;
}

.workbench-subtitle {
  overflow: hidden;
  color: #697386;
  font-size: 10px;
  line-height: 15px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.workbench-role-badge {
  width: auto;
  min-width: 70px;
  height: 26px;
  margin: 0;
  padding: 0 10px;
  border: 1px solid #c9d2ff;
  border-radius: 8px;
  color: #5b55d6;
  background: #eef2ff;
  font-size: 11px;
  line-height: 24px;
}
</style>
