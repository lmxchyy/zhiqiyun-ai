<template>
  <zq-page-shell>
    <view class="profile-card zq-card">
      <wd-avatar :text="user.avatarText" size="large" bg-color="#7D8DF6" color="#fff" />
      <view class="profile-card__main">
        <text class="profile-card__name">{{ user.name }}</text>
        <text class="profile-card__level">{{ user.memberLevel }} · 小程序用户端</text>
      </view>
      <wd-tag type="success" round>已登录</wd-tag>
    </view>

    <view class="profile-balance zq-card">
      <view>
        <text class="profile-balance__label">积分余额</text>
        <text class="profile-balance__value">{{ formatNumber(user.points) }}</text>
      </view>
      <wd-button type="primary" custom-class="profile-balance__button" @click="showMockToast('充值收银台接口已预留')">
        立即充值
      </wd-button>
    </view>

    <view class="zq-section">
      <view class="zq-section__head">
        <text class="zq-section__title">会员套餐</text>
        <text class="zq-section__more" @click="showMockToast('会员套餐详情页已预留')">更多</text>
      </view>
      <scroll-view scroll-x class="plan-scroll">
        <view class="plan-row">
          <zq-plan-card
            v-for="plan in plans"
            :key="plan.id"
            :name="plan.name"
            :price="plan.price"
            :points="plan.points"
            :benefits="plan.benefits"
            :recommended="plan.recommended"
          />
        </view>
      </scroll-view>
    </view>

    <view class="profile-menu zq-card">
      <view class="profile-menu__item" @click="showMockToast('代理商入口已预留')">
        <text>代理商入口</text>
        <text>邀请客户、查看收益</text>
      </view>
      <view class="profile-menu__item" @click="showMockToast('订单记录接口已预留')">
        <text>订单记录</text>
        <text>会员、充值和套餐订单</text>
      </view>
      <view class="profile-menu__item" @click="showMockToast('设置页面已预留')">
        <text>设置</text>
        <text>账号、安全与通知</text>
      </view>
    </view>

    <wd-button block plain custom-class="logout-button" @click="logout">退出登录</wd-button>
  </zq-page-shell>
</template>

<script setup lang="ts">
import ZqPageShell from '@/components/zq-page-shell.vue'
import ZqPlanCard from '@/components/zq-plan-card.vue'
import { useAppStore } from '@/stores/app'
import { useUserStore } from '@/stores/user'
import { formatNumber } from '@/utils/format'
import { showMockToast } from '@/utils/navigation'

const appStore = useAppStore()
const userStore = useUserStore()
const user = userStore.profile
const plans = userStore.plans

function logout() {
  appStore.logout()
  uni.reLaunch({ url: '/pages/login/index' })
  showMockToast('已退出 mock 登录态')
}
</script>

<style scoped lang="scss">
.profile-card {
  display: flex;
  align-items: center;
  gap: 20rpx;
  margin-top: 42rpx;
  padding: 28rpx;
}

.profile-card__main {
  min-width: 0;
  flex: 1 1 auto;
}

.profile-card__name,
.profile-card__level {
  display: block;
}

.profile-card__name {
  color: var(--color-text-primary);
  font-size: 34rpx;
  font-weight: 900;
}

.profile-card__level {
  margin-top: 8rpx;
  color: var(--color-text-secondary);
  font-size: 23rpx;
}

.profile-balance {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 20rpx;
  margin-top: 24rpx;
  padding: 30rpx;
  background:
    linear-gradient(135deg, rgba(90, 77, 178, 0.92), rgba(125, 141, 246, 0.94)),
    #5A4DB2;
}

.profile-balance__label,
.profile-balance__value {
  display: block;
  color: #fff;
}

.profile-balance__label {
  font-size: 24rpx;
  opacity: 0.82;
}

.profile-balance__value {
  margin-top: 8rpx;
  font-size: 52rpx;
  font-weight: 900;
  line-height: 1;
}

:deep(.profile-balance__button) {
  background: var(--color-accent) !important;
  border-color: var(--color-accent) !important;
}

.plan-scroll {
  width: 100%;
  white-space: nowrap;
}

.plan-row {
  display: inline-flex;
  gap: 18rpx;
  padding-bottom: 8rpx;
}

.profile-menu {
  margin-top: 30rpx;
  overflow: hidden;
}

.profile-menu__item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 18rpx;
  padding: 28rpx;
  border-bottom: 1rpx solid var(--color-border);
}

.profile-menu__item:last-child {
  border-bottom: 0;
}

.profile-menu__item text:first-child {
  color: var(--color-text-primary);
  font-size: 28rpx;
  font-weight: 800;
}

.profile-menu__item text:last-child {
  color: var(--color-text-secondary);
  font-size: 22rpx;
}

:deep(.logout-button) {
  height: 88rpx !important;
  margin-top: 28rpx;
  color: #dc2626 !important;
  border-color: rgba(220, 38, 38, 0.22) !important;
  border-radius: var(--radius-md) !important;
}
</style>
