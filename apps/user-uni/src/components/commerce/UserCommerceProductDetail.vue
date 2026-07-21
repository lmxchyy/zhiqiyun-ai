<template>
  <view :class="['commerce-page', kind]">
    <view class="commerce-header"><button type="button" @click="backOrHome('/pages/user/UserMinePage')">‹</button><text>{{ isMember ? 'Pro会员' : '代理商' }}</text><view /></view>
    <view class="commerce-content">
      <view v-if="loading" class="commerce-card"><text>正在加载商品...</text></view>
      <template v-else-if="product">
        <view class="product-hero">
          <text class="hero-tag">{{ isMember ? '年度会员 · 365天' : '商业身份 · 官方代理' }}</text>
          <text class="hero-title">{{ isMember ? '知启云AI Pro年度会员' : '知启云AI官方代理商' }}</text>
          <text class="hero-price">{{ amountLabel }}{{ isMember ? ' / 年' : '' }}</text>
          <text class="hero-copy">{{ isMember ? '适合长期使用AI，提高生产效率' : '适合推广知启云AI，获得客户与收益' }}</text>
        </view>
        <view class="commerce-card arrival-card"><text>开通即到账</text><strong>{{ pointsLabel }}</strong><small>{{ isMember ? '并开通或延长Pro会员365天' : '并开通官方代理商商业身份' }}</small></view>
        <view class="commerce-card benefits-card">
          <view v-for="benefit in benefits" :key="benefit"><text class="check">✓</text><text>{{ benefit }}</text></view>
        </view>
        <view v-if="oppositeIdentityNotice" class="identity-notice">{{ oppositeIdentityNotice }}</view>
        <view :class="['payment-capability', paymentCapability]">{{ capabilityLabel }}</view>
        <button class="buy-button" type="button" :disabled="!canPay" @click="confirmOrder">{{ buyButtonLabel }}</button>
      </template>
      <view v-else class="commerce-card"><text>{{ errorMessage || '商品暂不可购买' }}</text></view>
    </view>
  </view>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { onShow } from '@dcloudio/uni-app'
import { api, businessSdk, getClientPlatform } from '../../api/client'
import { getPaymentCapability } from '../../features/payment/api'
import type { PaymentCapability, VirtualPaymentProduct, VirtualPaymentProductsResponse } from '../../features/payment/types'
import { backOrHome } from '../../utils/miniProgramBusiness'

const props = defineProps<{ kind: 'member' | 'agent'; productCode: string }>()
const loading = ref(false)
const product = ref<VirtualPaymentProduct | null>(null)
const errorMessage = ref('')
const hasMember = ref(false)
const hasAgent = ref(false)
const paymentCapability = ref<PaymentCapability>('unavailable')
const paymentStatus = ref('UNAVAILABLE')
const isMember = computed(() => props.kind === 'member')
const amountLabel = computed(() => '¥' + ((product.value?.amountCent || 0) / 100).toFixed(2))
const pointsLabel = computed(() => Number(product.value?.creditUnits || 0).toLocaleString() + ' 点')
const canPay = computed(() => paymentCapability.value === 'available' && paymentStatus.value === 'READY')
const capabilityLabel = computed(() => {
  if (paymentCapability.value === 'preparing') return '支付准备中'
  if (paymentCapability.value === 'unavailable') return '当前平台暂未开放支付'
  return '支付服务已就绪'
})
const buyButtonLabel = computed(() => canPay.value ? '立即支付 ' + amountLabel.value : '暂未开放')
const benefits = computed(() => isMember.value
  ? ['365天Pro会员', '高级会员模型权限', 'AI生图 / 视频会员价格', 'PPT高级模板', '更多知识库容量', '更多智能体数量', '优先排队与更快生成']
  : ['官方代理商身份', '专属邀请码与推广二维码', '代理数据中心', '客户管理', '推广返佣资格', '营销素材中心', '专属代理服务支持'])
const oppositeIdentityNotice = computed(() => {
  if (!isMember.value && hasMember.value) return '您当前已是Pro会员。开通代理后，原会员权益继续有效，并额外到账20,000点。'
  if (isMember.value && hasAgent.value) return '您当前已是官方代理商。开通Pro会员后，代理身份与返佣权益继续有效，并额外到账40,000点。'
  return ''
})

function matchesCommerceProduct(item: VirtualPaymentProduct) {
  if (item.productCode === props.productCode) return true

  if (props.kind === 'member') {
    return item.id === 'plan_ai_creator_996' || item.productCode === 'MEMBER_YEAR_996'
  }

  return item.id === 'plan_agent_join_996' || item.productCode === 'AGENT_JOIN_996'
}

async function load() {
  loading.value = true
  errorMessage.value = ''
  try {
    const [catalog, profileResult, capabilityResult] = await Promise.allSettled([
      api<VirtualPaymentProductsResponse>('/api/v1/payment/products'),
      businessSdk.roleWorkbench.memberProfile(),
      getPaymentCapability(getClientPlatform()),
    ])
    if (catalog.status === 'rejected') throw catalog.reason
    product.value = catalog.value.items.find(item => item.active && matchesCommerceProduct(item)) || null
    if (!product.value) errorMessage.value = '当前环境未启用该商品'
    if (capabilityResult.status === 'fulfilled') {
      paymentCapability.value = capabilityResult.value.paymentCapability
      paymentStatus.value = capabilityResult.value.paymentStatus
    } else {
      paymentCapability.value = 'unavailable'
      paymentStatus.value = 'UNAVAILABLE'
    }
    if (profileResult.status === 'fulfilled') {
      const payload = profileResult.value as unknown as Record<string, any>
      const user = (payload.user || {}) as Record<string, any>
      const level = String(user.memberLevel || '').toUpperCase()
      hasMember.value = Boolean(level && level !== 'FREE')
      hasAgent.value = String(user.agentStatus || '').toUpperCase() === 'ACTIVE' || Boolean(payload.agent)
    }
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : '商品加载失败'
  } finally {
    loading.value = false
  }
}

function confirmOrder() {
  if (!product.value || !canPay.value) return
  uni.navigateTo({ url: `/pages/user/UserCommerceOrderConfirmPage?productCode=${encodeURIComponent(product.value.productCode)}&kind=${props.kind}` })
}

onShow(() => void load())
</script>

<style scoped>
.commerce-page { min-height: 100vh; color: #111827; background: #f6f8fc; }
.commerce-header { display: grid; height: 112rpx; padding: 0 32rpx; grid-template-columns: 60rpx 1fr 60rpx; align-items: center; background: #fff; font-size: 32rpx; font-weight: 700; text-align: center; }
.commerce-header button { width: 60rpx; height: 80rpx; margin: 0; padding: 0; border: 0; background: transparent; font-size: 56rpx; line-height: 80rpx; text-align: left; }
.commerce-header button::after { display: none; }
.commerce-content { display: flex; flex-direction: column; gap: 32rpx; padding: 32rpx; }
.product-hero { display: flex; height: 392rpx; box-sizing: border-box; flex-direction: column; gap: 20rpx; padding: 40rpx; border-radius: 40rpx; color: #fff; background: linear-gradient(115deg, #635bff, #4c3fd9); }
.agent .product-hero { background: linear-gradient(115deg, #ff7a1a, #f45d00); }
.hero-tag { align-self: flex-start; padding: 12rpx 20rpx; border-radius: 999rpx; background: rgba(255,255,255,.2); font-size: 22rpx; }
.hero-title { font-size: 44rpx; font-weight: 800; line-height: 64rpx; }.hero-price { font-size: 68rpx; font-weight: 800; line-height: 88rpx; }.hero-copy { font-size: 24rpx; }
.commerce-card { display: flex; box-sizing: border-box; flex-direction: column; gap: 16rpx; padding: 32rpx; border: 2rpx solid #e8ecf3; border-radius: 32rpx; background: #fff; box-shadow: 0 8rpx 24rpx rgba(16,24,40,.05); }
.arrival-card { color: #667085; font-size: 24rpx; }.arrival-card strong { color: #111827; font-size: 56rpx; line-height: 80rpx; }.arrival-card small { color: #635bff; font-size: 24rpx; }.agent .arrival-card small { color: #ff7a1a; }
.benefits-card { gap: 20rpx; }.benefits-card view { display: flex; align-items: center; gap: 20rpx; font-size: 26rpx; font-weight: 600; }.check { display: flex; width: 40rpx; height: 40rpx; flex: none; align-items: center; justify-content: center; border-radius: 50%; color: #fff; background: #635bff; }.agent .check { background: #ff7a1a; }
.identity-notice { padding: 24rpx; border: 2rpx solid #ffd5b5; border-radius: 24rpx; color: #f45d00; background: #fff3e9; font-size: 24rpx; line-height: 36rpx; }
.buy-button { display: flex; height: 104rpx; margin: 0; align-items: center; justify-content: center; border: 0; border-radius: 28rpx; color: #fff; background: #635bff; font-size: 30rpx; font-weight: 700; line-height: 104rpx; }.agent .buy-button { background: #ff7a1a; }.buy-button::after { display: none; }.buy-button[disabled] { opacity: .5; }
.payment-capability { padding: 20rpx 24rpx; border-radius: 20rpx; color: #667085; background: #fff; font-size: 24rpx; text-align: center; }.payment-capability.available { color: #087443; background: #effcf5; }.payment-capability.preparing { color: #b54708; background: #fffaeb; }
</style>
