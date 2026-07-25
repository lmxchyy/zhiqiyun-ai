<template>
  <view class="mpb-page" :style="miniProgramNavigationStyle">
    <view class="mpb-safe" />
    <view class="mpb-header"><button class="mpb-back" aria-label="返回" @click="backOrHome('/pages/agent/AgentCommissionPage')">‹</button><image class="mpb-logo" :src="loginLogo" mode="aspectFit" /><view class="mpb-header-copy"><text class="mpb-title">提现记录</text><text class="mpb-subtitle">佣金结算与到账状态</text></view><text class="mpb-role agent">代理商</text></view>
    <view class="mpb-stack">
      <view class="mpb-hero light"><view class="mpb-hero-top"><view><text class="mpb-hero-label">可提现佣金</text><text class="mpb-hero-value">{{ formatCurrency(availableCents) }}</text></view><text class="mpb-hero-badge success">可提现</text></view><text class="mpb-hero-copy">已结算佣金可申请提现，处理中金额与历史到账分开呈现。</text><view class="mpb-hero-metrics"><view class="mpb-hero-metric"><text class="mpb-hero-metric-value">{{ formatCurrency(pendingCents) }}</text><text class="mpb-hero-metric-label">处理中</text></view><view class="mpb-hero-metric"><text class="mpb-hero-metric-value">{{ successCount }}</text><text class="mpb-hero-metric-label">已到账笔数</text></view><view class="mpb-hero-metric"><text class="mpb-hero-metric-value">{{ records.length }}</text><text class="mpb-hero-metric-label">全部申请</text></view></view></view>
      <view class="mpb-tabs"><button v-for="tab in tabs" :key="tab.id" :class="{ active: filter === tab.id }" @click="filter = tab.id">{{ tab.label }}</button></view>
      <view v-if="filtered.length" class="mpb-card mpb-list"><view class="mpb-section-head"><text class="mpb-card-title">最近提现</text><text class="mpb-card-copy">审核与到账</text></view><button v-for="item in filtered" :key="rowString(item, 'id')" class="mpb-row-button" @click="openDetail(item)"><text :class="['mpb-row-icon', statusTone(rowString(item, 'status')) === 'success' ? 'green' : 'orange']">提</text><view class="mpb-row-main"><text class="mpb-row-title">提现至微信零钱</text><text class="mpb-row-meta">{{ formatDate(rowString(item, 'createdAt', 'reviewedAt')) }}</text></view><view class="mpb-row-side"><text class="mpb-amount">{{ formatCurrency(rowNumber(item, 'amountCents')) }}</text><text :class="['mpb-status', statusTone(rowString(item, 'status'))]">{{ statusText(rowString(item, 'status')) }}</text></view></button></view>
      <view v-else class="mpb-card mpb-empty"><text class="mpb-empty-title">暂无提现记录</text></view>
      <button class="mpb-button orange" @click="openApply">申请提现</button>
    </view>
  </view>
</template>

<script setup lang="ts">
import { computed, ref } from "vue";
import { onShow } from "@dcloudio/uni-app";
import { api, businessSdk } from "../../api/client";
import { backOrHome, formatCurrency, formatDate, rowItems, rowNumber, rowString, statusText, statusTone, type AnyRecord } from "../../utils/miniProgramBusiness";
import loginLogo from "../../assets/zhiqiyun-logo-transparent.png";
const tabs = [{ id: "all", label: "全部" }, { id: "pending", label: "处理中" }, { id: "success", label: "已通过" }, { id: "failed", label: "未通过" }]; const filter = ref("all"); const records = ref<AnyRecord[]>([]);
const availableCents = ref(0); const pendingCents = ref(0);
const successCount = computed(() => records.value.filter(item => ["APPROVED", "SUCCESS", "PAID"].includes(rowString(item, "status").toUpperCase())).length); const pendingCount = computed(() => records.value.filter(item => ["PENDING", "PROCESSING"].includes(rowString(item, "status").toUpperCase())).length);
const filtered = computed(() => records.value.filter(item => { const status = rowString(item, "status").toUpperCase(); if (filter.value === "pending") return ["PENDING", "PROCESSING"].includes(status); if (filter.value === "success") return ["APPROVED", "SUCCESS", "PAID"].includes(status); if (filter.value === "failed") return ["REJECTED", "FAILED"].includes(status); return true; }));
async function load() { try { const [rows, center] = await Promise.all([api("/api/v1/channel/withdrawals"), businessSdk.roleWorkbench.channelCenter()]); records.value = rowItems(rows); availableCents.value = rowNumber(center.summary, "availableToWithdraw"); pendingCents.value = rowNumber(center.summary, "pendingWithdrawal"); } catch (error) { uni.showToast({ title: error instanceof Error ? error.message : "提现记录加载失败", icon: "none" }); } }
function openApply() { uni.navigateTo({ url: "/pages/agent/AgentWithdrawalApplyPage" }); }
function openDetail(item: AnyRecord) { const id = rowString(item, "id", "withdrawalId"); if (id) uni.navigateTo({ url: `/pages/agent/AgentWithdrawalDetailPage?id=${encodeURIComponent(id)}` }); }
onShow(load);
</script>

<style>@import "../../styles/mini-program-business.css";</style>
