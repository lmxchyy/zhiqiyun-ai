<template>
  <view class="balance-card zq-card">
    <view>
      <text class="balance-card__label">积分余额</text>
      <text class="balance-card__value">{{ pointsText }}</text>
      <text class="balance-card__hint">{{ memberLevel }} · 今日可继续创作</text>
    </view>
    <view class="balance-card__actions">
      <wd-button type="primary" size="small" custom-class="balance-card__button" @click="$emit('recharge')">
        充值
      </wd-button>
      <wd-button plain size="small" custom-class="balance-card__ghost" @click="$emit('membership')">
        会员
      </wd-button>
    </view>
  </view>
</template>

<script setup lang="ts">
defineProps<{
  pointsText: string
  memberLevel: string
}>()

defineEmits<{
  recharge: []
  membership: []
}>()
</script>

<style scoped lang="scss">
.balance-card {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 20rpx;
  padding: 30rpx;
}

.balance-card__label,
.balance-card__hint {
  display: block;
  color: var(--color-text-secondary);
  font-size: 24rpx;
}

.balance-card__value {
  display: block;
  margin: 8rpx 0;
  color: var(--color-text-primary);
  font-size: 56rpx;
  font-weight: 900;
  line-height: 1;
}

.balance-card__actions {
  display: grid;
  gap: 14rpx;
  min-width: 132rpx;
}

:deep(.balance-card__button) {
  background: var(--color-accent) !important;
  border-color: var(--color-accent) !important;
}

:deep(.balance-card__ghost) {
  color: var(--color-primary-dark) !important;
  border-color: rgba(125, 141, 246, 0.34) !important;
}
</style>
