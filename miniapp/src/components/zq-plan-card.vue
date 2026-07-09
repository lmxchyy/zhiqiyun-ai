<template>
  <view class="plan-card zq-card" :class="{ 'plan-card--recommended': recommended }">
    <view class="plan-card__head">
      <view>
        <text class="plan-card__name">{{ name }}</text>
        <text class="plan-card__points">{{ points.toLocaleString('zh-CN') }} 积分</text>
      </view>
      <wd-tag v-if="recommended" type="warning" round>推荐</wd-tag>
    </view>
    <text class="plan-card__price">{{ price }}</text>
    <view class="plan-card__benefits">
      <text v-for="benefit in benefits" :key="benefit">· {{ benefit }}</text>
    </view>
  </view>
</template>

<script setup lang="ts">
defineProps<{
  name: string
  price: string
  points: number
  benefits: string[]
  recommended?: boolean
}>()
</script>

<style scoped lang="scss">
.plan-card {
  min-width: 280rpx;
  padding: 24rpx;
}

.plan-card--recommended {
  border-color: rgba(255, 119, 27, 0.4);
  background: linear-gradient(180deg, rgba(255, 119, 27, 0.09), #fff 48%);
}

.plan-card__head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12rpx;
}

.plan-card__name,
.plan-card__points,
.plan-card__price,
.plan-card__benefits text {
  display: block;
}

.plan-card__name {
  color: var(--color-text-primary);
  font-size: 28rpx;
  font-weight: 800;
}

.plan-card__points,
.plan-card__benefits text {
  color: var(--color-text-secondary);
  font-size: 22rpx;
  line-height: 1.55;
}

.plan-card__price {
  margin-top: 18rpx;
  color: var(--color-accent);
  font-size: 40rpx;
  font-weight: 900;
}

.plan-card__benefits {
  margin-top: 14rpx;
}
</style>
