<template>
  <view class="section-stack">
    <view class="profile-card">
      <image class="profile-logo" :src="logo" mode="aspectFit" />
      <view>
        <text class="profile-name">{{ operationName }}</text>
        <text class="profile-meta">运营中心 · {{ operationStatus }}</text>
      </view>
    </view>
    <view class="section-card">
      <view class="section-header compact">
        <text class="section-title">5000 运营中心开通包</text>
        <text class="price-badge">5000 元</text>
      </view>
      <text class="body-copy">用于开通区域运营中心，承接代理商团队、订单和分润看板。</text>
      <button type="button" class="outline-button" @click="emit('create-order')">创建运营中心订单</button>
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
  operationName: string;
  operationStatus: string;
  roleLabel: string;
  menuItems: Array<{ id: string; label: string; primary?: boolean }>;
}

defineProps<Props>();

const emit = defineEmits<{
  (e: 'create-order'): void;
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
