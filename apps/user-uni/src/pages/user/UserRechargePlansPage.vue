<template>
  <view class="mpb-page">
    <view class="mpb-safe" />
    <view class="mpb-header"><button class="mpb-back" aria-label="返回" @click="backOrHome('/pages/user/UserWalletPage')">‹</button><image class="mpb-logo" :src="loginLogo" mode="aspectFit" /><view class="mpb-header-copy"><text class="mpb-title">充值方案</text><text class="mpb-subtitle">点数充值与代理套餐</text></view><text class="mpb-role">普通用户</text></view>
    <view class="mpb-stack">
      <view class="mpb-hero"><view class="mpb-hero-top"><view><text class="mpb-hero-label">可用点数</text><text class="mpb-hero-value">{{ formatNumber(pointBalance) }} 点</text></view><text class="mpb-hero-badge">微信支付</text></view><text class="mpb-hero-copy">充值到账后不可提现，可用于全部 AI 创作服务；代理套餐将开通推广与分润权益。</text><view class="mpb-hero-metrics"><view class="mpb-hero-metric"><text class="mpb-hero-metric-value">{{ plans.length }}</text><text class="mpb-hero-metric-label">可选方案</text></view><view class="mpb-hero-metric"><text class="mpb-hero-metric-value">{{ recommendedPlan ? formatCurrency(recommendedPlan.priceCents) : '-' }}</text><text class="mpb-hero-metric-label">推荐档</text></view><view class="mpb-hero-metric"><text class="mpb-hero-metric-value">即时</text><text class="mpb-hero-metric-label">到账方式</text></view></view></view>
      <view v-if="loading" class="mpb-card mpb-empty"><text class="mpb-empty-title">正在加载充值方案...</text></view>
      <view v-else-if="plans.length" class="mpb-card mpb-list">
        <view class="mpb-section-head"><text class="mpb-card-title">选择套餐</text><text class="mpb-card-copy">以服务端配置为准</text></view>
        <button v-for="(plan, index) in plans" :key="plan.id" :class="['mpb-row-button', { active: selectedId === plan.id }]" @click="selectedId = plan.id"><text :class="['mpb-row-icon', plan.recommended ? 'green' : index === plans.length - 1 ? 'orange' : '']">{{ index === plans.length - 1 ? '代' : '充' }}</text><view class="mpb-row-main"><text class="mpb-row-title">{{ plan.name }}</text><text class="mpb-row-meta">到账 {{ formatNumber(plan.grantPoints || plan.points || plan.tokenAmount) }} 点{{ plan.recommended ? ' · 推荐' : '' }}</text></view><view class="mpb-row-side"><text class="mpb-amount">{{ formatCurrency(plan.priceCents) }}</text><text v-if="selectedId === plan.id" class="mpb-status success">已选择</text></view></button>
      </view>
      <view v-else class="mpb-card mpb-empty"><text class="mpb-empty-title">暂无可用充值方案</text><text class="mpb-empty-copy">请稍后刷新，或联系运营人员检查套餐配置。</text></view>
      <button class="mpb-button" :disabled="!selectedId" @click="confirm">选择套餐并继续</button>
      <text class="mpb-footer-note">支付成功后生成订单并更新到账点数</text>
    </view>
  </view>
</template>

<script setup lang="ts">
import { computed, ref } from "vue";
import { onLoad } from "@dcloudio/uni-app";
import { businessSdk } from "../../api/client";
import { asRecord, backOrHome, formatCurrency, formatNumber, loadPlans, rowNumber, type CommercePlanRecord } from "../../utils/miniProgramBusiness";
import loginLogo from "../../assets/zhiqiyun-logo-transparent.png";

const loading = ref(false);
const plans = ref<CommercePlanRecord[]>([]);
const selectedId = ref("");
const pointBalance = ref(0);
const recommendedPlan = computed(() => plans.value.find(item => item.recommended) || plans.value[0]);
async function load() { loading.value = true; try { const [planRows, wallet] = await Promise.all([loadPlans("recharge"), businessSdk.roleWorkbench.wallet()]); plans.value = planRows; const walletRow = asRecord(wallet); const balance = asRecord(walletRow.balance); const summary = asRecord(walletRow.summary); pointBalance.value = rowNumber(summary, "pointsAvailable", "availablePoints") || rowNumber(balance, "available", "pointsAvailable") || rowNumber(walletRow, "pointsAvailable"); selectedId.value = plans.value.find(item => item.recommended)?.id || plans.value[0]?.id || ""; } catch (error) { uni.showToast({ title: error instanceof Error ? error.message : "套餐加载失败", icon: "none" }); } finally { loading.value = false; } }
function confirm() { if (selectedId.value) uni.navigateTo({ url: `/pages/user/UserOrderConfirmPage?planId=${encodeURIComponent(selectedId.value)}&kind=recharge` }); }
onLoad(load);
</script>

<style>@import "../../styles/mini-program-business.css";</style>
