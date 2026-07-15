<template>
  <view class="mpb-page">
    <view class="mpb-safe" />
    <view class="mpb-header"><button class="mpb-back" aria-label="返回" @click="backOrHome('/pages/user/UserWalletPage')">‹</button><image class="mpb-logo" :src="loginLogo" mode="aspectFit" /><view class="mpb-header-copy"><text class="mpb-title">充值方案</text><text class="mpb-subtitle">点数充值与代理套餐</text></view><text class="mpb-role">普通用户</text></view>
    <view class="mpb-stack">
      <view class="mpb-hero"><view class="mpb-hero-top"><view><text class="mpb-hero-label">可用点数</text><text class="mpb-hero-value">{{ formatNumber(pointBalance) }} 点</text></view><text class="mpb-hero-badge">微信支付</text></view><text class="mpb-hero-copy">充值到账后不可提现，可用于全部 AI 创作服务；代理套餐将开通推广与分润权益。</text><view class="mpb-hero-metrics"><view class="mpb-hero-metric"><text class="mpb-hero-metric-value">{{ selectablePlanCount }}</text><text class="mpb-hero-metric-label">可选方案</text></view><view class="mpb-hero-metric"><text class="mpb-hero-metric-value">{{ recommendedPlan ? formatCurrency(recommendedPlan.priceCents) : '-' }}</text><text class="mpb-hero-metric-label">推荐档</text></view><view class="mpb-hero-metric"><text class="mpb-hero-metric-value">即时</text><text class="mpb-hero-metric-label">到账方式</text></view></view></view>
      <view v-if="isFirstPurchase" class="mpb-note first-recharge-note">首次充值仅可选择 996 元代理商开通包，支付成功后将开放其他点数套餐。</view>
      <view v-if="loading" class="mpb-card mpb-empty"><text class="mpb-empty-title">正在加载充值方案...</text></view>
      <view v-else-if="plans.length" class="mpb-card mpb-list">
        <view class="mpb-section-head"><text class="mpb-card-title">选择套餐</text><text class="mpb-card-copy">{{ isFirstPurchase ? '首次充值限 996 元套餐' : '以服务端配置为准' }}</text></view>
        <button v-for="plan in plans" :key="plan.id" :class="['mpb-row-button', { active: selectedId === plan.id, locked: isPlanLocked(plan) }]" :disabled="isPlanLocked(plan)" @click="selectPlan(plan)"><text :class="['mpb-row-icon', isAgentPlan(plan) ? 'orange' : plan.recommended ? 'green' : '']">{{ isAgentPlan(plan) ? '代' : '充' }}</text><view class="mpb-row-main"><text class="mpb-row-title">{{ plan.name }}</text><text class="mpb-row-meta">到账 {{ formatNumber(plan.grantPoints || plan.points || plan.tokenAmount) }} 点{{ isAgentPlan(plan) ? ' · 开通代理商' : plan.recommended ? ' · 推荐' : '' }}</text></view><view class="mpb-row-side"><text class="mpb-amount">{{ formatCurrency(plan.priceCents) }}</text><text v-if="selectedId === plan.id" class="mpb-status success">已选择</text><text v-else-if="isPlanLocked(plan)" class="mpb-status">首次不可选</text></view></button>
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
import { asRecord, backOrHome, formatCurrency, formatNumber, hasCompletedPurchase, listOf, loadPlans, rowNumber, type CommercePlanRecord } from "../../utils/miniProgramBusiness";
import loginLogo from "../../assets/zhiqiyun-logo-transparent.png";

const loading = ref(false);
const plans = ref<CommercePlanRecord[]>([]);
const selectedId = ref("");
const pointBalance = ref(0);
const isFirstPurchase = ref(false);
const recommendedPlan = computed(() => isFirstPurchase.value ? plans.value.find(isAgentPlan) : plans.value.find(item => item.recommended) || plans.value.find(item => !isAgentPlan(item)));
const selectablePlanCount = computed(() => plans.value.filter(item => !isPlanLocked(item)).length);
function isAgentPlan(plan: CommercePlanRecord) { return plan.id === "plan_agent_join_996" || String(plan.planType || "").toUpperCase().includes("AGENT"); }
function isPlanLocked(plan: CommercePlanRecord) { return isFirstPurchase.value && !isAgentPlan(plan); }
function selectPlan(plan: CommercePlanRecord) { if (!isPlanLocked(plan)) selectedId.value = plan.id; }
async function load() {
  loading.value = true;
  try {
    const [rechargeRows, agentRows, wallet] = await Promise.all([loadPlans("recharge"), loadPlans("agent_join"), businessSdk.roleWorkbench.wallet()]);
    const walletRow = asRecord(wallet);
    isFirstPurchase.value = !hasCompletedPurchase(listOf(walletRow.orders));
    const agentPlan = agentRows.find(item => item.id === "plan_agent_join_996");
    plans.value = isFirstPurchase.value && agentPlan ? [agentPlan, ...rechargeRows] : rechargeRows;
    const balance = asRecord(walletRow.balance || walletRow.account);
    const summary = asRecord(walletRow.summary);
    pointBalance.value = rowNumber(summary, "pointsAvailable", "availablePoints") || rowNumber(balance, "available", "pointsAvailable") || rowNumber(walletRow, "pointsAvailable");
    selectedId.value = isFirstPurchase.value ? agentPlan?.id || "" : rechargeRows.find(item => item.recommended)?.id || rechargeRows[0]?.id || "";
  } catch (error) {
    uni.showToast({ title: error instanceof Error ? error.message : "套餐加载失败", icon: "none" });
  } finally {
    loading.value = false;
  }
}
function confirm() {
  const selected = plans.value.find(item => item.id === selectedId.value);
  if (!selected || isPlanLocked(selected)) return;
  uni.navigateTo({ url: `/pages/user/UserOrderConfirmPage?planId=${encodeURIComponent(selected.id)}&kind=${isAgentPlan(selected) ? "agent" : "recharge"}` });
}
onLoad(load);
</script>

<style>
@import "../../styles/mini-program-business.css";
.first-recharge-note { color: #b54708; border: 1px solid #fed7aa; background: #fff7ed; }
.mpb-row-button.locked { opacity: .48; border-style: dashed; background: #f8fafc; }
</style>
