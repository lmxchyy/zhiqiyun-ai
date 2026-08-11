<template>
  <view class="test-payment-page">
    <view class="test-card">
      <text class="test-badge">白名单测试入口</text>
      <text class="test-title">微信虚拟支付白名单测试</text>
      <text class="test-copy">从「我的 → 支付测试」进入。报价、金额、权益和微信商品均由服务端返回；非白名单用户会被拒绝。</text>
    </view>

    <view v-if="!planId" class="test-card">
      <text class="test-name">选择测试方案</text>
      <button class="test-button" @click="selectTarget('member')">会员 ¥1 TEST</button>
      <button class="test-button secondary" @click="selectTarget('agent')">代理 ¥1 TEST</button>
    </view>
    <view v-else-if="loading" class="test-card"><text>正在校验白名单并获取测试报价…</text></view>
    <view v-else-if="quote" class="test-card">
      <text class="test-name">{{ quote.name }}</text>
      <text class="test-price">¥{{ (quote.amountCent / 100).toFixed(2) }}</text>
      <text class="test-copy">环境：{{ quote.environment }} · 报价有效期至 {{ expiresText }}</text>
      <text class="test-copy">pricePlanId：{{ quote.pricePlanId }}</text>
      <button class="test-button" :disabled="paying" @click="pay">{{ paying ? '正在发起支付…' : '使用此测试报价支付' }}</button>
      <button class="test-button secondary" :disabled="paying" @click="loadQuote">刷新测试报价</button>
      <button class="test-button secondary" :disabled="paying" @click="clearTarget">切换 MEMBER / AGENT</button>
    </view>
    <view v-else class="test-card">
      <text class="test-error">{{ errorMessage || '未获得测试报价' }}</text>
      <button class="test-button secondary" @click="loadQuote">重试</button>
      <button class="test-button secondary" @click="clearTarget">切换 MEMBER / AGENT</button>
    </view>
  </view>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { onLoad } from '@dcloudio/uni-app'
import { createTestPriceQuote, createVirtualPaymentOrderFromQuote } from '../../features/payment/api'
import { getWeChatVirtualPaymentLoginCode, requestWeChatVirtualPayment, virtualPaymentError } from '../../features/payment/platform'
import { hasValidToken, requireAuth } from '../../features/auth/gate'
import type { PricePlanQuote } from '../../features/payment/types'

const paymentTestTargets = {
  member: {
    planId: 'plan_ai_creator_996',
    pricePlanId: 'price_plan_20260728212634000000000_049a91b1',
  },
  agent: {
    planId: 'plan_agent_join_996',
    pricePlanId: 'price_plan_20260728212634000000000_2ec1c485',
  },
} as const

const loading = ref(false)
const paying = ref(false)
const quote = ref<PricePlanQuote | null>(null)
const errorMessage = ref('')
const planId = ref('')
const requestedPricePlanId = ref('')
const expiresText = computed(() => quote.value ? new Date(quote.value.expiresAt).toLocaleString() : '')

onLoad((options) => {
  planId.value = String(options?.planId || '').trim()
  requestedPricePlanId.value = String(options?.pricePlanId || '').trim()
  if (!hasValidToken()) {
    void requireAuth({
      action: 'recharge',
      route: '/pages/user/UserVirtualPaymentTestPage',
      payload: { planId: planId.value, pricePlanId: requestedPricePlanId.value },
      autoResume: false,
    })
    return
  }
  if (planId.value) void loadQuote()
})

function selectTarget(kind: keyof typeof paymentTestTargets) {
  const target = paymentTestTargets[kind]
  planId.value = target.planId
  requestedPricePlanId.value = target.pricePlanId
  void loadQuote()
}

function clearTarget() {
  planId.value = ''
  requestedPricePlanId.value = ''
  quote.value = null
  errorMessage.value = ''
}

async function loadQuote() {
  if (!planId.value) {
    errorMessage.value = '缺少 planId，无法请求测试报价。'
    return
  }
  loading.value = true
  quote.value = null
  errorMessage.value = ''
  try {
    const result = await createTestPriceQuote(planId.value, requestedPricePlanId.value)
    if (!result.testOnly) throw new Error('服务端未返回测试专用价格方案')
    quote.value = result
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : '测试报价获取失败'
  } finally {
    loading.value = false
  }
}

async function pay() {
  if (!quote.value || paying.value) return
  paying.value = true
  errorMessage.value = ''
  try {
    const wxLoginCode = await getWeChatVirtualPaymentLoginCode()
    const order = await createVirtualPaymentOrderFromQuote(quote.value.quoteId, wxLoginCode)
    await requestWeChatVirtualPayment(order)
    await uni.showModal({ title: '支付请求已返回', content: '请到订单页确认服务端回调与 V2 快照履约结果。', showCancel: false })
  } catch (error) {
    errorMessage.value = virtualPaymentError(error).message
  } finally {
    paying.value = false
  }
}
</script>

<style scoped>
.test-payment-page { min-height: 100vh; padding: 48rpx 28rpx; background: #f4f6fb; box-sizing: border-box; }
.test-card { display: flex; flex-direction: column; gap: 22rpx; margin-bottom: 24rpx; padding: 32rpx; background: #fff; border-radius: 24rpx; }
.test-badge { align-self: flex-start; padding: 8rpx 16rpx; color: #9a3412; background: #ffedd5; border-radius: 999rpx; font-size: 22rpx; }
.test-title, .test-name { color: #172033; font-size: 34rpx; font-weight: 700; }
.test-price { color: #e5484d; font-size: 56rpx; font-weight: 800; }
.test-copy { color: #667085; font-size: 26rpx; line-height: 1.65; word-break: break-all; }
.test-error { color: #b42318; font-size: 28rpx; }
.test-button { margin: 12rpx 0 0; color: #fff; background: #4a6cff; border-radius: 16rpx; }
.test-button.secondary { color: #344054; background: #eef1f7; }
</style>
