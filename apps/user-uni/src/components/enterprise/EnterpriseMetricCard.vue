<template>
  <view class="enterprise-metric-card">
    <text class="enterprise-metric-label">{{ label }}</text>
    <text class="enterprise-metric-value">{{ value }}</text>
  </view>
</template>

<script setup lang="ts">
defineProps<{ label: string; value: string }>();
</script>
