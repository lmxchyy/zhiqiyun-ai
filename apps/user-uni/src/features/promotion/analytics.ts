export type PromotionEventName =
  | "promotion_page_view"
  | "promotion_template_select"
  | "promotion_code_ready"
  | "promotion_poster_generate"
  | "promotion_poster_save"
  | "promotion_share"
  | "promotion_copy"
  | "promotion_records_view"
  | "promotion_stats_view"
  | "promotion_referral_visit"
  | "promotion_referral_bind";

const queueKey = "zhiqiyun.promotion.analytics.queue";

export function trackPromotion(event: PromotionEventName, properties: Record<string, unknown> = {}) {
  try {
    const queue = (uni.getStorageSync(queueKey) || []) as unknown[];
    queue.push({ event, properties, occurredAt: new Date().toISOString() });
    uni.setStorageSync(queueKey, queue.slice(-100));
  } catch { /* analytics must never affect business actions */ }
}
