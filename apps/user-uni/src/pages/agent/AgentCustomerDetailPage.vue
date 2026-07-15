<template>
  <view class="mpb-page">
    <view class="mpb-safe" />
    <view class="mpb-header"><button class="mpb-back" aria-label="返回" @click="backOrHome('/pages/agent/AgentCustomersPage')">‹</button><image class="mpb-logo" :src="loginLogo" mode="aspectFit" /><view class="mpb-header-copy"><text class="mpb-title">客户详情</text><text class="mpb-subtitle">客户价值与转化轨迹</text></view><text class="mpb-role agent">代理商</text></view>
    <view class="mpb-stack">
      <view v-if="loading" class="mpb-card mpb-empty"><text class="mpb-empty-title">正在加载客户...</text></view>
      <template v-else-if="customer">
        <view class="mpb-hero"><view class="mpb-hero-top"><view><text class="mpb-hero-label">客户等级</text><text class="mpb-hero-value">{{ rowString(customer, 'name', 'email') }}</text></view><text class="mpb-hero-badge success">{{ rowString(customer, 'planName', 'plan') || '基础用户' }}</text></view><text class="mpb-hero-copy">{{ rowString(customer, 'email') }} · 客户数据和订单归属来自渠道接口。</text><view class="mpb-hero-metrics"><view class="mpb-hero-metric"><text class="mpb-hero-metric-value">{{ orders.length }}</text><text class="mpb-hero-metric-label">订单</text></view><view class="mpb-hero-metric"><text class="mpb-hero-metric-value">{{ formatCurrency(totalSpentCents) }}</text><text class="mpb-hero-metric-label">累计消费</text></view><view class="mpb-hero-metric"><text class="mpb-hero-metric-value">{{ assets.length }}</text><text class="mpb-hero-metric-label">作品</text></view></view></view>
        <view class="mpb-card mpb-list"><view class="mpb-section-head"><text class="mpb-card-title">客户画像</text><text class="mpb-card-copy">实时资料</text></view><view class="mpb-row"><text class="mpb-row-icon">邮</text><view class="mpb-row-main"><text class="mpb-row-title">客户邮箱</text><text class="mpb-row-meta">{{ rowString(customer, 'email') || '-' }}</text></view></view><view class="mpb-row"><text class="mpb-row-icon green">套</text><view class="mpb-row-main"><text class="mpb-row-title">当前套餐</text><text class="mpb-row-meta">{{ rowString(customer, 'planName', 'plan') || '基础用户' }}</text></view></view><view class="mpb-row"><text class="mpb-row-icon orange">点</text><view class="mpb-row-main"><text class="mpb-row-title">可用点数</text><text class="mpb-row-meta">客户当前账户余额</text></view><text class="mpb-amount">{{ formatNumber(rowNumber(customer, 'pointsAvailable', 'available')) }}</text></view><view class="mpb-row"><text class="mpb-row-icon">创</text><view class="mpb-row-main"><text class="mpb-row-title">生成任务</text><text class="mpb-row-meta">真实任务记录</text></view><text class="mpb-status success">{{ tasks.length }} 条</text></view></view>
        <view class="mpb-card"><text class="mpb-card-title">最近订单</text><view v-if="orders.length" class="mpb-list"><button v-for="item in orders.slice(0, 5)" :key="orderId(item)" class="mpb-row-button" @click="openOrder(item)"><view class="mpb-row-main"><text class="mpb-row-title">{{ orderTitle(item) }}</text><text class="mpb-row-meta">{{ formatDate(rowString(item, 'createdAt')) }}</text></view><text class="mpb-amount">{{ formatCurrency(orderAmount(item)) }}</text></button></view><text v-else class="mpb-card-copy">暂无客户订单。</text></view>
        <button class="mpb-button orange" @click="openOrders">查看客户订单</button>
      </template>
      <view v-else class="mpb-card mpb-empty"><text class="mpb-empty-title">客户不存在或无权查看</text></view>
    </view>
  </view>
</template>

<script setup lang="ts">
import { computed, ref } from "vue";
import { onLoad } from "@dcloudio/uni-app";
import { api } from "../../api/client";
import { asRecord, backOrHome, formatCurrency, formatDate, formatNumber, listOf, orderAmount, orderId, orderTitle, rowNumber, rowString, type AnyRecord } from "../../utils/miniProgramBusiness";
import loginLogo from "../../assets/zhiqiyun-logo-transparent.png";
const id = ref(""); const loading = ref(false); const customer = ref<AnyRecord | null>(null); const orders = ref<AnyRecord[]>([]); const tasks = ref<AnyRecord[]>([]); const assets = ref<AnyRecord[]>([]);
const totalSpentCents = computed(() => orders.value.reduce((sum, item) => sum + orderAmount(item), 0));
async function load() { loading.value = true; try { const result = asRecord(await api(`/api/v1/channel/customers/${encodeURIComponent(id.value)}`)); customer.value = asRecord(result.item); orders.value = listOf(result.orders); tasks.value = listOf(result.generationTasks); assets.value = listOf(result.assets); } catch (error) { customer.value = null; uni.showToast({ title: error instanceof Error ? error.message : "客户加载失败", icon: "none" }); } finally { loading.value = false; } }
function openOrders() { uni.navigateTo({ url: `/pages/agent/AgentOrdersPage?customerId=${encodeURIComponent(id.value)}` }); }
function openOrder(item: AnyRecord) { const value = orderId(item); if (value) uni.navigateTo({ url: `/pages/agent/AgentOrderDetailPage?id=${encodeURIComponent(value)}` }); }
onLoad(options => { id.value = String(options?.id || ""); void load(); });
</script>

<style>@import "../../styles/mini-program-business.css";</style>
