<template>
  <view class="mpb-page">
    <view class="mpb-safe" />
    <view class="mpb-header"><button class="mpb-back" aria-label="返回" @click="backOrHome('/pages/user/UserRechargePlansPage')">‹</button><image class="mpb-logo" :src="loginLogo" mode="aspectFit" /><view class="mpb-header-copy"><text class="mpb-title">确认订单</text><text class="mpb-subtitle">核对套餐与微信支付金额</text></view><text class="mpb-role">普通用户</text></view>
    <view class="mpb-stack">
      <view v-if="loading" class="mpb-card mpb-empty"><text class="mpb-empty-title">正在加载方案...</text></view>
      <template v-else-if="plan">
        <view class="mpb-hero"><view class="mpb-hero-top"><view><text class="mpb-hero-label">应付金额</text><text class="mpb-hero-value">{{ formatCurrency(plan.priceCents) }}</text></view><text class="mpb-hero-badge purple">待支付</text></view><text class="mpb-hero-copy">确认后创建真实业务订单；微信支付结果与到账状态以服务端确认为准。</text><view class="mpb-hero-metrics"><view class="mpb-hero-metric"><text class="mpb-hero-metric-value">{{ formatCurrency(plan.priceCents) }}</text><text class="mpb-hero-metric-label">套餐金额</text></view><view class="mpb-hero-metric"><text class="mpb-hero-metric-value">{{ formatNumber(plan.grantPoints || plan.points || plan.tokenAmount) }}</text><text class="mpb-hero-metric-label">到账点数</text></view><view class="mpb-hero-metric"><text class="mpb-hero-metric-value">0</text><text class="mpb-hero-metric-label">平台手续费</text></view></view></view>
        <view class="mpb-card mpb-list"><view class="mpb-section-head"><text class="mpb-card-title">支付信息</text><text class="mpb-card-copy">请仔细核对</text></view><view class="mpb-row"><text class="mpb-row-icon green">套</text><view class="mpb-row-main"><text class="mpb-row-title">充值套餐</text><text class="mpb-row-meta">{{ plan.name }} · {{ planTypeLabel }}</text></view><text class="mpb-amount">{{ formatCurrency(plan.priceCents) }}</text></view><view class="mpb-row"><text class="mpb-row-icon">点</text><view class="mpb-row-main"><text class="mpb-row-title">到账点数</text><text class="mpb-row-meta">支付成功后即时到账</text></view><text class="mpb-status success">{{ formatNumber(plan.grantPoints || plan.points || plan.tokenAmount) }} 点</text></view><view class="mpb-row"><text class="mpb-row-icon orange">支</text><view class="mpb-row-main"><text class="mpb-row-title">支付方式</text><text class="mpb-row-meta">绑定微信账户完成支付</text></view><text class="mpb-status success">微信支付</text></view><view class="mpb-row"><text class="mpb-row-icon">议</text><view class="mpb-row-main"><text class="mpb-row-title">订单协议</text><text class="mpb-row-meta">创建订单即表示同意服务协议</text></view><text class="mpb-status">已阅读</text></view></view>
        <view class="mpb-note">当前后端会创建真实业务订单；订单创建成功不等于支付成功，到账状态需以后端支付确认结果为准。</view>
        <button class="mpb-button" :disabled="submitting" @click="submit">{{ submitting ? "正在创建..." : `确认支付 ${formatCurrency(plan.priceCents)}` }}</button>
      </template>
      <view v-else class="mpb-card mpb-empty"><text class="mpb-empty-title">方案不存在或已下架</text></view>
    </view>
  </view>
</template>

<script setup lang="ts">
import { computed, ref } from "vue";
import { onLoad } from "@dcloudio/uni-app";
import { businessSdk } from "../../api/client";
import { backOrHome, formatCurrency, formatNumber, loadPlan, rowString, type AnyRecord, type CommercePlanRecord } from "../../utils/miniProgramBusiness";
import loginLogo from "../../assets/zhiqiyun-logo-transparent.png";
const planId = ref(""); const kind = ref(""); const loading = ref(false); const submitting = ref(false); const plan = ref<CommercePlanRecord | null>(null);
const planType = computed(() => kind.value || rowString(plan.value, "planType") || "subscription");
const normalizedPlanType = computed(() => planType.value.toLowerCase());
const planTypeLabel = computed(() => normalizedPlanType.value.includes("recharge") ? "点数充值" : normalizedPlanType.value.includes("agent") ? "代理商升级" : normalizedPlanType.value.includes("operation") ? "运营中心开通" : "会员订阅");
async function load() { loading.value = true; try { plan.value = await loadPlan(planId.value); } catch (error) { uni.showToast({ title: error instanceof Error ? error.message : "方案加载失败", icon: "none" }); } finally { loading.value = false; } }
async function submit() { if (!plan.value || submitting.value) return; submitting.value = true; try { const amountCents = Number(plan.value.priceCents || 0); let result: AnyRecord; if (normalizedPlanType.value.includes("recharge")) result = await businessSdk.billing.createRechargeOrder({ rechargePackageId: plan.value.id, amountCents, paymentMethod: "wechat_mini_program" }); else if (normalizedPlanType.value.includes("agent")) result = await businessSdk.billing.createAgentJoinOrder({ planId: plan.value.id, amountCents, paymentMethod: "wechat_mini_program" }); else if (normalizedPlanType.value.includes("operation")) result = await businessSdk.billing.createOperationCenterJoinOrder({ planId: plan.value.id, amountCents, paymentMethod: "wechat_mini_program" }); else result = await businessSdk.billing.createSubscriptionOrder({ planId: plan.value.id, amountCents, paymentMethod: "wechat_mini_program" }); const item = (result.item || result.order || result.checkout || result) as AnyRecord; const id = rowString(item, "id", "orderId", "orderNo") || rowString(result, "orderId"); const status = rowString(item, "status") || "PENDING"; const message = rowString(result, "message") || "订单已创建"; const points = Number(plan.value.grantPoints || plan.value.points || plan.value.tokenAmount || 0); uni.redirectTo({ url: `/pages/user/UserOrderResultPage?id=${encodeURIComponent(id)}&status=${encodeURIComponent(status)}&message=${encodeURIComponent(message)}&amountCents=${amountCents}&points=${points}&planName=${encodeURIComponent(plan.value.name)}` }); } catch (error) { uni.showToast({ title: error instanceof Error ? error.message : "订单创建失败", icon: "none" }); } finally { submitting.value = false; } }
onLoad(options => { planId.value = String(options?.planId || ""); kind.value = String(options?.kind || ""); void load(); });
</script>

<style>@import "../../styles/mini-program-business.css";</style>
