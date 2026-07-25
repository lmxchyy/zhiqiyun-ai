<template>
  <view class="mpb-page" :style="miniProgramNavigationStyle">
    <view class="mpb-safe" />
    <view class="mpb-header"><button class="mpb-back" aria-label="返回" @click="backOrHome(customerId ? `/pages/agent/AgentCustomerDetailPage?id=${encodeURIComponent(customerId)}` : '/pages/agent/AgentOverviewPage')">‹</button><image class="mpb-logo" :src="loginLogo" mode="aspectFit" /><view class="mpb-header-copy"><text class="mpb-title">客户订单</text><text class="mpb-subtitle">查看客户成交与退款状态</text></view><text class="mpb-role agent">代理商</text></view>
    <view class="mpb-stack">
      <view class="mpb-hero"><view class="mpb-hero-top"><view><text class="mpb-hero-label">客户成交额</text><text class="mpb-hero-value">{{ formatCurrency(totalAmountCents) }}</text></view><text class="mpb-hero-badge">{{ customerId ? '单个客户' : '全部客户' }}</text></view><text class="mpb-hero-copy">代理可见客户产生的业务订单，可回溯对应客户和服务端状态。</text><view class="mpb-hero-metrics"><view class="mpb-hero-metric"><text class="mpb-hero-metric-value">{{ completedCount }}</text><text class="mpb-hero-metric-label">已完成</text></view><view class="mpb-hero-metric"><text class="mpb-hero-metric-value">{{ pendingCount }}</text><text class="mpb-hero-metric-label">待处理</text></view><view class="mpb-hero-metric"><text class="mpb-hero-metric-value">{{ orders.length }}</text><text class="mpb-hero-metric-label">全部订单</text></view></view></view>
      <view class="mpb-tabs"><button v-for="tab in tabs" :key="tab.id" :class="{ active: filter === tab.id }" @click="filter = tab.id">{{ tab.label }}</button></view>
      <view v-if="loading" class="mpb-card mpb-empty"><text class="mpb-empty-title">正在加载订单...</text></view>
      <view v-else-if="filtered.length" class="mpb-card mpb-list"><view class="mpb-section-head"><text class="mpb-card-title">订单列表</text><text class="mpb-card-copy">{{ filtered.length }} 笔</text></view><button v-for="item in filtered" :key="orderId(item)" class="mpb-row-button" @click="openOrder(item)"><text :class="['mpb-row-icon', statusTone(orderStatus(item)) === 'success' ? 'green' : 'orange']">单</text><view class="mpb-row-main"><text class="mpb-row-title">{{ rowString(item, 'customer', 'customerName', 'email') || orderTitle(item) }}</text><text class="mpb-row-meta">{{ orderTitle(item) }} · {{ formatDate(rowString(item, 'createdAt')) }}</text></view><view class="mpb-row-side"><text class="mpb-amount">{{ formatCurrency(orderAmount(item)) }}</text><text :class="['mpb-status', statusTone(orderStatus(item))]">{{ statusText(orderStatus(item)) }}</text></view></button></view>
      <view v-else class="mpb-card mpb-empty"><text class="mpb-empty-title">暂无客户订单</text></view>
      <button v-if="customerId" class="mpb-button" @click="backOrHome(`/pages/agent/AgentCustomerDetailPage?id=${encodeURIComponent(customerId)}`)">返回客户详情</button>
    </view>
  </view>
</template>

<script setup lang="ts">
import { computed, ref } from "vue";
import { onLoad, onShow } from "@dcloudio/uni-app";
import { api } from "../../api/client";
import { backOrHome, formatCurrency, formatDate, orderAmount, orderId, orderStatus, orderTitle, rowItems, rowString, statusText, statusTone, type AnyRecord } from "../../utils/miniProgramBusiness";
import loginLogo from "../../assets/zhiqiyun-logo-transparent.png";
const tabs = [{ id: "all", label: "全部" }, { id: "pending", label: "待处理" }, { id: "done", label: "已完成" }]; const filter = ref("all"); const loading = ref(false); const orders = ref<AnyRecord[]>([]); const customerId = ref("");
const customerOrders = computed(() => orders.value.filter(item => !customerId.value || rowString(item, "userId", "customerId", "buyerUserId") === customerId.value));
const filtered = computed(() => customerOrders.value.filter(item => filter.value === "all" || (filter.value === "pending" ? ["PENDING", "PROCESSING"].includes(orderStatus(item)) : ["PAID", "SUCCESS", "SUCCEEDED", "COMPLETED"].includes(orderStatus(item)))));
const completedCount = computed(() => customerOrders.value.filter(item => ["PAID", "SUCCESS", "SUCCEEDED", "COMPLETED"].includes(orderStatus(item))).length);
const pendingCount = computed(() => customerOrders.value.filter(item => ["PENDING", "PROCESSING"].includes(orderStatus(item))).length);
const totalAmountCents = computed(() => customerOrders.value.reduce((sum, item) => sum + orderAmount(item), 0));
async function load() { loading.value = true; try { orders.value = rowItems(await api("/api/v1/channel/orders")); } catch (error) { uni.showToast({ title: error instanceof Error ? error.message : "订单加载失败", icon: "none" }); } finally { loading.value = false; } }
function openOrder(item: AnyRecord) { const id = orderId(item); if (id) uni.navigateTo({ url: `/pages/agent/AgentOrderDetailPage?id=${encodeURIComponent(id)}` }); }
onLoad(options => { customerId.value = String(options?.customerId || ""); });
onShow(load);
</script>

<style>@import "../../styles/mini-program-business.css";</style>
