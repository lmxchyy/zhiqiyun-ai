<template>
  <view class="work-card zq-card">
    <view class="work-card__thumb" :class="`work-card__thumb--${item.type}`">
      {{ workTypeLabel(item.type).slice(0, 1) }}
    </view>
    <view class="work-card__body">
      <text class="work-card__title">{{ item.title }}</text>
      <text class="work-card__meta">{{ workTypeLabel(item.type) }} · {{ item.model }} · {{ item.createdAt }}</text>
      <text class="work-card__prompt">{{ item.prompt }}</text>
    </view>
    <view class="work-card__side">
      <wd-tag :type="item.status === 'failed' ? 'danger' : item.status === 'succeeded' ? 'success' : 'primary'" round>
        {{ statusLabel(item.status) }}
      </wd-tag>
      <slot name="actions" />
    </view>
  </view>
</template>

<script setup lang="ts">
import type { WorkItem } from '@/types/domain'
import { statusLabel, workTypeLabel } from '@/utils/format'

defineProps<{
  item: WorkItem
}>()
</script>

<style scoped lang="scss">
.work-card {
  display: flex;
  align-items: center;
  gap: 20rpx;
  padding: 20rpx;
}

.work-card__thumb {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 96rpx;
  height: 96rpx;
  flex: 0 0 96rpx;
  border-radius: var(--radius-md);
  font-size: 28rpx;
  font-weight: 900;
}

.work-card__thumb--image {
  background: rgba(125, 141, 246, 0.14);
  color: var(--color-primary-dark);
}

.work-card__thumb--video {
  background: rgba(24, 160, 88, 0.12);
  color: #0f766e;
}

.work-card__thumb--ppt {
  background: rgba(255, 119, 27, 0.14);
  color: var(--color-accent);
}

.work-card__body {
  min-width: 0;
  flex: 1 1 auto;
}

.work-card__title,
.work-card__meta,
.work-card__prompt {
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.work-card__title {
  color: var(--color-text-primary);
  font-size: 28rpx;
  font-weight: 800;
}

.work-card__meta {
  margin-top: 8rpx;
  color: var(--color-text-secondary);
  font-size: 22rpx;
}

.work-card__prompt {
  margin-top: 8rpx;
  color: var(--color-text-muted);
  font-size: 21rpx;
}

.work-card__side {
  display: grid;
  justify-items: end;
  gap: 12rpx;
}
</style>
