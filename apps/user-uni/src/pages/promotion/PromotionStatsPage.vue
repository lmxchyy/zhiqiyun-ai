<template>
  <view class="promotion-page">
    <PromotionPageHeader title="推广数据" subtitle="趋势、转化率与来源渠道" />
    <view class="promotion-content">
      <view class="promotion-period-tabs"><button v-for="item in periods" :key="item.days" :class="{ active: days === item.days }" @click="selectDays(item.days)"><text>{{ item.name }}</text></button></view>
      <PromotionStatePanel v-if="loading && !analytics" tone="loading" title="正在计算推广数据" />
      <PromotionStatePanel v-else-if="error && !analytics" tone="error" title="推广数据加载失败" :description="error" action-text="重试" @action="load" />
      <template v-else-if="analytics">
        <view class="promotion-stats-grid"><view v-for="item in stats" :key="item.label"><text>{{ item.value }}</text><text>{{ item.label }}</text><text v-if="item.note">{{ item.note }}</text></view></view>
        <view class="promotion-chart-card"><view class="promotion-card-heading"><view><text class="promotion-section-title">转化趋势</text><text class="promotion-section-copy">访问 / 注册 / 成交</text></view><view class="promotion-chart-legend"><text>访问</text><text>注册</text><text>成交</text></view></view><PromotionTrendChart :items="analytics.trend" /></view>
        <view class="promotion-chart-card"><text class="promotion-section-title">来源渠道</text><view v-if="analytics.channels.length" class="promotion-channel-list"><view v-for="channel in analytics.channels" :key="channel.source"><view><text>{{ channel.label }}</text><text>{{ channel.count }} 次</text></view><view class="promotion-channel-track"><view :style="{ width: `${channelWidth(channel.count)}%` }" /></view></view></view><PromotionStatePanel v-else tone="empty" title="暂无渠道数据" description="完成分享后即可看到渠道分布" /></view>
        <view class="promotion-insight-card"><text>数据说明</text><text>访问按登录用户与自然日去重；注册只记录首次有效归因；成交和奖励均由服务端订单、分润状态计算，页面不会直接发放奖励。</text></view>
      </template>
    </view>
  </view>
</template>
<script setup lang="ts">
import { computed, ref } from "vue";
import { onPullDownRefresh, onShow } from "@dcloudio/uni-app";
import PromotionPageHeader from "../../components/promotion/PromotionPageHeader.vue";
import PromotionStatePanel from "../../components/promotion/PromotionStatePanel.vue";
import PromotionTrendChart from "../../components/promotion/PromotionTrendChart.vue";
import { promotionAPI } from "../../features/promotion/api";
import { trackPromotion } from "../../features/promotion/analytics";
import type { PromotionAnalytics } from "../../features/promotion/types";
const analytics = ref<PromotionAnalytics | null>(null); const days = ref(7); const loading = ref(false); const error = ref(""); const periods = [{ days: 7, name: "近7天" }, { days: 30, name: "近30天" }, { days: 90, name: "近90天" }];
const stats = computed(() => { const item = analytics.value?.summary; return [{ label: "访问", value: item?.visitCount || 0 }, { label: "注册", value: item?.registerCount || 0, note: `转化 ${Number(item?.registerRate || 0).toFixed(1)}%` }, { label: "成交", value: item?.paidCount || 0, note: `转化 ${Number(item?.paidRate || 0).toFixed(1)}%` }, { label: "累计奖励", value: `¥${((item?.rewardAmountCents || 0) / 100).toFixed(2)}` }]; });
onShow(() => { trackPromotion("promotion_stats_view"); void load(); }); onPullDownRefresh(async () => { await load(); uni.stopPullDownRefresh(); });
async function load() { if (loading.value) return; loading.value = true; error.value = ""; try { analytics.value = await promotionAPI.analytics(days.value); } catch (reason) { error.value = reason instanceof Error ? reason.message : "请稍后重试"; } finally { loading.value = false; } }
function selectDays(value: number) { days.value = value; void load(); }
function channelWidth(value: number) { const max = Math.max(1, ...(analytics.value?.channels || []).map(item => item.count)); return Math.max(8, Math.round(value / max * 100)); }
</script>
<style>@import "../../styles/promotion-center.css";</style>
