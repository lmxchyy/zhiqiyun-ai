<template>
  <view class="promotion-page">
    <PromotionPageHeader title="推广记录" subtitle="访问、注册、成交与奖励状态" />
    <view class="promotion-content">
      <view class="promotion-record-summary"><view v-for="item in summaryItems" :key="item.label"><text>{{ item.value }}</text><text>{{ item.label }}</text></view></view>
      <scroll-view scroll-x class="promotion-status-tabs" :show-scrollbar="false"><view><button v-for="tab in tabs" :key="tab.id" :class="{ active: status === tab.id }" @click="selectStatus(tab.id)"><text>{{ tab.name }}</text></button></view></scroll-view>
      <PromotionStatePanel v-if="loading && !records.length" tone="loading" title="正在加载推广记录" />
      <PromotionStatePanel v-else-if="error && !records.length" tone="error" title="推广记录加载失败" :description="error" action-text="重试" @action="reload" />
      <PromotionStatePanel v-else-if="!records.length" tone="empty" title="暂无推广记录" description="分享小程序码或推广海报后，访问与转化会显示在这里" />
      <view v-else class="promotion-record-list">
        <view v-for="record in records" :key="record.id" class="promotion-record-item">
          <view class="promotion-record-avatar">{{ (record.visitorName || '访').slice(0, 1) }}</view>
          <view class="promotion-record-main"><text>{{ record.visitorName || record.maskedMobile || '微信访客' }}</text><text>{{ sourceLabel(record.source) }} · {{ formatDate(record.visitTime || record.createdAt) }}</text><text v-if="record.rewardAmountCents">奖励 ¥{{ (record.rewardAmountCents / 100).toFixed(2) }} · {{ record.rewardStatus }}</text></view>
          <text :class="['promotion-status-badge', record.status]">{{ statusLabel(record.status) }}</text>
        </view>
        <button v-if="hasMore" class="promotion-secondary-button promotion-load-more" :disabled="loading" @click="loadMore"><text>{{ loading ? '加载中…' : '加载更多' }}</text></button>
      </view>
    </view>
  </view>
</template>
<script setup lang="ts">
import { computed, ref } from "vue";
import { onPullDownRefresh, onReachBottom, onShow } from "@dcloudio/uni-app";
import PromotionPageHeader from "../../components/promotion/PromotionPageHeader.vue";
import PromotionStatePanel from "../../components/promotion/PromotionStatePanel.vue";
import { promotionAPI } from "../../features/promotion/api";
import { trackPromotion } from "../../features/promotion/analytics";
import type { PromotionRecord, PromotionSummary } from "../../features/promotion/types";
const records = ref<PromotionRecord[]>([]); const summary = ref<PromotionSummary | null>(null); const status = ref("all"); const page = ref(1); const hasMore = ref(false); const loading = ref(false); const error = ref("");
const tabs = [{ id: "all", name: "全部" }, { id: "visited", name: "已访问" }, { id: "registered", name: "已注册" }, { id: "paid", name: "已成交" }];
const summaryItems = computed(() => [{ label: "访问", value: summary.value?.visitCount || 0 }, { label: "注册", value: summary.value?.registerCount || 0 }, { label: "成交", value: summary.value?.paidCount || 0 }, { label: "累计奖励", value: `¥${((summary.value?.rewardAmountCents || 0) / 100).toFixed(2)}` }]);
onShow(() => { trackPromotion("promotion_records_view"); void reload(); });
onPullDownRefresh(async () => { await reload(); uni.stopPullDownRefresh(); });
onReachBottom(() => { if (hasMore.value && !loading.value) void loadMore(); });
async function load(reset = false) { if (loading.value) return; loading.value = true; error.value = ""; if (reset) { page.value = 1; records.value = []; } try { const [payload, analytics] = await Promise.all([promotionAPI.records({ page: page.value, pageSize: 20, status: status.value }), promotionAPI.analytics(7)]); records.value = reset ? payload.items : [...records.value, ...payload.items]; hasMore.value = payload.hasMore; summary.value = analytics.summary; } catch (reason) { error.value = reason instanceof Error ? reason.message : "请稍后重试"; } finally { loading.value = false; } }
async function reload() { await load(true); }
async function loadMore() { page.value += 1; await load(false); }
function selectStatus(value: string) { status.value = value; void reload(); }
function statusLabel(value: PromotionRecord["status"]) { return { visited: "已访问", registered: "已注册", paid: "已成交", invalid: "已失效" }[value]; }
function sourceLabel(value: string) { return { wechat_friend: "微信好友", wechat_group: "微信群", moments: "朋友圈", poster: "推广海报", copy_link: "复制链接", invite_code: "邀请码" }[value] || "微信推广"; }
function formatDate(value: string) { if (!value) return "-"; const date = new Date(value); return Number.isNaN(date.getTime()) ? value : `${date.getMonth() + 1}月${date.getDate()}日 ${String(date.getHours()).padStart(2, '0')}:${String(date.getMinutes()).padStart(2, '0')}`; }
</script>
<style>@import "../../styles/promotion-center.css";</style>
