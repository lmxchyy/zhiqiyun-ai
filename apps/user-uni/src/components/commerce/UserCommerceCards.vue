<template>
  <view class="zc-stack">
    <view class="zc-wallet">
      <view class="zc-wallet-glow" />
      <view class="zc-wallet-copy">
        <text class="zc-eyebrow">我的点数</text>
        <view class="zc-balance"><text>{{ pointBalanceReady ? pointBalance : '--' }}</text><text class="zc-wallet-unit">点</text></view>
        <text class="zc-wallet-hint">{{ pointBalanceLoading ? '正在同步账户余额…' : pointBalanceReady ? '用于 AI 创作与增值服务' : '余额暂未同步，下拉可重试' }}</text>
      </view>
      <button class="zc-wallet-button" type="button" @click="$emit('recharge')">充值点数 <text>›</text></button>
    </view>

    <view ref="productWrapperRef" class="product-scale-wrapper" :style="{ height: `${scaledProductHeight}px` }">
      <view
        class="product-scale-content"
        :style="{ transform: `scale(${productScale})`, transformOrigin: 'top left' }"
      >
        <view class="product-design-shell">
          <view class="product-card-container pro-container">
            <view class="recommend-pill">推荐 · 适合自己使用AI</view>
            <view class="product-main-card pro-card">
              <view class="badge-section">
                <view class="badge-visual">
                  <image class="badge-image" :src="proBadge" mode="aspectFit" />
                  <text class="badge-letter">V</text>
                </view>
                <text class="identity-pill pro-identity">PRO</text>
              </view>

              <view class="content-section">
                <text class="product-title">知启云AI Pro 年度会员</text>
                <text class="product-subtitle">降低AI使用成本，提升日常生产效率</text>
                <view class="core-benefits pro-benefits">
                  <view class="benefit-item"><text class="benefit-check">✓</text><text>到账 40,000 点</text></view>
                  <view class="benefit-item"><text class="benefit-check">✓</text><text>365天会员权益</text></view>
                </view>
              </view>

              <view class="product-divider pro-divider" />

              <view class="purchase-section">
                <text :class="['status-pill', { active: memberActive }]">{{ memberActive ? '生效中' : '未开通' }}</text>
                <view class="price-row"><text class="currency">¥</text><text class="price-value">996</text><text class="price-unit">/ 年</text></view>
                <text class="price-note">≈ 每天仅 ¥2.7</text>
                <button class="purchase-button pro-button" type="button" @click="$emit('member')">
                  {{ memberActive ? '续费 Pro 会员' : '开通 Pro 会员' }} <text>›</text>
                </button>
                <text class="detail-link">{{ memberActive ? `会员有效期至 ${memberExpiresText}` : '查看全部会员权益  ›' }}</text>
              </view>
            </view>
          </view>

          <view class="product-card-container agent-container">
            <view class="recommend-pill">推荐 · 适合推广赚钱</view>
            <view class="product-main-card agent-card">
              <view class="badge-section">
                <view class="badge-visual">
                  <image class="badge-image" :src="agentBadge" mode="aspectFit" />
                  <text class="badge-letter">↑</text>
                </view>
                <text class="identity-pill agent-identity">PARTNER</text>
              </view>

              <view class="content-section">
                <text class="product-title">知启云AI 官方代理商</text>
                <text class="product-subtitle">推广知启云AI，获得客户与分润收益</text>
                <view class="core-benefits agent-benefits">
                  <view class="benefit-item"><text class="benefit-check">✓</text><text>到账 20,000 点</text></view>
                  <view class="benefit-item"><text class="benefit-check">✓</text><text>推广与返佣资格</text></view>
                </view>
              </view>

              <view class="product-divider agent-divider" />

              <view class="purchase-section">
                <text :class="['status-pill', { active: agentActive }]">{{ agentActive ? '已认证' : '未开通' }}</text>
                <view class="price-row"><text class="currency">¥</text><text class="price-value">996</text></view>
                <text class="price-note">一次投入 · 长期收益</text>
                <button class="purchase-button agent-button" type="button" @click="$emit('agent')">
                  {{ agentActive ? '进入代理中心' : '成为代理商' }} <text>›</text>
                </button>
                <text class="detail-link agent-detail">查看全部代理权益  ›</text>
              </view>
            </view>
          </view>

          <view class="dual-identity-note">
            <text class="dual-pill">PRO + 代理</text>
            <text class="dual-title">两个产品可以同时拥有</text>
            <view class="dual-divider" />
            <text class="dual-copy">开通代理不会取消已有会员权益，会员 + 代理 = 更强的效率与收益！</text>
            <text class="dual-arrow">›</text>
          </view>
        </view>
      </view>
    </view>
  </view>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import proBadge from '../../assets/commerce/pro-badge.svg'
import agentBadge from '../../assets/commerce/agent-badge.svg'

const DESIGN_WIDTH = 1440
const DESIGN_HEIGHT = 846
const PAGE_HORIZONTAL_PADDING = 32

withDefaults(defineProps<{
  pointBalance: number
  pointBalanceLoading?: boolean
  pointBalanceReady?: boolean
  memberActive: boolean
  memberExpiresText: string
  agentActive: boolean
  inviteCode: string
  promotionCount: number
  pendingCommissionCents: number
}>(), {
  pointBalanceLoading: false,
  pointBalanceReady: true,
})

defineEmits<{ recharge: []; member: []; agent: [] }>()

function scaleForWidth(availableWidth: number) {
  return Math.max(0, availableWidth) / DESIGN_WIDTH
}

function currentWindowWidth() {
  let windowWidth = Number(uni.getSystemInfoSync().windowWidth || DESIGN_WIDTH + PAGE_HORIZONTAL_PADDING)
  // #ifdef H5
  if (typeof document !== 'undefined' && document.documentElement.clientWidth > 0) {
    windowWidth = document.documentElement.clientWidth
  }
  // #endif
  return windowWidth
}

function initialProductScale() {
  return scaleForWidth(currentWindowWidth() - PAGE_HORIZONTAL_PADDING)
}

const productScale = ref(initialProductScale())
const productWrapperRef = ref<unknown>(null)
const scaledProductHeight = computed(() => DESIGN_HEIGHT * productScale.value)

function updateProductScale(windowWidth?: number) {
  const measuredWidth = Number(windowWidth || currentWindowWidth())
  const availableWidth = Math.max(0, measuredWidth - PAGE_HORIZONTAL_PADDING)
  productScale.value = scaleForWidth(availableWidth)
}

function handleWindowResize(event: { size?: { windowWidth?: number } }) {
  updateProductScale(event?.size?.windowWidth)
  void nextTick(measureRenderedWidth)
}

function measureRenderedWidth() {
  const wrapper = productWrapperRef.value as { $el?: Element; getBoundingClientRect?: () => DOMRect } | null
  const element = wrapper?.$el || wrapper
  const rect = element && typeof element.getBoundingClientRect === 'function'
    ? element.getBoundingClientRect()
    : null
  if (rect && rect.width > 0) productScale.value = scaleForWidth(rect.width)
}

onMounted(() => {
  updateProductScale()
  void nextTick(measureRenderedWidth)
  uni.onWindowResize(handleWindowResize)
})

onBeforeUnmount(() => {
  uni.offWindowResize(handleWindowResize)
})
</script>

<style>
.zc-stack { display: flex; min-width: 0; max-width: 100%; flex-direction: column; gap: 20rpx; margin: 20rpx 0 32rpx; overflow: hidden; font-family: "PingFang SC", -apple-system, BlinkMacSystemFont, "Helvetica Neue", "Microsoft YaHei", sans-serif; }
.zc-wallet { position: relative; display: flex; min-height: 168rpx; box-sizing: border-box; align-items: center; justify-content: space-between; padding: 22rpx 26rpx; overflow: hidden; border: 1rpx solid rgba(255,255,255,.16); border-radius: 30rpx; color: #fff; background: linear-gradient(128deg, #695cff 0%, #5848f3 52%, #4936d8 100%); box-shadow: 0 14rpx 32rpx rgba(75,59,205,.20); }
.zc-wallet-glow { position: absolute; z-index: 0; top: -126rpx; right: -42rpx; width: 280rpx; height: 280rpx; border: 38rpx solid rgba(255,255,255,.055); border-radius: 50%; pointer-events: none; }
.zc-wallet-copy { position: relative; z-index: 1; display: flex; min-width: 0; flex-direction: column; }
.zc-eyebrow { color: rgba(255,255,255,.82); font-size: 22rpx; font-weight: 500; line-height: 32rpx; letter-spacing: .5rpx; }
.zc-balance { display: flex; margin-top: 1rpx; align-items: baseline; gap: 7rpx; font-size: 58rpx; font-weight: 700; line-height: 66rpx; letter-spacing: -.5rpx; }
.zc-wallet-unit { color: rgba(255,255,255,.94); font-size: 24rpx; font-weight: 600; line-height: 32rpx; }
.zc-wallet-hint { margin-top: 1rpx; color: rgba(255,255,255,.70); font-size: 20rpx; font-weight: 400; line-height: 28rpx; }
.zc-wallet-button { position: relative; z-index: 1; width: auto; min-width: 164rpx; height: 64rpx; margin: 0; padding: 0 22rpx; border: 1rpx solid rgba(255,255,255,.62); border-radius: 999rpx; color: #5144da; background: rgba(255,255,255,.97); font-size: 24rpx; font-weight: 600; line-height: 64rpx; box-shadow: 0 8rpx 20rpx rgba(38,26,132,.18); }
.zc-wallet-button:active { opacity: .94; transform: scale(.98); }
.zc-wallet-button text { margin-left: 6rpx; font-size: 29rpx; font-weight: 500; }
.zc-wallet-button::after, .purchase-button::after { display: none; }

.product-scale-wrapper { position: relative; width: 100%; min-width: 0; overflow: hidden; }
.product-scale-content { width: 1440px; height: 846px; transform-origin: top left; }
.product-design-shell { display: flex; width: 1440px; height: 846px; box-sizing: border-box; padding: 26px 30px; flex-direction: column; flex-wrap: nowrap; gap: 24px; overflow: hidden; border-radius: 28px; background: linear-gradient(90deg, #f7f8ff 0%, #eef2ff 100%); box-shadow: 0 16px 18px rgba(43,36,115,.12); font-family: "PingFang SC", -apple-system, BlinkMacSystemFont, "Helvetica Neue", "Microsoft YaHei", sans-serif; }
.product-card-container { position: relative; width: 1380px; height: 332px; flex: 0 0 332px; }
.recommend-pill { position: absolute; z-index: 3; top: 0; left: 0; padding: 8px 18px; border-radius: 999px; color: #fff; background: #6942ff; font-size: 22px; font-weight: 600; line-height: 28px; white-space: nowrap; }
.agent-container .recommend-pill { background: #ff6b00; }
.product-main-card { position: absolute; top: 32px; left: 0; display: flex; width: 1380px; height: 300px; box-sizing: border-box; padding: 18px 28px; flex-direction: row; flex-wrap: nowrap; align-items: center; gap: 22px; border: 1.5px solid #cec8ff; border-radius: 34px; background: linear-gradient(90deg, #f8f6ff 0%, #eef0ff 100%); box-shadow: 0 12px 14px rgba(41,31,107,.13); }
.agent-card { border-color: #ffcbaa; background: linear-gradient(90deg, #fff8f2 0%, #fff0e4 100%); }
.badge-section { display: flex; width: 190px; height: 264px; flex: 0 0 190px; align-items: center; justify-content: center; flex-direction: column; flex-wrap: nowrap; gap: 10px; }
.badge-visual { position: relative; width: 150px; height: 145px; flex: 0 0 145px; overflow: visible; }
.badge-image { width: 150px; height: 145px; }
.badge-letter { position: absolute; top: 46px; left: 0; width: 150px; color: #fff; font-size: 40px; font-weight: 800; line-height: 48px; text-align: center; }
.identity-pill { padding: 8px 18px; border-radius: 999px; color: #fff; font-size: 22px; font-weight: 600; line-height: 28px; white-space: nowrap; }
.pro-identity { background: #6b4bff; }
.agent-identity { background: #ff7a16; }
.content-section { display: flex; width: 760px; height: 264px; flex: 0 0 760px; align-items: flex-start; justify-content: center; flex-direction: column; flex-wrap: nowrap; gap: 16px; white-space: nowrap; }
.product-title { color: #0f1633; font-size: 48px; font-weight: 700; line-height: 58px; white-space: nowrap; }
.product-subtitle { color: #28304d; font-size: 28px; font-weight: 500; line-height: 36px; white-space: nowrap; }
.core-benefits { display: flex; flex-direction: row; flex-wrap: nowrap; align-items: center; gap: 14px; font-size: 36px; font-weight: 600; line-height: 44px; white-space: nowrap; }
.benefit-item { display: flex; min-height: 60px; flex: 0 0 auto; box-sizing: border-box; align-items: center; flex-direction: row; flex-wrap: nowrap; gap: 10px; padding: 8px 20px; border-radius: 999px; color: #27304a; background: #ece8ff; white-space: nowrap; }
.benefit-check { flex: 0 0 auto; color: #3827d6; font-weight: 800; }
.agent-benefits .benefit-item { background: #ffe9da; }
.agent-benefits .benefit-check { color: #c83c11; }
.product-divider { width: 1px; height: 246px; flex: 0 0 1px; background: #d8d3ff; }
.agent-divider { background: #ffd4b8; }
.purchase-section { display: flex; width: 330px; height: 264px; flex: 0 0 330px; align-items: center; justify-content: center; flex-direction: column; flex-wrap: nowrap; gap: 8px; white-space: nowrap; }
.status-pill { padding: 8px 18px; border-radius: 999px; color: #3521c5; background: #e8e4ff; font-size: 22px; font-weight: 600; line-height: 28px; white-space: nowrap; }
.status-pill.active { color: #fff; background: #6650ef; }
.agent-card .status-pill { color: #a73512; background: #ffe7d6; }
.agent-card .status-pill.active { color: #fff; background: #f36a13; }
.price-row { display: flex; align-items: baseline; flex-direction: row; flex-wrap: nowrap; gap: 7px; color: #080d27; white-space: nowrap; }
.currency { color: #0c1230; font-size: 30px; font-weight: 600; line-height: 38px; }
.price-value { font-size: 66px; font-weight: 700; line-height: 72px; }
.price-unit { color: #11152f; font-size: 24px; font-weight: 500; line-height: 30px; }
.price-note { color: #535b77; font-size: 20px; font-weight: 500; line-height: 26px; white-space: nowrap; }
.purchase-button { display: flex; width: auto; height: auto; margin: 0; padding: 16px 32px; align-items: center; justify-content: center; gap: 12px; border: 0; border-radius: 20px; color: #fff; font-size: 28px; font-weight: 700; line-height: 36px; white-space: nowrap; }
.purchase-button text { font-size: 34px; line-height: 34px; }
.pro-button { background: linear-gradient(90deg, #7456ff 0%, #3d23f3 100%); box-shadow: 0 8px 9px rgba(61,35,243,.28); }
.agent-button { background: linear-gradient(90deg, #ff8a18 0%, #f03b00 100%); box-shadow: 0 8px 9px rgba(240,59,0,.28); }
.detail-link { color: #3827d6; font-size: 18px; font-weight: 500; line-height: 24px; white-space: nowrap; }
.agent-detail { color: #c83c11; }
.dual-identity-note { display: flex; width: 1380px; height: 82px; flex: 0 0 82px; box-sizing: border-box; padding: 18px 22px; align-items: center; flex-direction: row; flex-wrap: nowrap; gap: 18px; border: 1px solid #dad6ff; border-radius: 26px; background: linear-gradient(90deg, #f6f4ff 0%, #eef1ff 100%); white-space: nowrap; }
.dual-pill { flex: 0 0 auto; padding: 8px 18px; border-radius: 999px; color: #4b32d5; background: #eae5ff; font-size: 20px; font-weight: 600; line-height: 26px; }
.dual-title { flex: 0 0 auto; color: #141936; font-size: 26px; font-weight: 700; line-height: 32px; }
.dual-divider { width: 1px; height: 30px; flex: 0 0 1px; background: #cfc9f4; }
.dual-copy { flex: 0 0 auto; color: #323a5b; font-size: 20px; font-weight: 500; line-height: 26px; }
.dual-arrow { margin-left: auto; color: #2f25d0; font-size: 32px; font-weight: 800; line-height: 38px; }
</style>
