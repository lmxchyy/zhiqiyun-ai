<template>
  <view class="mpb-page">
    <view class="mpb-safe" />
    <view class="mpb-header"><button class="mpb-back" aria-label="返回" @click="backOrHome('/pages/agent/AgentWithdrawalsPage')">‹</button><image class="mpb-logo" :src="loginLogo" mode="aspectFit" /><view class="mpb-header-copy"><text class="mpb-title">申请提现</text><text class="mpb-subtitle">核对账户、金额与到账</text></view><text class="mpb-role agent">代理商</text></view>
    <view class="mpb-stack">
      <view class="mpb-hero"><view class="mpb-hero-top"><view><text class="mpb-hero-label">本次提现</text><text class="mpb-hero-value">{{ withdrawalAmountLabel }}</text></view><text class="mpb-hero-badge">预计 1-3 天</text></view><text class="mpb-hero-copy">提交后进入运营审核，审核通过后由财务流程完成到账。</text><view class="mpb-hero-metrics"><view class="mpb-hero-metric"><text class="mpb-hero-metric-value">{{ formatCurrency(availableCents) }}</text><text class="mpb-hero-metric-label">可提现</text></view><view class="mpb-hero-metric"><text class="mpb-hero-metric-value">¥0.00</text><text class="mpb-hero-metric-label">服务费</text></view><view class="mpb-hero-metric"><text class="mpb-hero-metric-value">{{ withdrawalAmountLabel }}</text><text class="mpb-hero-metric-label">预计到账</text></view></view></view>
      <view class="mpb-card mpb-list"><view class="mpb-section-head"><text class="mpb-card-title">提现信息</text><text class="mpb-card-copy">人工审核</text></view><view class="mpb-row"><text class="mpb-row-icon green">微</text><view class="mpb-row-main"><text class="mpb-row-title">提现账户</text><text class="mpb-row-meta">当前微信实名账户</text></view><text class="mpb-status success">待校验</text></view><view class="mpb-field"><text class="mpb-label">提现金额（元）</text><input v-model="amount" class="mpb-input" type="digit" placeholder="请输入提现金额" /></view><button class="mpb-link-button" @click="amount = String(availableCents / 100)">全部提现</button><view class="mpb-row"><text class="mpb-row-icon orange">费</text><view class="mpb-row-main"><text class="mpb-row-title">服务费</text><text class="mpb-row-meta">当前规则由后台审核确定</text></view><text class="mpb-status success">¥0.00</text></view></view>
      <view class="mpb-note">到账与实名校验由微信支付和平台财务流程共同完成。</view>
      <button class="mpb-button orange" :disabled="submitting || availableCents <= 0" @click="submit">{{ submitting ? "提交中..." : "确认申请提现" }}</button>
    </view>
  </view>
</template>

<script setup lang="ts">
import { computed, ref } from "vue";
import { onLoad } from "@dcloudio/uni-app";
import { api, businessSdk } from "../../api/client";
import { backOrHome, formatCurrency, rowNumber } from "../../utils/miniProgramBusiness";
import loginLogo from "../../assets/zhiqiyun-logo-transparent.png";
const availableCents = ref(0); const amount = ref(""); const submitting = ref(false);
const withdrawalAmountLabel = computed(() => { const cents = Math.round(Number(amount.value || 0) * 100); return formatCurrency(Number.isFinite(cents) && cents > 0 ? cents : 0); });
async function load() { try { const center = await businessSdk.roleWorkbench.channelCenter(); availableCents.value = rowNumber(center.summary, "availableToWithdraw"); } catch (error) { uni.showToast({ title: error instanceof Error ? error.message : "余额加载失败", icon: "none" }); } }
function submit() { const amountCents = Math.round(Number(amount.value) * 100); if (!Number.isFinite(amountCents) || amountCents <= 0) return void uni.showToast({ title: "请输入正确金额", icon: "none" }); if (amountCents > availableCents.value) return void uni.showToast({ title: "超出可提现余额", icon: "none" }); uni.showModal({ title: "确认提现", content: `确认申请提现 ${formatCurrency(amountCents)} 至微信零钱？`, success: async result => { if (!result.confirm) return; submitting.value = true; try { await api("/api/v1/channel/withdrawals", { method: "POST", body: JSON.stringify({ amountCents }) }); uni.showToast({ title: "提现申请已提交", icon: "success" }); setTimeout(() => uni.redirectTo({ url: "/pages/agent/AgentWithdrawalsPage" }), 400); } catch (error) { uni.showToast({ title: error instanceof Error ? error.message : "提现申请失败", icon: "none" }); } finally { submitting.value = false; } }}); }
onLoad(load);
</script>

<style>@import "../../styles/mini-program-business.css";</style>
