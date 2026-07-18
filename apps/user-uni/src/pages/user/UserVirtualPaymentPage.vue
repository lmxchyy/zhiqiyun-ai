<template>
  <view class="mpb-page">
    <view class="mpb-safe" />
    <view class="mpb-header">
      <button class="mpb-back" aria-label="返回" @click="backOrHome('/pages/user/UserWalletPage')">‹</button>
      <image class="mpb-logo" :src="loginLogo" mode="aspectFit" />
      <view class="mpb-header-copy"><text class="mpb-title">微信虚拟支付</text><text class="mpb-subtitle">Token充值与会员/代理权益安全到账</text></view>
      <text class="mpb-role">普通用户</text>
    </view>
    <view class="mpb-stack">
      <view class="mpb-hero">
        <view class="mpb-hero-top"><view><text class="mpb-hero-label">支付环境</text><text class="mpb-hero-value">{{ environmentLabel }}</text></view><text class="mpb-hero-badge">微信支付</text></view>
        <text class="mpb-hero-copy">价格和到账权益均由知启云服务端商品配置决定；小程序不会自行修改会员或余额。</text>
      </view>

      <view v-if="loading" class="mpb-card mpb-empty"><text class="mpb-empty-title">正在加载商品...</text></view>
      <view v-else-if="products.length" class="mpb-card mpb-list">
        <view class="mpb-section-head"><text class="mpb-card-title">选择Token虚拟商品</text><text class="mpb-card-copy">以服务端价格与权益为准</text></view>
        <button v-for="product in products" :key="product.productCode" :class="['mpb-row-button', { active: selectedCode === product.productCode }]" :disabled="paying" @click="selectedCode = product.productCode">
          <text :class="['mpb-row-icon', product.productType === 'TOKEN_UPGRADE' || product.productType === 'MEMBER_PACKAGE' ? 'orange' : 'green']">{{ productIcon(product) }}</text>
          <view class="mpb-row-main"><text class="mpb-row-title">{{ product.name }}</text><text class="mpb-row-meta">{{ product.description }}</text></view>
          <view class="mpb-row-side"><text class="mpb-amount">{{ formatCurrency(product.amountCent) }}{{ product.customQuantity ? '/份' : '' }}</text><text v-if="selectedCode === product.productCode" class="mpb-status success">已选择</text></view>
        </button>
      </view>
      <view v-else class="mpb-card mpb-empty"><text class="mpb-empty-title">暂无可支付商品</text><text class="mpb-empty-copy">请稍后刷新，或检查微信虚拟支付商品映射。</text></view>

      <view v-if="selectedProduct?.customQuantity" class="mpb-card custom-recharge-card">
        <view class="mpb-section-head"><text class="mpb-card-title">自定义充值金额</text><text class="mpb-card-copy">整数金额，1元 = {{ selectedProduct.creditUnits || 0 }} Token</text></view>
        <view class="custom-recharge-input"><input v-model.number="customQuantity" type="number" :min="selectedProduct.minQuantity || 1" :max="selectedProduct.maxQuantity || 5000" /><text>份</text></view>
        <text class="mpb-card-copy">服务端预计：支付 {{ formatCurrency(payAmountCent) }}，到账 {{ customTokenAmount }} Token</text>
      </view>

      <view v-if="coupons.length" class="mpb-card mpb-list">
        <view class="mpb-section-head"><text class="mpb-card-title">选择权益优惠券</text><text class="mpb-card-copy">优惠券不改变微信实付金额</text></view>
        <button :class="['mpb-row-button', { active: !selectedCouponCode }]" :disabled="paying" @click="selectedCouponCode = ''">
          <text class="mpb-row-icon green">券</text><view class="mpb-row-main"><text class="mpb-row-title">不使用优惠券</text><text class="mpb-row-meta">按商品原权益到账</text></view>
        </button>
        <button v-for="coupon in coupons" :key="coupon.id" :class="['mpb-row-button', { active: selectedCouponCode === coupon.code }]" :disabled="paying" @click="selectedCouponCode = coupon.code">
          <text class="mpb-row-icon orange">赠</text><view class="mpb-row-main"><text class="mpb-row-title">{{ coupon.name }}</text><text class="mpb-row-meta">{{ couponBenefitText(coupon) }}</text></view><text v-if="selectedCouponCode === coupon.code" class="mpb-status success">已选择</text>
        </button>
      </view>

      <view v-if="resultMessage" :class="['mpb-note', resultTone]">{{ resultMessage }}</view>
      <button class="mpb-button" :disabled="!selectedProduct || paying || !paymentEnabled || !customQuantityValid" @click="pay()">
        {{ paying ? actionLabel : selectedProduct ? `微信支付 ${formatCurrency(payAmountCent)}` : '请选择商品' }}
      </button>
      <button v-if="orderNo && !completed" class="mpb-button secondary" :disabled="syncing" @click="manualSync()">{{ syncing ? '正在同步...' : '查询微信支付结果' }}</button>
      <text class="mpb-footer-note">Token与创作额度仅限知启云AI平台消费，不支持提现、转账或赠送。</text>
    </view>
  </view>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { onLoad, onUnload } from '@dcloudio/uni-app'
import { businessSdk } from '../../api/client'
import { backOrHome, formatCurrency } from '../../utils/miniProgramBusiness'
import { createVirtualPaymentOrder, getVirtualPaymentOrderStatus, listVirtualPaymentCoupons, listVirtualPaymentProducts, syncVirtualPaymentOrder } from '../../features/payment/api'
import { ensureWechatMiniProgramPaymentAvailable } from '../../features/payment/availability'
import { requestWeChatVirtualPayment, virtualPaymentError } from '../../features/payment/platform'
import { requireAuth } from '../../features/auth/gate'
import type { VirtualPaymentCoupon, VirtualPaymentOrderStatus, VirtualPaymentProduct } from '../../features/payment/types'
import loginLogo from '../../assets/zhiqiyun-logo-transparent.png'

const loading = ref(false)
const paying = ref(false)
const syncing = ref(false)
const paymentEnabled = ref(false)
const environment = ref('')
const products = ref<VirtualPaymentProduct[]>([])
const coupons = ref<VirtualPaymentCoupon[]>([])
const selectedCode = ref('')
const selectedCouponCode = ref('')
const customQuantity = ref(1)
const orderNo = ref('')
const resultMessage = ref('')
const resultTone = ref('')
const actionLabel = ref('正在创建订单...')
const completed = ref(false)
let disposed = false

const selectedProduct = computed(() => products.value.find(item => item.productCode === selectedCode.value) || null)
const customQuantityValid = computed(() => {
  const product = selectedProduct.value
  if (!product?.customQuantity) return true
  const quantity = Number(customQuantity.value)
  return Number.isInteger(quantity) && quantity >= (product.minQuantity || 1) && quantity <= (product.maxQuantity || 5000)
})
const purchaseQuantity = computed(() => selectedProduct.value?.customQuantity ? Number(customQuantity.value) : 1)
const payAmountCent = computed(() => (selectedProduct.value?.amountCent || 0) * (customQuantityValid.value ? purchaseQuantity.value : 0))
const customTokenAmount = computed(() => (selectedProduct.value?.creditUnits || 0) * (customQuantityValid.value ? purchaseQuantity.value : 0))
const environmentLabel = computed(() => environment.value === 'sandbox' ? '沙箱联调' : environment.value === 'production' ? '正式环境' : '待配置')

async function load() {
  loading.value = true
  try {
    const payload = await listVirtualPaymentProducts()
    products.value = payload.items.filter(item => item.active)
    paymentEnabled.value = payload.enabled
    environment.value = payload.environment
    selectedCode.value = products.value[0]?.productCode || ''
    if (!payload.enabled) resultMessage.value = '服务端尚未启用微信虚拟支付，请先完成环境变量和微信商品配置。'
  }
  catch (error) {
    resultMessage.value = error instanceof Error ? error.message : '商品加载失败'
    resultTone.value = 'danger'
  }
  finally {
    loading.value = false
  }
}

async function pay() {
  const product = selectedProduct.value
  if (!product || paying.value) return
  paying.value = true
  completed.value = false
  resultTone.value = ''
  resultMessage.value = ''
  try {
    actionLabel.value = '正在创建订单...'
    const order = await createVirtualPaymentOrder(product.productCode, purchaseQuantity.value, selectedCouponCode.value)
    orderNo.value = order.orderNo
    actionLabel.value = '等待微信支付...'
    await requestWeChatVirtualPayment(order)
    actionLabel.value = '支付结果确认中...'
    resultMessage.value = '微信支付已返回，正在等待服务端确认支付并发放权益。'
    await waitForEntitlements(order.orderNo)
  }
  catch (error) {
    const result = virtualPaymentError(error)
    resultMessage.value = result.message
    resultTone.value = result.kind === 'cancelled' ? '' : 'danger'
    if (result.kind === 'session') {
      void requireAuth({
        action: 'recharge',
        route: '/pages/user/UserVirtualPaymentPage',
        payload: { productCode: product.productCode },
        autoResume: false,
      })
    }
  }
  finally {
    paying.value = false
    actionLabel.value = '正在创建订单...'
  }
}

async function waitForEntitlements(currentOrderNo: string) {
  for (let attempt = 0; attempt < 12 && !disposed; attempt += 1) {
    if (attempt === 3 || attempt === 7) {
      await syncVirtualPaymentOrder(currentOrderNo).catch(() => undefined)
    }
    const status = await getVirtualPaymentOrderStatus(currentOrderNo)
    if (status.completed) {
      await finishPayment(status)
      return
    }
    if (status.entitlementStatus === 'FAILED') {
      throw new Error(status.entitlementError || '支付已确认，但权益发放暂时失败，系统将自动补偿')
    }
    await delay(1500)
  }
  resultMessage.value = '支付结果仍在确认中，可稍后点击“查询微信支付结果”或在订单列表查看。'
}

async function finishPayment(status: VirtualPaymentOrderStatus) {
  completed.value = true
  resultTone.value = 'success'
  const product = selectedProduct.value
  if (product?.productType === 'IMAGE_QUOTA_PACK') {
    resultMessage.value = `支付成功，${product.imageQuota || 0}张图片生成额度已到账。`
  }
  else if (product?.productType === 'TOKEN_UPGRADE' && product.agentLevel) {
    resultMessage.value = `支付成功，${product.creditUnits || 0} Token 与代理商身份已到账。`
  }
  else if (product?.productType === 'TOKEN_UPGRADE') {
    resultMessage.value = `支付成功，${product.creditUnits || 0} Token 与${product.memberLevel || '会员'}权益已到账。`
  }
  else {
    const tokenAmount = (product?.creditUnits || 0) * (product?.customQuantity ? purchaseQuantity.value : 1)
    resultMessage.value = `支付成功，${tokenAmount} Token 已到账。`
  }
  await Promise.allSettled([
    businessSdk.roleWorkbench.memberProfile(),
    businessSdk.roleWorkbench.wallet(),
    businessSdk.roleWorkbench.pointsAccount(),
  ])
  uni.$emit('virtual-payment:entitlements-updated', status.balances || {})
  await uni.showModal({ title: '权益已到账', content: resultMessage.value, showCancel: false })
}

async function manualSync() {
  if (!orderNo.value || syncing.value) return
  syncing.value = true
  try {
    await syncVirtualPaymentOrder(orderNo.value)
    const status = await getVirtualPaymentOrderStatus(orderNo.value)
    if (status.completed) await finishPayment(status)
    else resultMessage.value = '微信尚未确认到账，系统会继续自动补偿查询。'
  }
  catch (error) {
    resultMessage.value = error instanceof Error ? error.message : '同步失败，请稍后重试'
    resultTone.value = 'danger'
  }
  finally {
    syncing.value = false
  }
}

function delay(milliseconds: number) {
  return new Promise(resolve => setTimeout(resolve, milliseconds))
}

function productIcon(product: VirtualPaymentProduct) {
  if (product.productType === 'IMAGE_QUOTA_PACK') return '图'
  if (product.productType === 'TOKEN_UPGRADE' && product.agentLevel) return '代'
  if (product.productType === 'TOKEN_UPGRADE' || product.productType === 'MEMBER_PACKAGE') return '升'
  return '币'
}

function couponBenefitText(coupon: VirtualPaymentCoupon) {
  if (coupon.benefitType === 'BONUS_CREDITS') return `额外赠送 ${coupon.benefitValue} Token`
  if (coupon.benefitType === 'BONUS_IMAGE_QUOTA') return `额外赠送 ${coupon.benefitValue} 张图片额度`
  if (coupon.benefitType === 'EXTEND_MEMBERSHIP_DAYS') return `额外延长 ${coupon.benefitValue} 天会员`
  return `额外权益 ${coupon.benefitValue}`
}

watch(selectedCode, async (productCode) => {
  selectedCouponCode.value = ''
  coupons.value = []
  if (!productCode || !paymentEnabled.value) return
  try { coupons.value = (await listVirtualPaymentCoupons(productCode)).items || [] }
  catch { coupons.value = [] }
})

onLoad(() => {
  if (ensureWechatMiniProgramPaymentAvailable()) void load()
})
onUnload(() => { disposed = true })
</script>

<style>
@import '../../styles/mini-program-business.css';
.mpb-note.success { color: #087443; border-color: #a7e6c4; background: #effcf5; }
.mpb-note.danger { color: #b42318; border-color: #fecdca; background: #fef3f2; }
.custom-recharge-card { display: flex; flex-direction: column; gap: 16rpx; }
.custom-recharge-input { display: flex; align-items: center; gap: 12rpx; padding: 18rpx 22rpx; border: 2rpx solid #d0d5dd; border-radius: 16rpx; background: #fff; }
.custom-recharge-input input { flex: 1; font-size: 34rpx; font-weight: 700; color: #101828; }
</style>
