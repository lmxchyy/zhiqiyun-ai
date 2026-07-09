<template>
  <view class="feature-card zq-card" :class="`feature-card--${tone}`" @click="$emit('tap')">
    <view class="feature-card__icon">{{ icon }}</view>
    <text class="feature-card__title">{{ title }}</text>
    <text class="feature-card__subtitle">{{ subtitle }}</text>
  </view>
</template>

<script setup lang="ts">
defineProps<{
  title: string
  subtitle: string
  icon: string
  tone: 'primary' | 'accent' | 'dark' | 'green'
}>()

defineEmits<{
  tap: []
}>()
</script>

<style scoped lang="scss">
.feature-card {
  min-height: 184rpx;
  padding: 24rpx;
}

.feature-card__icon {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 64rpx;
  height: 64rpx;
  border-radius: var(--radius-md);
  color: #fff;
  font-size: 24rpx;
  font-weight: 900;
}

.feature-card--primary .feature-card__icon {
  background: linear-gradient(135deg, var(--color-primary), var(--color-primary-dark));
}

.feature-card--accent .feature-card__icon {
  background: linear-gradient(135deg, var(--color-accent), #ff9f45);
}

.feature-card--dark .feature-card__icon {
  background: #111827;
}

.feature-card--green .feature-card__icon {
  background: #18a058;
}

.feature-card__title {
  display: block;
  margin-top: 22rpx;
  color: var(--color-text-primary);
  font-size: 30rpx;
  font-weight: 800;
}

.feature-card__subtitle {
  display: block;
  margin-top: 10rpx;
  color: var(--color-text-secondary);
  font-size: 23rpx;
  line-height: 1.45;
}
</style>
