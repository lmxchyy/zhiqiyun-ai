import type { RequestVirtualPaymentFailure, VirtualPaymentOrderParams } from './types'

interface WeChatVirtualPaymentRuntime {
  canIUse?: (schema: string) => boolean
  requestVirtualPayment: (options: {
    signData: string
    paySig: string
    signature: string
    mode: string
    success: () => void
    fail: (error: RequestVirtualPaymentFailure) => void
  }) => void
}

function wechatRuntime(): WeChatVirtualPaymentRuntime | null {
  return (globalThis as unknown as { wx?: WeChatVirtualPaymentRuntime }).wx || null
}

function compareVersion(left: string, right: string) {
  const leftParts = left.split('.').map(value => Number.parseInt(value, 10) || 0)
  const rightParts = right.split('.').map(value => Number.parseInt(value, 10) || 0)
  const length = Math.max(leftParts.length, rightParts.length)
  for (let index = 0; index < length; index += 1) {
    const difference = (leftParts[index] || 0) - (rightParts[index] || 0)
    if (difference !== 0) return difference > 0 ? 1 : -1
  }
  return 0
}

function supportsVirtualPayment(runtime: WeChatVirtualPaymentRuntime) {
  const systemInfo = (globalThis as unknown as {
    wx?: { getSystemInfoSync?: () => { SDKVersion?: string } }
  }).wx?.getSystemInfoSync?.()
  const sdkVersion = String(systemInfo?.SDKVersion || '')
  return (sdkVersion !== '' && compareVersion(sdkVersion, '2.19.2') >= 0)
    || Boolean(runtime.canIUse?.('requestVirtualPayment'))
}

export function requestWeChatVirtualPayment(order: VirtualPaymentOrderParams) {
  return new Promise<void>((resolve, reject) => {
    const runtime = wechatRuntime()
    if (!runtime?.requestVirtualPayment || !supportsVirtualPayment(runtime)) {
      reject(new Error('当前微信版本不支持虚拟支付，请升级微信后重试'))
      return
    }
    runtime.requestVirtualPayment({
      signData: order.signData,
      paySig: order.paySig,
      signature: order.signature,
      mode: order.mode,
      success: resolve,
      fail: reject,
    })
  })
}

export function virtualPaymentError(error: unknown) {
  const item = (error || {}) as RequestVirtualPaymentFailure
  const code = Number(item.errCode || 0)
  if (code === -2) return { kind: 'cancelled', message: '已取消支付' }
  if (code === -15007) return { kind: 'session', message: '微信登录态已失效，请重新登录' }
  if (code === -15020 || code === -15021) return { kind: 'rate-limit', message: '操作过快，请稍后再试' }
  const message = error instanceof Error ? error.message : String(item.errMsg || '支付结果未知，请查询订单状态')
  return { kind: 'unknown', message }
}
