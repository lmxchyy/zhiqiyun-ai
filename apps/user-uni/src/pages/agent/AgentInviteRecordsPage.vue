<template>
  <view class="mpb-page">
    <view class="mpb-safe" />
    <view class="mpb-header"><button class="mpb-back" aria-label="返回" @click="backOrHome('/pages/agent/AgentPromotionPage')">‹</button><image class="mpb-logo" :src="loginLogo" mode="aspectFit" /><view class="mpb-header-copy"><text class="mpb-title">邀请记录</text><text class="mpb-subtitle">推广访问、注册与成交</text></view><text class="mpb-role agent">代理商</text></view>
    <view class="mpb-stack">
      <view class="mpb-hero"><view class="mpb-hero-top"><view><text class="mpb-hero-label">推广转化率</text><text class="mpb-hero-value">{{ conversionRate }}%</text></view><text class="mpb-hero-badge">邀请码 {{ inviteCode || '-' }}</text></view><text class="mpb-hero-copy">专属推广链接的注册、成交和升级数据均来自服务端邀请记录。</text><view class="mpb-hero-metrics"><view class="mpb-hero-metric"><text class="mpb-hero-metric-value">{{ stat('registered') }}</text><text class="mpb-hero-metric-label">注册</text></view><view class="mpb-hero-metric"><text class="mpb-hero-metric-value">{{ stat('paid') }}</text><text class="mpb-hero-metric-label">成交</text></view><view class="mpb-hero-metric"><text class="mpb-hero-metric-value">{{ stat('upgraded') }}</text><text class="mpb-hero-metric-label">升级</text></view></view></view>
      <view class="mpb-card accent"><view class="mpb-section-head"><text class="mpb-card-title">专属推广链接</text><button class="mpb-link-button" @click="copy">复制</button></view><text class="mpb-card-copy">{{ inviteLink || '推广链接由服务端生成' }}</text></view>
      <view v-if="customers.length" class="mpb-card mpb-list"><view class="mpb-section-head"><text class="mpb-card-title">最近邀请</text><text class="mpb-card-copy">{{ customers.length }} 人</text></view><button v-for="item in customers" :key="rowString(item, 'id')" class="mpb-row-button" @click="open(item)"><text class="mpb-row-icon green">客</text><view class="mpb-row-main"><text class="mpb-row-title">{{ rowString(item, 'invitee', 'inviteeUserId') }}</text><text class="mpb-row-meta">{{ formatDate(rowString(item, 'createdAt')) }} · {{ rowString(item, 'rechargeStatus') === 'PAID' ? '已成交' : '已注册' }}</text></view><text class="mpb-status success">{{ rowString(item, 'upgradeStatus') === 'UPGRADED' ? '已升级' : '已绑定' }}</text></button></view>
      <view v-else class="mpb-card mpb-empty"><text class="mpb-empty-title">暂无邀请记录</text><text class="mpb-empty-copy">分享小程序码或推广链接后，新客户会显示在这里。</text></view>
      <button class="mpb-button orange" open-type="share">分享专属推广页</button>
    </view>
  </view>
</template>

<script setup lang="ts">
import { computed, ref } from "vue";
import { onShow } from "@dcloudio/uni-app";
import { api, businessSdk } from "../../api/client";
import { asRecord, backOrHome, formatDate, rowItems, rowNumber, rowString, type AnyRecord } from "../../utils/miniProgramBusiness";
import loginLogo from "../../assets/zhiqiyun-logo-transparent.png";
const customers = ref<AnyRecord[]>([]); const inviteCode = ref(""); const inviteLink = ref(""); const summary = ref<AnyRecord>({});
const conversionRate = computed(() => stat("registered") > 0 ? Math.round(stat("paid") / stat("registered") * 100) : 0);
function stat(key: string) { return rowNumber(summary.value, key); }
async function load() { try { const [center, invitePayload] = await Promise.all([businessSdk.roleWorkbench.channelCenter(), api("/api/v1/channel/invite-records")]); const records = asRecord(invitePayload); customers.value = rowItems(records); inviteCode.value = rowString(center.agent, "inviteCode"); inviteLink.value = rowString(center.agent, "inviteLink") || rowString(asRecord(center).promotion, "inviteLink", "landingURL"); summary.value = asRecord(records.summary); } catch (error) { uni.showToast({ title: error instanceof Error ? error.message : "邀请记录加载失败", icon: "none" }); } }
function copy() { if (inviteLink.value) uni.setClipboardData({ data: inviteLink.value, success: () => uni.showToast({ title: "推广链接已复制", icon: "success" }) }); }
function open(item: AnyRecord) { const customerId = rowString(item, "inviteeUserId"); if (customerId) uni.navigateTo({ url: `/pages/agent/AgentCustomerDetailPage?id=${encodeURIComponent(customerId)}` }); }
onShow(load);
</script>

<style>@import "../../styles/mini-program-business.css";</style>
