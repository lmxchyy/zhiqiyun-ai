<template>
  <view class="mpb-page" :style="miniProgramNavigationStyle">
    <view class="mpb-safe" />
    <view class="mpb-header"><button class="mpb-back" aria-label="返回" @click="backOrHome('/pages/user/UserRechargeHistoryPage')">‹</button><image class="mpb-logo" :src="loginLogo" mode="aspectFit" /><view class="mpb-header-copy"><text class="mpb-title">电子凭证</text><text class="mpb-subtitle">订单支付与到账凭证</text></view><text class="mpb-role">普通用户</text></view>
    <view class="mpb-stack">
      <view class="mpb-hero light"><view class="mpb-hero-top"><view><text class="mpb-hero-label">电子凭证</text><text class="mpb-hero-value">{{ availableCount }} 份可用</text></view><text class="mpb-hero-badge purple">服务端记录</text></view><text class="mpb-hero-copy">已支付订单生成电子凭证；待支付订单会保留状态，但不会生成凭证编号。</text></view>
      <view v-if="loading" class="mpb-card mpb-empty"><text class="mpb-empty-title">正在加载电子凭证...</text></view>
      <view v-else-if="items.length" class="mpb-card mpb-list">
        <view class="mpb-section-head"><text class="mpb-card-title">凭证列表</text><text class="mpb-card-copy">{{ items.length }} 条</text></view>
        <button v-for="item in items" :key="rowString(item, 'id')" class="mpb-row-button" @click="openOrder(item)"><text :class="['mpb-row-icon', rowString(item, 'status') === 'AVAILABLE' ? 'green' : 'orange']">凭</text><view class="mpb-row-main"><text class="mpb-row-title">{{ rowString(item, 'planName') || '平台订单' }}</text><text class="mpb-row-meta">{{ rowString(item, 'invoiceNo') || '支付后生成编号' }} · {{ formatDate(rowString(item, 'paidAt', 'createdAt')) }}</text></view><view class="mpb-row-side"><text class="mpb-amount">{{ formatCurrency(rowNumber(item, 'amountCents')) }}</text><text :class="['mpb-status', rowString(item, 'status') === 'AVAILABLE' ? 'success' : 'warning']">{{ rowString(item, 'status') === 'AVAILABLE' ? '可查看' : '待支付' }}</text></view></button>
      </view>
      <view v-else class="mpb-card mpb-empty"><text class="mpb-empty-title">暂无电子凭证</text><text class="mpb-empty-copy">完成订单支付后，凭证会显示在这里。</text></view>
    </view>
  </view>
</template>

<script setup lang="ts">
import { computed, ref } from "vue";
import { onLoad } from "@dcloudio/uni-app";
import { api } from "../../api/client";
import { backOrHome, formatCurrency, formatDate, rowItems, rowNumber, rowString, type AnyRecord } from "../../utils/miniProgramBusiness";
import loginLogo from "../../assets/zhiqiyun-logo-transparent.png";

const loading = ref(false);
const items = ref<AnyRecord[]>([]);
const availableCount = computed(() => items.value.filter(item => rowString(item, "status") === "AVAILABLE").length);
async function load() {
  loading.value = true;
  try {
    items.value = rowItems(await api("/api/v1/member/invoices"));
  } catch (error) {
    uni.showToast({ title: error instanceof Error ? error.message : "电子凭证加载失败", icon: "none" });
  } finally {
    loading.value = false;
  }
}
function openOrder(item: AnyRecord) {
  const orderId = rowString(item, "orderId");
  if (orderId) uni.navigateTo({ url: `/pages/user/UserOrderDetailPage?id=${encodeURIComponent(orderId)}` });
}
onLoad(load);
</script>

<style>@import "../../styles/mini-program-business.css";</style>
