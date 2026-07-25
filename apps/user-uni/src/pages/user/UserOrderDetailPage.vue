<template>
  <view class="mpb-page" :style="miniProgramNavigationStyle">
    <view class="mpb-safe" />
    <view class="mpb-header"><button class="mpb-back" aria-label="返回" @click="backOrHome('/pages/user/UserOrdersPage')">‹</button><image class="mpb-logo" :src="loginLogo" mode="aspectFit" /><view class="mpb-header-copy"><text class="mpb-title">订单详情</text><text class="mpb-subtitle">查看金额、点数与支付凭证</text></view><text class="mpb-role">普通用户</text></view>
    <view class="mpb-stack">
      <view v-if="loading" class="mpb-card mpb-empty"><text class="mpb-empty-title">正在加载订单...</text></view>
      <template v-else-if="order">
        <view class="mpb-hero light"><view class="mpb-hero-top"><view><text class="mpb-hero-label">订单状态</text><text class="mpb-hero-value">{{ orderTitle(order) }}</text></view><text :class="['mpb-hero-badge', isCompleted ? 'success' : 'purple']">{{ statusText(orderStatus(order)) }}</text></view><text class="mpb-hero-copy">{{ statusDescription }}</text><view class="mpb-hero-metrics"><view class="mpb-hero-metric"><text class="mpb-hero-metric-value">{{ formatCurrency(orderAmount(order)) }}</text><text class="mpb-hero-metric-label">支付金额</text></view><view class="mpb-hero-metric"><text class="mpb-hero-metric-value">{{ orderPoints ? `${formatNumber(orderPoints)} 点` : '-' }}</text><text class="mpb-hero-metric-label">到账点数</text></view><view class="mpb-hero-metric"><text class="mpb-hero-metric-value">{{ rowString(order, 'paymentMethod') || '微信' }}</text><text class="mpb-hero-metric-label">支付渠道</text></view></view></view>
        <view class="mpb-card mpb-list">
          <view class="mpb-section-head"><text class="mpb-card-title">订单信息</text><text class="mpb-card-copy">服务端记录</text></view>
          <view class="mpb-row"><text class="mpb-row-icon">号</text><view class="mpb-row-main"><text class="mpb-row-title">订单编号</text><text class="mpb-row-meta">{{ orderId(order) }}</text></view></view>
          <view class="mpb-row"><text class="mpb-row-icon green">时</text><view class="mpb-row-main"><text class="mpb-row-title">创建时间</text><text class="mpb-row-meta">{{ formatDate(rowString(order, 'createdAt')) }}</text></view><text class="mpb-status success">已记录</text></view>
          <view class="mpb-row"><text class="mpb-row-icon green">支</text><view class="mpb-row-main"><text class="mpb-row-title">支付方式</text><text class="mpb-row-meta">{{ rowString(order, 'paymentMethod') || '微信小程序' }}</text></view><text class="mpb-status success">{{ statusText(orderStatus(order)) }}</text></view>
          <view class="mpb-row"><text class="mpb-row-icon">凭</text><view class="mpb-row-main"><text class="mpb-row-title">电子凭证</text><text class="mpb-row-meta">支付凭证与到账记录</text></view><text class="mpb-status">以后台为准</text></view>
        </view>
        <view class="mpb-inline-actions"><button class="mpb-button secondary" @click="goOrders">返回订单</button><button class="mpb-button" @click="rebuy">再次购买</button></view>
        <button v-if="canRequestRefund" class="mpb-button orange" @click="openRefund">申请退款</button>
      </template>
      <view v-else class="mpb-card mpb-empty"><text class="mpb-empty-title">未找到订单</text><text class="mpb-empty-copy">订单可能已被移除，或当前账户无权查看。</text></view>
    </view>
  </view>
</template>

<script setup lang="ts">
import { computed, ref } from "vue";
import { onLoad } from "@dcloudio/uni-app";
import { api } from "../../api/client";
import { asRecord, backOrHome, formatCurrency, formatDate, formatNumber, orderAmount, orderId, orderStatus, orderTitle, rowNumber, rowString, statusText, type AnyRecord } from "../../utils/miniProgramBusiness";
import loginLogo from "../../assets/zhiqiyun-logo-transparent.png";
const id = ref(""); const order = ref<AnyRecord | null>(null); const loading = ref(false);
const statusDescription = computed(() => { const value = orderStatus(order.value); if (value === "PENDING") return "订单已创建，正在等待支付或运营确认。"; if (value === "REFUND_REQUESTED") return "退款申请已提交，正在等待运营人员审核。"; if (["PAID", "SUCCESS", "COMPLETED"].includes(value)) return "订单已经完成，相关点数或身份权益以账户数据为准。"; return "当前订单已关闭，如有疑问请联系平台客服。"; });
const canRequestRefund = computed(() => ["PAID", "SUCCESS", "SUCCEEDED", "COMPLETED"].includes(orderStatus(order.value)));
const isCompleted = computed(() => ["PAID", "SUCCESS", "SUCCEEDED", "COMPLETED"].includes(orderStatus(order.value)));
const orderPoints = computed(() => rowNumber(order.value, "grantPoints", "points", "tokenAmount"));
async function load() { loading.value = true; try { const payload = asRecord(await api(`/api/v1/member/orders/${encodeURIComponent(id.value)}`)); const item = asRecord(payload.item); order.value = orderId(item) ? item : null; } catch (error) { uni.showToast({ title: error instanceof Error ? error.message : "订单加载失败", icon: "none" }); } finally { loading.value = false; } }
function rebuy() { uni.navigateTo({ url: "/pages/user/UserRechargePlansPage" }); }
function goOrders() { uni.navigateTo({ url: "/pages/user/UserOrdersPage" }); }
function openRefund() { uni.navigateTo({ url: `/pages/user/UserRefundRequestPage?id=${encodeURIComponent(id.value)}` }); }
onLoad(options => { id.value = String(options?.id || ""); void load(); });
</script>

<style>@import "../../styles/mini-program-business.css";</style>
