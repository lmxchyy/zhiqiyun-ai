<template>
  <view class="agent-card zq-card">
    <view class="agent-card__mark" :style="{ background: tone }">AI</view>
    <view class="agent-card__body">
      <text class="agent-card__title">{{ title }}</text>
      <text class="agent-card__desc">{{ description }}</text>
      <view class="agent-card__tags">
        <wd-tag v-for="tag in tags" :key="tag" round custom-class="agent-card__tag">
          {{ tag }}
        </wd-tag>
      </view>
    </view>
  </view>
</template>

<script setup lang="ts">
defineProps<{
  title: string
  description: string
  tags: string[]
  tone: string
}>()
</script>

<style scoped lang="scss">
.agent-card {
  display: flex;
  gap: 18rpx;
  padding: 24rpx;
}

.agent-card__mark {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 72rpx;
  height: 72rpx;
  flex: 0 0 72rpx;
  border-radius: var(--radius-md);
  color: #fff;
  font-size: 24rpx;
  font-weight: 900;
}

.agent-card__body {
  min-width: 0;
  flex: 1 1 auto;
}

.agent-card__title {
  display: block;
  color: var(--color-text-primary);
  font-size: 30rpx;
  font-weight: 800;
}

.agent-card__desc {
  display: block;
  margin-top: 8rpx;
  color: var(--color-text-secondary);
  font-size: 23rpx;
  line-height: 1.5;
}

.agent-card__tags {
  display: flex;
  flex-wrap: wrap;
  gap: 10rpx;
  margin-top: 16rpx;
}

:deep(.agent-card__tag) {
  background: rgba(125, 141, 246, 0.1) !important;
  color: var(--color-primary-dark) !important;
  border-color: transparent !important;
}
</style>
