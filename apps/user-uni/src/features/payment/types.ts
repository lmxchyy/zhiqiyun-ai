export interface VirtualPaymentProduct {
  id: string
  productCode: string
  name: string
  productType: 'TOKEN_ONLY' | 'TOKEN_UPGRADE' | 'IMAGE_QUOTA_PACK' | 'MEMBER_PACKAGE' | 'MEMBERSHIP' | 'IDENTITY' | string
  planType?: string
  amountCent: number
  memberLevel?: string
  agentLevel?: string
  memberDays?: number
  creditUnits?: number
  imageQuota?: number
  customQuantity?: boolean
  minQuantity?: number
  maxQuantity?: number
  mode: string
  env: number
  active: boolean
  validityText: string
  description: string
}

export interface VirtualPaymentProductsResponse {
  items: VirtualPaymentProduct[]
  enabled: boolean
  environment: 'production' | 'sandbox' | string
}

export interface VirtualPaymentCoupon {
  id: string
  code: string
  name: string
  benefitType: 'BONUS_CREDITS' | 'BONUS_IMAGE_QUOTA' | 'EXTEND_MEMBERSHIP_DAYS' | string
  benefitValue: number
}

export interface VirtualPaymentCouponsResponse {
  items: VirtualPaymentCoupon[]
  discountsPaymentAmount: false
}

export interface VirtualPaymentOrderParams {
  orderNo: string
  amountCent: number
  signData: string
  paySig: string
  signature: string
  mode: string
}

export interface VirtualPaymentOrderStatus {
  orderNo: string
  orderStatus: string
  paymentStatus: string
  entitlementStatus: string
  entitlementError?: string
  completed: boolean
  item?: Record<string, unknown>
  balances?: Record<string, unknown>
}

export interface RequestVirtualPaymentFailure {
  errMsg?: string
  errCode?: number
}

export type PaymentCapability = 'unavailable' | 'preparing' | 'available'

export interface PaymentCapabilityResponse {
  platform: string
  paymentCapability: PaymentCapability
  paymentStatus: string
  paymentChannel?: string
  message: string
  enabled: boolean
}

export interface UnifiedPaymentOrderParams {
  orderNo: string
  paymentNo: string
  productName: string
  amount: number
  currency: string
  platform: string
  channel: string
  orderStatus: string
  paymentStatus: string
  fulfillmentStatus: string
  paymentParams: Record<string, unknown>
}

export interface UnifiedPaymentOrderStatus {
  orderNo: string
  productName: string
  amount: number
  currency: string
  platform: string
  channel: string
  orderStatus: string
  paymentStatus: string
  fulfillmentStatus: string
  createdAt?: string
  paidAt?: string
  fulfilledAt?: string
}
