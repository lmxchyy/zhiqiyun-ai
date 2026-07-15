<template>
  <view :class="['enterprise-page', { 'has-fixed-action': fixedAction }]">
    <view class="enterprise-status-space" />
    <view class="enterprise-nav">
      <button class="enterprise-nav-back" type="button" hover-class="enterprise-pressed" @click="goBack()">‹</button>
      <text class="enterprise-nav-title">{{ title }}</text>
      <button
        v-if="actionLabel"
        class="enterprise-nav-action"
        type="button"
        hover-class="enterprise-pressed"
        :disabled="actionDisabled"
        @click="$emit('action')"
      >{{ actionLabel }}</button>
      <view v-else class="enterprise-nav-placeholder" />
    </view>
    <view class="enterprise-page-content"><slot /></view>
    <view v-if="fixedAction" class="enterprise-fixed-action"><slot name="fixed-action" /></view>
  </view>
</template>

<script setup lang="ts">
defineProps<{
  title: string;
  actionLabel?: string;
  actionDisabled?: boolean;
  fixedAction?: boolean;
}>();

defineEmits<{ action: [] }>();

function goBack() {
  const pages = getCurrentPages();
  if (pages.length > 1) uni.navigateBack();
  else uni.reLaunch({ url: "/pages/user/UserMinePage" });
}
</script>
