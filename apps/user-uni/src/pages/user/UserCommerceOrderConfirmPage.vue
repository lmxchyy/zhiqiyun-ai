<template>
  <view :class="['order-page', kind]">
    <view class="order-header"><button type="button" @click="backOrHome('/pages/user/UserMinePage')">‹</button><text>确认订单</text><view /></view>
    <view class="order-content">
      <view v-if="loading" class="order-card"><text>正在加载商品...</text></view>
      <template v-else-if="product">
        <view class="order-card product-card">
          <text class="product-name">{{ product.name }}</text>
          <view><text>商品类型</text><strong>{{ isMember ? '年度会员' : '商业身份' }}</strong></view>
          <view v-if="isMember"><text>会员期限</text><strong>365天</strong></view>
          <view><text>赠送点数</text><strong>{{ pointsLabel }}</strong></view>
          <view><text>核心权益</text><strong>{{ isMember ? '会员模型、会员价格、优先队列' : '代理身份、推广、返佣、代理后台' }}</strong></view>
        </view>
        <view class="order-card amount-card">
          <view><text>商品金额</text><strong>{{ amountLabel }}</strong></view>
          <view><text>优惠金额</text><strong>¥0.00</strong></view>
          <view><text>实付金额</text><strong class="accent">{{ amountLabel }}</strong></view>
        </view>
        <view class="order-card payment-card"><view><text>支付方式</text><strong>{{ paymentMethodLabel }}</strong></view><text class="selected">● {{ capabilityMessage }}</text></view>
        <label class="agreement"><checkbox :checked="agreed" @click="agreed = !agreed" />我已阅读并同意《{{ isMember ? '知启云AI会员服务协议' : '知启云AI代理商服务协议' }}》</label>
        <view v-if="message" :class="['order-message', messageTone]">{{ message }}</view>
        <button class="pay-button" type="button" :disabled="paying || !agreed || !canPay" @click="pay">{{ payButtonLabel }}</button>
        <button v-if="orderNo && !completed" class="status-button" type="button" :disabled="syncing" @click="manualSync">{{ syncing ? '正在查询...' : '查看订单状态' }}</button>
      </template>
      <view v-else class="order-card"><text>{{ message || '商品暂不可购买' }}</text></view>
    </view>
  </view>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { onLoad, onUnload } from '@dcloudio/uni-app'
import { getClientPlatform } from '../../api/client'
import {
  createUnifiedPaymentOrder,
  createVirtualPaymentOrder,
  getPaymentCapability,
  getUnifiedPaymentOrder,
  getVirtualPaymentOrderStatus,
  listVirtualPaymentProducts,
  syncVirtualPaymentOrder,
} from '../../features/payment/api'
import { getWeChatVirtualPaymentLoginCode, requestAppPayment, requestWeChatVirtualPayment, virtualPaymentError } from '../../features/payment/platform'
import type {
  PaymentCapability,
  UnifiedPaymentOrderStatus,
  VirtualPaymentOrderStatus,
  VirtualPaymentProduct,
} from '../../features/payment/types'
import { backOrHome } from '../../utils/miniProgramBusiness'

const productCode = ref('')
const kind = ref<'member' | 'agent'>('member')
const platform = ref(getClientPlatform())
const loading = ref(false)
const paying = ref(false)
const syncing = ref(false)
const paymentCapability = ref<PaymentCapability>('unavailable')
const paymentStatus = ref('UNAVAILABLE')
const paymentChannel = ref('')
const capabilityMessage = ref('正在检查支付能力')
const product = ref<VirtualPaymentProduct | null>(null)
const agreed = ref(true)
const orderNo = ref('')
const completed = ref(false)
const message = ref('')
const messageTone = ref('')
const actionLabel = ref('正在创建订单...')
let disposed = false

const isMember = computed(() => kind.value === 'member')
const amountLabel = computed(() => '¥' + ((product.value?.amountCent || 0) / 100).toFixed(2))
const pointsLabel = computed(() => Number(product.value?.creditUnits || 0).toLocaleString() + '点')
const canPay = computed(() => paymentCapability.value === 'available' && paymentStatus.value === 'READY' && Boolean(product.value))
const paymentMethodLabel = computed(() => {
  if (paymentChannel.value.includes('alipay')) return '支付宝支付'
  if (paymentChannel.value.includes('wechat')) return '微信支付'
  return '在线支付'
})
const payButtonLabel = computed(() => {
  if (paying.value) return actionLabel.value
  if (!canPay.value) return '暂未开放'
  return '立即支付 ' + amountLabel.value
})

function matchesCommerceProduct(item: VirtualPaymentProduct) {
  if (item.productCode === productCode.value) return true
  if (isMember.value) return item.id === 'plan_ai_creator_996' || item.productCode === 'MEMBER_YEAR_996'
  return item.id === 'plan_agent_join_996' || item.productCode === 'AGENT_JOIN_996'
}

function createIdempotencyKey() {
  return 'app_pay_' + Date.now().toString(36) + '_' + Math.random().toString(36).slice(2, 10)
}

async function load() {
  loading.value = true
  message.value = ''
  try {
    const [catalog, capability] = await Promise.all([
      listVirtualPaymentProducts(),
      getPaymentCapability(platform.value),
    ])
    product.value = catalog.items.find(item => item.active && matchesCommerceProduct(item)) || null
    paymentCapability.value = capability.paymentCapability
    paymentStatus.value = capability.paymentStatus
    paymentChannel.value = capability.paymentChannel || ''
    capabilityMessage.value = capability.message
    if (!product.value) message.value = '当前环境未启用该商品'
    else if (!capability.enabled) message.value = capability.message
  } catch (error) {
    paymentCapability.value = 'unavailable'
    paymentStatus.value = 'UNAVAILABLE'
    capabilityMessage.value = '支付能力检查失败'
    message.value = error instanceof Error ? error.message : '商品加载失败'
    messageTone.value = 'danger'
  } finally {
    loading.value = false
  }
}

async function pay() {
  if (!product.value || paying.value || !agreed.value || !canPay.value) return
  paying.value = true
  message.value = ''
  messageTone.value = ''
  try {
    if (platform.value === 'mp-weixin') await payWithWechatVirtual()
    else await payWithApp()
  } catch (error) {
    const result = virtualPaymentError(error)
    message.value = result.message
    messageTone.value = result.kind === 'cancelled' ? '' : 'danger'
  } finally {
    paying.value = false
    actionLabel.value = '正在创建订单...'
  }
}

async function payWithWechatVirtual() {
  actionLabel.value = '正在刷新微信登录态...'
  const wxLoginCode = await getWeChatVirtualPaymentLoginCode()
  actionLabel.value = '正在创建订单...'
  const order = await createVirtualPaymentOrder(product.value!.productCode, 1, '', wxLoginCode)
  orderNo.value = order.orderNo
  actionLabel.value = '等待微信支付...'
  await requestWeChatVirtualPayment(order)
  actionLabel.value = '正在确认权益...'
  message.value = '正在确认支付结果，请勿重复支付。'
  await waitForVirtualEntitlements(order.orderNo)
}

async function payWithApp() {
  actionLabel.value = '正在创建订单...'
  const order = await createUnifiedPaymentOrder(
    product.value!.productCode,
    platform.value,
    paymentChannel.value,
    createIdempotencyKey(),
  )
  orderNo.value = order.orderNo
  paymentStatus.value = order.paymentStatus
  actionLabel.value = '等待支付...'
  await requestAppPayment(order.paymentParams, paymentChannel.value)
  actionLabel.value = '正在确认权益...'
  message.value = '支付已提交，正在等待服务端确认。'
  await waitForUnifiedOrder(order.orderNo)
}

async function waitForVirtualEntitlements(currentOrderNo: string) {
  for (let attempt = 0; attempt < 12 && !disposed; attempt += 1) {
    if (attempt === 3 || attempt === 7) await syncVirtualPaymentOrder(currentOrderNo).catch(() => undefined)
    const status = await getVirtualPaymentOrderStatus(currentOrderNo)
    if (status.completed) { finishVirtual(status); return }
    if (status.entitlementStatus === 'FAILED') throw new Error(status.entitlementError || '支付已确认，但权益发放失败，系统将自动补偿')
    await new Promise(resolve => setTimeout(resolve, 1500))
  }
  message.value = '支付结果仍在确认中，可点击“查看订单状态”继续查询。'
}

async function waitForUnifiedOrder(currentOrderNo: string) {
  for (let attempt = 0; attempt < 12 && !disposed; attempt += 1) {
    const status = await getUnifiedPaymentOrder(currentOrderNo)
    paymentStatus.value = status.paymentStatus
    if (status.orderStatus === 'COMPLETED' && status.fulfillmentStatus === 'SUCCESS') {
      finishUnified(status)
      return
    }
    if (status.paymentStatus === 'FAILED' || status.orderStatus === 'FAILED') throw new Error('支付失败，请稍后重试')
    await new Promise(resolve => setTimeout(resolve, 1500))
  }
  message.value = '支付结果仍在确认中，可点击“查看订单状态”继续查询。'
}

function finishVirtual(status: VirtualPaymentOrderStatus) {
  completed.value = true
  paymentStatus.value = 'SUCCESS'
  messageTone.value = 'success'
  message.value = isMember.value ? '支付成功，会员权益已到账。' : '支付成功，代理商权益已到账。'
  uni.$emit('virtual-payment:entitlements-updated', status.balances || {})
  redirectToResult()
}

function finishUnified(_status: UnifiedPaymentOrderStatus) {
  completed.value = true
  paymentStatus.value = 'SUCCESS'
  messageTone.value = 'success'
  message.value = isMember.value ? '支付成功，会员权益已到账。' : '支付成功，代理商权益已到账。'
  redirectToResult()
}

function redirectToResult() {
  const amountCents = product.value?.amountCent || 0
  const points = product.value?.creditUnits || 0
  const url = '/pages/user/UserOrderResultPage?id=' + encodeURIComponent(orderNo.value)
    + '&status=PAID&message=' + encodeURIComponent(message.value)
    + '&amountCents=' + amountCents
    + '&points=' + points
    + '&planName=' + encodeURIComponent(product.value?.name || '')
  setTimeout(() => uni.redirectTo({ url }), 500)
}

async function manualSync() {
  if (!orderNo.value || syncing.value) return
  syncing.value = true
  try {
    if (platform.value === 'mp-weixin') {
      await syncVirtualPaymentOrder(orderNo.value)
      const status = await getVirtualPaymentOrderStatus(orderNo.value)
      if (status.completed) finishVirtual(status)
      else message.value = '微信尚未确认到账，系统会继续自动查询。'
    } else {
      const status = await getUnifiedPaymentOrder(orderNo.value)
      paymentStatus.value = status.paymentStatus
      if (status.orderStatus === 'COMPLETED' && status.fulfillmentStatus === 'SUCCESS') finishUnified(status)
      else message.value = '支付结果尚未确认，系统会继续自动查询。'
    }
  } catch (error) {
    message.value = error instanceof Error ? error.message : '查询失败，请稍后重试'
    messageTone.value = 'danger'
  } finally {
    syncing.value = false
  }
}

onLoad(options => {
  productCode.value = String(options?.productCode || '')
  kind.value = options?.kind === 'agent' ? 'agent' : 'member'
  void load()
})
onUnload(() => { disposed = true })
</script>

<style scoped>
.order-page { min-height: 100vh; color: #111827; background: #f6f8fc; }
.order-header { display: grid; height: 112rpx; padding: 0 32rpx; grid-template-columns: 60rpx 1fr 60rpx; align-items: center; background: #fff; font-size: 32rpx; font-weight: 700; text-align: center; }
.order-header button { width: 60rpx; height: 80rpx; margin: 0; padding: 0; border: 0; background: transparent; font-size: 56rpx; line-height: 80rpx; text-align: left; }.order-header button::after { display: none; }
.order-content { display: flex; flex-direction: column; gap: 32rpx; padding: 32rpx; }
.order-card { display: flex; box-sizing: border-box; flex-direction: column; gap: 20rpx; padding: 32rpx; border: 2rpx solid #e8ecf3; border-radius: 32rpx; background: #fff; box-shadow: 0 8rpx 24rpx rgba(16,24,40,.05); }
.product-name { color: #635bff; font-size: 32rpx; font-weight: 800; }.agent .product-name, .agent .accent { color: #ff7a1a; }
.order-card view { display: flex; align-items: flex-start; justify-content: space-between; gap: 30rpx; color: #667085; font-size: 26rpx; line-height: 38rpx; }.order-card strong { max-width: 65%; color: #111827; text-align: right; }
.amount-card .accent { color: #635bff; }.payment-card strong, .selected { color: #16a765; }.selected { font-size: 26rpx; }
.agreement { color: #667085; font-size: 22rpx; line-height: 34rpx; }.agreement checkbox { transform: scale(.75); transform-origin: left center; }
.order-message { padding: 20rpx 24rpx; border-radius: 20rpx; color: #667085; background: #fff; font-size: 24rpx; line-height: 36rpx; }.order-message.success { color: #087443; background: #effcf5; }.order-message.danger { color: #b42318; background: #fef3f2; }
.pay-button, .status-button { display: flex; height: 104rpx; margin: 0; align-items: center; justify-content: center; border: 0; border-radius: 28rpx; font-size: 30rpx; font-weight: 700; line-height: 104rpx; }.pay-button { color: #fff; background: #635bff; }.agent .pay-button { background: #ff7a1a; }.status-button { color: #635bff; background: #fff; }.pay-button::after, .status-button::after { display: none; }.pay-button[disabled] { opacity: .5; }
</style>
