<template>
  <view class="mpb-page" :style="miniProgramNavigationStyle">
    <view class="mpb-safe" />
    <view class="mpb-header"><image class="mpb-logo" :src="loginLogo" mode="aspectFit" /><view class="mpb-header-copy"><text class="mpb-title">支付结果</text><text class="mpb-subtitle">支付状态与点数到账反馈</text></view><text class="mpb-role">普通用户</text></view>
    <view class="mpb-stack">
      <view class="mpb-hero"><view class="mpb-hero-top"><view><text class="mpb-hero-label">{{ successful ? '支付成功' : '订单状态' }}</text><text class="mpb-hero-value">{{ successful && points ? `${formatNumber(points)} 点已到账` : statusText(status) }}</text></view><text :class="['mpb-hero-badge', successful ? 'success' : 'purple']">{{ successful ? '到账成功' : '等待确认' }}</text></view><text class="mpb-hero-copy">{{ message || statusText(status) }}</text><view class="mpb-hero-metrics"><view class="mpb-hero-metric"><text class="mpb-hero-metric-value">{{ formatCurrency(amountCents) }}</text><text class="mpb-hero-metric-label">订单金额</text></view><view class="mpb-hero-metric"><text class="mpb-hero-metric-value">{{ formatNumber(points) }}</text><text class="mpb-hero-metric-label">预计点数</text></view><view class="mpb-hero-metric"><text class="mpb-hero-metric-value">{{ statusText(status) }}</text><text class="mpb-hero-metric-label">服务端状态</text></view></view></view>
      <view class="mpb-card mpb-list"><view class="mpb-section-head"><text class="mpb-card-title">结果详情</text><text class="mpb-card-copy">订单已记录</text></view><view class="mpb-row"><text class="mpb-row-icon">号</text><view class="mpb-row-main"><text class="mpb-row-title">订单编号</text><text class="mpb-row-meta">{{ id || '-' }}</text></view></view><view class="mpb-row"><text class="mpb-row-icon green">支</text><view class="mpb-row-main"><text class="mpb-row-title">支付金额</text><text class="mpb-row-meta">{{ planName || '知启云订单' }}</text></view><text class="mpb-amount">{{ formatCurrency(amountCents) }}</text></view><view class="mpb-row"><text class="mpb-row-icon green">点</text><view class="mpb-row-main"><text class="mpb-row-title">到账点数</text><text class="mpb-row-meta">以钱包最终余额为准</text></view><text class="mpb-status success">{{ formatNumber(points) }} 点</text></view></view>
      <view class="mpb-note">{{ successful ? "相关权益已经进入账户，请返回钱包或我的页面查看。" : "当前状态不代表支付成功，请在订单列表中关注后续状态。" }}</view>
      <view class="mpb-inline-actions"><button class="mpb-button secondary" @click="goHome">返回首页</button><button :class="['mpb-button', successful ? 'green' : '']" @click="goOrders">查看订单</button></view>
    </view>
  </view>
</template>

<script setup lang="ts">
import { computed, ref } from "vue";
import { onLoad } from "@dcloudio/uni-app";
import { formatCurrency, formatNumber, statusText } from "../../utils/miniProgramBusiness";
import loginLogo from "../../assets/zhiqiyun-logo-transparent.png";
const id = ref(""); const status = ref("PENDING"); const message = ref(""); const amountCents = ref(0); const points = ref(0); const planName = ref("");
const successful = computed(() => ["PAID", "SUCCESS", "SUCCEEDED", "COMPLETED"].includes(status.value.toUpperCase()));
function goHome() { uni.reLaunch({ url: "/pages/user/UserHomePage" }); }
function goOrders() { uni.redirectTo({ url: "/pages/user/UserOrdersPage" }); }
onLoad(options => { id.value = String(options?.id || ""); status.value = String(options?.status || "PENDING"); message.value = decodeURIComponent(String(options?.message || "")); amountCents.value = Number(options?.amountCents || 0); points.value = Number(options?.points || 0); planName.value = decodeURIComponent(String(options?.planName || "")); });
</script>

<style>@import "../../styles/mini-program-business.css";</style>
