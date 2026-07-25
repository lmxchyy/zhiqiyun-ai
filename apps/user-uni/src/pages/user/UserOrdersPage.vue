<template>
  <view class="mpb-page" :style="miniProgramNavigationStyle">
    <view class="mpb-safe" />
    <view class="mpb-header"><button class="mpb-back" aria-label="返回" @click="backOrHome('/pages/user/UserMinePage')">‹</button><image class="mpb-logo" :src="loginLogo" mode="aspectFit" /><view class="mpb-header-copy"><text class="mpb-title">订单列表</text><text class="mpb-subtitle">充值、代理套餐与退款</text></view><text class="mpb-role">普通用户</text></view>
    <view class="mpb-stack">
      <view class="mpb-hero"><view class="mpb-hero-top"><view><text class="mpb-hero-label">累计支付</text><text class="mpb-hero-value">{{ formatCurrency(totalPaidCents) }}</text></view><text class="mpb-hero-badge purple">全部订单</text></view><text class="mpb-hero-copy">订单详情、到账点数、支付状态与退款边界统一查询。</text><view class="mpb-hero-metrics"><view class="mpb-hero-metric"><text class="mpb-hero-metric-value">{{ completedCount }}</text><text class="mpb-hero-metric-label">已完成</text></view><view class="mpb-hero-metric"><text class="mpb-hero-metric-value">{{ refundedCount }}</text><text class="mpb-hero-metric-label">已退款</text></view><view class="mpb-hero-metric"><text class="mpb-hero-metric-value">{{ pendingCount }}</text><text class="mpb-hero-metric-label">待处理</text></view></view></view>
      <view class="mpb-tabs"><button v-for="tab in tabs" :key="tab.id" :class="{ active: filter === tab.id }" @click="filter = tab.id">{{ tab.label }}</button></view>
      <view v-if="loading" class="mpb-card mpb-empty"><text class="mpb-empty-title">正在加载订单...</text></view>
      <view v-else-if="filtered.length" class="mpb-list">
        <button v-for="item in filtered" :key="orderId(item)" class="mpb-row-button" @click="open(item)"><text :class="['mpb-row-icon', statusTone(orderStatus(item)) === 'success' ? 'green' : statusTone(orderStatus(item)) === 'danger' ? 'orange' : '']">单</text><view class="mpb-row-main"><text class="mpb-row-title">{{ orderTitle(item) }}</text><text class="mpb-row-meta">{{ orderId(item) }} · {{ formatDate(rowString(item, 'createdAt', 'paidAt')) }}</text></view><view class="mpb-row-side"><text class="mpb-amount">{{ formatCurrency(orderAmount(item)) }}</text><text :class="['mpb-status', statusTone(orderStatus(item))]">{{ statusText(orderStatus(item)) }}</text></view></button>
      </view>
      <view v-else class="mpb-card mpb-empty"><text class="mpb-empty-title">暂无{{ currentLabel }}订单</text><text class="mpb-empty-copy">选择充值方案或成为代理商后，订单会显示在这里。</text></view>
      <button class="mpb-button orange" @click="goRecharge">继续充值</button>
    </view>
  </view>
</template>

<script setup lang="ts">
import { computed, ref } from "vue";
import { onShow } from "@dcloudio/uni-app";
import { backOrHome, formatCurrency, formatDate, loadUserOrders, orderAmount, orderId, orderStatus, orderTitle, rowString, statusText, statusTone, type AnyRecord } from "../../utils/miniProgramBusiness";
import loginLogo from "../../assets/zhiqiyun-logo-transparent.png";
const tabs = [{ id: "all", label: "全部" }, { id: "pending", label: "待处理" }, { id: "paid", label: "已完成" }, { id: "closed", label: "已关闭" }];
const filter = ref("all"); const loading = ref(false); const orders = ref<AnyRecord[]>([]);
const completedCount = computed(() => orders.value.filter(item => ["PAID", "SUCCESS", "SUCCEEDED", "COMPLETED"].includes(orderStatus(item))).length);
const refundedCount = computed(() => orders.value.filter(item => orderStatus(item) === "REFUNDED").length);
const pendingCount = computed(() => orders.value.filter(item => ["PENDING", "PROCESSING"].includes(orderStatus(item))).length);
const totalPaidCents = computed(() => orders.value.reduce((sum, item) => sum + (["PAID", "SUCCESS", "SUCCEEDED", "COMPLETED"].includes(orderStatus(item)) ? orderAmount(item) : 0), 0));
const filtered = computed(() => orders.value.filter(item => { const status = orderStatus(item); if (filter.value === "pending") return ["PENDING", "PROCESSING"].includes(status); if (filter.value === "paid") return ["PAID", "SUCCESS", "SUCCEEDED", "COMPLETED"].includes(status); if (filter.value === "closed") return ["CANCELLED", "FAILED", "REFUNDED", "REJECTED"].includes(status); return true; }));
const currentLabel = computed(() => tabs.find(item => item.id === filter.value)?.label === "全部" ? "" : tabs.find(item => item.id === filter.value)?.label || "");
async function load() { loading.value = true; try { orders.value = await loadUserOrders(); } catch (error) { uni.showToast({ title: error instanceof Error ? error.message : "订单加载失败", icon: "none" }); } finally { loading.value = false; } }
function open(item: AnyRecord) { uni.navigateTo({ url: `/pages/user/UserOrderDetailPage?id=${encodeURIComponent(orderId(item))}` }); }
function goRecharge() { uni.navigateTo({ url: "/pages/user/UserRechargePlansPage" }); }
onShow(load);
</script>

<style>@import "../../styles/mini-program-business.css";</style>
