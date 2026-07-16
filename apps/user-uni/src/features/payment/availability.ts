export const wechatMiniProgramPaymentAvailable = (() => {
  let available = false
  // #ifdef MP-WEIXIN
  available = true
  // #endif
  return available
})()

const iosApp = (() => {
  if (wechatMiniProgramPaymentAvailable) return false
  try {
    return String(uni.getSystemInfoSync().platform || '').toLowerCase() === 'ios'
  } catch {
    return false
  }
})()

export const paymentChannelLabel = (() => {
  if (wechatMiniProgramPaymentAvailable) return '微信支付'
  if (iosApp) return 'Apple 内购待接入'
  return 'App 支付未开放'
})()

export const paymentChannelDescription = (() => {
  if (wechatMiniProgramPaymentAvailable) return '充值到账后不可提现，可用于全部 AI 创作服务；代理套餐将开通推广与分润权益。'
  if (iosApp) return '当前 iOS 版支持登录、创作、作品和账户功能；数字权益购买将在 Apple IAP 接入后开放。'
  return '当前 App 可浏览服务端套餐，但不会创建微信小程序支付订单。'
})()

const appPaymentUnavailable = (() => {
  if (iosApp) return {
    title: 'iOS 内购暂未开放',
    content: '当前 iOS 版已开放登录、创作、作品和账户功能。数字权益购买需要接入 Apple IAP、服务端验签和幂等发放，完成前不会创建支付订单。',
  }
  return {
    title: 'App 支付暂未开放',
    content: '当前 App 已开放登录、创作、作品和账户功能。为避免错误创建小程序支付订单，App 支付通道完成服务端接入前暂不提供购买。',
  }
})()

export function ensureWechatMiniProgramPaymentAvailable(): boolean {
  if (wechatMiniProgramPaymentAvailable) return true
  uni.showModal({
    title: appPaymentUnavailable.title,
    content: appPaymentUnavailable.content,
    showCancel: false,
    confirmText: '我知道了',
  })
  return false
}
