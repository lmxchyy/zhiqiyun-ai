import { api } from '../../api/client'
import type {
  PaymentCapabilityResponse,
  PricePlanQuote,
  UnifiedPaymentOrderParams,
  UnifiedPaymentOrderStatus,
  VirtualPaymentOrderParams,
  VirtualPaymentOrderStatus,
  VirtualPaymentCouponsResponse,
  VirtualPaymentProductsResponse,
} from './types'

export function listVirtualPaymentProducts() {
  return api<VirtualPaymentProductsResponse>('/api/v1/payment/products')
}

export function listVirtualPaymentCoupons(productCode: string) {
  return api<VirtualPaymentCouponsResponse>(`/api/v1/payment/coupons?productCode=${encodeURIComponent(productCode)}`)
}

export function createVirtualPaymentOrder(productCode: string, quantity = 1, couponCode = '', wxLoginCode = '') {
  return api<VirtualPaymentOrderParams>('/api/v1/payment/wechat-virtual/orders', {
    method: 'POST',
    headers: { 'X-Tenant-Id': '' },
    body: JSON.stringify({ productCode, quantity, couponCode, wxLoginCode }),
  })
}

export function createPublicPriceQuote(planId: string) {
  return api<PricePlanQuote>('/api/v1/payment/price-quotes', {
    method: 'POST',
    headers: { 'X-Tenant-Id': '' },
    body: JSON.stringify({ planId }),
  })
}

export function createTestPriceQuote(planId: string, pricePlanId = '') {
  return api<PricePlanQuote>('/api/v1/payment/test-price-quotes', {
    method: 'POST',
    headers: { 'X-Tenant-Id': '' },
    body: JSON.stringify({ planId, pricePlanId }),
  })
}

export function createVirtualPaymentOrderFromQuote(quoteId: string, wxLoginCode = '') {
  return api<VirtualPaymentOrderParams>('/api/v1/payment/wechat-virtual/orders', {
    method: 'POST',
    headers: { 'X-Tenant-Id': '' },
    body: JSON.stringify({ quoteId, wxLoginCode }),
  })
}

export function getVirtualPaymentOrderStatus(orderNo: string) {
  return api<VirtualPaymentOrderStatus>(`/api/v1/payment/orders/${encodeURIComponent(orderNo)}/status`, {
    headers: { 'X-Tenant-Id': '' },
  })
}

export async function syncVirtualPaymentOrder(orderNo: string) {
  const payload = await api<{ item?: Record<string, unknown> }>(`/api/v1/payment/orders/${encodeURIComponent(orderNo)}/sync`, {
    method: 'POST',
    headers: { 'X-Tenant-Id': '' },
  })
  return payload.item || {}
}

export function getPaymentCapability(platform: string) {
  return api<PaymentCapabilityResponse>('/api/v1/payment/capability?platform=' + encodeURIComponent(platform))
}

export function createUnifiedPaymentOrder(
  productCode: string,
  platform: string,
  paymentChannel: string,
  idempotencyKey: string,
) {
  return api<UnifiedPaymentOrderParams>('/api/v1/payment/orders', {
    method: 'POST',
    headers: { 'Idempotency-Key': idempotencyKey },
    body: JSON.stringify({ productCode, quantity: 1, platform, paymentChannel }),
  })
}

export function getUnifiedPaymentOrder(orderNo: string) {
  return api<UnifiedPaymentOrderStatus>('/api/v1/payment/orders/' + encodeURIComponent(orderNo))
}
